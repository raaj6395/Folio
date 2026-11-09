(() => {
  const root = document.documentElement;
  const store = {
    get(k) { try { return localStorage.getItem(k); } catch (e) { return null; } },
    set(k, v) { try { localStorage.setItem(k, v); } catch (e) {} },
  };
  const folio = (window.folio = { store });

  folio.applyView = () => {
    const v = store.get("folio-view-mode") || "";
    root.dataset.theme = v || root.dataset.auto || "dark";
    document.querySelectorAll("[data-view]").forEach((b) => b.setAttribute("aria-pressed", String(b.dataset.view === v)));
    const meta = document.querySelector('meta[name="theme-color"]');
    if (meta) meta.content = getComputedStyle(root).getPropertyValue("--surface").trim();
  };
  folio.setView = (v) => {
    store.set("folio-view-mode", v);
    folio.applyView();
    document.dispatchEvent(new CustomEvent("folio:view", { detail: v }));
  };
  document.querySelectorAll("[data-view]").forEach((b) => (b.onclick = () => folio.setView(b.dataset.view)));
  folio.applyView();

  folio.toast = (msg, action, key, ms = 4500) => {
    let box = document.getElementById("toasts");
    if (!box) {
      box = document.createElement("div");
      box.id = "toasts";
      box.setAttribute("aria-live", "polite");
      document.body.append(box);
    }
    if (key) box.querySelector('[data-key="' + key + '"]')?.remove();
    const t = document.createElement("div");
    t.className = "toast";
    t.textContent = msg;
    if (key) t.dataset.key = key;
    if (action) {
      const b = document.createElement("button");
      b.type = "button";
      b.textContent = action.label;
      b.onclick = () => { action.run(); t.remove(); };
      t.append(b);
    }
    box.append(t);
    setTimeout(() => t.classList.add("out"), ms);
    setTimeout(() => t.remove(), ms + 400);
  };

  const plural = (n, unit) => n + " " + unit + (n === 1 ? "" : "s") + " ago";
  folio.ago = (d) => {
    const s = Math.max(0, (Date.now() - d) / 1000);
    if (s < 45) return "just now";
    if (s < 3600) return plural(Math.round(s / 60), "min");
    if (s < 86400) return plural(Math.round(s / 3600), "hour");
    if (s < 7 * 86400) return plural(Math.round(s / 86400), "day");
    return d.toLocaleDateString(undefined, { day: "numeric", month: "short", year: "numeric" });
  };
  const times = (scope = document) => scope.querySelectorAll("time[datetime]").forEach((t) => {
    const d = new Date(t.dateTime);
    t.textContent = folio.ago(d);
    t.title = d.toLocaleString();
    if (t.hasAttribute("data-recent")) t.classList.toggle("recent", Date.now() - d < 3600e3);
  });
  setInterval(times, 30000);

  folio.enhance = (scope) => {
    times(scope);
    scope.querySelectorAll("[data-initial]").forEach((el) => (el.textContent = (el.dataset.initial.trim()[0] || "?").toUpperCase()));

    scope.querySelectorAll("[data-copy]").forEach((b) => (b.onclick = () => {
      const el = document.getElementById(b.dataset.copy);
      const label = b.textContent;
      navigator.clipboard.writeText(el.textContent.trim()).then(() => {
        b.textContent = "Copied ✓";
        setTimeout(() => (b.textContent = label), 1500);
      }, () => {
        getSelection().selectAllChildren(el);
        folio.toast("Press Ctrl+C (⌘C on Mac) to copy");
      });
    }));

    scope.querySelectorAll("[data-close]").forEach((b) => (b.onclick = () => b.closest("dialog").close()));

    scope.querySelectorAll("button[data-confirm]").forEach((b) => {
      const label = b.textContent;
      b.addEventListener("click", (e) => {
        if (b.classList.contains("armed")) return;
        e.preventDefault();
        b.classList.add("armed");
        b.textContent = b.dataset.confirm;
        setTimeout(() => { b.classList.remove("armed"); b.textContent = label; }, 3000);
      });
    });
  };
  folio.enhance(document);

  document.querySelectorAll("form[data-busy]").forEach((f) => f.addEventListener("submit", () => {
    const b = f.querySelector("button[type=submit]");
    b.disabled = true;
    b.classList.add("busy");
    b.textContent = f.dataset.busy;
  }));

  document.querySelectorAll("[data-reveal]").forEach((b) => {
    const input = document.getElementById(b.dataset.reveal);
    b.onclick = () => {
      const show = input.type === "password";
      input.type = show ? "text" : "password";
      b.textContent = show ? "Hide" : "Show";
      b.setAttribute("aria-pressed", String(show));
      input.focus();
    };
  });

  const pw = document.getElementById("pw");
  if (pw) {
    const caps = document.getElementById("caps");
    const hint = document.getElementById("pwhint");
    const capsCheck = (e) => (caps.hidden = !e.getModifierState?.("CapsLock"));
    pw.addEventListener("keydown", capsCheck);
    pw.addEventListener("keyup", capsCheck);
    if (hint) pw.addEventListener("input", () => {
      const left = +hint.dataset.min - pw.value.length;
      hint.textContent = left > 0 ? left + " more character" + (left === 1 ? "" : "s") : "✓ Long enough";
      hint.classList.toggle("ok", left <= 0);
    });
  }

  document.querySelectorAll(".tabs").forEach((tabs) => {
    const btns = [...tabs.querySelectorAll("[role=tab]")];
    const show = (id) => {
      btns.forEach((b) => {
        const on = b.dataset.tab === id;
        b.setAttribute("aria-selected", String(on));
        document.getElementById(b.dataset.tab).hidden = !on;
      });
      store.set("folio-tab", id);
    };
    btns.forEach((b) => (b.onclick = () => show(b.dataset.tab)));
    const saved = tabs.dataset.initial || store.get("folio-tab");
    show(btns.some((b) => b.dataset.tab === saved) ? saved : btns[0].dataset.tab);
  });

  // Owner's "Share link…" item on a notebook page: turns the public read-only link on or off.
  const shareBtn = document.getElementById("shareBtn");
  if (shareBtn) {
    const d = document.createElement("dialog");
    d.setAttribute("aria-labelledby", "shareTitle");
    d.innerHTML = '<h2 id="shareTitle">Share link</h2>' +
      '<p class="meta">Anyone with the link can view this, with live updates. They can\'t edit it or see your other notes.</p>' +
      '<div data-on><div class="share-row"><input type="text" id="shareUrl" readonly aria-label="Share link"><button class="btn" type="button" id="shareCopy">Copy</button></div>' +
      '<div class="actions"><button class="btn secondary" type="button" id="shareStop">Stop sharing</button><button class="btn secondary" type="button" data-close>Done</button></div></div>' +
      '<div data-off class="actions"><button class="btn secondary" type="button" data-close>Cancel</button><button class="btn" type="button" id="shareMake">Create link</button></div>';
    document.body.append(d);
    folio.enhance(d);
    const el = (id) => d.querySelector("#" + id);
    let armed = false;
    const show = (path) => {
      shareBtn.dataset.path = path;
      document.getElementById("shareState").textContent = path ? "On" : "Off";
      d.querySelector("[data-on]").hidden = !path;
      d.querySelector("[data-off]").hidden = !!path;
      el("shareUrl").value = path ? location.origin + path : "";
      el("shareStop").textContent = "Stop sharing";
      armed = false;
    };
    const post = (what) => fetch(shareBtn.dataset.base + "/" + what, { method: "POST" }).then((r) => {
      if (!r.ok) throw new Error(r.statusText);
      return r;
    });
    el("shareMake").onclick = () => post("share").then((r) => r.json()).then((j) => {
      show(j.path);
      el("shareUrl").select();
    }, () => folio.toast("Couldn't create the link. Try again."));
    el("shareCopy").onclick = () => navigator.clipboard.writeText(el("shareUrl").value).then(() => {
      el("shareCopy").textContent = "Copied ✓";
      setTimeout(() => (el("shareCopy").textContent = "Copy"), 1500);
    }, () => {
      el("shareUrl").select();
      folio.toast("Press Ctrl+C (⌘C on Mac) to copy");
    });
    el("shareStop").onclick = () => {
      if (!armed) {
        armed = true;
        el("shareStop").textContent = "Click again: the link will stop working";
        return;
      }
      post("unshare").then(() => {
        show("");
        folio.toast("Sharing stopped. The old link no longer works.");
      }, () => folio.toast("Couldn't stop sharing. Try again."));
    };
    shareBtn.onclick = () => {
      show(shareBtn.dataset.path);
      d.showModal();
    };
  }

  const closeMenus = (except) => document.querySelectorAll("details.menu[open]").forEach((m) => { if (m !== except) m.open = false; });
  document.addEventListener("click", (e) => closeMenus(e.target.closest("details.menu")));
  document.addEventListener("keydown", (e) => { if (e.key === "Escape") closeMenus(); });
})();
