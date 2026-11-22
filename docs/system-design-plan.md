# System notebooks: interactive system designs built by AI

Status: implemented on branch `system-notebooks` (M0–M5; M6 eval still to run) · 2026-11-22

## 1. What we are building

A new kind of notebook: a **system**. It sits in the library next to A4 notebooks and canvases.

- **Claude builds it.** The student chats ("design a URL shortener"). Claude writes the system through MCP: nodes, how requests flow, and what happens when parts fail.
- **The student plays with it.** Opening it shows the whole system full screen. The student can:
  - pick a request and **Send** it, then watch a dot travel node to node, with a log line for each step and a result like `200 OK · 34 ms`;
  - **kill** any node (or a whole zone), send again, and see the new path or the error;
  - **revive** nodes, flush caches and reset;
  - click a node to read what it does and what breaks when it is down;
  - pick a ready-made **scenario** ("Redis is down") that sets up the failures and sends the request.
- **Changes go through Claude.** "Add a queue before the database." The open screen updates live. The browser never edits anything.

### Out of scope

PDF and export, editing in the browser, load or traffic simulation, quizzes, slow nodes and timeouts, circuit breakers, replication lag. Each could be added to the engine later without changing the format.

## 2. Requirements

| # | Requirement |
|---|---|
| R1 | Claude writes JSON, never coordinates or code. Every mistake comes back as an error it can fix, pointing to where the mistake is. |
| R2 | Killing **any** node or combination of nodes behaves correctly, with no failure paths written by hand. |
| R3 | Covers the usual system design building blocks: load balancers, services, caches, databases with replicas, queues with workers, parallel calls, async calls, retries, locks and rate limits. |
| R4 | Live updates while Claude edits. Undo, versions, folders, rename, delete and share links work as they do for other notebooks. |
| R5 | Safe on public share links: no HTML injection, bounded CPU, rate limited. |
| R6 | A system keeps behaving the same after engine upgrades. |
| R7 | One person can maintain it: Go plus plain JS, no Node toolchain, no new Go dependencies. |

## 3. Architecture

```
Claude ──MCP──► system block (JSON spec) ──► store (versions, undo, share) ──SSE──► viewer
                     │
                     ├─ validate: structure + static walk + exhaustive runs + scenario checks
                     ├─ layout:   spec → D2 source → diagram.Render → node boxes + edge paths (cached)
                     └─ engine:   Run(spec, state, action) → events + new state   (pure Go, deterministic)

viewer (system.html + system.js)
   GET  /n/{id}/system.json   → spec + layout
   POST /n/{id}/system/run    → engine result; the browser animates it
```

Decisions behind this:

1. **Behaviour model, not scripted paths.** Each node has a *kind* with fixed behaviour. Services run *handlers*: short step lists with a branch per outcome. The engine works out the path, so any set of failures works (R2). Scripted paths (the React Flow demo) need a hand-written scenario for every failure combination.
2. **One engine, in Go, with no stored state.** The same `Run` is used for validation, the `simulate_system` MCP tool and the viewer. The browser keeps the simulation state (dead nodes, cache contents…) and sends it with every action. The server stores nothing per viewer, and the browser holds no logic of its own that could drift from the Go code.
3. **D2 does the layout on the server; the browser draws.** D2 is already used, already cached, and already returns node boxes and edge routes (`diagram.Geometry`). The browser draws its own cards and edges from that geometry, so it fully controls the look, the buttons and the moving dots. No coordinates in the spec.
4. **No React Flow.** With no browser editing, its features (drag, connect, edit) would go unused. Plain SVG plus `<animateMotion>` gives the same result with no build step.
5. **A system is a notebook with `format: "system"` and exactly one `system` block.** Versions, undo, restore, folders, share links and SSE all work unchanged. Claude edits the system with the existing `update_block`.

## 4. The spec (engine 1)

### 4.1 Example: product read path

```json
{
  "type": "system",
  "engine": 1,
  "direction": "right",
  "nodes": [
    {"id": "client", "kind": "client",        "label": "Browser"},
    {"id": "lb",     "kind": "load_balancer", "label": "Load Balancer", "pool": ["app1", "app2"]},
    {"id": "app1",   "kind": "service",       "label": "App 1", "group": "Zone A"},
    {"id": "app2",   "kind": "service",       "label": "App 2", "group": "Zone B"},
    {"id": "redis",  "kind": "cache",         "label": "Redis",
     "about": "Keeps hot products in memory.", "when_down": "Every read goes to Postgres; latency jumps."},
    {"id": "pg",     "kind": "database",      "label": "Postgres", "ms": 30,
     "about": "Source of truth for products.", "when_down": "Cache misses fail with 502."}
  ],
  "handlers": {
    "get_product": [
      {"call": "redis", "op": "get", "on": {"hit": "return", "down": "next"}},
      {"call": "pg",    "op": "read"},
      {"call": "redis", "op": "set", "async": true}
    ]
  },
  "requests": [
    {"id": "read", "label": "GET /product/{key}", "to": "lb", "handler": "get_product",
     "keys": ["product:7", "product:9"]}
  ],
  "scenarios": [
    {"label": "Redis is down", "kill": ["redis"], "request": "read",
     "expect": {"status": 200, "visits": ["pg"]},
     "explain": "Cache-aside falls back to the database, so reads still work but take ~33 ms instead of ~3 ms."}
  ]
}
```

What this example shows:
- **There are no links and no positions.** Edges are derived from `pool`, `call`, `publish`, `replicas` and `consumers`. An optional `links: [{"from","to","label"}]` only adds labels such as `gRPC`, and it must match a derived edge.
- **There are no reroute tables.** Kill Redis and its `down` branch runs. Kill both app servers and the LB answers 503. Kill Postgres and the error travels back up the chain as a 502.
- **`say` is optional.** Every kind has default log lines.
- **`expect` is checked when Claude writes the block.** A scenario that doesn't behave as described is a validation error.

### 4.2 Nodes

| Field | Meaning |
|---|---|
| `id` | `[a-z0-9_]{1,40}`, unique |
| `kind` | `client`, `load_balancer`, `service`, `cache`, `database`, `queue` |
| `label` | ≤ 40 characters; defaults to the id |
| `icon` | optional, from a fixed list (server, cache, database, queue, worker, cdn, gateway, external, storage, search, auth, phone); defaults by kind |
| `group` | optional zone, tier or region name; drawn as a container, and the whole group can be killed at once |
| `ms` | processing time; defaults: client 0, load_balancer 1, service 5, cache 1, database 20, queue 2 |
| `about`, `when_down` | text for the node panel |
| `pool` | load_balancer only: service ids |
| `strategy` | load_balancer only: `round_robin` (default) or `hash` (by key, so killing a member visibly re-maps keys) |
| `replicas` | database only: database ids. Reads go round robin to live replicas, falling back to the primary; writes go to the primary. |
| `consumers`, `handler` | queue only: service ids that take messages, and the handler they run |
| `persistent` | cache only: keep keys when killed (by default a killed cache comes back empty) |

### 4.3 Steps (inside handlers)

| Step | Meaning |
|---|---|
| `{"call": "<node>", "op": "<op>", "key": "{key}"}` | Call a cache or database. `key` defaults to the request key. |
| `{"call": "<service or lb>", "handler": "<name>"}` | Call a service (or an internal LB) and run that handler there. |
| `{"publish": "<queue>"}` | Put a message on a queue; a consumer handles it later. |
| `{"parallel": [[steps], [steps]]}` | Run branches at the same time. Fails if any branch fails (unless that branch handles it). |
| modifiers on a call | `"async": true` (fire and forget), `"retry": n` (on down or error), `"on": {...}`, `"say": "..."` |
| `{"call": "<node>", "force": "<outcome>", "say": "..."}` | Escape hatch: a fixed outcome with a narration, for logic the engine doesn't model (geo matching, ranking…) |

Each entry in `on` maps an outcome to one of: `"next"` (continue), `"return"` (stop, success), `"fail"` (stop, error), or `[steps]` (run these steps, then continue unless one of them returns or fails).

### 4.4 Outcomes

| Target / op | Outcomes | Default action | HTTP status if it fails the request |
|---|---|---|---|
| service (handler) | ok · error · down | ok → next, else fail | error 500, down 502 |
| load_balancer | ok · unavailable (no live member) · down | ok → next, else fail | unavailable 503, down 502 |
| cache `get` | hit · miss · down | hit, miss → next; down → fail | 502 |
| cache `set` `del` | ok · down | ok → next | 502 |
| cache `setnx` (locks, idempotency) | ok · taken · down | taken → fail | taken 409 |
| cache `incr` + `"limit": n` (rate limit) | ok · limited · down | limited → fail | limited 429 |
| database `read` `write` | ok · down | ok → next | 502 |
| queue `publish` | ok · down | ok → next | 502 |

### 4.5 Requests and scenarios

- **Request:** `id`, `label` (may use `{key}`), `to` (the entry node), `handler`, `keys` (1–10). `from` is needed only when there is more than one client.
- **Scenario:** `label`, `kill` (node ids or group names), `request`, optional `key`, optional `expect` (`status`, `visits`, `avoids`), `explain`.

### 4.6 Engine state

The browser holds this and sends it with every action. The server validates its size and contents.

```json
{"down": ["redis"], "cache": {"redis": ["product:7"]}, "counters": {"redis": {"ip:1": 3}},
 "rr": {"lb": 5}, "backlog": {"orders_q": [{"handler": "send_email", "key": "order:1"}]}}
```

Actions are `send`, `kill`, `revive`, `reset` and `flush`. Reviving a queue consumer delivers the waiting messages, which comes back as events.

### 4.7 Engine output

```json
{"status": 200, "total_ms": 34,
 "events": [{"t": 0, "dur": 1, "from": "client", "to": "lb", "dir": "req", "text": "Browser → Load Balancer: GET /product/product:7"},
            {"t": 1, "dur": 1, "from": "lb", "to": "app2", "dir": "req", "text": "Load Balancer → App 2 (round robin; App 1 is down)"},
            {"t": 7, "dur": 1, "from": "redis", "to": "app2", "dir": "res", "outcome": "miss", "text": "Redis: MISS for product:7"},
            {"t": 40, "dur": 1, "from": "app2", "to": "redis", "dir": "req", "async": true, "text": "App 2 → Redis: SET product:7 (async)"}],
 "state": {"...": "..."}}
```

- A request hop takes the target's `ms`, and a response hop takes 1 ms, so a slow database visibly takes longer.
- Parallel branches are just events with overlapping `t`.
- Every event's `from → to` is a derived edge, in one direction or the other. This always holds, and a test checks it.

## 5. Validation (when Claude writes the block)

Errors point to where the mistake is and suggest a fix, for example `handlers.get_product[1].call: unknown node "postgres" (did you mean "pg"?)`.

1. **Structure.** Unique ids; known kinds; fields allowed only on their kind (`pool` only on a load_balancer…); pool members are services; replicas are databases; consumers are services.
2. **References.** Every `call`, `publish`, `handler`, `request.to`, `scenario.kill` and `links` entry exists. Each `op` is valid for its target, and each `on` outcome is possible for that op.
3. **Static walk.** Starting from every request, follow every branch to derive edges and which node runs which handler. It reports call cycles (A → B → A), handlers no request ever reaches, and nodes with no edges. Cycles are errors; the rest are warnings.
4. **Exhaustive runs.** For every request × first 2 keys × (nothing dead, plus each single node dead): run the engine. It must finish within its limits, and every event must sit on a derived edge.
5. **Scenarios.** Run each one and check `expect`.
6. **Limits.** At most: 60 nodes, 40 handlers, 300 steps, nesting depth 5, 40 KB of spec, 2,000 events per action, 50 waiting messages per queue.

`engine` defaults to 1 and is saved explicitly, so a future engine 2 never changes how old systems behave (R6).

## 6. MCP: how Claude works with it

**Typical session**
1. `get_authoring_guide` (new "System" section) → `create_notebook {format: "system"}` → `append_blocks [system block]`. Errors are fixed until the block is accepted.
2. `simulate_system` on each request, plus a kill or two, to check the behaviour → `get_viewer_url` → reply with the link.
3. "Add a rate limiter" → `read_blocks` → `update_block` with the new spec. The open viewer updates live.
4. "What happens at checkout if Redis dies?" → `simulate_system {kill: ["redis"]}` → Claude explains using the real log.

**Changes**

| Tool | Change |
|---|---|
| `create_notebook` | `format: "system"` is accepted (`store.Formats` gets "system"). Template is ignored. |
| `append_blocks`, `update_block`, `insert_blocks`… | A system notebook holds exactly one block, of type `system`. A `system` block is rejected anywhere else. |
| write result (`MutationResult`) | For systems, `layout` is replaced by a `system` summary: node/edge/request counts, `scenarios: "3/3 pass"`, warnings. |
| **new `simulate_system`** | `{notebook_id, request, key?, kill?[]}` → status, total ms and the numbered log. Read-only. |
| `render_pages`, `inspect_render`, `check_layout`, `get_layout_info`, `export_pdf` | On a system notebook they return a clear error that points to `simulate_system`. |
| `set_theme` | Theme and mode work. Switching format to or from `system` is refused. |
| `undo`, `list_versions`, `restore_version`, `rename`, `move`, `delete`, `get_outline`, `read_blocks` | Unchanged. |
| server instructions + `guide.md` | Routing rule: "design / build / simulate a system" → system notebook; "notes on system design of X" → A4 `system_design` template. Also the spec reference from §4, with one full example. |
| `skills/folio-notes` | A short section on system notebooks. |

## 7. Viewer

**Routes** (each also at `/s/{token}/…` for share links)
- `GET /n/{id}` → `system.html` when the format is `system`.
- `GET /n/{id}/system.json` → `{version, spec, layout: {w, h, nodes: [{id, x, y, w, h}], groups: [...], edges: [{id, src, dst, points, curve, label}]}}`. The layout comes from spec → D2 (fixed card size per node, groups as containers, `direction`) → `diagram.Cached`.
- `POST /n/{id}/system/run` → engine result. Request body is limited to 64 KB, with cross-origin protection (`cop`). On share routes it is rate limited per IP with the existing `account.Limiter`.
- `GET /n/{id}/thumb.png` → for systems, the D2 SVG of the layout (an `<img>` shows it fine).
- `GET /n/{id}/pdf` and `canvas.svg` → 404 for systems.
- `state()` (used by SSE) skips `lb.Build` (Typst) for systems and only reports the version.

**Screen**

```
┌ ← Title ·························· theme · share · ⋯ ┐
│                                                    │ ┌ Node / Log ────────┐
│   [Browser] ──► [LB] ──► [App 1]──►[Redis]          │ │ Redis        DOWN  │
│                     └──► [App 2]──►[Postgres]       │ │ Keeps hot items…   │
│   (pan · zoom · cards show UP·3 hops / DOWN, Kill)  │ │ When down: …       │
│                                                    │ │ [Revive]           │
│                                                    │ ├────────────────────┤
│                                                    │ │ 3 Redis: MISS …    │
├────────────────────────────────────────────────────┤ │ 4 App 2 → Postgres │
│ Request [GET /product ▾] Key [product:7 ▾] [Send]   │ └────────────────────┘
│ Try: [Redis is down] [Zone A outage]   Speed 1× · Reset · 200 OK · 34 ms │
└────────────────────────────────────────────────────┘
```

- **Cards** are HTML on top of an SVG for edges, both in one board that pans and zooms. Each card shows an icon, the label, a status line (`UP · 3 hops` or a red `DOWN`), and a Kill/Revive button. Caches show their key count and queues their backlog.
- **Groups** are dashed containers with a label and their own Kill button.
- **Dots** are a `<circle>` with `<animateMotion>` along the edge path. Responses use `keyPoints="1;0"`, async calls show as a hollow dot, failures in red. Hop duration is `(350 + ms × 25) / speed`. Send is disabled while a request plays, so only one request is in flight at a time.
- **The path** of the last request stays highlighted, and the other nodes are dimmed, so large systems stay readable.
- **Panel:** clicking a node shows its details; the log panel is a `column-reverse` container (stays scrolled to the bottom with no JS) and has `aria-live`.
- **Live updates:** an SSE version change refetches `system.json`, keeps dead nodes that still exist, and shows a toast "Updated by Claude".
- **Reuse:** the pan/zoom code moves from `canvas.html` into `app.js` and both pages use it. The header, theme toggle, share menu and toasts are reused as they are.
- **Accessibility:** Enter sends, Esc closes the panel, cards are focusable and `k` kills or revives the focused one. With `prefers-reduced-motion`, there are no moving dots; path edges light up in order instead. On a phone, the panel becomes a bottom sheet.
- **Library:** system cards get a "System" label and the layout thumbnail.

## 8. Code map

| File | Work |
|---|---|
| `internal/sim/spec.go` | Spec types, defaults, structural validation |
| `internal/sim/walk.go` | Static walk: edges, which node runs which handler, cycles, unreached handlers |
| `internal/sim/engine.go` | `Run(spec, state, action) → Result`; all node kinds, ops and step types |
| `internal/sim/check.go` | Exhaustive runs and scenario `expect` |
| `internal/sim/layout.go` | Spec → D2 source → `diagram.Render` → layout JSON |
| `internal/sim/*_test.go`, `testdata/system/*.json` | Golden traces, table of validation errors, fuzz test |
| `internal/model/blocks.go` | Register the `system` block type (wraps `sim.Spec`; `model` imports `sim`, never the reverse) |
| `internal/store/store.go` | `Formats` gets `"system"` |
| `internal/mcpserver/{tools,edit,server,inspect}.go` | Guards from §6, system result summary, `simulate_system` |
| `internal/mcpserver/guide.md`, server instructions | System section and routing rule |
| `internal/viewer/viewer.go` | Routes from §7, `state()` and `thumb` branches |
| `internal/viewer/system.html`, `system.js` | The simulator page |
| `internal/viewer/app.js`, `canvas.html` | Shared pan/zoom |
| `internal/viewer/icons.svg`, `style.css` | System icons, card/edge/dot styles with light and dark tokens |
| `README.md`, `DECISIONS.md`, `PROGRESS.md`, `skills/folio-notes` | Docs |

## 9. Milestones

| # | Deliverable | Done when | Size |
|---|---|---|---|
| **M0** | Three reference systems in `testdata/system/`, each with a hand-written trace for its main request and two failures: **read path** (LB, cache, DB), **URL shortener** (DB replicas, hash LB, rate limit), **ticket booking** (seat lock via `setnx`, payment `parallel`, email via queue + worker). | All three can be expressed with the §4 vocabulary without the escape hatch, or the vocabulary is fixed first | 1 d |
| **M1** | `internal/sim`: spec, walk, engine, checks, tests | Engine output matches all hand traces; the fuzz test (random kill/revive/send sequences) never panics and always finishes; every validation rule has a test with its exact message | 3 d |
| **M2** | Storage and MCP wiring (§6), `simulate_system`, guide | In Claude Code: create a system, add the read path, simulate with Redis killed, undo. Layout tools give clear errors. | 1.5 d |
| **M3** | Layout + `system.json` + thumbnail | All three references lay out readably (checked by eye in the browser), with groups drawn as containers | 1 d |
| **M4** | Simulator page (§7) | Every interaction in §1 works on all three references, in light and dark, on desktop and phone width; a share link works logged out; live update while Claude edits | 4 d |
| **M5** | Hardening | Rate limit and body limits tested; viewer route tests (auth, share, 404 for pdf); reduced motion; README/DECISIONS/PROGRESS updated | 1 d |
| **M6** | Authoring eval | Claude builds 10 different systems from one-line prompts via MCP; ≥ 8 are accepted on the first write. Improve error messages and the guide until they are. | 1 d |

**Total: about 12 working days.** M0 comes first because it is the cheapest place to find out the spec is wrong. M6 is what makes this production quality: Claude is the only author, so how good its systems are depends on the guide and the error messages.

## 10. Risks

| Risk | Mitigation |
|---|---|
| Spec too rich, so Claude makes mistakes | Small fixed vocabulary, defaults everywhere, errors that suggest a fix, the M6 eval loop |
| D2 layout unreadable at 25+ nodes | Groups as containers, highlighting the path; if still poor, try D2's ELK layout or add optional `rank` hints. Never coordinates. |
| Engine semantics change and break old systems | `engine` version field; golden traces per version |
| Share-link abuse of `/system/run` | Per-IP limiter, 64 KB body, event and backlog limits, deterministic engine with no stored state |
| Viewer JS grows large | Plain modules; if the panel UI grows past what plain JS handles, add Preact + htm as one vendored ES module (no build step) |

## 11. Assumptions (say if any is wrong)

1. "Design X" goes to a **system notebook**; "notes on the system design of X" stays an A4 note with the `system_design` template.
2. System notebooks have **no PDF button** and are not exported.
3. Share links can run the simulation (kill, send) but never change the system.
4. Package name `internal/sim`; block type and format both named `system`.
