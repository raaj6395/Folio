# Folio — Layout guidelines & layout linting: build plan

> **Status: IMPLEMENTED 2026-10-22 (L0–L8 done; L9, the manual run after deploy, is pending).** Progress, test changes and screenshots: `docs/layout-guidelines-progress.md`.
> Source: the "Built-in layout guidelines and layout linting" spec.
> How this file was made:
> - **Draft (Sections 1–5):** the spec read against the codebase.
> - **Iteration 2 (Section 6):** a design review that changed several decisions.
> - **Iteration 3 (Section 7):** a recheck of every touch point against the actual code.
>
> **Section 8 is the final task list. It replaces the task lists in the draft.** Section 9 lists what needs your OK.

---

## 1. Goal (from the spec)

Agents keep producing cramped, stacked, badly paginated pages, and the user has to correct them. Folio should:
1. **Tell** every agent the layout rules automatically.
2. **Check** the result itself: warnings with block IDs and concrete fixes, plus a `check_layout` score.
3. **Avoid** problems with better defaults: automatic spacing, density, left-aligned tables, H3 headings inside `two_column`.

---

## 2. Codebase map (explored first, as the spec requires)

| Concern | Where it lives today | Notes |
|---|---|---|
| MCP server and `initialize` instructions | `internal/mcpserver/server.go`: `const instructions` (≈10 lines), `New()` passes `ServerOptions{Instructions}` | Delivered to every client on `initialize` |
| `get_authoring_guide` content | `internal/mcpserver/guide.md` (embedded, 94 lines), served by the tool and the `folio://guide` resource | Some advice **conflicts** with the new rules (see 6.4) |
| Write tools and their response | `internal/mcpserver/server.go`: `mutate()` → `result()` → `MutationResult{Version, ChangedBlockIDs, Layout{PageCount, Blocks, PageFill, LastPageFill, LastPageFreeMM}, Warnings}` | Every write goes through `result()`, the single place to add `layout_hints` |
| Layout positions | `internal/layout/layout.go`: `build()` runs `typst eval` querying `<blockpos>` marks → `analyze()` | Marks are placed by the template (`mark(id)` / `endmark(id)`) |
| Warnings and page fill | `internal/layout/analyze.go`: `analyze()`, `warnings()` | Existing checks: orphan heading, block taller than a page, last page <15%, page ended by `page_break` <30%, almost-empty page <5%, unbreakable block pushed to the next page |
| Unreadable-diagram check | `internal/layout/themes.go`: `docBlocks()` + `Fit()`; `minReadablePt = 6`, `maxDiagramHeight = 0.6 × content` | Warnings come back as `dwarns` strings |
| Rendering of tables, spacers, two_column, headings | `templates/main.typ` → `render(b)` | Tables are wrapped in `align(center, table(...))`, **which also centers every cell** (the root cause of the "centered cells" complaint). Heading gaps: H1 `above: 1.6em`, H2 `1.3em`. Boxes: `above/below: 1em–1.1em`. Spacer: small 4mm, medium 10mm, large 20mm |
| Block model and validation | `internal/model/blocks.go`: `Table`, `TwoColumn` (`nestable` list excludes `heading`), `Spacer`, `Diagram` | Strict decoding: unknown fields are rejected, so new fields must be added to the structs |
| Notebook fields | `internal/store/store.go`: `Notebook` struct, `nbCols`, `scanNotebook`, `CreateNotebook(userID,title,theme,paper,mode)` (13 callers), `mutate()` UPDATE (title/theme/paper/mode) | `Mutate` persists notebook fields changed in the callback |
| Versioning (undo/restore) | `snapshots` table (blocks + theme/paper/mode), `snapshot()`, `restore()` sets theme/paper/mode | New notebook fields must be added here too, or undo won't restore them |
| Migrations | `store.go`: `migrations` (SQLite, 4 so far) and `pgMigrations` (Postgres, 3 so far) | Append-only |
| Import | `internal/store/import.go` copies explicit notebook and snapshot columns | Must copy the new columns |
| Layout cache key | `layout.go: key(nb)` = version-theme-paper-mode-templateHash | Must include density |
| Viewer page fingerprints | `internal/viewer/viewer.go: signatures()` hashes title/theme/paper/mode | Must include density |
| MCP sessions | **HTTP is stateless** (`StreamableHTTPOptions{Stateless: true}`); stdio has one session | "First write response of a session" needs an explicit definition (see 6.2) |
| Existing tests asserting warning text | `internal/layout/analyze_test.go`, `layout_test.go:92` (expects exactly 1 warning), `m25_test.go:53` (expects 0), `mcpserver/server_test.go:237` (expects 0) | New checks may add warnings to these; each case gets reviewed, not blindly updated |

**Measured fact used below:** a block's end mark reports y roughly at the baseline of its last line (it's inline), about 1mm above the visual bottom. Measured in a scratch Typst file: paragraph end → next H2 start = 4.8mm with today's spacing. So "gap" = `next.YMM − prev.EndYMM` is a usable proxy (±1mm) for visible space.

---

## 3. Draft design: Part 1, guidance

### 3.1 Rules document
- New file `internal/mcpserver/layout_rules.md`, embedded next to `guide.md`. The spec's `guidelines/layout.md` is only an example; keeping it beside the guide avoids a new embed package.
- It contains the spec's rules (page structure, spacing, arrangement, diagrams, density, working with the user), reworded to match Folio's real block names and limits.

### 3.2 Delivery
- **Server instructions:** replace `const instructions` with a ≤ 40-line version: what Folio is, dark/light, IDs, a condensed layout rules list, and the workflow (`write → read warnings/layout_hints → fix → check_layout → render changed pages once → done`).
- **`get_authoring_guide`:** returns `guide.md` + "\n\n## Layout rules\n\n" + `layout_rules.md`. The `folio://guide` resource returns the same text.
- **Write responses:** `MutationResult` gains `layout_hints []string`, at most 6 short, concrete fixes with block IDs, present only when there are issues.

### 3.3 Per-notebook guidelines
- New notebook column `guidelines TEXT` (≤ 2,000 chars).
- Tools `set_notebook_guidelines(notebook_id, text)` (a normal versioned write) and `get_notebook_guidelines(notebook_id)`.
- Shown in `get_outline` and in write responses (definition in 6.2).

## 4. Draft design: Part 2, enforcement

### 4.1 Checks
Restructure `warnings()` to produce typed issues:
```go
type Issue struct {
    Check   string   `json:"check"`   // e.g. "cramped", "under_filled"
    Page    int      `json:"page"`
    Blocks  []string `json:"blocks"`
    Problem string   `json:"problem"`
    Fix     string   `json:"fix"`
}
```
`Result.Issues []Issue` is new. `Result.Warnings []string` stays as `Problem + "; " + Fix`, so clients and tests that read warnings keep working. All thresholds go in one file, `internal/layout/rules.go`.

| Check | Trigger (draft) |
|---|---|
| `cramped` | `next` ∈ {H2, table, diagram, callout} directly follows a non-spacer block on the same page with gap < `MinGapMM` (6mm) |
| `under_filled` | Page fill < 50% and the page is ended by a `page_break`, or the next page starts with an H1 |
| `spill_over` | An H1 section spans ≥ 2 pages and its part on its last page is < 15% (generalizes today's last-page check) |
| `oversized_diagram` | Top-level diagram height > 40% of the content height (102.8mm) |
| `unreadable_diagram` | Existing (labels < 6pt); converted to an Issue |
| `monotone_stack` | A page has ≥ 4 consecutive full-width top-level blocks, no `two_column`, and a full-width diagram |
| `orphan_heading` | Existing behavior, kept (verified: a heading whose next block starts on a later page) |
| Existing checks | `overflow` (taller than a page), `pushed_block` (unbreakable block moved, leaving a gap), `almost_empty` (<5%), all kept as Issues |

### 4.2 `check_layout(notebook_id, pages?)`
Returns `{score, pages: [{page, fill_pct, score, issues}]}`. Page score = 100 − penalties, clamped to 0–100. The notebook score = mean of the page scores.

### 4.3 Rendering defaults
- **Density** `compact | comfortable | airy`, plus a hidden `legacy`, controls the heading gaps and the gaps between boxes, in the template.
- **Tables:** per-column `align`, default left. The table block is no longer wrapped in `align(center)`.
- **two_column:** allow `heading` level 3 in the `nestable` list.

## 5. Draft: risks noticed
Instructions get longer (cost on every turn), there are more tools, and rendering "changed pages" at the end costs CPU (see PERFORMANCE_PLAN.md). Addressed in 6.6.

---

## 6. Iteration 2: design review (changes to the draft)

**6.1 Density and existing notebooks (the spec asks us to pick one).**
- **Decision:** a new `density` column. **Existing rows get `legacy`** (the migration default) and render exactly as today. **New notebooks default to `comfortable`.**
- `set_theme` and `create_notebook` accept `density` (`compact|comfortable|airy`; `legacy` can't be chosen, only kept).
- Re-theming does **not** silently change density; only an explicit `density` does. This keeps "existing layouts never change unless asked" simple and testable.
- `""` (Go zero value, as in layout tests that build `store.Notebook{}` literals) is treated as `legacy`, so existing layout tests keep today's spacing.

**6.2 "First write response of a session" with stateless HTTP.** HTTP has no sessions, so we define one: **the first write by a given user to a given notebook in 30 minutes** (in-memory map `user|notebook → last seen`, single instance). That write's response includes `notebook_guidelines` (if non-empty).
- `get_outline` **always** includes them.
- stdio mode uses the same rule.
- Documented as a deliberate interpretation. *(If Folio ever runs on several instances, the worst case is guidelines shown twice; harmless.)*

**6.3 Gap values per density** (first proposal; tuned with screenshots in step 3):

| Gap | legacy (today) | compact | comfortable | airy |
|---|---|---|---|---|
| Above H1 | 1.6em | 1.4em | 3.0em (≈11mm) | 3.6em |
| Above H2 | 1.3em | 1.1em | 2.6em (≈9.6mm ≈ "medium") | 3.2em |
| Above H3 | 1.3em | 1.0em | 1.8em | 2.2em |
| Around boxes (table/callout/diagram/qa/fill_in/two_column/divider) | 1em–1.1em | 0.8em | 1.6em (≈6mm) | 2.1em |
| Between other blocks | Typst default | 0.9em | 1.3em | 1.6em |

Explicit `spacer` blocks still add on top, as the spec requires.

**6.4 The existing guide contradicts the new rules.**
- `guide.md` says "at most ~8 nodes", "about one H2 section per half page", and "don't add page_break unless a new H1 really should start a fresh page". The new rules say ~6 nodes and start every H1 on a new page.
- **Change:** edit `guide.md` so it has **no style advice of its own**. Its "Style" and "Checking your work" sections are replaced by a pointer to the Layout rules section. Workflow steps 4–5 are updated to the new loop.
- The rules also tell agents **not** to insert spacers before H2 when density isn't `legacy` (the renderer does it), to avoid double gaps.

**6.5 Fix suggestions depend on density.**
- In `legacy` notebooks, `cramped` would fire on almost every H2 (measured gap 4.8mm < 6mm). The fix text becomes: "switch this notebook to density "comfortable" with set_theme (adds gaps automatically), or insert a medium spacer before the H2".
- To avoid flooding, `cramped` produces **one issue per page** listing all affected block IDs.

**6.6 Speed (PERFORMANCE_PLAN.md).**
- The spec's "render at least the changed pages once before finishing" costs one Typst PNG render (~0.7–2s at 0.5–1 CPU, much worse on today's free tier).
- **Decision:** keep it, but word it as "**one** `render_pages` call with at most 2 changed pages, after `check_layout` shows no issues". `check_layout` is cheap: it reuses the cached layout and never renders when the layout is current.
- `set_notebook_guidelines` bumps the version, which today triggers a full re-render through the viewer. Accepted (a rare action).

**6.7 Oversized diagrams: warn, don't auto-shrink.** `Fit()` already caps diagrams at 60% of the page. Lowering that cap to 40% would shrink labels below readability. So the new check warns at > 40% and suggests `half` inside `two_column` or `direction: right`, as the spec says.

**6.8 Table defaults.**
- Default alignment is **left for text columns and right for numeric columns**, where every body cell looks like a number: `12`, `3.5`, `-4`, `40%`, `$10`, `1,200`.
- Optional `align: ["left","center","right", ...]` must have exactly one entry per column.
- This applies to **all** notebooks, including `legacy`. Alignment doesn't change heights, so page layout is unaffected and old notebooks just look better.
- The table also stops being centered as a block; it starts at the text's left edge.
- The caption stays centered (the spec allows centered captions).

**6.9 Global default guidelines file (optional in the spec): skipped in v1.** A file on the server (`~/.folio/guidelines.md`) only makes sense for the local stdio mode. On the hosted service every user shares one server, so it would apply to everyone. If wanted later: a per-user default stored on the user row. Listed in Section 9.

---

## 7. Iteration 3: recheck against the code (more changes)

1. **Versioning needs snapshot columns.**
   - `restore()` only restores theme/paper/mode. Add `guidelines` and `density` to `snapshots` and to `snapshot()`'s INSERT, and restore them in `restore()`.
   - Old snapshots have NULL in the new columns. `COALESCE` on read means "keep the current value", so undoing past the feature's introduction doesn't wipe guidelines.
2. **`mutate()`'s UPDATE** must also write `guidelines` and `density`. `nbCols` and `scanNotebook` gain both columns, and `Notebook` gains `Guidelines`, `Density`.
3. **`CreateNotebook` signature.** 13 call sites. Add a `density` parameter rather than an options struct; it's the smallest diff that stays explicit. Guidelines start empty.
4. **Import** (`import.go`) copies both new columns for notebooks and snapshots. Older SQLite source files without the columns get defaults (`legacy`, `''`).
5. **Cache key and viewer fingerprints:** `layout.key()` adds density, and `viewer.signatures()` adds density. Otherwise switching density would show stale pages.
6. **`analyze()` needs heading levels and the density.**
   - `Pos` gains `Level int` (headings only, `omitempty`), filled from block data in `analyze()`.
   - `analyze(marks, blocks, density)` gets the density so fix texts can follow 6.5.
   - `Result` gains `Issues`.
7. **Nested blocks.** Two-column children only have start marks (`Result.Nested`), so checks consider top-level blocks. A diagram inside `two_column` is never "oversized", and it counts as "paired" for `monotone_stack`.
8. **Under-filled vs the existing page_break check.** The existing check (<30%, ended by `page_break`) becomes part of `under_filled` at <50%. The message keeps the existing wording ("…because page_break X ends it…"), so `layout_test.go:92` and `analyze_test.go:42` still match. Their "exactly N warnings" assertions are re-checked once the new checks exist.
9. **`server_test.go:237` and `m25_test.go:53` expect zero warnings.**
   - These build small `legacy` docs. If `cramped` fires on them, that's correct new behavior: the assertions change to "no warnings other than `cramped`".
   - Each changed assertion is listed in the progress file with the reason.
10. **`two_column` with H3:**
    - Add `heading` to `nestable`, with a level check (only level 3) and a clear error otherwise.
    - The template's heading branch works inside columns already: `render(k)` is recursive, and the sticky block is harmless inside a grid cell.
    - The viewer's contents sidebar only lists top-level headings, so H3s in columns don't clutter it.
11. **`check_layout` page filter:** `pages` is optional. Unknown pages are an error naming the valid range, like `render_pages`.
12. **Tool count.** Three new tools (`set_notebook_guidelines`, `get_notebook_guidelines`, `check_layout`) add to the tools list (PERFORMANCE_PLAN.md M3). Their descriptions are kept to one sentence each.
13. **"Renderer snapshot tests"** don't exist in this repo; tests assert layout positions, not images. Mapping:
    - **density:** a position test showing the measured gap before an H2 grows from legacy → comfortable, and comfortable is ≥ `MinGapMM`.
    - **tables:** a doc-generation test (the `align` defaults, the numeric detection, and validation of the explicit field), plus a template-level check that a table is no longer centered: its start mark's x equals the text's left edge. That needs x in the marks; we add `x` to the query (cheap).
    - **visual:** PNGs saved under `docs/screenshots/layout-guidelines/` and linked from the progress file.
14. **Manual verification.** We push straight to `main` (no PRs), so the PNGs go in the repo folder above and the progress file instead of a PR description. The "fresh Claude Code session" run needs the deployed (or local) server and a real agent. **You run that prompt after deploy**; I render the result and save the PNGs.

---

## 8. Final task list (replaces the draft lists)

Progress is tracked in `docs/layout-guidelines-progress.md`: a checklist with these IDs, each changed test assertion and why, and links to the screenshots.

| ID | Task | Files | Tests |
|---|---|---|---|
| **L0** | Calibrate: measure the gaps per block pair on the kitchen sink (legacy) and on a comfortable prototype; set `MinGapMM` and the density values | scratch only | numbers recorded in the progress file |
| **L1** | Store: `guidelines`, `density` columns (SQLite migration 5, Postgres migration 4; existing rows `legacy`); snapshots columns; `Notebook`, `nbCols`, `scanNotebook`, `mutate` UPDATE, `snapshot`, `restore` (COALESCE), `CreateNotebook(…, density)`, import | `store.go`, `import.go`, callers | store test: guidelines/density survive undo and restore; old snapshot (NULL) keeps current values; import copies them |
| **L2** | Model: `Table.Align` (validated, numeric default), `two_column` allows `heading` level 3 | `model/blocks.go` | model tests: align defaults, numeric detection, bad align length/value, H3 ok and H1/H2 rejected in columns |
| **L3** | Template: density gap table; tables with per-column align, not centered; marks also report x | `templates/main.typ`, `layout.go` (doc gets `density`; query adds x) | layout tests: H2 gap legacy < comfortable; comfortable ≥ MinGap; table start x = text left edge; all existing layout tests green |
| **L4** | Checks: `rules.go` constants; `Issue`; `Pos.Level`; all checks of 4.1 (with 6.5, 7.6–7.8); `Warnings` derived; diagram warnings become Issues | `layout/analyze.go`, `rules.go`, `themes.go` | **one trigger and one non-trigger test per check** (synthetic positions), plus real-render tests for `cramped` and `oversized_diagram` |
| **L5** | Cache key and viewer fingerprints include density | `layout.go`, `viewer.go` | viewer test: switching density changes page fingerprints |
| **L6** | MCP: `layout_hints` in `MutationResult`; `density` on `create_notebook` and `set_theme`; `set/get_notebook_guidelines`; guidelines in `get_outline` and in the first write per user+notebook in 30 min; `check_layout` with score | `mcpserver/server.go`, `tools.go`, `edit.go` | MCP tests: hints appear only with issues; guidelines appear once then not again; undo restores guidelines; `check_layout` issues and score on a known-bad and a clean notebook |
| **L7** | Guidance: `layout_rules.md`; new ≤ 40-line instructions; `guide.md` de-conflicted and with "Layout rules" appended | `mcpserver/*.md`, `server.go` | test: `initialize` instructions contain the key rules and are ≤ 40 lines; `get_authoring_guide` contains the "Layout rules" section |
| **L8** | Verify: `go test ./...` + `make test-pg`; render a sample notebook in each density and theme, save PNGs; tools/list size before and after recorded | — | — |
| **L9** | After deploy: you run the spec's prompt in a fresh Claude Code session; I render the result and save the PNGs; acceptance checklist ticked | progress file | — |

Suggested order: L0 → L1 → L2 → L3 → L5 → L4 → L6 → L7 → L8 → L9. The store and template come first because the checks and tools depend on them.

**Acceptance criteria (from the spec) → task:**
- Rules arrive via `initialize`: L7.
- The guide includes the rules: L7.
- Guidelines tools, versioned, in the outline: L1 + L6.
- Each check has a trigger and a non-trigger test: L4.
- `check_layout` with a score, tested: L6.
- Tables left-aligned, `align` works: L2 + L3.
- Comfortable density adds visible gaps before H2: L3.
- H3 in `two_column`: L2.
- Existing tests pass, only optional new JSON fields (`align` on table; `density`, `guidelines` are notebook-level, not block JSON): L8.

---

## 9. Decisions for you
1. **Density for existing notebooks:** keep them on `legacy` (unchanged look) until someone sets a density (my pick, 6.1), or move everything to `comfortable` now (all notebooks re-flow)?
2. **Table alignment on old notebooks:** apply left-aligned cells to all notebooks (my pick, 6.8: looks change, page layout doesn't), or only to non-legacy?
3. **"First write per session"** = first write per user+notebook in 30 minutes (6.2). OK?
4. **Global default guidelines file:** skip in v1 (6.9), or add a per-user default now?
5. **Rendering at the end:** keep the spec's "render changed pages once" (with the speed note in 6.6), or make it "only if `check_layout` reports issues" to save time on the slow server?
