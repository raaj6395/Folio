# Layout guidelines & linting: progress

Built from the "Built-in layout guidelines and layout linting" spec. The plan file was removed once implemented; the task IDs below come from it.
Decisions (Section 9), taken as the plan's picks:
- Existing notebooks stay on `legacy` density.
- Tables are left-aligned in all notebooks.
- A "session" = first write per user+notebook in 30 min.
- No global guidelines file in v1.
- Render once at the end (≤ 2 pages) after `check_layout` is clean.

| ID | Task | Status |
|---|---|---|
| L0 | Calibrate gaps and MinGapMM | ✅ |
| L1 | Store: guidelines, density, snapshots, migrations, import | ✅ |
| L2 | Model: table align, H3 in two_column | ✅ |
| L3 | Template: density gaps, table alignment, x in marks | ✅ |
| L4 | Layout checks + Issue type + rules.go | ✅ |
| L5 | Cache key + viewer fingerprints include density | ✅ |
| L6 | MCP: layout_hints, density, guidelines tools, check_layout | ✅ |
| L7 | Guidance: layout_rules.md, instructions, guide | ✅ |
| L8 | Full tests + screenshots | ✅ |
| L9 | Post-deploy manual run (you) | ☐ |

## Changed test assertions
(each with the reason)

- `layout/m25_test.go` `TestDiagramRender`: was "zero warnings", now "no issues other than `cramped` and `oversized_diagram`".
  - The test notebook uses legacy spacing (flagged `cramped` by design) and a 6-node top-down chain that's 158mm tall (62% of the page), which the new rules flag on purpose.
  - The test's real intent (diagrams render with readable labels, nothing overflows) is still asserted.
- `mcpserver/server_test.go` `TestDiagrams`: was "zero warnings", now expects exactly one `oversized_diagram` warning and a matching `layout_hints` entry. Its 2-node top-down half diagram is 110mm tall (43% of the page), which the new rule flags.
- `mcpserver/server_test.go` `TestTwoColumn`: an H2 inside a column is now rejected with "only level 3 headings are allowed inside two_column" (H3 is allowed).
- `httpserver/auth_test.go`: the isolation matrix now covers `check_layout`, `get_notebook_guidelines` and `set_notebook_guidelines`. The test itself demanded it: every tool taking `notebook_id` must be listed.
- `layout/analyze_test.go`: `mark` literals gained the new `x` field; the `analyze()` calls pass a density. Expected warnings are unchanged.

## Calibration (L0)

Kitchen sink, clean theme. The gap is next block start − previous block end, in mm. Callouts and Q&A cards report positions inside their 8pt (2.8mm) padding, so the check subtracts 2.8mm per side for them.

| Pair | legacy | compact | comfortable | airy |
|---|---|---|---|---|
| paragraph → H2 | 4.8 | 4.1 | 9.7 | 11.9 |
| table → table | 4.4 | 3.0 | 5.9 | 7.8 |
| diagram → diagram | 4.4 | 2.9 | 6.0 | 7.7 |
| callout → callout (−5.6 inset) | 4.5 | 3.0 | 6.0 | 7.9 |
| pages | 8 | 8 | 9 | 9 |

**Finding:** the `above`/`below` gaps passed to boxed blocks (`nosplit`) never took effect. They were set on a block inside `layout()`, and Typst applies spacing to the outer container, so every box used the default 1.2em (4.4mm).
- **Fix:** for non-legacy densities, the gap goes on an outer block. Legacy keeps the old structure; its numbers above are identical before and after the change.

**Thresholds** (`internal/layout/rules.go`): `MinHeadingGapMM = 7` (before an H2) and `MinBoxGapMM = 5.2` (before a table, diagram or callout).
- Comfortable and airy pass by construction. Legacy is flagged, with the fix "switch to comfortable".
- The check is skipped for `compact`, which is deliberately tight.

## Rule conflict found during verification

The spec says both "chains of 4+ nodes use `direction: right`" and "prefer a `half` diagram inside `two_column`". Combined, they fail:

| Half-width diagram | Size on the page | Labels |
|---|---|---|
| 3 nodes, sideways | 87×15–19mm | 8–10pt ✓ |
| 4 nodes, sideways | — | **5.7pt ✗** (`unreadable_diagram` fired on the sample) |
| 4 nodes, top-down | **17×138mm ✗** | 10.8pt |

**Rule added:** a `half` diagram holds about 3 nodes across; chains of 4+ nodes go full width with `direction: right`. Applied to `layout_rules.md` and the server instructions.

## Verification (L8)
- `go test ./...` and `make test-pg` are green; Postgres ran twice in a row, so the migrations apply to an existing database.
- `tools/list` grew from 25.7 KB to 29.9 KB: 3 new tools, plus the `density` and `align` fields.
- Template change made during L8: **table captions are left-aligned under their table**. Tables now start at the text edge, so a page-centered caption looked detached; heights are unchanged.

### Sample notebook, "REST vs gRPC"
Built through MCP the way the rules describe: an H1, an intro, a `two_column` with a half diagram and an H3 + steps, a text table, a numeric table, paired tip callouts, and Q&A. `check_layout` results:

| Density | Page 1 fill | Score | Issues |
|---|---|---|---|
| legacy | 75.8% | 90 | `cramped` (H2s and tables without breathing room) |
| compact | 73.0% | 100 | — |
| comfortable (default) | 88.2% | 100 | — |
| airy | 96.4% | 100 | — |

## Screenshots
- [legacy, light](screenshots/layout-guidelines/rest-vs-grpc-legacy-light.png)
- [compact, light](screenshots/layout-guidelines/rest-vs-grpc-compact-light.png)
- [comfortable, light](screenshots/layout-guidelines/rest-vs-grpc-comfortable-light.png)
- [airy, light](screenshots/layout-guidelines/rest-vs-grpc-airy-light.png)
- [comfortable, dark](screenshots/layout-guidelines/rest-vs-grpc-comfortable-dark.png)

## Acceptance criteria
- [x] A fresh MCP session receives the layout rules via `initialize` instructions (≤ 40 lines): `TestLayoutRulesDelivered`
- [x] `get_authoring_guide` includes the full layout rules: `TestLayoutRulesDelivered`
- [x] `set_notebook_guidelines` / `get_notebook_guidelines` work, are versioned, and appear in `get_outline`: `TestGuidelinesDensityAndCheckLayout`, `TestGuidelinesAndDensityAreVersioned`
- [x] Each check has a triggering and a non-triggering test: `TestChecks`, `TestUnreadableDiagramIssue`
- [x] `check_layout` returns issues and a score: `TestReportAndHints`, `TestGuidelinesDensityAndCheckLayout`
- [x] Tables left-aligned by default; `align` works: `TestTableAlignment`, `TestDensityAndTableRendering` (table x = text edge), screenshots
- [x] Comfortable density adds visible gaps before H2s: `TestDensityAndTableRendering` (gap ≥ 7mm and ≥ legacy + 3mm), screenshots
- [x] Level-3 headings work inside `two_column`: `TestHeadingInsideTwoColumn`, screenshots
- [x] Existing tests pass; the only new block JSON field is the optional table `align`
- [ ] Manual run in a fresh Claude Code session after deploy (L9, yours)
