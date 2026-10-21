# Folio — Progress

**Last updated:** 2026-10-22 00:10
**Current milestone:** M3
**Current task:** M3.1 — D2 diagram block
**Overall status:** On track

## Status summary
Milestones 0, 1 and 2 are done. Claude Code can create a notebook, append core blocks (heading, paragraph, bullets, numbered, callout, code, divider, page_break), get an outline with page numbers, render pages as PNG and export a PDF, all with the `clean` theme. Milestone 2 added targeted editing (insert/update/delete/move/read), versions/undo/restore with optimistic concurrency, per-block page/position info, pagination rules (keep-with-next, no-split under 40%) with warnings, and table/qa/fill_in/spacer blocks. Not built yet: diagrams, the notebook/dotgrid themes, two_column, the viewer.

**M2 acceptance (2026-10-22):** Headless Claude (sonnet) was asked to "add a comparison table of HTTP methods after the methods section" of `n_hku4c2ig`. It used get_outline and inserted the table right after the section's last block. I checked every outline page against `pdftotext`: all match (one false alarm from inline-code spacing in pdftotext output). `undo` removed the table (3→2 pages) and `restore_version` brought it back. Seen: the table sat alone on page 2 because of an existing page_break, so I added a warning for pages under 30% full that end in a page_break.

**M1 acceptance (2026-10-22):** `claude -p "Make 2 pages of notes on REST APIs using folio…" --allowedTools "mcp__folio__*" --model sonnet` produced `~/.folio/exports/rest-apis-2026-10-21.pdf` (2 pages, 86%/77% fill). Claude rendered pages and reported them clean. I checked both pages by eye: well structured and readable. Minor: paragraphs sit tight under H1s, and there's extra space under code captions.

## How to verify the current state
```sh
make build && make test
bin/folio doctor
claude -p "Make 1 page of notes on DNS using folio, render it and export the PDF." --allowedTools "mcp__folio__*"
bin/folio export <notebook-id> -o /tmp/x.pdf
```

## Task checklist
Legend: [ ] todo · [~] in progress · [x] done · [!] blocked

### Milestone 0 — Foundations
- [x] M0.1 Repo init — `8a1cb87` — verified: `go build ./...` ok
- [x] M0.2 folio doctor — `9649ac0` — verified: `bin/folio doctor` prints PASS for typst 0.15.1 and the data dir
- [x] M0.3 Verify/record versions and APIs — `345ae8b` — verified: `go doc` on SDK/D2, `typst eval` spike returned block positions
- [x] M0.4 ping spike + registration — `1791fcc` — verified: `claude mcp list` shows Connected; `claude -p` returned `{"message":"pong"}`; `TestPing` passes

### Milestone 1 — Markdown-ish to PDF loop
- [x] M1.1 SQLite schema and migrations — `0ffedb1` — verified: store tests (mutate/undo/restore/conflict/pruning/soft delete)
- [x] M1.2 Model, validation, inline parser (core types) — `9a980a2` — verified: model tests incl. malformed inline and error-message content
- [x] M1.3 main.typ template, clean theme — `aa299b6` — verified: compiled sample doc, inspected PNG
- [x] M1.4 Layout package: doc.json, PDF, PNG, timeout — `c70c9ff` — verified: layout tests (page break positions, 12-page PNG render)
- [x] M1.5 MCP tools (create/list/append/outline/render/export/guide) — `cb74cb0` — verified: TestM1Flow over in-memory transport; tools/list schema inspected
- [x] M1.6 Injection-safety test — `ace131a` — verified: pdftotext shows Typst syntax literally; javascript: links neutralised

### Milestone 2 — Editing and layout awareness
- [x] M2.1 Stable IDs, fractional ordering, insert/update/delete/move/read — `e1c3c6e` — verified: TestM2Editing (insert/start/update type change/move order/delete/read)
- [x] M2.2 Snapshots, list_versions, undo, restore, expected_version — `a98f8f2` — verified: TestM2Editing undo×2, restore, stale expected_version error names current version
- [x] M2.3 Layout info via typst, page numbers, get_layout_info — `c0e95b4` — verified: TestPagesMatchPDF (60 blocks, every reported page checked with pdftotext)
- [x] M2.4 Pagination rules and warnings — `a514050` — verified: sweep test (heading+callout never separated or split across a page boundary), synthetic warning tests
- [x] M2.5 table, qa, fill_in, spacer — `d3ad164` — verified: validator tests, rendered page inspected (hidden answer upside down at page bottom)
- [x] M2.6 Build caching and performance measurement — `793da13` — verified: `FOLIO_PERF=1 go test ./internal/layout -run Perf -v`

### Milestone 3 — Diagrams, themes, viewer
- [ ] M3.1 D2 diagram block, preview_diagram
- [ ] M3.2 notebook + dotgrid themes, set_theme, list_themes, fonts
- [ ] M3.3 two_column block
- [ ] M3.4 folio serve viewer + SSE, get_viewer_url
- [ ] M3.5 Final authoring guide, tuned on 3 notebooks
- [ ] M3.6 README polish

### Milestone 4 — Free-form layer (optional, ask first)
- [ ] M4 Not started — needs user approval

## Blockers
- (none)

## Open questions for the user
- Undo semantics: repeated `undo` walks further back; `undo` after `restore_version` reverts the restore (DECISIONS.md). OK?
- Links: only http/https/mailto become clickable; other schemes render as plain text (DECISIONS.md). OK?

## Known issues / tech debt
- (none)

## Performance measurements
| Date | Notebook | Pages | Rebuild time | Notes |
|---|---|---|---|---|
| 2026-10-22 | synthetic (perf_test) | 10 (133 blocks) | 147 ms avg of 5 | Apple M1, parallel compile+eval, clean theme; cached 0.2 ms |
| 2026-10-22 | synthetic (perf_test) | 30 (413 blocks) | 274 ms avg of 5 | same; cached 0.5 ms |

## Kitchen sink visual check
| Date | Theme | Result | Issues seen |
|---|---|---|---|

## Session log
### 2026-10-22 — Session 1 (continued)
- Did: M2.1–M2.6, M2 acceptance.
- Verified: all package tests green; perf numbers above.
- Problems: in-place edits didn't persist (the store diffed against the same slice), fixed in store.Mutate; the inferred output schema rejected flattened block fields, fixed with an open Block schema; Typst `flatten()` recursed into span arrays; marker positions around `v()` are unreliable (spacer now uses a fixed-height block).
- Next: M3.1 D2 diagrams.

### 2026-10-21 — Session 1
- Did: M0.1–M0.4. Installed Typst 0.15.1 via Homebrew. Registered folio with Claude Code at user scope.
- Verified: doctor PASS; headless Claude Code call to ping returned pong.
- Problems: none.
- Next: M1.1.
- Later in the same session: M1.1–M1.6 done and M1 acceptance run (see Status summary). `ping` was removed once real tools existed. Tests: model, store, layout, mcpserver all green.
