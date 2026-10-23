# Folio — render optimisation: stop redrawing everything on every edit

> **Status: DONE 2026-10-23 (O0–O6; O7 not needed so far).** Results in Section 6. Separate from `PERFORMANCE_PLAN.md` (hosting, MCP round trips, etc.). Don't do that plan's items unless listed here.
> **Goal:** a small edit should cost one layout pass plus redrawing only the pages that really changed. Today it costs three full Typst passes.

---

## 1. What happens today (verified in the code)

For one edit, e.g. a word changed on page 3 of an 8-page notebook:

| Step | Code | Work |
|---|---|---|
| 1. Save | `store.Mutate`: block JSON written, version 7 → 8 | cheap (DB) |
| 2. Layout | `layout.build()` runs **two** Typst processes in parallel: `compile` → `out.pdf` **and** `eval` → block positions | **2 full compiles** |
| 3. Viewer state | `viewer.state()` → `signatures()`: one fingerprint per page, sent over SSE | cheap |
| 4. Browser | `notebook.html` re-fetches only pages whose fingerprint changed | good |
| 5. Page images | `viewer.page()` → `png(all=true)` → `RenderPNG`; the cache folder is `pages/<layout key>-<ppi>/`, the key contains the **version**, and old folders are deleted, so **every page is missing** → one Typst compile rasterises **all 8 pages** | **1 more full compile + 8 rasterisations** |

**Measured (8-page kitchen sink, inside the production image):**
- `compile` PDF = 0.66–0.95 CPU-s
- `eval` = 0.58–0.91 CPU-s
- PNG 8 pages = 0.85–1.33 CPU-s

That's **about 2–3 CPU-s per edit**, which takes 20+ s on Render Free (0.1 CPU).

### Also found while reading the code
- **F1 (over-invalidation):** `signatures()` mixes `r.PageCount` into every page's hash. Adding a page at the end changes **all** fingerprints, so every page is re-downloaded and redrawn.
  - The page footer prints only the page number (`counter(page).display("1")`, no total), so the count isn't needed for a page's pixels.
- **F2 (would become a bug with reuse):** Q&A cards are numbered with a document-wide counter (`counter("qa")`). Inserting a Q&A on page 2 renumbers Q&A cards on page 5 while page 5's blocks and positions stay the same.
  - Harmless today, since everything is redrawn. But once images are reused by fingerprint, page 5 would show **stale numbers**.
- **F3:** the fingerprint doesn't include the template hash / render version, so a template change wouldn't invalidate reused images.

---

## 2. Target behaviour

| Edit | Today | Target |
|---|---|---|
| Change a word on page 3 of 8 | 2 compiles + 1 compile rasterising 8 pages | **1 eval** + 1 compile rasterising **only the pages whose content moved** (page 3, plus later pages only if they shifted) |
| Append a paragraph at the end | same as above, and all 8 pages re-downloaded (F1) | 1 eval + rasterise **only the last page or two** |
| Download PDF | ready (built on every edit) | built **on first download** after a change, then cached |
| Claude makes 5 edits in 10 s with the viewer open | up to 5 × full redraw | layout per edit (Claude needs it); the viewer redraws **once**, after a short pause |

---

## 3. Tasks (do them in this order)

### O0. Baseline and instrumentation (before changing anything)
- Add a **Typst call counter** to `layout.Builder`, plus a count of pages rasterised. It's an internal counter that tests can read, which gives machine-independent assertions such as "this edit ran 1 Typst process".
- Extend `internal/layout/perf_test.go` (`FOLIO_PERF=1`) to report, for an 8-page and a 30-page notebook:
  - edit latency (edit at start / middle / end),
  - number of Typst processes per edit,
  - pages rasterised when the viewer refreshes.
- Record the baseline numbers in a "Results" section at the bottom of this file. Also measure with the production image at `--cpus=0.5` and `--cpus=1` (the harness used for PERFORMANCE_PLAN.md).

### O1. Cheaper Typst runs (flags)
- Pass `--ignore-system-fonts` (we ship our own fonts; the server has none) and `-j <runtime.GOMAXPROCS(0)>` (Go respects the container CPU limit, so this is 1 on fractional CPUs) to every Typst call in `layout.typst()`.
- Check: rendered output is unchanged (the existing layout and kitchen-sink tests stay green; compare a PDF's text and page count before and after).

### O2. Lazy PDF
- `build()` runs **only** the `eval` (positions). Remove the parallel `compile → out.pdf`.
- New `Builder.PDF(ctx, nb, blocks) (path string, err error)`: if `out-<key>.pdf` exists, return it; else compile it (same notebook lock) and return it.
- **Callers to switch:**
  - `viewer.pdf` (the `/n/{id}/pdf` download),
  - `mcpserver` `export_pdf`: the local mode writes a file, so call `PDF()`; the hosted mode returns a URL and needs no PDF now,
  - the `folio export` command,
  - any code that reads `Result.PDF`.
- `Result.PDF` is no longer filled by `build()`. Grep for every use and remove or replace it.
- Delete older PDFs when a new one is written (one per notebook is enough).
- **Tests:** an edit runs exactly 1 Typst process (O0 counter); `/n/{id}/pdf` still returns a valid PDF with the right page count; `export_pdf` works in both modes.

### O3. Correct page fingerprints (F1, F2, F3)
In `viewer.signatures()`, or better, move it into `layout` so the PNG cache can use it too:
- **Remove** `PageCount` from the per-page hash (F1).
- **Add**, per page, the Q&A counter value at the start of that page: the number of `qa` blocks, including nested ones, that start on earlier pages (F2).
- **Add** `templateHash` and the render version (F3).
- **Keep:** title (it's in the page header), theme, paper, mode (including the viewer's `?mode=` override), density, and for each block touching the page: its ID, its data and its start position.
- **Tests (each must fail before the fix):**
  1. Appending a block that creates a new last page leaves pages 1…n-1's fingerprints unchanged.
  2. Inserting a `qa` on page 1 changes the fingerprint of a later page that holds a `qa`, and nothing else.
  3. Changing the title or theme changes every page.
  4. Changing one paragraph on page 2 doesn't change page 1.

### O4. Reuse page images across versions (the big win)
- Change the PNG cache from `pages/<layout key>-<ppi>/page-N.png` to **content-addressed** files: `pages/<page fingerprint>-<ppi>.png`.
- **`RenderPNG(pages, ppi)`:**
  1. Get the layout (cached `eval`) and the fingerprints.
  2. For each requested page, use the existing file if `<fp>-<ppi>.png` exists.
  3. Rasterise **all missing requested pages in one** `typst compile --pages a,b,c` call, then rename the outputs to their fingerprint names.
- **Viewer `page()`:** keep the batch behaviour, but limited to **missing** pages. One request renders every missing page of the current version in one Typst call, so the browser's parallel requests don't each start a compile.
- **Cleanup:** after writing new files, delete PNGs in that notebook's `pages/` folder whose fingerprint isn't in the **current** fingerprint set for any of the 2 modes. Run the cleanup under the notebook lock.
- Thumbnails (72 ppi, page 1) and `render_pages` (any ppi) go through the same cache automatically.
- **Tests (using the O0 counters):**
  - after an edit on the last page of 8, a viewer refresh rasterises **≤ 2 pages**, and the earlier pages are served with **0** Typst calls;
  - the content of a reused PNG is byte-identical to a fresh render of that page;
  - switching dark/light keeps a separate image set;
  - cleanup leaves only the current pages.

### O5. Viewer redraw debounce
- In `notebook.html`: when an SSE update arrives, wait **1.2 s** for further updates before fetching changed page images. Each new update restarts the wait, and nothing waits longer than 4 s in total.
- The page fingerprints and the "Updated" badge still update instantly; only the image fetching waits.
- Manual check: a burst of 5 edits makes the browser request each changed page **once**.

### O6. Measure again and record
- Re-run O0's measurements (local + Docker at 0.5 / 1 CPU) and fill in the Results table below.
- **Targets:**
  - an edit is about 50% cheaper than the baseline;
  - an end-of-notebook edit rasterises ≤ 2 pages;
  - unchanged pages cost 0 Typst calls.

### O7. (Optional, only if O6 shows lock contention) Per-notebook locks
- Replace the global `Builder.mu` with a lock per notebook, plus a small semaphore (one Typst at a time per CPU).
- Check caches *before* locking, so cache hits never wait behind another notebook's render.

---

## 4. Out of scope here
- A long-running / incremental Typst compiler (watch mode or a Rust sidecar). It measured 0.27 s vs 0.70 s per edit, but it's a separate project.
- Hosting changes, MCP response size and tool-list trimming (PERFORMANCE_PLAN.md).
- Changing what the pages look like. **Output must be pixel- and text-identical** before and after.

## 5. Risks
| Risk | Guard |
|---|---|
| A stale image shown for a page that actually changed | The O3 fingerprint tests, plus the O4 byte-identical check. When unsure, put more into the fingerprint (it's cheap). |
| Disk growth in `FOLIO_HOME` | O4 cleanup keeps only the current fingerprints; `/tmp` is wiped on deploy anyway. |
| PDF download is slower the first time after an edit | It's one compile (~1 CPU-s), the same work that today happens on *every* edit. |
| Something still reads `Result.PDF` | O2: grep all uses; tests for export and download in both modes. |

## 6. Results

### What changed compared with these instructions
- **O1, `-j`:** the plan said `-j GOMAXPROCS`. Measured in the container, **one thread is clearly fastest under a CPU limit**:

  | CPU limit | 1 thread | 2 threads | 8 threads |
  |---|---|---|---|
  | 0.5 | 2.3 s | 4.5 s | 5.4 s |
  | 1.0 | 1.0 s | 1.4 s | 1.9 s |

  Go reports 2 processors here, never fewer. So Folio now always passes `-j 1`. On an unlimited multi-core laptop, full redraws are a bit slower (30 pages: 354 → 639 ms), which is acceptable because production always runs under a CPU limit.
- **O1, system fonts:** with `--ignore-system-fonts`, every page is byte-identical except where a character is missing from Folio's bundled fonts (the kitchen sink's emoji and Chinese characters). On macOS, Typst used to borrow those from system fonts. The production image has **no** system fonts (`fc-list` lists 0), so production output is unchanged, and local rendering now matches production.
- **O3, extra fingerprint input:** the **page number** (the footer prints it, so page 3's content moving to page 2 must not reuse page 3's image).
- **Q&A numbering:** only pages that show a Q&A card include the running Q&A count, so pages without Q&A cards aren't redrawn needlessly.
- **New safety check:** `Result.DocHash`. Before drawing images or a PDF, Folio checks that `doc.json` on disk belongs to the cached layout, and rebuilds if not (this guards against a failed build leaving them out of sync).
- **Browser cache:** page image URLs now carry only the fingerprint (`?v=<fp>`), so unchanged pages are reused across versions in the browser too. The server marks a page image cacheable **only if the requested fingerprint matches what it is sending**; otherwise it sends `Cache-Control: no-store`. That prevents a race plus undo from leaving a wrong image stuck in the browser cache.

### Local (Apple Silicon, `FOLIO_PERF=1 go test -run TestPerf ./internal/layout`)
| | Before | After |
|---|---|---|
| Typst processes per edit | 2 | **1** |
| Pages redrawn after an edit at the end (8 / 30 pages) | 8 / 30 | **1 / 1** |
| Pages redrawn after an edit in the middle | 8 / 30 | **1 / 1** (when the following pages don't move) |
| Pages redrawn after an edit at the very start | 8 / 30 | 8 / 30 (correct: everything after it moved) |
| Edit latency, 8 / 30 pages | 133 / 286 ms | **72 / 212 ms** |

### Production image, Render-like CPU limits (`--cpus`, 512 MB), 8-page kitchen sink
| | Before, 1 CPU | **After, 1 CPU** | Before, 0.5 CPU | **After, 0.5 CPU** |
|---|---|---|---|---|
| Edit (append 1 paragraph) | 1.67–1.74 s | **0.65–0.67 s** | 5.7–6.7 s | **1.5–2.2 s** |
| Viewer refresh after an edit | 1.00–1.01 s | **0.68–0.71 s** | 2.40–2.55 s | **1.67–1.76 s** |
| First full viewer load | 0.99 s | 1.09 s | 2.46 s | **2.24 s** |
| Append 36 blocks | 2.27 s | **1.26 s** | 6.58 s | **2.62 s** |
| Create notebook | 0.47 s | **0.18 s** | 1.59 s | **0.34 s** |
| `render_pages [1]` | 0.87 s | **0.70 s** | 1.68 s | **1.58 s** |
| PDF download (first after a change) | ~0 s (built on every edit) | 0.75 s | ~0.04 s | 1.71 s |

**Reading the numbers:**
- Edits are 60–70% faster.
- The viewer refresh gains ~30%, not more, because even one redrawn page needs a full Typst layout of the document. Removing that is the "long-running Typst" project in Section 4.
- The PDF now costs one build when someone downloads it, instead of on every edit.
