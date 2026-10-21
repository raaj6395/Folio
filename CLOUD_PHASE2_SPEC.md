# Folio Cloud — Phase 2: harden the Render deployment

> **Read this whole file before starting.** It continues `CLOUD_BUILD_SPEC.md` (Phase 1, done) and follows all working rules in Section 0 of `docs/SPEC.md`: session rituals, one task at a time, nothing marked done until its check has actually run and passed, no guessing at failures, verify library APIs, record deviations in `DECISIONS.md`, commit after every task, keep `PROGRESS.md` current.
>
> **Authentication is NOT part of this phase** (user decision, 2026-10-22). Don't add any auth layer. Section 7 describes it only so other work doesn't block it later.

---

## 1. Where things stand

- **Live:** https://folio-1i2o.onrender.com. Render Web Service, **Docker** runtime, region **Oregon**, branch `main`, auto-deploy on commit, **Free** instance.
- **Database:** Render Postgres `folio_db_2vji` (Oregon). The service uses the **internal** URL through `FOLIO_DB_URL`. The local `.env` holds the **external** URL for admin work from the Mac.
- **Verified 2026-10-22:**
  - `/healthz` → `folio ok (postgres)`.
  - Claude Code (`folio-render`, user scope) connected, created a notebook, rendered a page and exported a PDF in about 27s.
  - The PDF URL, viewer page and page PNG are all served.
- **Fixed during deploy:** the MCP endpoint returned **403** on Render. The SDK's DNS-rebinding protection rejected requests that arrive on a localhost connection with a public `Host` header, which is how Render's proxy delivers them. In public mode (`--unsafe-public`) that protection is now off; on loopback it stays on (commit `f9616d8`, test `TestPublicModeAcceptsForeignHost`).
- **Known problems this phase fixes:**
  1. `export_pdf` returns a server-side file path (`/tmp/folio/exports/...`). Remote users can't use it, and writing exports to the container disk is pointless.
  2. MCP sessions live in server memory. Every deploy, restart or free-tier sleep drops them, and clients may then fail with "session not found" until they reconnect.
  3. The 7 existing notebooks are still in the local SQLite file (`~/.folio/folio.db`). The Render database only has test data.
  4. Render's settings exist only in the dashboard. The service was first created with the wrong runtime (native Go) and no health check, so the config should live in the repo.
  5. The database password was shared in chat and must be rotated.
  6. `/healthz` doesn't say which commit is running, so verifying a deploy means guessing.

---

## 2. Tasks

Work in order. Commit as `D1.2: …`. Pushing to `main` auto-deploys to Render, so push only after local tests pass, and check the live service after each push that changes code.

### D1 — Remote-friendly behaviour

- **D1.1 `export_pdf` in HTTP mode.** When Folio runs with a public URL (HTTP mode), return only `url` and don't write a file. Make `path` `omitempty`. stdio mode keeps writing the file and returning `path`, as today. Update the tool description to match. **Check:** the HTTP integration test asserts there's no `path` and a working `url`. The stdio tests are unchanged.
- **D1.2 Survive restarts (MCP sessions).**
  1. Reproduce the failure first: start `folio http` locally, connect a client, restart the server, and make another tool call. Record the exact error.
  2. Then switch the HTTP handler to **stateless** mode (`StreamableHTTPOptions{Stateless: true}`; verify the field with `go doc`). Folio never sends server→client requests, so it loses nothing.
  3. Re-run the same reproduction.
  4. Record the before/after in `DECISIONS.md`. If stateless breaks something (e.g. a client that requires a session ID), record that and keep stateful.

  **Check:** the integration test, plus a manual restart test with Claude Code.
- **D1.3 Deploy identity in `/healthz`.** Render sets `RENDER_GIT_COMMIT`. Include its first 7 chars in the response, e.g. `folio ok (postgres) commit f9616d8`. Show nothing extra when unset. **Check:** after the push, live `/healthz` shows the pushed commit.

### D2 — Move the existing notebooks to Render

- **D2.1 `folio import --from <sqlite-file>`.** Copies notebooks, blocks and snapshots from a SQLite file into the store selected by `FOLIO_DB_URL`. Requirements:
  - Keep IDs, positions, versions, `base`, timestamps, mode, theme and paper exactly, so outlines, undo and `restore_version` behave the same after import.
  - **Idempotent:** skip notebooks whose ID already exists in the target and report them. Never overwrite.
  - Each notebook imports in its own transaction. Report per-notebook results (`imported`, `skipped (exists)`, `failed: <reason>`).
  - Skip soft-deleted notebooks unless `--include-deleted`.
  - `--dry-run` lists what would be imported.
  - Add the needed bulk-insert method to `store`. Don't build a second store.

  **Check:** a test that imports a populated SQLite store into a fresh SQLite store and into Postgres (when `FOLIO_TEST_DB_URL` is set), then compares outlines, versions and undo behaviour. Running it a second time imports nothing.
- **D2.2 Run it against Render** from the Mac, using the external URL in `.env`:
  1. Run `--dry-run` first.
  2. Ask the user before the real run (it writes to the production DB).
  3. Then run it.

  **Check:** live `list_notebooks` shows all 7 notebooks with the same page counts as locally. Spot-check 2 with `get_outline`, and open one in the live viewer.

### D3 — Render config in the repo

- **D3.1 `render.yaml` (Blueprint).** Describe the web service and the database: Docker runtime, `./Dockerfile`, region `oregon`, branch `main`, `healthCheckPath: /healthz`, auto-deploy, and `FOLIO_DB_URL` from the database's internal connection string (`fromDatabase` … `connectionString`). Verify the current Blueprint schema in Render's docs and record the doc version you used. The plan stays **free** unless the user says otherwise.
  - Don't apply it automatically to the existing service. Tell the user how to sync it, or keep it as the documented source of truth, and record which.

  **Check:** a YAML lint passes, and every key exists in Render's Blueprint spec (cite the doc).
- **D3.2 `docs/DEPLOY_RENDER.md`:** the step-by-step guide (create DB, create service with the **Docker** runtime, env var, health check, connect Claude Code and the Claude app, troubleshooting table including the 403 cause). Link it from the README.

### D4 — Operations

- **D4.1 Rotate the database password** (manual, done by the user in the Render dashboard). Afterwards: update `FOLIO_DB_URL` on the web service (it may update automatically if it's wired with `fromDatabase`) and the external URL in the local `.env`. Confirm live `/healthz` and local `folio doctor` both pass. Record in `PROGRESS.md` that it's done (never record the password).
- **D4.2 Measure performance on the Free instance.** Time live `append_blocks` and `render_pages` for a ~10-page notebook. Record in `PROGRESS.md` (Performance table) next to the local numbers. If renders take over ~10s, note it and propose options (a bigger instance, a pre-render cache). Don't change the instance without the user.
- **D4.3 Clean up Claude Code registrations.** `folio-http` (local, only works while `make run-http` runs) shows "Failed to connect" when that server is down. Ask the user whether to keep `folio` (stdio, SQLite), `folio-http` (local Postgres) and `folio-render`, and remove the ones they don't want.

---

## 3. Acceptance (whole phase)

- [ ] Live `/healthz` shows the deployed commit.
- [ ] `export_pdf` over HTTP returns a working URL and no server path.
- [ ] Claude Code keeps working after a Render redeploy without manual reconnecting (or the remaining limitation is documented, with the reproduction).
- [ ] All 7 local notebooks are on Render with matching page counts. Re-running the import changes nothing.
- [ ] `render.yaml` and `docs/DEPLOY_RENDER.md` are in the repo.
- [ ] The DB password is rotated, and the service and local `.env` work with the new one.
- [ ] Free-instance timings are recorded.
- [ ] `make test` and `make test-pg` are green. No auth code was added.

---

## 4. Risks to keep in mind

- **The service is public with no auth.** Anyone with the URL can read, change and delete all notebooks. Don't post the URL anywhere. Moving real notebooks there (D2) increases what's exposed, so remind the user before D2.2.
- **The Free instance** sleeps after inactivity, and the first request takes about 50s. Claude's first tool call may time out, and a retry works.
- **Container disk is ephemeral.** Only Postgres is durable. Never store anything important in `FOLIO_HOME` on Render.
- **The external DB URL** in `.env` allows direct writes to production. Keep `.env` out of git (it is), and prefer the local Docker DB for development.

---

## 5. Out of scope for this phase

- **Authentication** (the user deferred it).
- Multiple users, share links, object storage, custom domain, multi-instance scaling.

## 6. Next phase (for context only)

**Phase 3, authentication** (only when the user asks for it): a bearer token for Claude Code; a secret URL prefix for claude.ai connectors, which can't send headers; a cookie so viewer pages keep working; `/healthz` stays open. Then OAuth and per-user data (`user_id` on notebooks) for other users.

## 7. Notes that keep Phase 3 easy

- Keep all HTTP routing in `internal/httpserver.Handler`, so an auth middleware can wrap one place.
- Don't add new unauthenticated endpoints that expose notebook data besides `/healthz`.
