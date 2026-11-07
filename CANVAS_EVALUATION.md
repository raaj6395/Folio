# Folio Canvas — evaluation report

> **2026-11-07, branch `canvas`.** Real headless Claude sessions (Claude Code CLI) drove a local Folio server with only the Folio tools. I checked every result myself: overview images, zoomed tiles, the live viewer in Chrome and WebKit, and the server's own checks.

---

## 1. Verdict

**It works.** Asked for a flowchart, Claude picks a canvas without being told. It writes a complete, correct diagram in one go, at full size, and can edit it later by request. Across 7 canvas sessions:
- all 7 produced a correct diagram;
- 0 had layout warnings;
- each cost $0.22–0.68 and took 9–71 s.

On A4, the same diagrams would have shrunk to unreadable 2–5 pt labels. In the A4 control session, Claude gave up and replaced its diagram with bullets.

**It is not yet "Excalidraw"**, and Section 6 lists what's missing. You can't move or draw anything by hand. Notes can't sit beside a diagram. Dense graphs get crossing edges. And Claude never zooms in to check a big canvas.

---

## 2. How it was tested

| What | How |
|---|---|
| **Claude's behaviour** | 10 headless sessions. Each had only the Folio MCP tools (`--strict-mcp-config`) and ran in an empty directory. None of the prompts named a tool. Logs were summarised per tool call. |
| **Output quality** | Each canvas was rendered by me as an overview and as zoomed tiles, then judged by eye. `check_layout` was run on each. |
| **Viewer** | Headless Chrome driven over CDP: load, fit, ctrl+scroll zoom, drag-pan, follow mode during a real Claude edit. Real WebKit (`WKWebView`, Safari's engine) via a small Swift harness. |
| **Regression** | The same A4 prompt run on `main` and on this branch. |
| **Code** | `go test ./...` passes on SQLite, including new tests for every canvas rule (Section 4). |

---

## 3. Results

| # | Prompt (abridged) | Format chosen | Outcome | Turns | Cost |
|---|---|---|---|---|---|
| 1 | "Make me a flowchart of how a pull request goes from opened to merged…" (canvas not mentioned) | **canvas, unprompted** | 10-node flowchart with 3 feedback loops, correct, 236×361 mm | 6 | $0.40 |
| 2 | "Put a mind map of machine learning on a canvas" | canvas | 18-node tree, correct and clean | 5 | $0.44 |
| 3 | "Make revision notes on TCP's handshake" | *(answered in chat, no Folio)* | n/a; reruns 4 and 5 used Folio, so this was variance | 1 | $0.18 |
| 4 | same, on `main` | A4 | 2 pages, score 100. **Its diagram came out at 3.2 pt on A4 and was replaced by bullets** | 15 | $0.77 |
| 5 | same, this branch | A4 (correctly not canvas) | 1 page, score 100 | 17 | $0.91 |
| 6 | "In my PR flowchart, add a security review step… and show missing approvals" | edited the canvas | Found the notebook, read the diagram, rewrote it with both changes; canvas grew to 451×411 mm | 8 | $0.51 |
| 7 | "Map the full architecture of a ride-hailing app on a canvas… with notes on two flows" | canvas | 18 nodes in 4 groups + 2 flow callouts, 513×540 mm, score 100 | 9 | $0.60 |
| 8 | "Sketch the microservices of a food delivery app **like an Excalidraw whiteboard**…" | canvas + **`notebook` (sketch) theme, unprompted** | Hand-drawn board, 759×452 mm, numbered order flow; one failed write first (bug F5, now fixed) | 7 | $0.47 |
| 9 | "Turn my TCP notes into a canvas" | switched format | One `set_theme` call; everything reflowed, no warnings | 4 | $0.22 |
| 10 | #7 again, after adding the "zoom in with tiles" hint | canvas | Same quality, 744×495 mm; **still didn't zoom in** | 11 | $0.68 |

**Viewer:**
- Chrome:
  - Live updates work.
  - With "follow live edits" on, the view panned to the changed block 14 s into session 6.
  - One wheel notch zooms 1.28×.
  - Text stays sharp at 150–400%.
- WebKit: the canvas renders correctly inside the page, with the right hand-drawn font.

---

## 4. Bugs found by this evaluation (all fixed, each with a test)

| # | Bug | How it showed up | Fix |
|---|---|---|---|
| F1 | Diagram labels rendered in a serif font in browsers | Chrome screenshot of the viewer | `diagram.retarget` threw away D2's embedded fonts; A4 never noticed because browsers only saw PNGs. It now keeps them, with the theme font as Typst's fallback. |
| F2 | Bold/italic labels still serif after F1 | Chrome screenshot | A Go slice-aliasing bug (`append` inside `ReplaceAllFunc`) overwrote the start of each font's data. Rewritten; the test now checks that the font data survives. |
| F3 | Zoom tiles showed the wrong part of the canvas (about 140 mm too low) | Comparing tile 1 with the overview | Typst centres an image taller than its region. It now goes in a tall enough block. A pixel-comparison test fails without the fix. |
| F4 | One mouse-wheel notch zoomed ~2.7× | CDP zoom test hit 400% in 8 events | Clamped and scaled: 1.28× per notch, trackpad pinch still smooth. |
| F5 | `size: "canvas"` was accepted by `preview_diagram` but rejected on the block | Session 8's first write failed | The block accepts it and stores it as `full`. |
| F6 | The overview offered zoom tiles even when it was already readable ("1 columns") | Session 1 | Tiles are only offered below 90 ppi; wording fixed. |
| F7 | `render_pages` ignored an explicit `ppi` on a canvas | Manual check | It is now respected, still within the 1568 px cap. |
| F8 | Q&A cards said "upside down at the bottom of this page" on a canvas | Session 9 render | They now say "canvas". |

---

## 5. What works well

- **Choosing the format.** Claude picked canvas unprompted for a flowchart (1), picked A4 for notes (4, 5), and picked the sketch theme from the word "Excalidraw" (8). The two-line hint in the server instructions is enough.
- **Completeness.** Diagrams covered what was asked, including failure paths and loops, in a single write, with 1–2 previews at most.
- **Editing.** "Add a step" works the way it does for notes: find, read, rewrite, done in 8 turns.
- **Size.** Every canvas was between 0.24 and 0.76 m. None hit a warning or limit. The largest D2 source was 1.3 KB (cap 10 KB).
- **Conversion.** Notes ↔ canvas is one call, and undo switches back.

---

## 6. What isn't there yet (ranked by how much it matters for the goal)

| # | Gap | Evidence | Why it matters |
|---|---|---|---|
| G1 | **No hand editing.** You can't drag a box, fix a label or draw an arrow yourself. | By design (read-only viewer). | It's the core of Excalidraw. Today every fix goes through Claude, including a 2-second nudge. |
| G2 | **Notes are stuck in a 174 mm column under the diagram.** | Sessions 7, 8, 10: on a 513–759 mm canvas, the notes fill the bottom-left third and the rest is empty. | It reads like a page with a big picture, not a whiteboard. |
| G3 | **Dense graphs get crossing edges and labels on lines.** | Session 7: edges cross "Ride matching" and "Location tracking". Session 1: "changes requested" sits on a crossing edge. | Readability drops as graphs grow, which is exactly where canvas is meant to win. D2's default layout (dagre) causes it; Claude can't fix it from the source. |
| G4 | **Claude never inspects big canvases up close.** | 0 of 3 sessions over 500 mm used tiles, even with an explicit hint (10). | Today's diagrams were still correct, but a broken big diagram would go unnoticed. |
| G5 | **Following edits is block-level.** | Session 6: the view jumped to the diagram's top-left, not to the new "Security review" node. | A canvas is usually one diagram block, so "show me what changed" points at the whole thing. |
| G6 | **The default look isn't Excalidraw.** | Default is `clean`, dark. The sketch look needs the word "sketch" or "Excalidraw". | First impressions differ from what users expect from "a canvas". |
| G7 | **Text-only canvases add nothing.** | Session 9: the converted notes became a 222 mm-wide strip. | Not harmful; Claude said so itself. Worth stating in the guide. |

**Not verified:**
- Postgres tests: no Docker here; run `make test-pg` before merging.
- Firefox, and pinch-zoom on a real iPhone or iPad.
- The published claude.ai skill (`anthropic-skills:folio-notes`) is the old version. The repo copy in `skills/folio-notes/` (untracked) has the canvas row and needs re-publishing.

---

## 7. How to improve (ordered by value per effort)

| Priority | Change | Fixes | Effort | Notes |
|---|---|---|---|---|
| **1** | **Export to Excalidraw (`.excalidraw`)** | G1, and partly G6 | 2–3 days | D2 already computes every shape's position, size, label and arrow route (`d2target.Diagram`). Map them to Excalidraw elements: rectangle, ellipse, diamond, text, arrow with points. Add a **"Open in Excalidraw"** button that downloads the file. Users then hand-edit in excalidraw.com without Folio building an editor. One-way (no import back) in v1. |
| **2** | **Notes beside the diagram** | G2 | 1–2 days | On a canvas, a top-level `two_column` becomes `grid(columns: (auto, 120mm))`. The left diagram is drawn at natural size; Go adds the note column's width to the canvas width. The guide says "put notes beside the diagram". No new block type. |
| **3** | **Better layout for dense graphs** | G3 | 1 day + measuring | Allow D2's ELK engine (`d2elklayout`, ships with D2) when the source asks for it, or by default above ~20 nodes. Measure speed first: ELK runs through an embedded JS engine and may be several times slower than dagre on the 0.1-CPU host. |
| 4 | **Node-level "what changed"** | G5 | 1–2 days | D2's SVG gives each shape a class derived from its ID. Diff the D2 sources between versions, send the changed shapes' positions in the event stream, and pan to and briefly highlight them. |
| 5 | **Decide the canvas default look** | G6 | minutes | Option: a new canvas defaults to `notebook` theme + light mode (Excalidraw-like). Your call, because the rest of Folio is dark-first. |
| 6 | **Verification: accept or force** | G4 | ~1 hour | Either accept it (diagrams were correct and Claude can't fix layout anyway), or have `render_pages` on a big canvas attach up to 2 tiles automatically. That costs ~3k extra tokens per check. I'd accept it and revisit if users report broken big diagrams. |
| 7 | Guide line: "use a canvas for diagram-led content, not text" | G7 | minutes | |
| 8 | Close the gaps in "Not verified" above | | ½ day | Postgres run, Firefox, iPad pinch, re-publish the skill. |

---

## 8. Aligning the system with the goal

**The goal:** Claude makes complete, Excalidraw-quality diagrams on an unbounded canvas, and people can view, share and adjust them.

| Goal | Today | After 1 + 2 + 3 + 5 |
|---|---|---|
| Claude writes the whole thing | ✅ 7/7 sessions | ✅ |
| Unbounded, readable at any size | ✅ natural size, sharp SVG zoom | ✅ |
| Looks like a whiteboard | ⚠️ only when asked | ✅ sketch + light by default (if you choose it) |
| Notes arranged around the drawing | ❌ stacked below | ✅ beside the diagram |
| Readable when dense | ⚠️ crossings above ~20 nodes | ✅ ELK for big graphs |
| Adjust by hand | ❌ ask Claude | ✅ via Excalidraw export (one-way) |
| See what Claude changed | ⚠️ jumps to the diagram | ✅ highlights changed nodes (item 4) |

**One principle to keep:** Claude describes the graph and a layout engine places it. Every session produced a correct diagram *because* Claude never chose coordinates. The improvements above add hand editing **after** Claude's draft (export to Excalidraw) instead of asking Claude to place boxes. They keep that strength.

---

## Appendix: reproduce

- **Server:** `FOLIO_HOME=<dir> FOLIO_HTTP_ADDR=127.0.0.1:8792 bin/folio http --no-auth`.
- **Session:** run `claude -p "<prompt>" --strict-mcp-config --mcp-config mcp.json --allowedTools mcp__folio --output-format stream-json --verbose` in an empty directory, with `mcp.json` = `{"mcpServers":{"folio":{"type":"http","url":"http://127.0.0.1:8792/mcp"}}}`.
- **Model:** claude-opus-4-8 (the CLI default on this machine).
- **Totals:** 10 sessions, $5.18 in all.
