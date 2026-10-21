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
