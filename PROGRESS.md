# Folio — Progress

**Last updated:** 2026-10-22 00:30
**Current milestone:** Deployed on Render (https://folio-1i2o.onrender.com); next: Cloud Phase 2 (CLOUD_PHASE2_SPEC.md)
**Current task:** none — waiting for go-ahead on CLOUD_PHASE2_SPEC.md
**Overall status:** Needs user input

## Status summary
Milestones 0–3 are done: the whole v1 scope of the spec. Claude Code can create a notebook, append core blocks (heading, paragraph, bullets, numbered, callout, code, divider, page_break), get an outline with page numbers, render pages as PNG and export a PDF, all with the `clean` theme. Milestone 2 added targeted editing (insert/update/delete/move/read), versions/undo/restore with optimistic concurrency, per-block page/position info, pagination rules (keep-with-next, no-split under 40%) with warnings, and table/qa/fill_in/spacer blocks. Milestone 3 added D2 diagrams (validated at write time, cached, font-matched, size-capped), the notebook and dotgrid themes with ruled/dotted/grid paper, two_column with addressable nested blocks, the `folio serve` live viewer, and a tuned authoring guide and README.

**M3 acceptance (2026-10-22):** kitchen sink renders in all 3 themes (checked by Claude; **needs your human check**: `make kitchen-sink`, then open `build/kitchen-sink/*.pdf`). The viewer pushed live SSE updates while headless Claude wrote Kafka notes (**please confirm in a browser**: `bin/folio serve`, open http://localhost:7420). New-topic session (TCP handshake, notebook theme) came out readable with no manual fixes: `~/.folio/exports/tcp-three-way-handshake-connection-teardown-2026-10-22.pdf`.

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
- [x] M3.1 D2 diagram block, preview_diagram — `0bb2f5c` — verified: diagram tests (font retarget, error line numbers, cache keys), Fit tests, rendered page inspected
- [x] M3.2 notebook + dotgrid themes, set_theme, list_themes, fonts — `e0205ab` — verified: TestThemes, kitchen sink in all themes, caption-split regression sweep
- [x] M3.3 two_column block — `a9310aa` — verified: TestTwoColumn (nested update/read/delete, move refused, ID round-trip), kitchen sink page inspected
- [x] M3.4 folio serve viewer + SSE, get_viewer_url — `4185392` — verified: TestViewer (SSE update in ~570ms), live run with curl: SSE pushed v1→v6 while Claude wrote Kafka notes; page JS passes `node --check`. Not yet checked in a real browser by a human.
- [x] M3.5 Final authoring guide, tuned on 3 notebooks — `d018f45` — verified: REST/consistent hashing/Kafka generated, friction reports acted on, then TCP notes on a new topic came out readable with no manual fixes
- [x] M3.6 README polish — `312ce41` — verified: commands in README checked against `--help`; screenshots from generated notebooks

### Milestone 4 — Free-form layer (optional, ask first)
- [ ] M4 Not started — needs user approval

## Cloud Phase 1 — Postgres + MCP over HTTP (CLOUD_BUILD_SPEC.md)
- [x] C0.1 docker compose Postgres, .env.example, .env ignored — `25aeef4` — verified: `make db-up` healthy, `folio` + `folio_test` DBs listed, `git check-ignore .env`
- [x] C0.2 Makefile db/test-pg/run-http targets — `65ace13` — verified: targets run
- [x] C0.3 doctor reports backend + checks Postgres — `ac11a6c` — verified: PASS on sqlite and postgres, clear FAIL with a wrong password (redacted)
- [x] C1.1 pgx v5.11.0 — `f70ea88`
- [x] C1.2/C1.3 store dialect support + tests on both backends — `9938a9d` — verified: all store tests pass on sqlite and postgres; collation test pins `COLLATE "C"`
- [x] C1.4 FOLIO_DB_URL selects backend for every command — `ac11a6c` — verified: stdio `create_notebook` row visible in Postgres
- [x] C2.1–C2.4 `folio http`, loopback guard, tool URLs, HTTP integration test — `a50ea09` (+ `6fce5e3` serial pg tests) — verified: guard refuses 0.0.0.0; `/healthz` ok; SIGINT clean shutdown; TestMCPOverHTTP passes on both backends; `make test` and `make test-pg` green
- [x] C3.1 Claude Code registered over HTTP (`folio-http`, user scope) — verified: `claude mcp list` shows both folio (stdio) and folio-http (HTTP) Connected
- [x] C3.2 real session — verified: headless Claude (sonnet) wrote DNS notes `n_cjuzapi3` via folio-http, rendered, exported (URL served a PDF)
- [x] C3.3 persistence — verified: rows in Postgres (10 blocks, 9 snapshots); same version/pages after restarting `folio http`, and again after `docker compose down` + `make db-up`
- [x] C3.4 live viewer on Postgres — verified: SSE delivered versions 1→9 while Claude wrote
- [x] C4 docs — README "Run with Postgres + HTTP (local)", DECISIONS entry, this section

## Blockers
- (none)

## Open questions for the user
- Please check the kitchen-sink PDFs and the live viewer in a browser (the two human checks M3 acceptance asks for).
- Milestone 4 (free-form layer) is optional. Build it?
- `insert_blocks`/`move_blocks` refuse nested (two_column) IDs; only update/delete/read accept them (DECISIONS.md). OK?
- Undo semantics: repeated `undo` walks further back; `undo` after `restore_version` reverts the restore (DECISIONS.md). OK?
- Links: only http/https/mailto become clickable; other schemes render as plain text (DECISIONS.md). OK?

## Known issues / tech debt
- Sequence diagrams: D2 places self-loop labels across the lifeline.
- Ruled paper lines are decorative and don't align with text baselines.
- Callouts in the notebook theme have clean rounded borders, not irregular hand-drawn ones (the spec said "if feasible").
- `go test` prints INFO/WARN slog lines to stderr (harmless noise).
- Old `build/` and `build-viewer/` dirs for deleted notebooks are never pruned.

## Performance measurements
| Date | Notebook | Pages | Rebuild time | Notes |
|---|---|---|---|---|
| 2026-10-22 | synthetic (perf_test) | 10 (133 blocks) | 147 ms avg of 5 | Apple M1, parallel compile+eval, clean theme; cached 0.2 ms |
| 2026-10-22 | synthetic (perf_test) | 30 (413 blocks) | 274 ms avg of 5 | same; cached 0.5 ms |
| 2026-10-22 | synthetic (perf_test) | 10 (133 blocks) | 156 ms avg of 5 | after M3 (more template work per block); cached 0.2 ms |
| 2026-10-22 | synthetic (perf_test) | 30 (413 blocks) | 303 ms avg of 5 | after M3; cached 0.5 ms |

## Kitchen sink visual check
| Date | Theme | Result | Issues seen |
|---|---|---|---|
| 2026-10-22 | clean (plain + grid) | Good (checked by Claude, not yet by a human) | Found and fixed: a tall diagram's caption split onto the next page; two_column's right column split across pages. Remaining: sequence-diagram self-loop labels cross a lifeline (D2 placement). |
| 2026-10-22 | notebook | Good (checked by Claude) | Ruled lines don't align with text baselines (by design, see DECISIONS). Kalam lacks the `‣` list marker, so Libertinus is the fallback for it. |
| 2026-10-22 | dotgrid | Good (checked by Claude) | none |

## Session log
### 2026-10-22 — Session 1 (cloud only)
- User decision: run Folio only on Render from now on; keep the code paths (SQLite/stdio) for tests and debugging.
- Did: built `folio import` (Phase 2 D2.1, done early). Imported all 7 local notebooks into Render Postgres. Then removed the local setup: the `folio-http` Claude Code registration (stdio `folio` was already gone), the `folio` entry in the Claude desktop config (backup in the session scratchpad), the local Docker Postgres container and volume, `~/.folio`, and the scratch SQLite backup.
- Verified: the live service lists all 7 notebooks with the same version, block count, page count, title, theme and mode as local, and today's TCP-vs-UDP edit ("1. Short Answer") is on Render. After cleanup, `claude mcp list` shows only `folio-render` (Connected) and live `/healthz` is ok (postgres).
- Note: `.env` still holds the Render external DB URL for admin commands. `make test-pg` recreates the local Docker DB on demand (tests only).
- Next: CLOUD_PHASE2_SPEC.md minus D2 (done): D1 remote export/stateless sessions/commit in healthz, D3 render.yaml + deploy doc, D4 password rotation + perf.
### 2026-10-22 — Session 1 (Render deploy)
- Did: Dockerfile (Folio + Typst 0.15.1), `$PORT` support, `RENDER_EXTERNAL_URL` fallback for links. The user created Render Postgres + a Docker web service (first attempt used the native Go runtime and was recreated as Docker). Fixed the MCP 403 on Render (SDK localhost protection, now off only in public mode). No auth (user decision).
- Verified: the container ran locally like Render (create/append/diagram/render/PDF). Live: `/healthz` ok (postgres); MCP initialize 200 after the fix; Claude Code `folio-render` session made `n_wh2y2onk` end to end (~27s); PDF, viewer (0.27s) and page PNG (1.9s) served.
- Problems: native Go runtime first (no Typst, wrong build path); 403 from DNS-rebinding protection; `export_pdf` reports a container path (fix in Phase 2 D1.1).
- Next: CLOUD_PHASE2_SPEC.md (remote export, restart-safe sessions, deploy identity, SQLite→Postgres import, render.yaml, password rotation).
### 2026-10-22 — Session 1 (Cloud Phase 1)
- Did: C0–C4 from CLOUD_BUILD_SPEC.md.
- Verified: see the Cloud Phase 1 checklist. `make test` (sqlite) and `make test-pg` (sqlite + postgres) green.
- Problems: the Alpine Postgres collation is byte order, so the order test couldn't fail there (added a collation assertion). Postgres test packages raced on the shared test DB (now `-p 1`). One C2 commit went in while `make test-pg` failed, because commands were chained with `;` (fixed in the next commit).
- Next: Phase 2 (Dockerfile for Folio + Render deploy) or Phase 3 (auth). Note: with no auth, keep `folio http` on loopback.
### 2026-10-22 — Session 1 (dark mode, user request)
- Did: added notebook `mode` (dark default, light on request) across store (migration 2), templates (dark palettes for all 3 themes, dark code highlighting), D2 (dark theme, canvas stripped), MCP tools, guide and viewer.
- Verified: TestModes (default dark, explicit light, switch via set_theme, mode kept when omitted, undo restores, bad value rejected); kitchen sink builds in all themes × both modes, dark pages inspected; the real DB migrated with all 6 existing notebooks kept light (backup of the pre-migration DB in the session scratchpad).
- Next: same as before (user checks, M4 decision).
### 2026-10-22 — Session 1 (end)
- Did: M3.1–M3.6. Ran 4 headless Claude note sessions (REST, consistent hashing, Kafka, TCP) and fixed what they and the kitchen sink exposed.
- Verified: `go vet` clean; all tests green (model, store, layout, diagram, mcpserver, viewer); perf 156ms/10 pages, 303ms/30 pages.
- Problems: Typst ignores SVG `@font-face` (fixed by retargeting fonts and registering them with D2); D2 measured labels with its own font, so line gaps were too narrow (fixed); the layout cache didn't change when templates did (now keyed by template hash).
- Next: user checks (kitchen sink PDFs, viewer in a browser); decide on M4.
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
