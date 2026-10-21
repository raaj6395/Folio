# Folio Cloud — Phase 1: Postgres storage + MCP over HTTP (local)

> **Read this whole file before starting.** It extends `docs/SPEC.md`. All working rules from Section 0 of that spec still apply: session start/end rituals, one task at a time, no task marked done until its check has actually run and passed, no guessing at failures, verify library APIs, record deviations in `DECISIONS.md`, commit after every task, keep `PROGRESS.md` current.
>
> If anything here is unclear or looks wrong, stop and ask.

---

## 1. Goal

Make Folio ready to run as a cloud service, but test everything **locally** first:

1. **Postgres storage.** Folio can store notebooks in Postgres instead of SQLite. For local testing, Postgres runs in a Docker container. Folio connects to it through a URL in an `.env` file.
2. **MCP over HTTP.** A new `folio http` command serves the same MCP tools over HTTP (the MCP "streamable HTTP" transport), so Claude Code can connect with a URL instead of starting a local process.

When this phase is done, Claude Code talks to `http://127.0.0.1:8080/mcp`, and every notebook it creates lands in the Postgres container.

### 1.1 Out of scope for this phase
- **Authentication.** There is none in this phase. Because of that, the HTTP server **must listen on localhost only** (Section 5.3).
- Deploying to Render, Dockerizing the Folio binary itself, OAuth, multiple users, share links, object storage (S3/R2).
- Copying existing SQLite notebooks into Postgres. Note it as a future task; don't build it.

### 1.2 What must keep working
- `folio mcp` (stdio) and SQLite stay the default. With no database URL set, Folio behaves exactly as it does today.
- All existing tests keep passing.

---

## 2. Target architecture (local)

```
Claude Code ──HTTP (MCP)──► folio http  (runs on your Mac, 127.0.0.1:8080)
                              ├─ /mcp       MCP tools (same tools as stdio)
                              ├─ /healthz   health check
                              └─ /, /n/...  web viewer (same as folio serve)
                                   │
                                   │ FOLIO_DB_URL (from .env)
                                   ▼
                         Postgres in Docker (localhost:5433)
```

- Only the database runs in Docker. Folio runs as a normal binary on the Mac, so Typst, fonts and the build cache in `~/.folio` work as they do today.
- Rendered files (PDF, PNG, SVG) stay a disposable cache on local disk. **Postgres is the only durable store.**

---

## 3. Configuration

### 3.1 Environment variables

| Variable | Example | Meaning |
|---|---|---|
| `FOLIO_DB_URL` | `postgres://folio:folio@localhost:5433/folio?sslmode=disable` | If set, use Postgres. If unset, use SQLite in `FOLIO_HOME` (today's behaviour). |
| `FOLIO_HTTP_ADDR` | `127.0.0.1:8080` | Listen address for `folio http`. Default `127.0.0.1:8080`. |
| `FOLIO_PUBLIC_URL` | `http://127.0.0.1:8080` | Base URL used in links that tools return (viewer, PDF download). Defaults to Render's `RENDER_EXTERNAL_URL`, then `http://` + `FOLIO_HTTP_ADDR`. |
| `FOLIO_HOME` | `~/.folio` | Unchanged: build cache, logs, SQLite file. |
| `POSTGRES_USER` / `POSTGRES_PASSWORD` / `POSTGRES_DB` | `folio` / `folio` / `folio` | Read by the Postgres container. |
| `FOLIO_TEST_DB_URL` | same form as `FOLIO_DB_URL`, database `folio_test` | Postgres tests run only when this is set. |

### 3.2 Files to create
- **`.env.example`**: committed. Lists every variable above with safe local values and a one-line comment each.
- **`.env`**: created from `.env.example` for local use. **Never committed.** Add `.env` to `.gitignore` **before** creating it.
- **`docker-compose.yml`**: one service, `db`:
  - Image: an exact pinned Postgres tag (e.g. `postgres:17.x-alpine`). Check that the tag exists and record it in `DECISIONS.md`.
  - Port `127.0.0.1:5433:5432`. 5433, so it doesn't clash with any Postgres already on the Mac. Bound to localhost.
  - A named volume, so data survives `docker compose down` (only `down -v` wipes it).
  - A healthcheck using `pg_isready`.
  - An init script that also creates the `folio_test` database.

### 3.3 Loading `.env`
- Don't add a dotenv library. Docker Compose reads `.env` automatically. For Folio, the Makefile loads it (`include .env` + `export`) for the targets that need it, and the README shows `set -a; source .env; set +a` for running by hand.

---

## 4. Storage layer

### 4.1 Approach
Today `internal/store` is a single SQLite implementation used directly by `mcpserver`, `viewer` and `cmd/folio`. Make it work on both databases with the **least code**:

- **Recommended:** keep one `Store` built on `database/sql`, plus a small dialect value (`sqlite` | `postgres`) that handles the few differences below. Both databases then share the same queries and the same test suite.
- Alternative: two implementations behind an interface. Only do this if the dialect approach turns out messy, and record why in `DECISIONS.md`.
- Pick the backend in one place (`store.Open(url)` or similar): a `postgres://` URL → Postgres, otherwise SQLite.

### 4.2 Driver
- Use `github.com/jackc/pgx/v5`, through its `stdlib` package so `database/sql` keeps working. Pin the version and add a one-line justification in `DECISIONS.md` (spec rule: every new dependency needs one).

### 4.3 Differences to handle

| Topic | SQLite (today) | Postgres |
|---|---|---|
| Placeholders | `?` | `$1, $2, …` (rewrite in one helper, or write queries with `$n` and convert for SQLite) |
| Migrations | `PRAGMA user_version` | A `schema_migrations(version)` table. Use it for both backends, with migration lists per dialect. |
| Locking a notebook during a write | `_txlock=immediate` | `SELECT … FROM notebooks WHERE id = $1 FOR UPDATE` inside the transaction |
| **Block ordering keys** | byte-order compare by default | Declare `position TEXT COLLATE "C"`. **Critical:** without it, Postgres sorts with locale rules and silently breaks block order. Add a test that would catch this. |
| Block data | `TEXT` holding JSON | `JSONB` is fine, but the code reads it back as text. Make sure canonical JSON survives a round trip, and that diff detection (comparing old and new `data`) still works. JSONB reorders keys, so either compare after decoding or store `TEXT`. Pick one and record it. |
| Timestamps | RFC3339 text | `TIMESTAMPTZ` is preferred. Keep the `time.Time` behaviour identical for callers. |
| Busy/locked errors | `busy_timeout` | Connection-pool settings, plus a sensible statement timeout |

### 4.4 Schema (Postgres)
Same tables and columns as the current SQLite schema after migration 2 (`notebooks` with `mode`, `blocks`, `snapshots` with `mode`). Keep the same primary keys and the `blocks(notebook_id, position)` index.

Add no `user_id` yet (no auth in this phase), but the schema shouldn't make adding it hard later.

### 4.5 Behaviour that must not change
These are covered by tests today and must pass unchanged on both backends:
- mutate with diffing
- snapshots, plus pruning to the last 200
- undo/restore semantics (the `base` logic)
- soft delete
- `expected_version` conflicts

---

## 5. HTTP transport

### 5.1 Command
`folio http`: one process, one port, serving:
- `/mcp`: MCP over streamable HTTP. Go SDK v1.8.0 API (verified with `go doc`): `mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server, *mcp.StreamableHTTPOptions)`. Use the default stateful sessions unless there's a reason not to, and record the choice.
- `/healthz`: returns 200 when the DB is reachable.
- The existing viewer routes (`/`, `/n/{id}`, `/n/{id}/events`, `/n/{id}/page/…`, `/n/{id}/pdf`, `/style.css`), so one URL serves both Claude and the browser.

### 5.2 Behaviour
- Uses the same `mcpserver.New(...)` and the same tools as stdio. No tool behaves differently because of the transport, except the links in 5.4.
- Logs go to the log file and stderr. stdout isn't special in this mode, but keep the same logger.
- Graceful shutdown on Ctrl-C (SIGINT/SIGTERM): stop accepting requests, finish in-flight ones, close the DB.
- The viewer and the MCP handler in one process share a single layout `Builder`. That's fine, because its lock is per process. `folio serve` keeps using its own build dir, as now.

### 5.3 Safety while there is no auth
- Refuse to start if `FOLIO_HTTP_ADDR` isn't a loopback address (`127.0.0.1`, `::1`, `localhost`), unless `--unsafe-public` is passed. That flag prints a loud warning.
- Mention this clearly in the README.

### 5.4 Tool changes for HTTP mode
- `export_pdf`: also return `url` (`FOLIO_PUBLIC_URL/n/<id>/pdf`) next to `path`, so a remote client has something usable.
- `get_viewer_url`: build the URL from `FOLIO_PUBLIC_URL` (or the default port in stdio mode, as today).
- Additive changes only. Existing fields stay.

---

## 6. Tasks

Work in order. IDs continue the project's scheme. Commit as `C1.2: …`.

### C0 — Local database setup
- **C0.1** Add `.env` to `.gitignore`. Create `.env.example`, `.env`, and `docker-compose.yml` (Section 3.2).
- **C0.2** Makefile targets: `db-up` (`docker compose up -d --wait`), `db-down`, `db-reset` (down -v + up), `db-psql` (psql shell in the container), `test-pg`.
- **C0.3** `folio doctor`: report which backend is in use, and check the Postgres connection when `FOLIO_DB_URL` is set.

**Check:** `make db-up` reports the DB healthy. `make db-psql` opens a shell showing the `folio` and `folio_test` databases. `folio doctor` with `.env` loaded prints PASS for the Postgres connection.

> Note: on this Mac the Docker daemon is often not running. If `docker` says it can't connect, run `open -a Docker` and wait for it.

### C1 — Storage layer on Postgres
- **C1.1** Verify the pgx API (`go doc`). Pin the version and record the dependency.
- **C1.2** Dialect support in `internal/store`: placeholders, migrations table, Postgres schema, `COLLATE "C"`, `FOR UPDATE`.
- **C1.3** Run the whole store test suite against both backends (Postgres tests skip unless `FOLIO_TEST_DB_URL` is set). Add a Postgres-specific test proving block order is byte order (e.g. keys `"Z"`, `"a"`, `"z1"` come back in byte order).
- **C1.4** `cmd/folio`: pick the backend from `FOLIO_DB_URL` for `mcp`, `serve`, `export`, `doctor` and the new `http` command.

**Check:** `make test` passes (SQLite). `make test-pg` passes (Postgres). `folio mcp` with `FOLIO_DB_URL` set works with Claude Code over stdio.

### C2 — MCP over HTTP
- **C2.1** `folio http` with `/mcp`, `/healthz`, the viewer routes, and graceful shutdown (Section 5).
- **C2.2** Loopback-only guard and the `--unsafe-public` flag (Section 5.3).
- **C2.3** Tool link changes (Section 5.4).
- **C2.4** Integration test: run the handler in `httptest`, connect with the SDK's streamable HTTP client transport (verify its name with `go doc`), then create a notebook, append blocks, get the outline, render a page and export. Run it against SQLite always, and against Postgres when `FOLIO_TEST_DB_URL` is set.

**Check:** `curl http://127.0.0.1:8080/healthz` → 200. The integration test passes.

### C3 — End-to-end with Claude Code
- **C3.1** Register over HTTP: `claude mcp add --transport http folio-http http://127.0.0.1:8080/mcp` (verify syntax with `claude mcp add --help`). Leave the existing stdio `folio` registration alone. `claude mcp list` should show both connected.
- **C3.2** Run a real session: "Make 1 page of notes on DNS using folio-http, render a page, export the PDF."
- **C3.3** Prove the data is in Postgres:
  1. Query `notebooks` and `blocks` with `make db-psql`.
  2. Stop and restart `folio http`, and confirm the notebook is still there (`get_outline`).
  3. Run `docker compose down` + `make db-up` (no `-v`), and confirm the data survived.
- **C3.4** Open the viewer at `http://127.0.0.1:8080/n/<id>` while Claude edits. Live updates must work with Postgres.

**Check:** all of C3.1–C3.4 observed and recorded in `PROGRESS.md`.

### C4 — Docs
- **C4.1** README: a "Run with Postgres + HTTP (local)" section covering `.env`, `make db-up`, `folio http`, Claude Code registration, and the no-auth/localhost warning.
- **C4.2** `PROGRESS.md`: add a "Cloud Phase 1" checklist with C0–C4, plus session log entries. `DECISIONS.md`: Postgres image tag, pgx version, dialect approach, JSONB vs TEXT, stateful vs stateless HTTP sessions, loopback guard.

---

## 7. Acceptance (whole phase)

- [ ] With `.env` loaded and `make db-up` run, `folio http` starts, and `/healthz` returns 200.
- [ ] Claude Code, registered over HTTP, creates, edits, renders and exports a notebook. The data is in Postgres and survives restarts of both Folio and the DB container.
- [ ] The viewer at the same URL shows live updates.
- [ ] Without `FOLIO_DB_URL`, everything works exactly as before on SQLite. `make test` passes.
- [ ] `make test-pg` passes, including the byte-order test.
- [ ] `folio http` refuses non-loopback addresses unless `--unsafe-public` is given.
- [ ] `.env` is not tracked by git. `.env.example` is.

---

## 8. Next phases (not now, for context)

- **Phase 2:** a Dockerfile for Folio (binary + Typst), deploy on Render (Web Service + Render Postgres), secrets in Render.
- **Phase 3:** authentication (bearer token first, then OAuth for claude.ai connectors), `user_id` on notebooks, per-user isolation tests, rate limits.
- **Later:** share links, object storage for archived PDFs, Postgres `LISTEN/NOTIFY` for live updates, multi-instance build locking, a SQLite → Postgres import command.
