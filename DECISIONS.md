# Folio — Decisions

## 2026-10-21 — Repository root and module path
- **Decision:** The repo root is `~/AI_NOTEBOOK` (its own `git init`), module `github.com/raaj6395/folio`. The spec's `folio/` tree lives directly at that root.
- **Why:** The folder sat inside a git repo rooted at the user's home directory; a nested repo keeps Folio's history separate.
- **Alternatives considered:** committing into the home-directory repo; a `folio/` subfolder.
- **Affects interface/behaviour?** no — **User approved?** n.a.

## 2026-10-21 — Pinned versions
- **Decision:** Go 1.25.6, Typst CLI 0.15.1 (Homebrew), `github.com/modelcontextprotocol/go-sdk v1.8.0`, `oss.terrastruct.com/d2 v0.7.2`, `modernc.org/sqlite v1.61.0`.
- **Why:** Latest stable at project start.
- **Alternatives considered:** none.
- **Affects interface/behaviour?** no — **User approved?** n.a.

## 2026-10-21 — Dependencies
- **Decision:** `modelcontextprotocol/go-sdk`: required MCP server. `d2`: in-process diagram layout/SVG. `modernc.org/sqlite`: pure-Go SQLite, no CGO, so `go build` gives a static binary.
- **Why:** Spec Section 3.
- **Alternatives considered:** `mattn/go-sqlite3` (needs CGO).
- **Affects interface/behaviour?** no — **User approved?** n.a.

## 2026-10-21 — API notes (verified with `go doc` / `--help`)
- **Decision:** Use these exact APIs:
  - MCP: `mcp.NewServer(&mcp.Implementation{Name, Version}, &mcp.ServerOptions{Instructions})`; tools via generic `mcp.AddTool(s, &mcp.Tool{Name, Description}, func(ctx, *mcp.CallToolRequest, In) (*mcp.CallToolResult, Out, error))`. The input schema is inferred from the `In` struct, field descriptions come from `jsonschema:"..."` tags, and input is validated before the handler runs. A returned `error` becomes a tool error (`isError: true`). Images are returned as `&mcp.ImageContent{Data: pngBytes, MIMEType: "image/png"}` in `CallToolResult.Content` (the SDK base64-encodes `Data`). Resources: `s.AddResource(&mcp.Resource{URI, Name, MIMEType}, handler)`. Transport: `s.Run(ctx, &mcp.StdioTransport{})`. Tests: `mcp.NewInMemoryTransports()`.
  - D2: `d2lib.Compile(ctx, src, &d2lib.CompileOptions{LayoutResolver, Ruler}, &d2svg.RenderOpts{ThemeID, Sketch, Pad})`, then `d2svg.Render(diagram, renderOpts)`. Ruler from `textmeasure.NewRuler()`, layout `d2dagrelayout.DefaultLayout`, context from `log.WithDefault(ctx)`.
  - Typst: `typst compile main.typ out.pdf --root <dir> --font-path <fonts>`; PNG via `typst compile main.typ 'page-{p}.png' --pages N --ppi P`. Layout: `typst eval 'query(<blockpos>).map(it => it.value)' --in main.typ`, which returns JSON `[{..., pos: {page, x: "70.87pt", y: "70.87pt"}}]`.
- **Why:** Spec 0.2 says verify APIs instead of trusting memory.
- **Alternatives considered:** `typst query`, which is deprecated in 0.15 and prints a warning saying to use `typst eval`.
- **Affects interface/behaviour?** no — **User approved?** n.a.

## 2026-10-21 — Claude Code registration
- **Decision:** Registered with `claude mcp add -s user folio -- <abs path>/bin/folio mcp`.
- **Why:** Spec Section 10 asks for user scope.
- **Alternatives considered:** project scope.
- **Affects interface/behaviour?** no — **User approved?** n.a.

## 2026-10-21 — Block ordering keys
- **Decision:** Fractional string keys over base-36 digits (`0-9a-z`), via `model.KeyBetween(a, b)`. Keys never end in `0`, so there is always room before any key. Appends step one digit (`1`, `2`, … `z`, `z1`, …), which keeps keys short (≤10 chars after 300 appends). Mutations diff the old and new block lists, so only blocks whose position, type or data changed are written.
- **Why:** Spec 5.1. Inserts and moves never renumber siblings.
- **Alternatives considered:** float positions (run out of precision after repeated bisection); integer index (renumbers everything).
- **Affects interface/behaviour?** no — **User approved?** n.a.

## 2026-10-21 — Soft delete is a column, not a trash table
- **Decision:** `delete_notebook` sets `notebooks.deleted_at`. All reads filter `deleted_at IS NULL`. Rows stay, so a notebook can be recovered by clearing the column.
- **Why:** Same effect as a trash table, without copying rows across tables.
- **Alternatives considered:** a separate `trash` table (spec wording).
- **Affects interface/behaviour?** no — **User approved?** n.a.

## 2026-10-21 — Undo semantics
- **Decision:** Every snapshot stores a `base`, the version whose state it represents. A normal mutation (including `restore_version`) has `base = its own version`. `undo` restores snapshot `base - 1` and inherits that snapshot's base, so repeated undos walk further back instead of toggling. Undo after `restore_version` reverts the restore. Snapshots also store theme/paper, so undoing `set_theme` works.
- **Why:** Spec 5.1/8.2 asks for undo without defining repeated undo.
- **Alternatives considered:** undo = "restore version N-1", which makes two undos cancel each other out.
- **Affects interface/behaviour?** yes (defines undo behaviour) — **User approved?** no — conventional editor semantics; flagged in PROGRESS open questions.

## 2026-10-21 — SQLite connection settings
- **Decision:** DSN pragmas `journal_mode(WAL)`, `busy_timeout(5000)`, `foreign_keys(1)` and `_txlock=immediate`. Schema migrations are tracked with `PRAGMA user_version`.
- **Why:** Lets `folio mcp` and `folio serve` share the DB. Immediate transactions avoid read-to-write lock upgrade deadlocks.
- **Alternatives considered:** a migration library (unneeded dependency).
- **Affects interface/behaviour?** no — **User approved?** n.a.

## 2026-10-21 — Links restricted to http, https and mailto
- **Decision:** `[text](url)` becomes a link only when the URL starts with `http://`, `https://` or `mailto:`. Other schemes (e.g. `javascript:`, `file:`) render as plain text.
- **Why:** The injection test showed a `javascript:` URL becoming a live PDF link. Notes never need other schemes.
- **Alternatives considered:** rejecting the block with an error (too harsh for something harmless to drop).
- **Affects interface/behaviour?** yes, minor — **User approved?** no — a security hardening; flagged in PROGRESS.

## 2026-10-21 — Templates and fonts embedded in the binary
- **Decision:** `templates/` and `fonts/` are embedded with `go:embed` (root package `folio`). Each build writes the templates into `~/.folio/build/<id>/`. Fonts go to `~/.folio/fonts/` (rewritten only if changed). Typst runs with `--root <build dir> --font-path ~/.folio/fonts`. System fonts stay enabled as a fallback for glyphs the bundled fonts lack (CJK, emoji).
- **Why:** A single binary works from any directory.
- **Alternatives considered:** locating `templates/` next to the binary (breaks when the binary moves).
- **Affects interface/behaviour?** no — **User approved?** n.a.

## 2026-10-21 — Build = parallel compile + eval
- **Decision:** Each build runs `typst compile` (PDF) and `typst eval` (block positions) in parallel. Results are cached in `layout.json`, keyed by version, theme and paper.
- **Why:** Typst has no single command that outputs both. Running them in parallel halves wall time.
- **Alternatives considered:** sequential runs.
- **Affects interface/behaviour?** no — **User approved?** n.a.

## 2026-10-22 — Pagination via measure + breakable, warnings from block extents
- **Decision:** Every block emits start and end `<blockpos>` markers. No-split blocks (callout and code now; table, qa, fill_in, diagram later) are wrapped in `nosplit()`, which measures the block at the current width and sets `breakable: height > 40% of the 257mm content height`. Headings are `block(sticky: true)`. Warnings: heading last on a page; block running past the page bottom; a mid-document page under 5% full; and a no-split block that moved to the next page and left the previous page under 70% full (a 60% threshold could never fire, because anything under 40% always fits). Pages ending in an explicit `page_break` are never flagged.
- **Why:** Spec 6.4/6.5.
- **Alternatives considered:** a fixed width for measuring (breaks inside `two_column`).
- **Affects interface/behaviour?** no — **User approved?** n.a.

## 2026-10-22 — Hidden Q&A answers
- **Decision:** With `hide_answer: true`, the card shows the question and "Answer: upside down at the bottom of this page". The answer is emitted as `place(bottom, float: true)` rotated 180°, so hidden answers stack at the bottom of the page where the card lands, numbered `A<n>` to match `Q<n>`.
- **Why:** Spec 5.3 offers two options. Upside down keeps the answer in the notes for self-checking; an empty answer area would lose it.
- **Alternatives considered:** an empty answer area.
- **Affects interface/behaviour?** yes (picks the spec's option 1) — **User approved?** n.a. (spec said pick one and record it)

## 2026-10-22 — Diagram rendering details
- **Decision:** D2 SVGs are post-processed: `@font-face` blocks are removed, D2's bold/italic font classes are pointed at the theme font (`font-weight`/`font-style` added), and a default `text{font-family}` rule is added. Size mapping: width = min(fraction × 174mm text width, 0.9 × the SVG's natural width at 96 dpi), so small diagrams aren't blown up past body-text size. Then the height is capped at 60% of the content height (154mm). D2 labels are 16px, i.e. 12pt at natural size. If scaling puts them below 6pt, the build warns. D2 runs in-process behind a mutex (the layout engine is not documented as concurrency-safe). The dagre layout is used. SVGs are cached in `build/<id>/diagrams/<sha256(theme,sketch,font,source)[:16]>.svg`. Write-time validation compiles with the default style.
- **Why:** Typst's SVG renderer ignores `@font-face`, so diagram text fell back to a serif font. Retargeting makes diagrams match the notes.
- **Alternatives considered:** installing D2's fonts (Source Sans Pro) into the font dir (diagrams wouldn't match the theme); ELK layout (dagre is D2's default and looked fine).
- **Affects interface/behaviour?** no — **User approved?** n.a.

## 2026-10-22 — Diagram `size` optional
- **Decision:** `diagram.size` defaults to `full` when omitted.
- **Why:** Fewer pointless validation round-trips for the AI.
- **Alternatives considered:** required field.
- **Affects interface/behaviour?** yes, minor — **User approved?** n.a. (lenient superset of the spec)

## 2026-10-22 — Themes, paper and handwriting font
- **Decision:** The handwriting font is Kalam (OFL). It ships real Light/Regular/Bold files, and Typst does not fake bold. Caveat and Patrick Hand ship as a variable font or regular-only. Paper backgrounds use Typst `tiling` fills on the page background. Ruled paper uses 8mm lines with a red margin line at 14mm in the notebook theme; the dot grid and squared grid use 5mm/7mm steps. Callouts in notebook/dotgrid use a full rounded border; clean keeps the left accent bar. No irregular hand-drawn callout borders: Typst has no cheap way to draw them, and D2 sketch mode already gives diagrams the hand-drawn feel. Ruled lines don't align with text baselines, because block spacing varies. Theme metadata (name, description, default paper, D2 style) lives in `layout.Themes`. Typst look-and-feel lives in `templates/themes/<name>.typ`. `set_theme` without `paper` switches to the new theme's default paper. Diagrams never split (height is capped at 60% of a page, so they always fit), which keeps captions with their images.
- **Why:** Spec 6.3. The kitchen-sink check found a tall diagram's caption stranded on the next page.
- **Alternatives considered:** keeping the current paper on theme switch (a notebook theme on plain paper loses the point).
- **Affects interface/behaviour?** yes, minor (set_theme default paper) — **User approved?** n.a.

## 2026-10-22 — two_column nesting
- **Decision:** Nested blocks are stored inside the parent's `data` (`left`/`right` hold full flat blocks with their own `b_` IDs). `update_block`, `delete_blocks` and `read_blocks` accept nested IDs. `insert_blocks`/`move_blocks` refer only to top-level blocks, and using a nested ID there returns an error naming the parent. Updating a two_column keeps nested IDs it already had and gives new ones to everything else. Append/insert always assign fresh nested IDs (so copying a block can't duplicate IDs). Deleting the last block of a column is refused. The outline lists nested blocks right after their parent, with `parent` set. A two_column stays whole unless it is taller than 90% of a page (other no-split blocks use 40%), because splitting a grid sends one column's tail to the next page.
- **Why:** Spec 5.3 says nested blocks are directly addressable. The kitchen-sink check showed a split right column.
- **Alternatives considered:** a `parent_id` column in the blocks table (the nested order and positions would need their own keys, for little gain).
- **Affects interface/behaviour?** yes (insert/move don't take nested IDs) — **User approved?** no — flagged in PROGRESS.

## 2026-10-22 — Viewer
- **Decision:** `folio serve` is plain `net/http` with embedded HTML templates and vanilla JS. It binds 127.0.0.1 only. It builds into `~/.folio/build-viewer/` so it never races the MCP process over `build/` (each builder's lock is per process). SSE polls `notebooks.version` every 500ms per connection and sends `{version, page_count, pages: [signature…]}`, where a page signature hashes the blocks on that page (data and y position) plus title/theme/paper. The browser reloads only pages whose signature changed and flashes them for 2 seconds. The first page request renders all pages of that version in one Typst call, at 110 ppi. `get_viewer_url` probes `/healthz`.
- **Why:** Spec 9. Each process owning its build dir avoids cross-process file locking.
- **Alternatives considered:** fsnotify on the DB file (new dependency; WAL writes make it noisy).
- **Affects interface/behaviour?** no — **User approved?** n.a.

## 2026-10-22 — Changes from the guide-tuning runs (M3.5)
- **Decision:** After generating REST APIs (clean), a consistent-hashing cheat sheet (notebook) and Kafka basics (dotgrid) with headless Claude, and reading its friction reports: (1) a new warning when the last page is under 15% full, unless it starts after an explicit page_break; (2) mutation results now include `page_fill_pct` for every page, `last_page_free_mm`, and `height_mm` per changed block, for page budgeting; (3) bullets/numbered lists under 25% of a page no longer split; (4) D2 now measures text with the theme font (`d2fonts.AddFontFamily` with the embedded Inter/Kalam), so label gaps in lines fit the rendered text; (5) the layout cache key includes a hash of the embedded templates plus a render version, and the D2 cache key a render version, so upgrading Folio invalidates stale builds; (6) the guide gained a D2 section (syntax, flow + sequence examples, sizing tips), a "checking your work" section and advice against unneeded page breaks.
- **Why:** Spec 3.5: tune the guide using real sessions.
- **Alternatives considered:** a "fit to remaining space" diagram size (left out; free-space numbers let the AI pick a size).
- **Affects interface/behaviour?** yes (extra fields in mutation results) — **User approved?** n.a. (additive)
