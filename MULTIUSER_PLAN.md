# Folio — Multi-user plan (product phase)

> **Status: APPROVED 2026-10-22. Executing tasks U0–U11 (Section 10b table).**
> This file went through three passes: draft 1 (Sections 1–9), a review that changed it (Section 10, "Iteration 2"), and a recheck against the actual code (Section 10b, "Iteration 3"; its task table replaces Section 12). Section 11 lists the decisions we need to make together before building. Once we agree, it becomes the build spec, with the same working rules as `docs/SPEC.md` Section 0.

---

## 1. Goal

Turn Folio from "one public notebook store" into a product where:
- people **sign up and log in** on a web page,
- every notebook **belongs to one user**, and nobody else can see or change it,
- after signing up, a user **gets a token to copy**, pastes it once when connecting Claude, and from then on Claude reads and writes only that user's notebooks.

### What we have today (starting point)
- One Render web service (`https://folio-1i2o.onrender.com`) and one Render Postgres.
- `/mcp` (MCP over HTTP, stateful sessions), the viewer (`/`, `/n/{id}`, …) and `/healthz`.
- **No authentication.** The service runs with `--unsafe-public`, so anyone with the URL has full access.
- 7 notebooks in Postgres that belong to nobody (the `notebooks` table has no owner).

---

## 2. User experience (target)

1. **Sign up** at `https://<folio>/signup` with email + password. The user lands on their dashboard.
2. **Dashboard** (`/`): "My notebooks" (only theirs), plus a **"Connect Claude"** card.
3. **Connect Claude** (`/connect`):
   - The user clicks **"Create token"**, and Folio shows it **once**: `folio_pat_7Hk2…`, with a **Copy** button.
   - Next to it are **ready-to-paste commands** with the token filled in:
     ```sh
     claude mcp add -s user --transport http folio https://<folio>/mcp \
       --header "Authorization: Bearer folio_pat_7Hk2…"
     ```
   - Instructions for other clients (Section 10, item 1).
   - A list of the user's tokens (name, created, last used, last 4 chars) with **Revoke**.
4. **In Claude:** "make notes on DNS" → the notebook is created **under that user**, and the viewer link opens only when they're logged in.
5. **Log out**, **change password**, **delete account** (later; see Section 8).

---

## 3. Architecture

```
Browser ──cookie session──┐
                          ▼
                 ┌──────── folio http (Render) ────────┐
Claude Code ──Bearer──►   │  /signup /login /logout       │
                          │  /connect (tokens)            │
                          │  /  /n/{id}...  (viewer)  ◄── session required
                          │  /mcp  ◄── bearer token required
                          │  /healthz (open)              │
                          └──────────────┬────────────────┘
                                         ▼
                                 Postgres (Render)
            users · web_sessions · api_tokens · notebooks(user_id) · blocks · snapshots
```

Two kinds of credentials, both random and both stored only as hashes:

| | Web session | API token |
|---|---|---|
| Used by | Browser (viewer, dashboard) | Claude (MCP) |
| Sent as | `HttpOnly; Secure; SameSite=Lax` cookie | `Authorization: Bearer folio_pat_…` header |
| Lifetime | 30 days, sliding | Until revoked (optional expiry) |
| Created by | Login / signup | "Create token" on `/connect` |

---

## 4. Data model (Postgres; same migrations in SQLite for tests)

```sql
CREATE TABLE users (
  id            TEXT PRIMARY KEY,                     -- u_xxxxxxxx
  email         TEXT NOT NULL UNIQUE,                 -- stored lower-cased
  password_hash TEXT NOT NULL,                        -- argon2id, encoded with params
  created_at    TEXT NOT NULL,
  disabled_at   TEXT                                  -- admin can disable
);

CREATE TABLE web_sessions (
  id_hash    TEXT PRIMARY KEY,                        -- sha256(random 32 bytes)
  user_id    TEXT NOT NULL REFERENCES users(id),
  created_at TEXT NOT NULL,
  expires_at TEXT NOT NULL
);

CREATE TABLE api_tokens (
  id           TEXT PRIMARY KEY,                      -- t_xxxxxxxx
  user_id      TEXT NOT NULL REFERENCES users(id),
  name         TEXT NOT NULL,                         -- "MacBook Claude Code"
  token_hash   TEXT NOT NULL UNIQUE,                  -- sha256(token)
  last4        TEXT NOT NULL,
  created_at   TEXT NOT NULL,
  last_used_at TEXT,
  expires_at   TEXT,
  revoked_at   TEXT
);

ALTER TABLE notebooks ADD COLUMN user_id TEXT REFERENCES users(id);   -- NOT NULL after backfill
CREATE INDEX notebooks_user ON notebooks(user_id, updated_at);
```

- **Blocks and snapshots** don't get a `user_id`. They're always reached through their notebook, so checking ownership at the notebook is enough (Section 5).
- **Token format:** `folio_pat_` + 32 random bytes in base32. The prefix makes leaked tokens recognisable and lets GitHub secret scanning catch them. Since the token is high-entropy, a plain SHA-256 hash is enough to store it (no slow hash needed).
- **Passwords:** argon2id (`golang.org/x/crypto/argon2`, already in `go.sum`) with OWASP-recommended parameters, tuned so a login takes ≤ ~300ms on the Render instance.

---

## 5. Ownership enforcement (the most important part)

**Rule: no code path may read or write a notebook without a user ID.** Enforce it in the **store**, not in each tool, so a forgotten check fails closed.

- The store API becomes user-scoped. `store.Notebook(id)` becomes `store.Notebook(userID, id)`, and the same goes for `Notebooks`, `Blocks`, `Mutate`, `Undo`, `Restore`, `Versions` and `Delete`. Every query adds `AND user_id = ?`.
- **Someone else's notebook looks exactly like a missing one:** the same `ErrNotFound` and the same message, so nobody can probe whether an ID exists.
- **MCP tools** read the user from `req.Extra.TokenInfo.UserID`, which the SDK's `auth.RequireBearerToken` middleware fills from our token verifier. No user, no tool call (the middleware already returned 401).
- **Viewer routes** (`/n/{id}`, events, page PNG, PDF) read the user from the session cookie and pass it to the store.
- The **render/build cache** is keyed by notebook ID on disk. That's safe, because every HTTP route checks ownership before serving a cached file.
- **Admin and CLI paths** (`folio users …`, `folio import`) use an explicit `store.Admin()` handle, so unscoped access is visible in code review.

---

## 6. Auth flows

**Sign up** `POST /signup`:
1. Validate the email format and a password of ≥ 10 characters.
2. Hash the password and create the user.
3. Create a web session, set the cookie, and redirect to `/connect`.

**Log in** `POST /login`:
1. Look up the email and verify the password **in constant time**. Unknown emails still get a dummy hash check, so response time doesn't reveal which emails exist.
2. On success, create the session and set the cookie.
3. On failure, the message is always "Invalid email or password".

**Log out** `POST /logout`: delete the session row and clear the cookie.

**Create token** `POST /connect/tokens`, session required:
1. Generate the token and store its hash plus last 4 characters.
2. Render the page once with the plain token and the ready-made commands.

**Revoke** `POST /connect/tokens/{id}/revoke`: set `revoked_at`. The verifier rejects the token from that moment.

**MCP request:**
1. `RequireBearerToken` calls our verifier with the token.
2. The verifier hashes it and finds a token that isn't revoked or expired and whose user is enabled.
3. It returns `TokenInfo{UserID}` and updates `last_used_at`, at most once per minute so we don't write on every call.

**CSRF:** wrap the form routes with Go's `http.CrossOriginProtection` (checks the `Origin` / `Sec-Fetch-Site` headers), plus `SameSite=Lax` cookies.

---

## 7. Rollout and migrating the existing data

The 7 existing notebooks must end up owned by you, with no window where they're lost or visible to someone else.

1. **Deploy 1 (schema + code, auth enforced):**
   - Migrations add the tables and a **nullable** `notebooks.user_id`.
   - The app requires auth everywhere, and **orphan notebooks (no owner) are invisible to everyone**, so nothing leaks while they're unowned.
2. **Create your account** at `/signup` on the live site.
3. **Adopt the orphans** with an admin CLI, run from the Mac against the external DB URL:
   `folio admin adopt --email you@… --all-orphans`
4. **Deploy 2:** a migration sets `user_id NOT NULL`, and it fails safely if any orphans remain.
5. **Reconnect Claude:**
   - On `/connect`, create a token.
   - Run `claude mcp remove folio-render -s user`, then the new `claude mcp add … --header …` command.
6. **Remove `--unsafe-public`.** Public listening becomes allowed only when auth is enabled (the guard changes from "no auth = loopback only" to "auth on = public OK").

Local/test mode (SQLite, stdio `folio mcp`) keeps working as a **single implicit local user** (`u_local`), so tests and offline debugging don't need a login.

---

## 8. Product hardening (in scope, minimal versions)

- **Rate limits** (in memory, per IP + per account): signup 5/hour/IP, login 10/15 min/IP+email, failed bearer auth 30/min/IP, and `render_pages` / `export_pdf` 30/min/user (Typst is the expensive part).
- **Quotas per user:** max 100 notebooks and max 2,000 blocks per notebook (configurable via env). Hitting one gives a clear tool error.
- **Signup mode:** env `FOLIO_SIGNUP=open|invite|closed`. Start with `invite` (a shared invite code in env), so random people can't fill the free DB.
- **Security headers** on HTML pages: CSP (`default-src 'self'`), `X-Frame-Options: DENY`, `Referrer-Policy: same-origin`.
- **Logging:** never log passwords, tokens, cookies or the `Authorization` header. Log user IDs, not emails.
- **Account basics:** change password and log out of all sessions (deletes all web sessions). Delete account (with all notebooks) and a "download my data" export come later.

---

## 9. Testing (must pass before deploy)

- **Isolation matrix:** user B tries every MCP tool and every viewer route on user A's notebook → always "not found" / 404, never data. This is a table-driven test over all tools and routes, so new tools must be added to it.
- **Tokens:** valid → OK. Revoked or expired → 401. A disabled user → 401. A token never appears in logs (assert on captured log output).
- **Sessions:** login/logout, an expired session goes to `/login`, CSRF-blocked cross-origin POSTs, and the cookie flags are set.
- **Session hijack:** an MCP session started with A's token can't be used with B's token (the SDK checks `TokenInfo.UserID`; we test it).
- **Migration:** orphans stay invisible until adopted, and the `NOT NULL` migration refuses to run while orphans exist.
- **Load sanity:** one argon2id login on the Render instance takes ≤ ~300ms.
- **Both backends:** the full suite on SQLite plus `make test-pg`.

---

## 10. Iteration 2 — review of draft 1 and what changed

I re-read draft 1 looking for things that would break, leak, or annoy users. Findings, and how the plan changed:

1. **claude.ai web/phone/voice can't send a custom header.** Your earlier voice-chat use of Folio is exactly this case. Custom connectors in the Claude app authenticate with **OAuth**, not pasted headers, so with bearer-only auth the Claude app would stop working once auth is on.
   **Change:** split client support into two stages:
   - **Stage 1 (this phase):** bearer tokens for **Claude Code**, plus **Claude desktop through `mcp-remote`**, a small bridge that runs locally and adds the header.
   - **Stage 2 (next phase):** **OAuth 2.1** (authorization code + PKCE + dynamic client registration) so the Claude app's "Add custom connector" just shows Folio's login page. The go-sdk already ships OAuth helpers (`auth.ProtectedResourceMetadataHandler`, `RequireBearerToken`), and our login page becomes the OAuth consent screen. That reuses everything from Stage 1.
   - Rejected: a token in the URL (`/mcp/t/<token>`). It works without OAuth, but URLs leak into logs, history and screenshots. Listed as a fallback in Section 11 only if you need the app sooner.
2. **Stateful MCP sessions plus multiple users means extra risk and restart pain.** **Change:** make **stateless MCP** (Phase 2 task D1.2) a **prerequisite**. Every request is then authenticated on its own, there's no session to hijack, and Render restarts/sleeps stop breaking Claude's connection. We keep the `TokenInfo.UserID` hijack check anyway, as a second layer.
3. **Ownership checks scattered across tools would eventually miss one.** **Change:** enforcement moved **into the store API** (Section 5), plus the table-driven isolation test (Section 9) that fails when a new tool isn't covered.
4. **Draft 1 had no plan for the 7 existing notebooks.** **Change:** the two-deploy rollout with "orphans invisible" and an adopt CLI (Section 7), so nothing is ever exposed or lost.
5. **Open signup on a free-tier DB is an abuse magnet.** **Change:** `FOLIO_SIGNUP=invite` by default, plus rate limits and quotas (Section 8).
6. **Password reset needs email,** which we don't have. **Change:** explicitly out of scope for now. Use the admin CLI `folio admin reset-password --email` in the meantime. Email (verification, reset) moves to the OAuth/product phase.
7. **Login timing could reveal which emails exist.** **Change:** a dummy hash for unknown emails, and one generic error message (Section 6).
8. **Tool links** (`get_viewer_url`, the export `url`) now need a login in the browser. Opening one on the phone without a session would show a confusing 404. **Change:** viewer routes redirect to `/login?next=…` when there's no session, and return 404 only when logged in as someone else.
9. **argon2id on a small Render instance:** the free tier's CPU share is small, and memory-hard hashing could make logins slow or exhaust memory under parallel attempts. **Change:** tune the parameters on the real instance (the Section 9 load sanity check), plus a login rate limit and a limit on concurrent hashes.
10. **The `--unsafe-public` flag** becomes misleading once auth exists. **Change:** replace the rule with "public listening requires auth enabled". The flag remains only for a deliberate no-auth debug run.
11. **Local/test mode:** requiring logins in tests would slow every test. **Change:** an implicit local user for stdio/SQLite mode (Section 7). The isolation tests create real users on both backends.

---

## 10b. Iteration 3 — recheck against the actual code (2026-10-22)

Checked the plan against the code as it is now (store API, viewer, MCP handler, SDK `auth` package). Findings, and what changes:

1. **The SDK rejects tokens without an expiry.** `auth.RequireBearerToken` refuses any `TokenInfo` whose `Expiration` is zero unless `RequireBearerTokenOptions.AllowMissingExpiration` is set. **Change:** the verifier sets `Expiration` from `api_tokens.expires_at`. For tokens that never expire, we set `AllowMissingExpiration: true`. A test covers both cases.
2. **Page images are served as `Cache-Control: public, max-age=31536000, immutable`** (`viewer.go`). Fine for a single user, wrong for private data, because shared caches could keep one user's pages. **Change:** use `private, max-age=…` for pages, PNGs and PDFs, and `no-store` for `/connect` (where tokens are shown) and the login/signup pages.
3. **The viewer index (`/`) lists every notebook** (`s.st.Notebooks()`). **Change:** it becomes the logged-in user's dashboard (already planned; now confirmed as a concrete call site).
4. **The original spec's `rename_notebook` and `delete_notebook` tools were never built**, so users have no way to delete or rename. **Change:** new task U5b adds both (user-scoped; delete stays a soft delete, needs `confirm: true`, and can be undone by an admin).
5. **Scope of the store refactor:** 16 call sites in 5 files (`mcpserver/server.go`, `edit.go`, `tools.go`, `viewer/viewer.go`, `cmd/folio/main.go`). Small enough to switch the whole store API to user-scoped in one task (U2), with no compatibility layer.
6. **SQLite can't add `NOT NULL` to an existing column** (no `ALTER COLUMN`). **Change:** the `NOT NULL` step (rollout deploy 2) is **Postgres-only**. In SQLite (tests/local), ownership is enforced by the store API, and the migration inserts the implicit local user row `u_local` (the foreign key needs it) and assigns existing local notebooks to it.
7. **`folio import` doesn't assign an owner.** **Change:** add `--owner <email>`. Without it, imported notebooks stay orphans until `folio admin adopt`.
8. **The database password was shared in chat** and is still unchanged. With user accounts in that database, this matters even more. **Change:** rotating it, and restricting the external access list to your IP, becomes part of **U0** (before any user data exists).
9. **How Claude Code reacts to 401:** when the server answers 401, Claude Code may start an OAuth sign-in flow, which we don't offer yet. With a correct `--header` that never happens, but a mistyped or revoked token could show a confusing "needs authentication" message. **Change:** the 401 response body says plainly "invalid or revoked Folio token: create a new one at /connect". We don't advertise OAuth metadata until the OAuth phase. Docs and the `/connect` page explain it.
10. **`mcp-remote`** (the desktop app bridge) needs Node.js on the user's machine. **Change:** `/connect` shows the desktop snippet with that requirement stated. Claude Code needs nothing extra.
11. **Turning auth on breaks the current claude.ai connector** (web, phone, voice) until OAuth ships, because the app can't send the token. **Change:** listed explicitly in decision 1 below, so it's an informed choice.
12. **Stateless MCP + bearer auth:** confirmed compatible. Each POST carries the header, and the SDK passes `TokenInfo` per request. In stateless mode the SDK answers GET with 405, which Claude Code handles (it falls back to POST-only).

Updated task list (replaces Section 12):

| ID | Task | Depends on |
|---|---|---|
| U0 | Rotate the DB password + restrict external DB access; stateless MCP; `/healthz` shows commit | — |
| U1 | Schema: users, web_sessions, api_tokens, `notebooks.user_id` (+ `u_local` in SQLite) | U0 |
| U2 | User-scoped store API (all 16 call sites), admin handle, local implicit user | U1 |
| U3 | Web auth pages (signup/login/logout), sessions, CSRF, security + cache headers | U2 |
| U4 | `/connect`: create/list/revoke tokens, copy-paste commands, `no-store` | U3 |
| U5 | `/mcp` bearer auth (expiry handling, clear 401), tools use `TokenInfo.UserID` | U2, U4 |
| U5b | `rename_notebook`, `delete_notebook` tools (user-scoped) | U2 |
| U6 | Viewer behind session, `/login?next=` redirect, private caching | U3 |
| U7 | Rate limits, quotas, signup mode, log redaction, argon2 concurrency limit | U3–U5 |
| U8 | Isolation test matrix (every tool + route), hijack test, migration tests, both backends | U5, U5b, U6 |
| U9 | Admin CLI: `adopt`, `reset-password`, `disable-user`; `import --owner` | U2 |
| U10 | Rollout: deploy 1 → sign up → adopt → deploy 2 (NOT NULL, Postgres) → reconnect Claude → public needs auth | all |
| U11 | Docs: README "Accounts & connecting Claude", DECISIONS, PROGRESS | all |

## 11. Decisions (resolved 2026-10-22)

**Resolved by the user:**
1. **Claude app:** tokens now (Claude Code, desktop via `mcp-remote`). **OAuth in the next phase.** The claude.ai connector stops working once auth is on, until OAuth ships.
2. **Signup:** **open** (`FOLIO_SIGNUP=open`). Rate limits and quotas (U7) are the abuse protection.
3. **Defaults accepted:**
   - Tokens never expire (manual revoke; `AllowMissingExpiration`).
   - Quotas: 100 notebooks and 2,000 blocks per notebook.
   - No email this phase (password reset by admin CLI).
   - Stay on the Free plan.
   - Pages match the viewer's dark/light style.

The original questions are kept below for reference.

### Original questions

1. **Claude app (web/phone/voice) timing:**
   - (a) Claude Code + desktop via `mcp-remote` now, OAuth next phase **(recommended)**,
   - (b) OAuth now (bigger, about 1–2 extra weeks),
   - (c) a temporary token-in-URL fallback.
2. **Signup mode at launch:** `invite` (recommended), `open` or `closed`.
3. **Token expiry:** never (recommended for simplicity, revoke manually) or a default like 90 days.
4. **Quotas:** 100 notebooks / 2,000 blocks per notebook OK?
5. **Email:** skip for now (recommended) or add a provider (e.g. Resend/Postmark) for verification + reset in this phase.
6. **Instance:** stay on the Free plan for this phase, or move to Starter before inviting anyone? Sleeping instances will look broken to new users.
7. **Look of the pages:** match the viewer's dark/light style (recommended), or design something bigger now.

---

## 12. Proposed task order (after we agree)

| ID | Task | Depends on |
|---|---|---|
| U0 | Stateless MCP (Phase 2 D1.2) + `/healthz` commit (D1.3) | — |
| U1 | Schema: users, web_sessions, api_tokens, `notebooks.user_id` (nullable) | U0 |
| U2 | User-scoped store API + admin handle + local implicit user | U1 |
| U3 | Web auth: signup/login/logout pages, sessions, CSRF, security headers | U2 |
| U4 | `/connect`: create/list/revoke tokens, copy-paste commands | U3 |
| U5 | `/mcp` bearer auth (`auth.RequireBearerToken`) + tools use `TokenInfo.UserID` | U2, U4 |
| U6 | Viewer behind session (+ `/login?next=` redirect) | U3 |
| U7 | Rate limits, quotas, signup mode, log redaction | U3–U5 |
| U8 | Isolation test matrix + hijack test + migration tests (both backends) | U5, U6 |
| U9 | Admin CLI: `folio admin adopt / reset-password / disable-user` | U2 |
| U10 | Rollout: deploy 1 → signup → adopt → deploy 2 (NOT NULL) → reconnect Claude → drop `--unsafe-public` | all |
| U11 | Docs: README "Connect Claude" section, DECISIONS, PROGRESS | all |

Rough size: about 1.5–2 weeks of focused work for U0–U11. OAuth (Stage 2) is a separate phase.
