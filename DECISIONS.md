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
