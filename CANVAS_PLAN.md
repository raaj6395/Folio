# Folio Canvas — plan: one big sheet for flowcharts, maps and mind maps

> **Status: C1 built 2026-11-07 on branch `canvas`**, with the Section 11 defaults (switchable format, stacked layout, SVG viewer, 10,000-char diagram cap, dotted paper, C1 only). C2 built the same day: follow live edits (the event stream carries each canvas block's position and content hash), pan to a heading from the contents list, keyboard panning, a remembered view, and `render_pages` **tiles** in place of a `region` (numbered 390×260 mm tiles at 100 ppi, so Claude zooms in without coordinates). Three changes from this draft, found while building:
> 1. **No `measure`/wrapper blocks.** Wrapping each block in a fixed-width card dropped its `above` spacing (a heading's 30 mm gap became 5 mm). Instead, Go computes the canvas width from the diagram widths it already knows. The page gets that width and a wide right margin, so text keeps the 174 mm column and wider diagrams extend into the margin (5.3).
> 2. **Diagram fonts in browsers.** `diagram.retarget` used to strip D2's embedded fonts, which was fine while browsers only ever saw PNGs. In the canvas SVG, the browser renders the diagram itself, so labels fell back to serif. D2's subset fonts are now kept, with the theme font as the fallback for Typst. `renderVersion` is r4 so caches refresh.
> 3. **Spike done:** Chrome (driven over CDP) and real WebKit (`WKWebView`) both render the nested SVG with its fonts inside `<img>`, sharp at 150%. Firefox and a real iPhone pinch are still unchecked.

---

## 1. The idea in one paragraph

Today every notebook is A4. A diagram is squeezed into a 174 mm column and capped at 60% of a page, so a 30-step flowchart becomes unreadable (`unreadable_diagram`) or has to be split. **A canvas is a notebook with no pages**: one sheet that grows to fit its content, in the spirit of Excalidraw. Diagrams keep their natural size, the viewer pans and zooms, and the output is SVG, PNG or a single-page PDF. Claude still never uses coordinates. It writes the same blocks and the same D2, and Folio lays everything out.

```
You: "Draw the whole checkout flow as a canvas: cart, payment, fraud checks, retries, emails"
 └─► Claude: create_notebook(format: "canvas") ─► append_blocks: title + one 40-node D2 diagram + notes
        └─► Folio: diagram at natural size (no A4 cap) ─► one auto-sized sheet
               ├─► viewer: drag to pan, pinch/ctrl+scroll to zoom, live updates
               └─► Download: SVG (infinitely sharp) · PDF (one page, custom size) · PNG
```

---

## 2. How it works today (the flow an engineer must know)

### 2.1 Write path (Claude → pages)
1. **MCP tool** (`internal/mcpserver/tools.go`, `edit.go`), e.g. `append_blocks`.
2. **Validate:** `model.ParseAll` checks every block (`internal/model/blocks.go`). `checkDiagram` compiles each D2 source once to catch errors.
3. **Store:** `store.Mutate` (`internal/store/store.go`) locks the notebook, diffs and writes blocks, bumps `version`, and writes a **snapshot** (blocks + theme/paper/mode/guidelines/density) for undo.
4. **Layout:** `Server.build` → `layout.Builder.Build` (`internal/layout/layout.go`), cached in `layout.json` by `key(nb)`.
   - `docBlocks` (`themes.go`) renders each diagram with D2 to an SVG file and sizes it with `Fit()`.
   - It writes `doc.json`. `templates/main.typ` is fixed; content enters only as data.
   - `typst eval` queries the `<blockpos>` markers, and `analyze()` (`analyze.go`) turns them into page, y, fill % and issues.
5. **Reply:** `MutationResult` (page per block, fill %, warnings, `layout_hints`).

### 2.2 Read path (viewer)
- `/n/{id}` serves `notebook.html`.
- An SSE stream at `/n/{id}/events` polls `version` every 500 ms and sends `State` (page fingerprints and the outline).
- The browser fetches `/n/{id}/page/N.png?v=<fingerprint>`. `RenderPNG` runs `typst compile --pages N --ppi 110`, cached per fingerprint.
- PDF comes from `/n/{id}/pdf` (`Builder.PDF`). Dashboard thumbnails come from `/n/{id}/thumb.png` (page 1 at 72 ppi).

### 2.3 Where "A4" is baked in (everything a canvas must change)
| Where | A4 assumption | Canvas change |
|---|---|---|
| `templates/main.typ` | `paper: "a4"`, header/footer, `content-height = 257mm`, `nosplit`, `pagebreak`, diagrams centred inside `layout()` | Auto-sized page, fixed-width text blocks, `page_break` does nothing (5.3) |
| `layout/themes.go` `Fit()` | Width ≤ 174 mm text column, height ≤ 60% of the page | Natural size, no height cap (5.4) |
| `layout/analyze.go` + `rules.go` | Fill %, overflow past 277 mm, orphan heading, under-filled, pushed block, spill, oversized diagram | Canvas-only checks (5.4) |
| `layout/layout.go` `RenderPNG` | Fixed ppi (110 viewer, 80 Claude, 72 thumb) | A 2 m canvas at 110 ppi is 8,700 px wide. Cap by pixels instead (5.5) |
| `viewer/style.css` / `notebook.html` | `.sheet { aspect-ratio: 210/297 }`, pager, outline maths `y_mm / 297` | New `canvas.html` with pan/zoom (5.7) |
| `viewer/style.css` dashboard | `.thumb img { object-fit: cover; object-position: top }` | `contain` for canvases, plus a "Canvas" pill |
| `mcpserver/server.go` instructions, `guide.md`, `layout_rules.md` | "typesets … onto A4 pages", "each H1 fits on one page" | Short canvas section (5.6) |

---

## 3. What we verified before planning (spike, Typst 0.15.1 and D2 0.7.2 on this machine)

| Question | Result | Consequence |
|---|---|---|
| Does `set page(width: auto, height: auto)` give one sheet that fits its content? | **Yes.** Paper tilings (dots, grid, ruled) still fill the background. | No new engine needed: same template, one extra `set page … if canvas`. |
| Do text blocks wrap on an auto-width page? | **No.** An unwrapped paragraph rendered as a single line **875 mm** long. | Every non-diagram block must sit inside `block(width: 174mm)`. Inside it, `width: 100%`, callouts, tables and `line(length: 100%)` all work unchanged. |
| `grid(columns: (1fr, 1fr))` (our `two_column`) on an auto-width page? | Unwrapped, it **collapses to near-zero width** (one word per line). Wrapped in a 174 mm block, it's fine. | `two_column` gets the same wrapper. |
| Can we read the canvas size with `context page.width`? | **No**, it returns `auto`. `measure(body)` inside `context` does work. | *Built differently:* Go sets the width (it knows every diagram's width before Typst runs); the height is the `_end` marker plus the margin. |
| `pagebreak(weak: true)` on an auto page? | **Creates a second page.** | `page_break` must render nothing on a canvas. |
| `place(bottom, float: true)` (hidden Q&A answers)? | Works; lands at the bottom of the canvas. | Keep as is. |
| `typst compile main.typ out.svg`? | Works for a single page. The D2 diagram is embedded as a `data:` image. Small test: 137 KB. | The viewer uses SVG: sharp at any zoom. |
| D2 dagre layout time on big graphs (tree + back-edges) | 20 nodes 0.1 s · 60 nodes 0.2 s · 120 nodes 0.45 s · **250 nodes 1.1 s** (7 KB source, 194 KB SVG) | Big flowcharts are feasible. Expect ~10× slower on Render's free tier, so cap the source size (5.8). |

The spike scripts were throwaway; the layout tests in Section 8 encode each of these findings, so a regression fails CI.

---

## 4. What users get

### C1 — Canvas you can see (must have)
1. **"Make it a canvas"**: `create_notebook(format: "canvas")`. Claude picks it on its own for flowcharts, architecture maps, mind maps, whiteboards and "too big for a page".
2. **Diagrams at natural size**: no 174 mm cap and no 60% height cap. Labels stay at a readable ~11 pt however big the graph gets.
3. **Notes around the diagram**: a title, paragraphs, callouts, tables, code and Q&A, as 174 mm cards stacked top to bottom. Use `two_column` to put notes beside a small diagram.
4. **Pan-and-zoom viewer**: drag to pan; pinch, ctrl/⌘+scroll or `+ − 0` to zoom; fit-to-screen. Live updates keep your pan/zoom position. Dot-grid background.
5. **Exports**: **SVG** (new), **PDF** (one page sized to the canvas) and **PNG**.
6. **Switch any time**: *"turn these notes into a canvas"* / *"put it back on A4 pages"* is one undoable change, because blocks are identical in both formats.
7. **Everything else for free**: themes (the `notebook` theme gives hand-drawn, Excalidraw-like sketch diagrams), dark/light, versions and undo, edit by block ID, folders, privacy.

### C2 — Canvas polish (should have)
8. **Follow live edits** pans to the block Claude just changed. **Contents** sidebar: click a heading to pan to it.
9. **`render_pages` with a `region`**, so Claude can zoom in on part of a huge canvas to check details.
10. Keyboard: arrows pan, `1` = 100%, `0` = fit; zoom level remembered per notebook.

### Not in this plan → Section 10.

---

## 5. The design

### 5.1 A canvas is a *format*, not a new kind of document
- New notebook property **`format`**: `a4` (default) or `canvas`. It sits next to `paper`/`mode`/`density`, and is stored on the notebook **and in snapshots**, so switching is undoable.
- **Why not a separate "canvas" product?** Storage, versions, block tools, the isolation matrix, folders, themes, SSE and exports all work unchanged. Switching formats costs nothing, because the same blocks render both ways.
- **Why no coordinates?** Folio's rule is "never think in coordinates" (server instructions), and for good reason: LLMs place boxes poorly, and D2 already does graph layout well. The canvas is free-*size*, not free-*placement*. Free placement is Section 10.

### 5.2 Content rules on a canvas
- **All block types are allowed**, so switching never fails validation.
- Top-level blocks stack **top to bottom**. Diagrams take their natural width; every other block is a **174 mm card**, the same width as A4 text, so text looks identical.
- `diagram.size` (`full`/`half`/`small`) is **ignored at top level** on a canvas (natural size). Inside `two_column` it keeps today's meaning.
- `page_break` renders nothing and raises a `page_break_ignored` issue. Its marks are kept, so `analyze` still sees the block.
- The diagram source limit is the same for both formats (5.8).

### 5.3 Template (`templates/main.typ`), as built
```typ
#let canvas = doc.at("format", default: "a4") == "canvas"
// after the existing A4 `set page`:
#set page(width: doc.canvas_w_mm * 1mm, height: auto, header: none, footer: none,
  margin: (left: 24mm, top: 24mm, bottom: 24mm, right: (doc.canvas_w_mm - 198) * 1mm)) if canvas
```
- **Width:** Go sets `canvas_w_mm` = 24 + max(174, widest top-level diagram) + 24. Text flows in the normal 174 mm column, so spacing, `two_column` and callouts are untouched, with no wrappers. Wrapping each block was tried and dropped the blocks' `above` spacing.
- **Top-level canvas diagrams** (`"canvas": true` in `doc.json`) skip `nosplit`/`layout()`/centring. They are drawn at `width_mm` and extend into the right margin; the caption is centred under the image.
- **`page_break`:** only calls `pagebreak` `if not canvas`.
- **Title:** a small muted title at the top, which replaces the A4 header.
- **A4 output is unchanged:** guarded by `TestA4Unchanged` (golden block positions for every theme).

### 5.4 Layout and analysis (Go, `internal/layout`)
- **`key()`** (`layout.go`) and the **`Fingerprints`** header (`fingerprint.go`): add `nb.Format`.
- **`doc.json`:** add `"format"`.
- **Canvas size:** the width comes from Go (5.3); the height is the `_end` marker + 24 mm. Floated hidden-Q&A answers can add a few mm that this misses; that only affects the PNG pixel cap, which already rounds down.
- **`Result`:** add `CanvasWMM` and `CanvasHMM`, as `json:"canvas_w_mm,omitempty"` and `json:"canvas_h_mm,omitempty"`.
- **`Fit()`:** add a `canvas bool` parameter, threaded through `docBlocks`. A canvas has **no column or height cap**: `wmm = natural × maxUpscale`, so labels are about 10.8 pt and `unreadable_diagram` cannot trigger at top level. Diagrams nested inside `two_column` keep today's `Fit` (a size fraction of 174 mm). The template still clamps them to the column width.
- **`analyze()`:** add `canvas bool`. When true:
  - `PageCount = 1` and `PageFill = [100]`.
  - **Skip** `checks()`' page logic (overflow at 277 mm, orphan, under-filled, almost-empty, pushed, spill, oversized_diagram) and `monotoneIssues`. Keep `crampedIssues` (gaps between stacked blocks still matter).
  - **Add**, with thresholds in `rules.go` and penalties in `penalty`:
    - `page_break_ignored` (penalty 5): "page_break b_x has no effect on a canvas; delete it".
    - `canvas_too_large` (penalty 15): either side over **`MaxCanvasMM = 5000`** → "split it into several canvases, or group nodes into D2 containers".
- **`Report()`, `Hints()`, `Height()`, `Page()`:** work unchanged with one page. Verify with tests; don't refactor them.

### 5.5 Outputs
- **SVG (new):** `Builder.SVG(ctx, nb, blocks)` is a twin of `PDF()`: `typst compile main.typ out-<key>.svg`, cached and pruned the same way. Fold the two into one `export(ext)` helper rather than copying.
- **PNG:** in `RenderPNG`, a canvas uses `ppi = min(requested, maxPx / (max(w, h)_mm / 25.4))`, with a floor of 10.
  - `maxPx` is **1568** for `render_pages` (Claude's image sweet spot) and **600** for thumbnails.
  - Put this in one helper, `CanvasPPI(r, want, maxPx)`, used by both.
  - A 3 m canvas therefore costs the same as an A4 page.
- **PDF:** unchanged. Typst writes one page at the canvas's size.

### 5.6 Claude (MCP) changes
| Tool | Change |
|---|---|
| `create_notebook` | New `format`: `a4` (default) or `canvas`. Schema text: *"canvas: one unbounded sheet for flowcharts, architecture maps and mind maps; diagrams keep their natural size. Use a4 for printable study notes."* Validate with `validFormat()`, modelled on `validMode`. |
| `set_theme` | New optional `format` (omit = keep). It switches format and is undoable. |
| `list_themes` | Add `formats`. |
| `NotebookInfo`, `Outline` | Add `format`. |
| `MutationResult.layout` | On a canvas, add `canvas_mm: [w, h]`. The page fields are still sent (`page_count` 1); the instructions say to ignore them on a canvas. |
| `render_pages` | On a canvas, `pages` must be `[1]` (the error says so). PPI is auto-capped (5.5). The caption reads *"Canvas W×H mm, shown at N% scale"*. |
| `preview_diagram` | New `size: "canvas"` → natural-size `Fit`. |
| Everything else | Unchanged (block tools, undo, versions, check_layout, export_pdf, folders). |

**Guidance text** (keep it short; these strings are paid for in every session):
- `server.go` `instructions`:
  - Change line 1 to "…onto A4 pages, or onto one unbounded **canvas**".
  - Add 3 lines: when to pick canvas; text blocks are 174 mm cards stacked top to bottom; no page_break, and page-fill numbers don't apply.
- `guide.md`: new `## Canvas` section.
  - When to use it.
  - The shape: title → one big diagram → short notes/legend → optional `two_column`.
  - D2 for big graphs: containers to group, `direction: right` for flows, the `notebook` theme for a sketch look.
  - The source limit.
- `skills/folio-notes/SKILL.md` (currently **untracked**): add a row to "Decide these yourself": *flowchart / canvas / whiteboard / mind map / architecture map → `format: canvas`*.

### 5.7 Viewer (web)
**`canvas.html`** is a new template (cleaner than branching `notebook.html`). It reuses `style.css`, `app.js` (`folio.store`, `folio.toast`, `folio.setView`), the toolbar markup and the SSE handling. `Server.notebook()` picks the template by `nb.Format`.
- **Stage:**
  - `#stage` is full-window, with `overflow: hidden` and `touch-action: none`.
  - Background is a CSS `radial-gradient` dot grid using theme tokens.
  - Inside it, an `<img id="board">` is placed absolutely with `transform: translate(tx, ty)`.
- **Zoom via `width`, not `transform: scale`:** set `board.style.width = naturalPx × z`. Browsers rasterise a scaled `<img>` at layout size, so `scale()` blurs when zoomed in; changing the width keeps the SVG sharp. The natural size comes from `canvas_mm` in `State` (mm × 96 / 25.4).
- **Input (pointer events only, no library):**
  - One pointer drags to pan; two pointers pinch to zoom.
  - Wheel pans; ctrl/⌘ + wheel (also trackpad pinch) zooms around the cursor.
  - Zoom range 0.1–4. Buttons: − / fit / +. Keys: `+ − 0`.
- **First open:** fit if the canvas is wider than the window, otherwise 100%. Remember `{tx, ty, z}` per notebook in `folio.store`.
- **Live updates:**
  - When the fingerprint changes, preload `/n/{id}/canvas.svg?v=<sig>` (same `load()` pattern) and swap `src`. **Keep tx/ty/z** and toast "Canvas updated".
  - If `state.format` stops matching the page, call `location.reload()`. `notebook.html` gets the same one-line check, so Claude's format switch shows up live.
- **Toolbar:** back, title, live dot, zoom group, and the ⋯ menu (appearance, version). Downloads: PDF and SVG.
- **`State`** (`viewer.go`): add `Format` and `CanvasMM`. Add `XMM` to outline `Heading`; it is used in C2 to pan to a heading.
- **Dashboard** (`index.html` / `style.css`): canvas cards get class `canvas`, a "Canvas" pill, and a thumbnail with `object-fit: contain; object-position: center`.

**Routes**
| Route | Notes |
|---|---|
| `GET /n/{id}/canvas.svg` | `require(false, …)`. **404 unless the notebook is a canvas** (and for other users' notebooks, like every route). `Content-Type: image/svg+xml`. Caching as for `page/N.png` (immutable when `?v=` matches). `?download=1` adds `Content-Disposition: attachment`. Set `Content-Security-Policy: default-src 'none'; img-src data:; style-src 'unsafe-inline'` **on this response**, so the SVG can't run anything even if opened directly in a tab. |
| `GET /n/{id}/thumb.png`, `/page/1.png` | Unchanged routes; the pixel cap from 5.5 applies. |

### 5.8 Limits (validate at the trust boundary)
- **Diagram `source` ≤ 10,000 characters**, in `Diagram.validate`, for **both** formats. Today it has **no limit at all**.
  - 10 KB is about 350 nodes, or about 1.5 s of D2 on a laptop.
  - The error message suggests splitting the diagram or grouping nodes into containers.
- `MaxCanvasMM = 5000` is a warning, not a rejection. Users can still build big things; Claude is told it's too big.
- Existing limits apply unchanged: 2,000 blocks per notebook and 30 renders per minute.

---

## 6. Data model (follow the `density` migration exactly: same shape, same restore rule)
- **SQLite:** migration **8**. **Postgres:** migration **7**. Both run:
  ```sql
  ALTER TABLE notebooks ADD COLUMN format TEXT NOT NULL DEFAULT 'a4';
  ALTER TABLE snapshots ADD COLUMN format TEXT
  ```
- **`store.go`:**
  - Add `Notebook.Format` and `format` to `nbCols` / `scanNotebook`.
  - Add a `format` parameter to `CreateNotebook`. It is called from 7 files, mostly tests; the update is mechanical.
  - `mutate`'s `UPDATE notebooks SET …` gains `format = ?`.
  - The `snapshot()` INSERT gains `format`.
  - **`restore()`:** read `format` as `NullString`. **NULL means `"a4"`**: every pre-migration snapshot was A4. This differs from `density`, which keeps the current value. Add a comment saying why.
- **`import.go`:** copy `format` in both the notebook and snapshot SELECT/INSERT.
- Add `store.Formats = []string{"a4", "canvas"}`.

---

## 7. Files touched (checklist)
`internal/store/store.go`, `import.go` · `internal/model/blocks.go` (source cap) · `templates/main.typ` · `internal/layout/layout.go`, `analyze.go`, `themes.go`, `rules.go`, `fingerprint.go` · `internal/mcpserver/tools.go`, `server.go`, `guide.md` · `internal/viewer/viewer.go`, `canvas.html` (new), `notebook.html` (format reload), `index.html`, `style.css` · `skills/folio-notes/SKILL.md` · `README.md` (feature row + screenshot) · `DECISIONS.md` (record the format/no-coordinates/SVG decisions).

---

## 8. Build order

| Phase | Steps (each one lands green) | Effort |
|---|---|---|
| **C1: Canvas you can see** | 1. **Half-day spike, first:** confirm in Chrome, Safari and Firefox that a Typst SVG with an embedded `data:` D2 image renders inside `<img>`. If not, the viewer falls back to PNG at the 5.5 pixel cap (≤ 8000 px). 2. Migration + store + import + tests. 3. Template branch + `Fit`/`analyze`/`key` + layout tests. 4. SVG export + PNG cap + route. 5. MCP params, guide, instructions, skill. 6. `canvas.html` + dashboard pill. 7. Kitchen sink: add canvas variants to `make kitchen-sink`. | 3 days |
| **C2: Polish** | Follow live edits (send changed block positions in `State`), outline pan-to-heading, `render_pages` `region` (crop via Typst `page` offset or PNG crop), keyboard panning, remembered view | 1.5 days |

### Tests the engineer must add (each fails if its rule breaks)
- **Store (SQLite + Postgres):**
  - `format` round-trips.
  - `set_theme(format: canvas)` then `undo` gives `a4` back.
  - Restoring a snapshot with NULL `format` gives `a4`.
  - `folio import` keeps `format`.
- **Layout:**
  - A canvas with a 1200 px-wide diagram has 1 page, `CanvasWMM` > 300, and the diagram's `width_mm` = natural × 0.9 (proves no 174 mm cap).
  - A canvas with a 2,000-character paragraph has `CanvasWMM` ≤ 174 + 2 × 24 + 1 (proves wrapping).
  - On a canvas, the first child of the right column of `two_column` has x > left x + 50 mm (proves the grid didn't collapse).
  - A canvas with `page_break` still has 1 page and has a `page_break_ignored` issue.
  - A 5,100 mm canvas gets `canvas_too_large`.
  - `RenderPNG` on a 3,000 mm canvas returns an image whose long edge is ≤ 1568 px.
- **A4 regression:** the kitchen-sink notebook's block positions (page, y, x) are **identical** before and after the template change. Save them as a golden file in the test.
- **Model:** a diagram source of 10,001 characters is rejected with the split/containers message.
- **MCP:**
  - `create_notebook` with `format: "canvas"` works; `format: "poster"` returns an error listing the valid formats.
  - `render_pages(pages: [2])` on a canvas fails with a clear message.
  - `preview_diagram(size: "canvas")` works.
  - The isolation matrix still passes (no new tools).
- **Viewer:**
  - `/n/{id}/canvas.svg` serves SVG with the strict CSP for the owner's canvas, 404 for an A4 notebook, and 404 for another user's canvas.
  - `/n/{id}` serves `canvas.html` for a canvas.
- **Injection:** rerun the existing injection test with `format: canvas`.

### Done means
- `make test-pg` passes.
- **Real session:** headless Claude is asked *"Make a canvas of the full e-commerce checkout flow, ~35 steps, with retries and failure paths"*. It picks `canvas` unprompted from the guide, finishes with `check_layout` at ≥ 90, and the screenshot goes in `docs/screenshots/canvas.png`.
- **Viewer** checked on desktop Chrome, Safari and Firefox, and pinch-checked on an iPhone.
- **Timing:** a 150-node canvas measured on the Render instance type in use. Record the numbers in `PERFORMANCE_PLAN.md`.

---

## 9. Risks

| Risk | Impact | Mitigation |
|---|---|---|
| The template change alters A4 output | Every existing notebook shifts | All canvas logic sits behind `if canvas`. The golden-position A4 test is the guard. |
| Big D2 graphs on a slow CPU | Writes wait; D2's global mutex (`diagram.mu`) and the builder lock serialise all users | 10 KB source cap; the existing 20 s `layoutBudget` fallback ("layout still being computed"); timings measured in Done. Later: `checkDiagram` validates with a style that differs from the theme's, so D2 runs twice per write; make it reuse the cached render. |
| Nested `data:` SVG not rendered inside `<img>` in some browser | Blank diagrams in the viewer | C1 step 1 spike; PNG fallback with the pixel cap. |
| Claude can't see detail on a huge canvas at 1568 px | Misses small layout problems | The numeric checks still report them; C2 adds `region` zoom. |
| Text-heavy canvases look like a long narrow page | "Why is this a canvas?" | The guide says canvas is for diagram-led content; the A4 format stays the default. |
| Very large SVGs (several MB) | Slow first paint on phones | Same pixel cap for thumbnails; SVG only on the canvas page; `canvas_too_large` warns early. |

---

## 10. Not doing (and why)
- **Free x/y placement of boxes.** LLMs place coordinates badly, it breaks Folio's "no coordinates" contract, and D2 lays out graphs better. If users ask for it, the next step is *relative* anchors (`beside: b_x`, `below: b_y`), not raw x/y.
- **Drawing or editing in the viewer (a true Excalidraw clone).** The viewer is read-only by design (DECISIONS 2026-10-22). Editing needs a client editor, conflict handling against Claude's versions, and a JS bundle, which the CSP (`script-src 'self'`) and the no-build-step setup don't support today.
- **`.excalidraw` export.** No D2 → Excalidraw converter exists; we'd have to write and maintain one. SVG and PDF cover sharing. Revisit if asked.
- **Infinite empty space beyond the content.** The canvas is as big as its content; panning stops a margin past the edge.
- **Live cursors or collaboration**, and **user-uploaded images**: out of scope, same as the rest of Folio.

---

## 11. Decisions for you
1. **Switchable format** (A4 ↔ canvas, undoable; my pick) or fixed at creation? Switchable costs one snapshot column.
2. **Layout model:** stacked blocks with natural-size diagrams (my pick) now, and relative anchors only if people ask? Or invest in free placement now?
3. **Viewer rendering:** SVG (sharp at any zoom; my pick) with a PNG fallback, or PNG only (simpler, blurry when zoomed)?
4. **Diagram source limit:** 10,000 characters (~350 nodes) for both formats? Today there is none.
5. **Default paper for a new canvas:** `dotted` (whiteboard feel; my pick) or the theme's own default (`plain` for clean)?
6. **Scope:** ship C1 alone first (my pick), or C1 + C2 together?
