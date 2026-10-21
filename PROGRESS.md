# Folio — Progress

**Last updated:** 2026-10-22 00:05
**Current milestone:** M2
**Current task:** M2.1 — insert/update/delete/move/read tools
**Overall status:** On track

## Status summary
Milestones 0 and 1 are done. Claude Code can create a notebook, append core blocks (heading, paragraph, bullets, numbered, callout, code, divider, page_break), get an outline with page numbers, render pages as PNG and export a PDF, all with the `clean` theme. Editing tools, undo, tables/Q&A, diagrams, other themes and the viewer are not built yet.

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
- [ ] M2.1 Stable IDs, fractional ordering, insert/update/delete/move/read
- [ ] M2.2 Snapshots, list_versions, undo, restore, expected_version
- [ ] M2.3 Layout info via typst, page numbers, get_layout_info
- [ ] M2.4 Pagination rules and warnings
- [ ] M2.5 table, qa, fill_in, spacer
- [ ] M2.6 Build caching and performance measurement

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

## Kitchen sink visual check
| Date | Theme | Result | Issues seen |
|---|---|---|---|

## Session log
### 2026-10-21 — Session 1
- Did: M0.1–M0.4. Installed Typst 0.15.1 via Homebrew. Registered folio with Claude Code at user scope.
- Verified: doctor PASS; headless Claude Code call to ping returned pong.
- Problems: none.
- Next: M1.1.
- Later in the same session: M1.1–M1.6 done and M1 acceptance run (see Status summary). `ping` was removed once real tools existed. Tests: model, store, layout, mcpserver all green.
