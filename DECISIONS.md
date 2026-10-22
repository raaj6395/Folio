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

## 2026-10-22 — Dark mode by default
- **Decision:** Each notebook has a `mode` (`dark` | `light`) separate from the theme, so every theme has both looks. New notebooks default to `dark`. `create_notebook` takes `mode`. `set_theme` takes an optional `mode` (omitted = unchanged). `preview_diagram` takes `mode`. `list_themes` lists the modes. Migration 2 adds `mode` to `notebooks` and `snapshots` with SQL default `light`, so notebooks made before this change stay as they looked, and undo/restore covers mode changes. Each theme file exports a `dark` dict that `main.typ` merges over `theme` (page fill, ink, callout tints, table/code colours, paper ink, margin line). Dark code uses a small bundled TextMate theme (`templates/themes/code-dark.tmTheme`). Dark diagrams use D2 theme 200 (Dark Mauve), with D2's canvas rect removed so they sit directly on the page. The mode is part of the build cache key, the viewer page signatures and the server instructions/guide ("use light only when the user asks").
- **Why:** User request: dark by default, light when asked.
- **Alternatives considered:** separate `clean-dark`/`notebook-dark` themes (doubles the theme list and can't switch modes independently of the theme).
- **Affects interface/behaviour?** yes (new `mode` field; default look is dark) — **User approved?** yes (user asked for it, 2026-10-22)

## 2026-10-22 — Viewer dark/light toggle is view-only
- **Decision:** The viewer toolbar has a "View" selector: Notebook (default, follows the notebook's stored mode, including when Claude switches it) / Dark / Light. The choice is stored in the browser's localStorage and sent as `?mode=` on the events, page and PDF URLs. The server renders with that mode but never writes it to the notebook. The viewer stays read-only (spec 4/9).
- **Why:** User request. Writing the mode from the viewer would create a version per click, fill the undo history and cause expected_version conflicts with Claude's edits.
- **Alternatives considered:** a toggle that calls set_theme (rejected for the reasons above).
- **Known limit:** two tabs viewing the same notebook in different modes rebuild alternately (one layout cache per notebook dir); fine for a single user.
- **Affects interface/behaviour?** yes (viewer UI) — **User approved?** yes (user asked for it, 2026-10-22)

## 2026-10-22 — Viewer chrome follows dark/light
- **Decision:** Both viewer pages share `/style.css` (embedded), built on CSS variables with `:root` (light) and `:root[data-theme="dark"]` tokens, plus `color-scheme` so native controls match. A tiny head script sets `data-theme` before first paint (no white flash) from the stored View choice. The home page uses the choice, defaulting to dark (the app default). A notebook page uses the choice, or the notebook's own mode under "Auto", and follows live mode changes from Claude. The home page has the same View selector; both pages share the localStorage key `folio-view-mode`. The "Notebook" option is renamed "Auto".
- **Why:** User reported the home page stayed light.
- **Alternatives considered:** `prefers-color-scheme` (follows the OS rather than the user's Folio choice; the user asked for dark by default).
- **Affects interface/behaviour?** yes (viewer UI) — **User approved?** yes (user request)

## 2026-10-22 — Cloud Phase 1: Postgres + MCP over HTTP
- **Decision:**
  - Postgres image `postgres:17.7-alpine` on `127.0.0.1:5433`, with a named volume. The init script also creates `folio_test`.
  - Driver `github.com/jackc/pgx/v5 v5.11.0` through `database/sql` (`pgx/v5/stdlib`). It's the most widely used pure-Go Postgres driver, and the existing `database/sql` code keeps working.
  - **One store with a dialect flag**, not two implementations. Queries are written with `?` and rewritten to `$n` for Postgres. Postgres uses `SELECT … FOR UPDATE` to lock the notebook during a write. `blocks.position` is `TEXT COLLATE "C"`.
  - Data and timestamps stay `TEXT` on Postgres. Text keeps the canonical JSON bytes that diff detection compares (JSONB would reorder keys), and avoids dialect-specific scan types. Timestamps now use a fixed-width UTC format, so text order is chronological. The previous variable-length RFC3339Nano format sorted e.g. `.5Z` after `.53Z`.
  - SQLite keeps `PRAGMA user_version` migrations, so existing local databases are untouched. Postgres uses a `schema_migrations` table, with each migration applied in a transaction. This deviates from the spec's "one migrations table for both"; it's less code and avoids migrating existing SQLite files.
  - `folio http` serves `/mcp` (`mcp.NewStreamableHTTPHandler`, default **stateful** sessions, one shared `mcp.Server`), `/healthz` (DB ping) and the viewer routes on one port. It refuses non-loopback addresses unless `--unsafe-public` is passed, and shuts down gracefully on SIGINT/SIGTERM.
  - `export_pdf` adds `url` and `get_viewer_url` uses `FOLIO_PUBLIC_URL` when serving over HTTP. Both changes are additive.
  - `make test-pg` runs packages serially (`-p 1`): test packages share `folio_test` and truncate it, so in parallel they wiped each other's data.
- **Why:** CLOUD_BUILD_SPEC.md Phase 1.
- **Alternatives considered:** a separate Postgres implementation behind an interface (two copies of every query); JSONB/TIMESTAMPTZ columns; stateless HTTP sessions (fine too, but stateful is the SDK default and works with Claude Code); a schema per test package (more setup than `-p 1`).
- **Notes:** the Alpine image's default collation already sorts by bytes (musl), so a byte-order test alone can't catch a missing `COLLATE "C"`. A Postgres-only test asserts the column's collation instead. ICU collation (`und-x-icu`) gives `_,a,a0b,Z,z1` versus byte order `Z,_,a,a0b,z1`, showing the risk on glibc/managed Postgres.
- **Affects interface/behaviour?** yes (new `http` command, additive tool fields) — **User approved?** yes (CLOUD_BUILD_SPEC.md, requested by the user)

## 2026-10-22 — Render deploy via Docker, no auth yet
- **Decision:** Multi-stage `Dockerfile`: `golang:1.26-alpine` builds a static binary (`CGO_ENABLED=0`; the SQLite driver is pure Go). Typst 0.15.1 comes from its official `x86_64-unknown-linux-musl` release, and the runtime is `alpine:3.22`, running as a non-root user. Fonts and templates are embedded in the binary. `FOLIO_HOME=/tmp/folio` holds only the disposable build cache. `folio http` listens on `0.0.0.0:$PORT` when `PORT` is set (Render) and `FOLIO_HTTP_ADDR` isn't. The container runs `folio http --unsafe-public`.
- **Why:** Render's native Go runtime has no Typst. The user asked to deploy now **without an auth layer**.
- **Risk accepted by the user:** the public service has no authentication. Anyone with the URL can read and change every notebook. Adding auth is the next step before sharing the URL or connecting it in claude.ai long-term.
- **Alternatives considered:** a token/secret-path lock (deferred at the user's request).
- **Affects interface/behaviour?** yes — **User approved?** yes (user explicitly said no auth for now, 2026-10-22)

## 2026-10-22 — MCP 403 on Render: DNS-rebinding protection off in public mode only
- **Decision:** `httpserver.Handler` takes a `public` flag. When `folio http` listens on a non-loopback address (`--unsafe-public`, i.e. Render), it sets `StreamableHTTPOptions.DisableLocalhostProtection`. On loopback the protection stays on.
- **Why:** On Render every `/mcp` request returned 403. Reproduced locally: the same request succeeds with `Host: localhost` and gets 403 with `Host: folio-1i2o.onrender.com`. The go-sdk rejects requests that arrive on a localhost connection with a non-localhost Host header. Render's proxy delivers public traffic that way. The protection only guards local-only servers against DNS rebinding, which doesn't apply to a deliberately public service.
- **Alternatives considered:** rewriting the Host header in middleware (it hides the real host from the SDK for no benefit).
- **Affects interface/behaviour?** no — **User approved?** n.a.

## 2026-10-22 — U0: stateless MCP, commit in /healthz
- **Decision:** `/mcp` uses `StreamableHTTPOptions{Stateless: true}`. `/healthz` appends `commit <7 chars>` from Render's `RENDER_GIT_COMMIT`.
- **Why:** Reproduced in `TestClientSurvivesServerRestart`: with stateful sessions, after a server restart the client fails with `session not found` (what Claude hit after every Render redeploy or free-tier sleep). Stateless mode fixes it, and makes each request independently authenticated, which multi-user needs. Folio never sends server→client requests, so nothing is lost.
- **Alternatives considered:** a persistent session store (EventStore); not needed.
- **Affects interface/behaviour?** no — **User approved?** n.a. (part of the approved multi-user plan, U0)

## 2026-10-22 — Multi-user accounts (MULTIUSER_PLAN.md, U1–U9)
- **Decision:**
  - **Schema:** `users`, `web_sessions`, `api_tokens`, `notebooks.user_id`. SQLite migration 3 creates the implicit `u_local` user and assigns existing local notebooks to it. In Postgres (migration 2), existing notebooks stay **orphans** (invisible to everyone) until `folio admin adopt`, then `folio admin require-owners` sets `NOT NULL`. That's an admin command, not a migration, because a migration that fails on orphans would stop the service from booting.
  - **Ownership lives in the store:** every notebook query takes a user ID, and another user's notebook is reported exactly like a missing one. Mutation check: removing the user filter from the notebook lookup made the isolation test fail for 7 read tools and all 3 viewer routes, while write tools stayed protected by their own filter.
  - **Credentials:** passwords use argon2id (`golang.org/x/crypto` v0.58.0; OWASP parameters m=19MiB, t=2, p=1; at most 2 hashes at once). Sessions and API tokens are 32 random bytes, stored only as SHA-256. Tokens look like `folio_pat_…` (recognisable to secret scanners), are shown once, can be revoked, and never expire (`AllowMissingExpiration`, because the go-sdk rejects tokens without an expiry by default). Cookies are `HttpOnly`, `SameSite=Lax`, and `Secure` behind HTTPS.
  - **HTTP:** `/mcp` uses the SDK's `auth.RequireBearerToken`; tools read `req.Extra.TokenInfo.UserID` (stateless mode, so there are no sessions to hijack). Site routes are wrapped in `http.CrossOriginProtection` (CSRF) and security headers (CSP allows inline script and style because the pages use small inline scripts; all user data is escaped by `html/template`). Page images use `Cache-Control: private`, and account pages use `no-store`.
  - **Limits:** signup 5/hour/IP, login 10 per 15 min per IP+email, failed tokens 30/min/IP (then 429), token creation 20/hour/user, render/export/preview 30/min/user. Quotas: 100 notebooks per user and 2,000 blocks per notebook. Limiters are in memory (one instance).
  - `folio http` requires auth by default. `--no-auth` works only on loopback, and `--unsafe-public` is gone. `export_pdf` over HTTP returns only a URL (no files on the server).
  - Added the missing `rename_notebook` and `delete_notebook` tools (delete needs `confirm: true`).
- **User decisions:** tokens now, OAuth next phase (the claude.ai web/phone connector pauses until then). Open signup. Defaults: no token expiry, the quotas above, no email (admin password reset), Free plan, viewer-style pages.
- **Rehearsed:** the binary currently live (548f3d0) created a notebook in a fresh local Postgres at schema v1. The new binary migrated it (the notebook became 1 orphan, invisible), then signup → adopt → require-owners (`user_id` NOT NULL) → create a token → MCP `list_notebooks` with the token returned it. A bad token got 401.
- **Alternatives considered:** a token in the URL for claude.ai (rejected: URLs leak); bcrypt (argon2id is OWASP's first choice); a NOT NULL migration (would block startup).
- **Affects interface/behaviour?** yes — **User approved?** yes (MULTIUSER_PLAN.md approved 2026-10-22)

## 2026-10-22 — OAuth 2.1 for Claude connectors (CIMD + DCR)
- **Decision:** Folio is its own authorization server.
  - **Discovery:** protected-resource metadata (RFC 9728) at `/.well-known/oauth-protected-resource` and `/.well-known/oauth-protected-resource/mcp`, with `resource` = `<public>/mcp` and `authorization_servers` = `[<public>]`. AS metadata (RFC 8414) at `/.well-known/oauth-authorization-server` advertises `client_id_metadata_document_supported: true`, `token_endpoint_auth_methods_supported: ["none"]` (Claude needs **both** to choose CIMD), S256 PKCE, `authorization_response_iss_parameter_supported` (RFC 9207), and a `registration_endpoint` (DCR fallback). The 401 from `/mcp` carries `WWW-Authenticate: Bearer resource_metadata="…/mcp"`.
  - **Clients:**
    - CIMD: client documents are fetched with an SSRF guard (dial-time IP check on the resolved address, no redirects, HTTPS only, 64KB cap, 5s timeout), their `client_id` must match exactly, and they're cached 1h.
    - Non-loopback redirects must share the `client_id` origin.
    - Loopback redirects (`localhost`, `127.0.0.1`, `::1`) match **without the port**, as Anthropic's docs require for Claude Code. Verified against Claude Code's live document at `https://claude.ai/oauth/claude-code-client-metadata`.
    - DCR clients are public (`none`).
  - **Flow:** authorize requires login (redirects to `/login?next=`), then a consent page that names the **host of the client_id URL** (not the self-asserted `client_name`), the redirect host, and a warning when the app has only loopback redirects. Codes are single-use, last 10 min and are bound to the client, redirect_uri, PKCE challenge and resource.
  - **Tokens:** opaque access tokens (`folio_oat_`, 1h, stored hashed in `api_tokens` with `grant_id`) and refresh tokens (`folio_ort_`, rotated on every use, invalid → `invalid_grant`). Grants are listed on `/connect`; Disconnect revokes the grant and its access tokens.
  - Token, register and metadata endpoints sit outside CSRF protection (no cookies; called server-to-server). The consent POST sits inside it.
  - Personal access tokens stay available under "Advanced" on `/connect`. The `mcp-remote` desktop snippet is gone.
- **Sources read before building:** MCP authorization spec (draft): authorization, authorization-server discovery, client registration. Anthropic: "Authentication for connectors" (claude.com/docs/connectors/building/authentication) and "Lazy authentication" (…/lazy-authentication): CIMD selection rule, callback `https://claude.ai/api/mcp/auth_callback`, port-agnostic loopback matching, consent naming the client_id host, refresh rotation with `invalid_grant`, form-encoded token endpoint, 10s endpoint timeout.
- **Why:** the Claude app's custom connector dialog only does OAuth (no token field), and the user wants name + URL + sign-in.
- **Alternatives considered:** a token in the URL (rejected: the spec forbids it); `static_headers` (beta, organization-wide, can't identify the individual user).
- **Affects interface/behaviour?** yes — **User approved?** yes (user asked to implement the auth flow properly, 2026-10-22)
