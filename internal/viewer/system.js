// System notebook player. The server owns all behaviour (POST …/system/run returns the hops to
// animate and the new state); this file only draws, animates and keeps the state between actions.
(() => {
  const $ = (id) => document.getElementById(id);
  const base = document.body.dataset.base;
  const stage = $("stage"), world = $("world"), wires = $("wires"), nodesEl = $("nodes");
  const NS = "http://www.w3.org/2000/svg";
  const reduced = matchMedia("(prefers-reduced-motion: reduce)");
  const viewKey = "folio-system-" + document.body.dataset.id;
  const kindIcon = { client: "client", load_balancer: "load_balancer", service: "server", cache: "cache", database: "database", queue: "queue" };
  const kindName = { client: "Client", load_balancer: "Load balancer", service: "Service", cache: "Cache", database: "Database", queue: "Queue" };
  const defaultMS = { client: 0, load_balancer: 1, service: 5, cache: 1, database: 20, queue: 2 };
  const statusText = { 200: "OK", 409: "Conflict", 429: "Too Many Requests", 500: "Internal Server Error", 502: "Bad Gateway", 503: "Service Unavailable" };

  const fresh = () => ({ down: [], cache: {}, counters: {}, rr: {}, backlog: {} });
  let spec = null, lay = null, version = -1, updatedAt = null;
  let st = fresh(), counts = {}, playing = false, selected = null;
  let tx = 24, ty = 24, z = 1, placed = false, es = null, saveTimer = null;
  const edges = new Map(); // "from>to" → <path>
  const cards = new Map(); // node id → card element

  const node = (id) => spec.nodes.find((n) => n.id === id);
  const name = (id) => { const n = node(id); return n ? n.label || n.id : id; };
  const isDown = (id) => st.down.includes(id);
  const groupIDs = (g) => spec.nodes.filter((n) => n.group === g).map((n) => n.id);
  function el(tag, cls, text) {
    const e = document.createElement(tag);
    if (cls) e.className = cls;
    if (text != null) e.textContent = text;
    return e;
  }
  function svg(tag, attrs) {
    const e = document.createElementNS(NS, tag);
    for (const k in attrs) e.setAttribute(k, attrs[k]);
    return e;
  }
  function icon(id) {
    const s = svg("svg", { class: "i", "aria-hidden": "true" });
    s.append(svg("use", { href: "/icons.svg#" + id }));
    return s;
  }
  function button(cls, text, onclick, title) {
    const b = el("button", cls, text);
    b.type = "button";
    b.onclick = onclick;
    if (title) b.title = title;
    return b;
  }

  // ---- Pan and zoom: the world is one transformed layer holding the wires SVG and the cards.
  const clampZ = (v) => Math.min(2.5, Math.max(0.15, v));
  function draw() {
    world.style.transform = "translate(" + tx + "px," + ty + "px) scale(" + z + ")";
    stage.style.backgroundPosition = tx + "px " + ty + "px";
    $("zreset").textContent = Math.round(z * 100) + "%";
    clearTimeout(saveTimer);
    saveTimer = setTimeout(() => folio.store.set(viewKey, JSON.stringify({ tx, ty, z })), 300);
  }
  function zoomAt(nz, cx, cy) {
    nz = clampZ(nz);
    tx = cx - ((cx - tx) * nz) / z;
    ty = cy - ((cy - ty) * nz) / z;
    z = nz;
    draw();
  }
  const stepZoom = (f) => zoomAt(z * f, stage.clientWidth / 2, stage.clientHeight / 2);
  function fit() {
    if (!lay) return;
    z = clampZ(Math.min(1.1, (stage.clientWidth - 48) / lay.w, (stage.clientHeight - 48) / lay.h));
    tx = (stage.clientWidth - lay.w * z) / 2;
    ty = Math.max(24, (stage.clientHeight - lay.h * z) / 2);
    draw();
  }
  folio.panzoom(stage, {
    pan: (dx, dy) => { tx += dx; ty += dy; draw(); },
    zoom: (f, x, y) => zoomAt(z * f, x, y),
    skip: (e) => e.type === "pointerdown" && e.target.closest(".node, .grp, button"),
  });
  let downAt = null;
  stage.addEventListener("pointerdown", (e) => (downAt = [e.clientX, e.clientY]));
  stage.addEventListener("click", (e) => {
    const still = downAt && Math.hypot(e.clientX - downAt[0], e.clientY - downAt[1]) < 5;
    if (still && !e.target.closest(".node, .grp")) select(null);
  });

  // ---- Drawing
  function pathD(e) {
    const p = e.points;
    let d = "M" + p[0];
    if (e.curve) for (let i = 1; i + 2 < p.length; i += 3) d += " C" + p[i] + " " + p[i + 1] + " " + p[i + 2];
    else for (let i = 1; i < p.length; i++) d += " L" + p[i];
    return d;
  }

  function render() {
    wires.replaceChildren();
    nodesEl.replaceChildren();
    edges.clear();
    cards.clear();
    world.style.width = lay.w + "px";
    world.style.height = lay.h + "px";
    for (const [k, v] of Object.entries({ viewBox: "0 0 " + lay.w + " " + lay.h, width: lay.w, height: lay.h })) wires.setAttribute(k, v);
    const marker = svg("marker", { id: "arrow", viewBox: "0 0 10 10", refX: "9", refY: "5", markerWidth: "7", markerHeight: "7", orient: "auto-start-reverse" });
    marker.append(svg("path", { d: "M0,0 L10,5 L0,10 z", class: "arrowhead" }));
    const defs = svg("defs", {});
    defs.append(marker);
    wires.append(defs);

    for (const g of lay.groups) {
      wires.append(svg("rect", { x: g.x, y: g.y, width: g.w, height: g.h, rx: 14, class: "group" }));
      const tag = el("div", "grp");
      tag.style.left = g.x + 12 + "px";
      tag.style.top = g.y + 8 + "px";
      tag.append(el("span", "", g.id), button("mini", "Kill", () => toggleGroup(g.id)));
      tag.lastChild.dataset.group = g.id;
      nodesEl.append(tag);
    }
    for (const e of lay.edges) {
      const p = svg("path", { d: pathD(e), class: "edge" + (e.replicate ? " rep" : ""), "marker-end": "url(#arrow)" });
      wires.append(p);
      edges.set(e.from + ">" + e.to, p);
      const label = e.label || (e.replicate ? "replicates" : "");
      if (label) {
        const m = p.getPointAtLength(p.getTotalLength() / 2);
        const t = svg("text", { x: m.x, y: m.y - 6, class: "elabel", "text-anchor": "middle" });
        t.textContent = label;
        wires.append(t);
      }
    }
    wires.append(svg("g", { id: "dots" }));

    for (const b of lay.nodes) {
      const n = node(b.id);
      if (!n) continue;
      const c = el("div", "node k-" + n.kind);
      c.tabIndex = 0;
      c.setAttribute("role", "button");
      c.setAttribute("aria-label", (n.label || n.id) + ", " + kindName[n.kind]);
      Object.assign(c.style, { left: b.x + "px", top: b.y + "px", width: b.w + "px", height: b.h + "px" });
      const ic = el("span", "ic");
      ic.append(icon("sys-" + (n.icon || kindIcon[n.kind])));
      const txt = el("span", "txt");
      txt.append(el("span", "nm", n.label || n.id), el("span", "st"));
      c.append(ic, txt);
      if (n.kind !== "client") {
        const k = button("kill icon-btn", null, (ev) => { ev.stopPropagation(); toggle(n.id); });
        k.append(icon("power"));
        c.append(k);
      }
      c.onclick = () => select(n.id);
      c.onkeydown = (ev) => {
        if (ev.key === "Enter" || ev.key === " ") {
          ev.preventDefault();
          select(n.id);
        }
      };
      nodesEl.append(c);
      cards.set(n.id, c);
    }
    fillControls();
    applyState();
  }

  function fillControls() {
    const req = $("req"), keep = req.value;
    req.replaceChildren(...spec.requests.map((r) => {
      const o = el("option", "", r.label || r.id);
      o.value = r.id;
      return o;
    }));
    if (spec.requests.some((r) => r.id === keep)) req.value = keep;
    fillKeys();
    const sc = spec.scenarios || [];
    $("scenWrap").hidden = !sc.length;
    $("scenarios").replaceChildren(...sc.map((s, i) => button("chip", s.label, () => runScenario(i), s.explain)));
  }
  function fillKeys() {
    const r = spec.requests.find((x) => x.id === $("req").value) || spec.requests[0];
    const key = $("key"), keep = key.value;
    key.replaceChildren(...r.keys.map((k) => { const o = el("option", "", k); o.value = k; return o; }));
    if (r.keys.includes(keep)) key.value = keep;
  }
  $("req").onchange = fillKeys;

  function applyState() {
    for (const [id, c] of cards) {
      const n = node(id), down = isDown(id);
      c.classList.toggle("down", down);
      // DOWN always shows; UP only when there's nothing more useful to say (cards are narrow).
      const parts = down ? ["DOWN"] : [];
      if (counts[id]) parts.push(counts[id] + (counts[id] === 1 ? " hop" : " hops"));
      if (n.kind === "cache" && !down) parts.push((st.cache[id] || []).length + " keys");
      if (n.kind === "queue" && (st.backlog[id] || []).length) parts.push(st.backlog[id].length + " waiting");
      c.querySelector(".st").textContent = parts.join(" · ") || "UP";
      const k = c.querySelector(".kill");
      if (k) {
        k.title = down ? "Revive" : "Kill";
        k.setAttribute("aria-label", (down ? "Revive " : "Kill ") + name(id));
        k.setAttribute("aria-pressed", String(down));
      }
    }
    for (const b of nodesEl.querySelectorAll("[data-group]")) {
      const dead = groupIDs(b.dataset.group).every(isDown);
      b.textContent = dead ? "Revive" : "Kill";
      b.setAttribute("aria-label", (dead ? "Revive " : "Kill ") + b.dataset.group);
    }
    showDetails(selected);
  }

  // ---- Details panel
  function select(id) {
    selected = id;
    for (const [nid, c] of cards) c.classList.toggle("sel", nid === id);
    showDetails(id);
    if (id) setPanel(true);
  }
  function showDetails(id) {
    const d = $("details");
    const n = id && spec && node(id);
    if (!n) {
      d.hidden = true;
      return;
    }
    const down = isDown(id);
    d.hidden = false;
    d.replaceChildren();
    const head = el("div", "d-head");
    const x = button("icon-btn", null, () => select(null), "Close (Esc)");
    x.setAttribute("aria-label", "Close details");
    x.append(icon("close"));
    head.append(el("h2", "", n.label || n.id), x);
    const state = el("span", down ? "badge down" : "badge", down ? "DOWN" : "UP");
    const sub = el("div", "meta", kindName[n.kind] + (n.group ? " · " + n.group : "") + " ");
    sub.append(state);
    d.append(head, sub);
    if (n.about) d.append(el("p", "", n.about));
    if (n.when_down) {
      const p = el("p", "when");
      p.append(el("b", "", "When it's down: "), n.when_down);
      d.append(p);
    }
    const facts = [];
    if (n.pool) facts.push(["Pool", n.pool.map(name).join(", ") + " · " + (n.strategy === "hash" ? "hash of key" : "round robin")]);
    if (n.replicas) facts.push(["Replicas", n.replicas.map(name).join(", ")]);
    if (n.consumers) facts.push(["Consumers", n.consumers.map(name).join(", ")]);
    if (n.kind === "cache") facts.push(["Keys", (st.cache[id] || []).join(", ") || "empty"]);
    if (n.kind === "queue") facts.push(["Waiting", (st.backlog[id] || []).join(", ") || "none"]);
    if (counts[id]) facts.push(["Requests", String(counts[id])]);
    facts.push(["Latency", (n.ms || defaultMS[n.kind]) + " ms"]);
    const dl = el("dl", "facts");
    for (const [k, v] of facts) dl.append(el("dt", "", k), el("dd", "", v));
    d.append(dl);
    const acts = el("div", "actions");
    if (n.kind !== "client") acts.append(button(down ? "btn" : "btn secondary", down ? "Revive" : "Kill", () => toggle(id), "k"));
    if (n.kind === "cache") acts.append(button("btn secondary", "Empty cache", () => act({ action: "flush", node: id }, null, name(id) + " emptied")));
    d.append(acts);
  }

  // ---- Log
  const logInner = $("logInner");
  function log(t, text, cls) {
    const ln = el("div", "ln " + (cls || ""));
    ln.append(el("span", "t", t), el("span", "x", text));
    logInner.append(ln);
    while (logInner.childElementCount > 500) logInner.firstElementChild.remove();
  }
  $("clearLog").onclick = () => logInner.replaceChildren();

  // ---- Actions: every one goes to the server, which returns events to play and the next state.
  async function post(input) {
    const r = await fetch(base + "/system/run", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ ...input, state: st }),
    });
    const j = await r.json().catch(() => ({ error: r.statusText }));
    if (!r.ok) throw new Error(j.error || "Couldn't run that (" + r.status + ")");
    return j;
  }

  async function act(input, title, note) {
    if (playing || !spec) return;
    let res;
    try {
      res = await post(input);
    } catch (e) {
      folio.toast(e.message);
      return;
    }
    if (note) log("", note, "head");
    st = { ...st, down: res.state.down }; // dead nodes show while the request plays
    applyState();
    if (res.events.length) await play(res, title);
    st = res.state;
    applyState();
  }

  const speed = () => +$("speed").value * (reduced.matches ? 3 : 1);
  const toggle = (id) => act({ action: isDown(id) ? "revive" : "kill", node: id }, null, name(id) + (isDown(id) ? " revived" : " killed"));
  function toggleGroup(g) {
    const dead = groupIDs(g).every(isDown);
    act({ action: dead ? "revive" : "kill", node: g }, null, g + (dead ? " revived" : " killed"));
  }
  function sendNow() {
    const r = spec.requests.find((x) => x.id === $("req").value);
    const key = $("key").value;
    act({ action: "send", request: r.id, key }, "▶ " + (r.label || r.id).replaceAll("{key}", key));
  }
  function runScenario(i) {
    if (playing) return;
    const sc = spec.scenarios[i];
    counts = {};
    log("", "Scenario: " + sc.label, "head");
    if (sc.explain) log("", sc.explain, "explain");
    act({ action: "scenario", scenario: i }, null);
  }
  async function reset() {
    if (playing) return;
    counts = {};
    clearTrace();
    $("result").textContent = "";
    await act({ action: "reset" }, null, "Reset: everything is up, caches and queues are empty");
  }

  // ---- Playback. Simulated times map onto real time so short hops stay visible and slow
  // ones (a 30 ms database) visibly take longer; parallel and async hops overlap naturally.
  function timeline(evs) {
    const pts = [...new Set(evs.flatMap((e) => [e.t, e.t + (e.dur || 0)]))].sort((a, b) => a - b);
    const at = new Map();
    let acc = 0;
    pts.forEach((p, i) => {
      if (i) acc += (200 + Math.min(p - pts[i - 1], 120) * 12) / speed();
      at.set(p, acc);
    });
    return (p) => at.get(p);
  }

  function clearTrace() {
    stage.classList.remove("traced");
    for (const c of cards.values()) c.classList.remove("active");
    for (const p of edges.values()) p.classList.remove("on", "bad");
  }

  function setBusy(on) {
    playing = on;
    document.body.classList.toggle("busy", on);
    for (const b of document.querySelectorAll("#send, #reset, .chip")) b.disabled = on;
  }

  function play(res, title) {
    setBusy(true);
    clearTrace();
    stage.classList.add("traced");
    if (title) log("", title, "head");
    const f = timeline(res.events);
    let last = 0;
    return new Promise((done) => {
      for (const e of res.events) {
        const start = f(e.t), end = e.dur ? f(e.t + e.dur) : start;
        last = Math.max(last, end);
        setTimeout(() => {
          log(e.t + " ms", e.text, (e.fail ? "fail" : "") + (e.async ? " async" : "") + (e.to ? "" : " note"));
          if (e.to) hop(e, end - start);
        }, start);
        if (e.to && e.dir === "req" && !e.fail) setTimeout(() => arrive(e.to), end);
      }
      setTimeout(() => {
        if (res.status) {
          const ok = res.status < 400;
          $("result").textContent = res.status + " " + (statusText[res.status] || "") + " · " + res.total_ms + " ms";
          $("result").className = "result " + (ok ? "ok" : "bad");
        }
        setBusy(false);
        done();
      }, last + 60);
    });
  }

  function hop(e, ms) {
    let p = edges.get(e.from + ">" + e.to), back = false;
    if (!p) {
      p = edges.get(e.to + ">" + e.from);
      back = true;
    }
    if (!p) return;
    p.classList.add(e.fail ? "bad" : "on");
    cards.get(e.from)?.classList.add("active");
    if (!e.fail) cards.get(e.to)?.classList.add("active");
    if (reduced.matches || ms < 16) return;
    const dot = svg("circle", { r: 7, class: "dot " + e.dir + (e.fail ? " fail" : "") + (e.async ? " async" : "") });
    const m = svg("animateMotion", { dur: ms + "ms", fill: "freeze", begin: "indefinite", path: p.getAttribute("d"), keyPoints: back ? "1;0" : "0;1", keyTimes: "0;1", calcMode: "linear" });
    dot.append(m);
    $("dots").append(dot);
    m.beginElement();
    setTimeout(() => dot.remove(), ms + 150);
  }

  function arrive(id) {
    counts[id] = (counts[id] || 0) + 1;
    const c = cards.get(id);
    if (!c) return;
    c.classList.remove("ping");
    void c.offsetWidth;
    c.classList.add("ping");
    applyState();
  }

  // ---- Loading and live updates
  async function load() {
    let j;
    try {
      const r = await fetch(base + "/system.json", { cache: "no-store" });
      if (!r.ok) throw new Error(r.statusText);
      j = await r.json();
    } catch (e) {
      folio.toast("Couldn't load the system. Try reloading the page.", null, "load");
      return;
    }
    const updated = version >= 0 && j.version !== version;
    version = j.version;
    $("version").textContent = j.version;
    $("empty").hidden = !!j.spec;
    document.body.classList.toggle("empty", !j.spec);
    if (!j.spec) return;
    spec = j.spec;
    lay = j.layout;
    const ids = new Set(spec.nodes.map((n) => n.id));
    st.down = st.down.filter((id) => ids.has(id));
    if (selected && !ids.has(selected)) selected = null;
    clearTrace();
    render();
    if (selected) select(selected);
    if (!placed) {
      placed = true;
      let saved = null;
      try { saved = JSON.parse(folio.store.get(viewKey)); } catch (e) {}
      if (saved && saved.z) {
        ({ tx, ty } = saved);
        z = clampZ(saved.z);
        draw();
      } else fit();
    }
    if (updated) folio.toast("The system was updated", null, "update");
  }

  function setStatus(text, cls) {
    $("status").textContent = text;
    $("status").className = "live " + cls;
  }
  function tick() {
    if (updatedAt) $("ago").textContent = "updated " + folio.ago(updatedAt);
  }
  setInterval(tick, 15000);

  function connect() {
    if (es) es.close();
    es = new EventSource(base + "/events");
    es.onopen = () => setStatus("Live", "on");
    es.onerror = () => setStatus(navigator.onLine ? "Reconnecting…" : "Offline", "off");
    es.onmessage = (e) => {
      const s = JSON.parse(e.data);
      if (s.format !== "system") return location.reload();
      $("title").textContent = s.title;
      document.title = s.title + " · Folio";
      if (!view) {
        document.documentElement.dataset.auto = s.mode;
        folio.applyView();
      }
      updatedAt = new Date(s.updated_at);
      tick();
      if (s.version !== version) load();
    };
    es.addEventListener("gone", () => {
      setStatus(base.startsWith("/s/") ? "Link off" : "Deleted", "off");
      es.close();
      $("gone").showModal();
    });
  }

  // ---- Chrome
  function setPanel(open) {
    document.body.classList.toggle("nopanel", !open);
    $("panelBtn").setAttribute("aria-expanded", String(open));
  }
  $("panelBtn").onclick = () => {
    const open = document.body.classList.contains("nopanel");
    setPanel(open);
    folio.store.set("folio-system-panel", open ? "" : "0");
  };
  setPanel(folio.store.get("folio-system-panel") !== "0" && matchMedia("(min-width: 821px)").matches);
  // The stage ends where the dock starts; the dock's height depends on wrapping, so measure it.
  new ResizeObserver(() => document.body.style.setProperty("--dock-h", $("dock").offsetHeight + "px")).observe($("dock"));
  $("send").onclick = sendNow;
  $("reset").onclick = reset;
  $("zin").onclick = () => stepZoom(1.25);
  $("zout").onclick = () => stepZoom(0.8);
  $("zreset").onclick = fit;
  $("helpBtn").onclick = () => $("help").showModal();
  document.addEventListener("folio:view", (e) => (view = e.detail));
  addEventListener("resize", () => { if (!placed) fit(); });

  document.addEventListener("keydown", (e) => {
    if (e.metaKey || e.ctrlKey || e.altKey || e.target.closest("input, select, textarea") || document.querySelector("dialog[open]")) return;
    const order = ["", "dark", "light"];
    const fn = {
      s: sendNow, r: reset, k: () => selected && node(selected).kind !== "client" && toggle(selected), Escape: () => select(null),
      "+": () => stepZoom(1.25), "=": () => stepZoom(1.25), "-": () => stepZoom(0.8), "0": fit,
      l: () => $("panelBtn").click(), t: () => folio.setView(order[(order.indexOf(view) + 1) % order.length]),
      "?": () => $("help").showModal(),
    }[e.key];
    if (fn && spec) {
      e.preventDefault();
      fn();
    }
  });

  connect();
})();
