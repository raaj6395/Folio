# 📒 Folio

### Ask Claude for notes. Get a real notebook.

> *"Make 2 pages of revision notes on REST APIs, with a diagram of the request lifecycle."*
>
> …and a few seconds later there are two properly typeset A4 pages: headings, definition boxes, a comparison table, a diagram, and Q&A cards with the answers printed **upside down at the bottom of the page**, like a real revision book.

Folio is a tool server for Claude (it speaks [MCP](https://modelcontextprotocol.io)). **Claude is the author; Folio is the typesetter.** Claude writes structured content. Folio lays it out on A4 pages, renders the diagrams, keeps headings with their text, avoids splitting boxes across pages, and then **shows Claude its own pages** so Claude can spot and fix layout problems itself.

| clean | notebook | dotgrid |
|---|---|---|
| ![clean theme](docs/screenshots/clean.png) | ![notebook theme](docs/screenshots/notebook.png) | ![dotgrid theme](docs/screenshots/dotgrid.png) |

<sub>Three themes, each in **dark** (the default) and **light** mode. Screenshots are light mode.</sub>

---

## Contents

1. [What it feels like](#-what-it-feels-like)
2. [What Claude can put on a page](#-what-claude-can-put-on-a-page)
3. [Themes, paper and dark mode](#-themes-paper-and-dark-mode)
4. [How it works](#-how-it-works)
5. [Deploy your own (Render)](#-deploy-your-own-render)
6. [Connect Claude](#-connect-claude)
7. [The live viewer](#-the-live-viewer)
8. [Configuration](#-configuration)
9. [Developing Folio](#-developing-folio)
10. [Project layout](#-project-layout)
11. [Roadmap](#-roadmap)
12. [FAQ and troubleshooting](#-faq-and-troubleshooting)

---

## ✨ What it feels like

A typical session, in your own words:

1. **"Make notes on Kafka basics, about 3 pages, dotgrid theme."**
   Claude reads Folio's authoring guide, plans an outline and writes it section by section. After each write, Folio reports **which page every block landed on, how full each page is, and any layout problems** ("this diagram's labels are too small to read", "page 2 is only 14% full").
2. **Claude checks its work.** It renders a page, looks at the image, and fixes what looks wrong: shortening a table, changing a diagram's direction, moving a block.
3. **You watch it happen.** Open the viewer link Claude gives you, and pages appear and flash as they change.
4. **You keep editing in plain language.**
   - *"Add a comparison table of HTTP methods after the methods section."*
   - *"Page 3 looks cramped, fix it."*
   - *"Change the first heading to 'Short Answer'."*
   - *"Switch it to light mode, I want to print it."*
5. **Undo is built in.** Every change is a version. *"Undo that"* steps back, and *"restore version 4"* jumps back further.
6. **Export:** a PDF you can print, read on an iPad, or write on (the fill-in blanks have real ruled lines).

<details>
<summary><b>More prompts to try</b></summary>

- "Make a cheat sheet for consistent hashing with blanks I can fill in."
- "Turn what we discussed in this session into notes, with a Q&A card at the end of each section."
- "Make interview notes on TCP vs UDP: a short answer first, then a comparison table, then 3 self-test questions."
- "Explain the TCP three-way handshake with a sequence diagram."
- "Put the pros and cons of SQL vs NoSQL side by side in two columns."
- "Make the Kafka notes use the notebook theme on ruled paper."

</details>

---

## 🧱 What Claude can put on a page

Claude never thinks in coordinates. It writes **blocks**, and every block has a stable ID (`b_7f2a…`), so later edits are small and targeted instead of rewriting the whole document.

| Block | What it's for | How Folio treats it |
|---|---|---|
| **heading** (H1–H3) | Section titles | Never left alone at the bottom of a page |
| **paragraph** | Explanations | Flows freely across pages |
| **bullets / numbered** | Lists, nested up to 3 levels | Short lists stay on one page |
| **callout** | `note`, `tip`, `warning`, `definition`, `key_point` | Tinted box with an accent colour; doesn't split if it fits |
| **code** | Snippets with syntax highlighting | Splits only when longer than ~40% of a page |
| **table** | Comparisons (2–6 columns) | Long tables split and **repeat the header row** |
| **diagram** | Flows, architectures, sequence diagrams, written in [D2](https://d2lang.com) | Laid out automatically, sized to stay readable, never split |
| **qa** | Self-test cards | The answer can be printed **upside down at the page bottom** |
| **fill_in** | "Explain X in your own words" | Prompt plus ruled lines to write on |
| **two_column** | Side-by-side content (pros/cons, before/after) | Columns stay together |
| **divider / spacer / page_break** | Rhythm and structure | Folio warns when a page break wastes a page |

Text inside blocks supports a small, safe set of inline styles: `**bold**`, `*italic*`, `` `code` ``, `[links](https://…)` and `==highlight==`.

<details>
<summary><b>What a block looks like (the JSON Claude sends)</b></summary>

```json
{"type": "callout", "variant": "definition", "title": "Idempotent",
 "body": ["Doing it twice has the same effect as doing it once."]}

{"type": "diagram", "size": "half", "caption": "Request path",
 "source": "direction: right\nclient -> api: HTTPS\napi -> db: SQL"}

{"type": "qa", "question": "Why is PUT idempotent?",
 "answer": "It sets the resource to a given state; repeating it changes nothing.",
 "hide_answer": true}
```

Every block is validated before anything is saved. If one block in a batch of 10 is wrong, nothing is written, and Claude gets a precise error such as `block[3] (table): row 2 has 4 cells, expected 3 (columns: Method, Idempotent, Safe)`.

</details>

---

## 🎨 Themes, paper and dark mode

| Theme | Look | Default paper |
|---|---|---|
| **clean** | Crisp and typeset: Inter + JetBrains Mono, left-accent callouts | plain |
| **notebook** | Handwritten: Kalam font, red margin line, **hand-drawn (sketch) diagrams** | ruled |
| **dotgrid** | Bullet-journal: clean fonts on a dot grid | dotted |

- **Paper:** `plain`, `ruled`, `dotted` or `grid`, in any theme.
- **Dark mode is the default.** Ask for light mode and Claude switches it; only the look changes, not the content. Dark pages get dark-friendly code colours and dark diagrams.
- **The viewer has its own toggle** (Auto / Dark / Light) that changes only how *you* see the pages, without touching the notebook.

---

## ⚙️ How it works

```
 You ──"make notes on DNS"──► Claude ──MCP tools──► Folio ──► Postgres (your notebooks)
                                 ▲                    │
                                 │                    ├─► D2   → diagrams (SVG)
                                 │                    ├─► Typst → A4 pages (PDF / PNG)
                                 └── page images, ────┘
                                     layout report,
                                     warnings
```

1. **Blocks are the source of truth.** They live in Postgres (or SQLite for local development). PDFs and images are always regenerated from them.
2. **One fixed Typst template** turns the blocks into pages. Your text reaches Typst only as data, never as code, so nothing in a note can break or hijack the layout. A test throws Typst syntax at it to prove this.
3. **Folio measures where everything landed** and reports it back after every write, so Claude knows the page count, how full each page is, and what went wrong, without asking for images.
4. **Diagrams** are written in D2 and laid out automatically. Their text uses the theme font, and Folio warns when a diagram would be too small to read.
5. **Every change is a version** (the last 200 are kept). Writes can pass `expected_version`, so two editors never silently overwrite each other.

**Speed:** rebuilding a 10-page notebook takes ~150ms and a 30-page one ~300ms (measured on an Apple M1). Unchanged notebooks come from cache in under a millisecond.

<details>
<summary><b>All MCP tools</b></summary>

| Group | Tools |
|---|---|
| Notebooks | `list_notebooks`, `create_notebook`, `set_theme`, `list_themes` |
| Reading | `get_outline` (compact: ID, type, page, summary per block), `read_blocks`, `get_layout_info` |
| Writing | `append_blocks`, `insert_blocks`, `update_block`, `delete_blocks`, `move_blocks` |
| Seeing | `render_pages` (page images for Claude), `preview_diagram` |
| History | `list_versions`, `undo`, `restore_version` |
| Output | `export_pdf`, `get_viewer_url` |
| Help | `get_authoring_guide` (also the resource `folio://guide`) |

</details>

---

## 🚀 Deploy your own (Render)

Folio runs as a single Docker web service plus a Postgres database. The repo's `Dockerfile` builds Folio and installs Typst; fonts and templates are built into the binary.

> 🔐 **Accounts built in.** Every notebook belongs to one account. People sign up on your site, create a personal token, and Claude only ever sees their own notebooks.

**1. Create the database:** Render → **New → Postgres**. Pick a region (e.g. Oregon) and note the **Internal Database URL**.

**2. Create the web service:** **New → Web Service** → connect this repo, then:

| Setting | Value |
|---|---|
| Language / Runtime | **Docker** (not Go: the native Go runtime has no Typst) |
| Region | **The same region as the database** |
| Branch | `main` |
| Health check path | `/healthz` |
| Instance type | **Starter** recommended. Free sleeps when idle, and the first request then takes ~50s, long enough for Claude's tool calls to time out. |

**3. Add one environment variable:**

| Key | Value |
|---|---|
| `FOLIO_DB_URL` | the database's **Internal** URL |

Render sets `PORT` itself, and Folio picks up its public address from Render automatically.

**4. Check it:** open `https://<your-service>.onrender.com/healthz`. It should say `folio ok (postgres) commit …`. Then open the site itself and **sign up**.

Every `git push` to `main` redeploys. Your notebooks are safe across deploys, because they live in Postgres.

<details>
<summary><b>Moving existing notebooks from a local SQLite file</b></summary>

```sh
FOLIO_DB_URL='<External Database URL>?sslmode=require' bin/folio import --from ~/.folio/folio.db --dry-run
FOLIO_DB_URL='<External Database URL>?sslmode=require' bin/folio import --from ~/.folio/folio.db
```

The import keeps every notebook's ID, blocks and full version history, so undo still works. It skips notebooks that already exist, so running it twice is safe.

</details>

---

## 🔌 Connect Claude

1. **Sign up** at `https://<your-service>.onrender.com/signup`. You land on **Connect Claude**.
2. Click **Create token**. Folio shows your personal token (`folio_pat_…`) **once**, with ready-to-copy commands.
3. **Claude Code:** paste the command it gives you. It looks like this:

```sh
claude mcp add -s user --transport http folio https://<your-service>.onrender.com/mcp \
  --header "Authorization: Bearer folio_pat_…"
claude mcp list        # folio: ... (HTTP) - ✔ Connected
```

**Claude desktop app:** the Connect page also gives you a `claude_desktop_config.json` snippet that uses `mcp-remote` to add the header (needs Node.js).

**Claude web and phone app:** these connect through OAuth, which is next on the roadmap. Until then, use Claude Code or the desktop app.

You can create a token per device and **revoke** any of them on the Connect page. A revoked token stops working immediately.

Then just ask: *"Make 1 page of notes on DNS using Folio and give me the viewer link."*

<details>
<summary><b>Tip: tell Claude when to use Folio (CLAUDE.md snippet)</b></summary>

Add this to the `CLAUDE.md` of any project:

```markdown
## Notes with Folio
When I ask for notes, a cheat sheet or a revision sheet, use the `folio` MCP tools:
1. Call `get_authoring_guide` once per session.
2. `create_notebook` (or `list_notebooks` / `get_outline` to continue one).
3. Write section by section with `append_blocks`. Read the returned page fill and warnings.
4. `render_pages` on a page or two to check the result, and fix problems with `update_block` / `insert_blocks` / `move_blocks`.
5. End each section with `qa` or `fill_in` blocks for active recall.
6. `export_pdf` and give me the link. Offer `get_viewer_url` if I want to watch live.
```

</details>

<details>
<summary><b>Tired of approving every tool call in Claude Code?</b></summary>

Add an allow rule to `~/.claude/settings.json`:

```json
{ "permissions": { "allow": ["mcp__folio__*"] } }
```

Folio keeps every version, so even a deletion can be undone with `restore_version`.

</details>

---

## 👀 The live viewer

The same service that Claude talks to also serves a viewer:

- `/`: all notebooks
- `/n/<id>`: pages as you'd see them in a PDF reader, updating **live** while Claude writes. Changed pages flash orange for a moment.
- The toolbar shows the version, "updated 5s ago", the theme and mode, a **View: Auto / Dark / Light** switch, and **Download PDF**.

---

## 🔧 Configuration

| Variable | Default | What it does |
|---|---|---|
| `FOLIO_DB_URL` | *(unset → SQLite in `FOLIO_HOME`)* | `postgres://…` or `postgresql://…` selects Postgres |
| `PORT` | — | Set by Render; Folio listens on `0.0.0.0:$PORT` |
| `FOLIO_HTTP_ADDR` | `127.0.0.1:8080` | Listen address for `folio http` (overrides `PORT`) |
| `FOLIO_PUBLIC_URL` | Render's `RENDER_EXTERNAL_URL`, else `http://` + listen address | Base for the PDF/viewer links that tools return |
| `FOLIO_HOME` | `~/.folio` (`/tmp/folio` in Docker) | Render cache, logs, and the SQLite file in local mode |
| `FOLIO_SIGNUP` | `open` | `closed` turns off public sign-up (create accounts with an admin process instead) |
| `FOLIO_TEST_DB_URL` | — | Runs the test suite against Postgres too |

<details>
<summary><b>All commands</b></summary>

| Command | What it does |
|---|---|
| `folio http [--no-auth]` | MCP at `/mcp` (token required), accounts, viewer and `/healthz`. `--no-auth` is for local development and only works on loopback addresses. |
| `folio admin adopt --email <e>` | Give every notebook without an owner to that account |
| `folio admin reset-password --email <e>` | Set and print a new random password (there's no email yet) |
| `folio admin disable-user --email <e>` | Block that account's login and tokens |
| `folio admin require-owners` | (Postgres) Make ownership mandatory once no orphans remain |
| `folio import --from <file.db> [--owner <email>] [--dry-run] [--include-deleted]` | Copy notebooks from SQLite into `FOLIO_DB_URL` |
| `folio export <notebook-id> [-o file.pdf]` | Export a PDF from the command line |
| `folio doctor` | Checks Typst, fonts, data dir and the database (password masked) |
| `folio mcp` | MCP over stdio, for local development |
| `folio serve [--port 7420]` | Standalone local viewer, for local development |

</details>

---

## 🛠 Developing Folio

You need **Go 1.26+**, the **Typst CLI 0.15+** (`brew install typst`) and, for Postgres tests, **Docker**.

```sh
make build          # bin/folio
make test           # whole suite on SQLite: model, store, layout, diagrams, MCP over stdio and HTTP, viewer
make test-pg        # starts Postgres in Docker and runs the suite on both databases
make kitchen-sink   # every block type × every theme × dark/light → build/kitchen-sink/*.pdf
make lint
```

Run it locally against Docker Postgres: `cp .env.example .env`, then `make run-http`. The server is on `http://127.0.0.1:8080` with accounts on. Add `--no-auth` for quick local hacking without logging in (loopback only).

Some tests use `pdftotext` (poppler) when it's installed, to prove that the page numbers Folio reports match the actual PDF.

<details>
<summary><b>What the tests guard</b></summary>

- **Injection safety:** Typst code inside notes renders as plain text, and `javascript:` links are neutralised.
- **Layout truth:** every reported page number is checked against the real PDF text.
- **Pagination:** headings never sit alone at the bottom of a page, boxes don't split, and diagrams keep their captions (checked across every page position).
- **History:** undo, restore, version conflicts and snapshot pruning, on both SQLite and Postgres.
- **Ordering:** block order is byte-order sorted on Postgres (`COLLATE "C"`). Otherwise some server locales would silently scramble your notes.
- **Accounts:** an isolation matrix where user B attacks user A's notebook with **every** tool and viewer route and must always get "not found". The test fails if a new tool isn't covered, and it's proven to catch a removed ownership check. Also: tokens (missing/invalid/revoked), sessions, logout, CSRF, open redirects, identical errors for unknown emails, and tokens never appearing in logs.
- **HTTP:** a full Claude-style session over MCP/HTTP, surviving server restarts (stateless MCP), the 403 fix for proxies like Render, and the loopback guard.
- **Import:** keeps IDs and history, and running it twice changes nothing.

</details>

---

## 🗂 Project layout

```
cmd/folio/            the folio command (http, mcp, import, export, doctor, serve)
internal/model/       block types, validation, inline-markdown parser, ordering keys
internal/store/       SQLite + Postgres storage, versions/undo, import
internal/layout/      doc.json → Typst → PDF/PNG, page positions, warnings, cache
internal/diagram/     D2 → SVG with theme fonts, cache
internal/mcpserver/   the MCP tools and the authoring guide Claude reads
internal/httpserver/  HTTP: /mcp, /healthz, viewer
internal/viewer/      live web viewer (SSE)
templates/            the fixed Typst template and themes
fonts/                bundled OFL fonts (Inter, JetBrains Mono, Kalam)
Dockerfile            Folio + Typst image used on Render
```

The thinking behind the design lives in [`DECISIONS.md`](DECISIONS.md), with every pinned version and every trade-off. Build history and verification notes are in [`PROGRESS.md`](PROGRESS.md), and the original spec is [`docs/SPEC.md`](docs/SPEC.md).

---

## 🧭 Roadmap

- [x] Typeset notes, 14 block types, D2 diagrams, three themes, dark/light
- [x] Layout feedback, page images for Claude, undo/versions
- [x] Live viewer
- [x] Postgres + MCP over HTTP, deployed on Render, SQLite import
- [x] **Accounts:** sign up, log in, notebooks private to each user, personal access tokens to connect Claude ([plan](MULTIUSER_PLAN.md))
- [ ] **OAuth** so the Claude app's "Add custom connector" shows Folio's login page
- [ ] Anki export from Q&A cards, iPad-friendly fill-in PDFs, note styles (Cornell, flashcards)

---

## ❓ FAQ and troubleshooting

<details>
<summary><b>Claude's first tool call times out</b></summary>

You're probably on Render's Free instance, which sleeps after inactivity. The first request wakes it (~50s). Retry, or move to the Starter instance.

</details>

<details>
<summary><b>Claude says the connection needs authentication (401)</b></summary>

The token is missing, mistyped or revoked. Create a new one on the **Connect Claude** page, then remove and re-add the server with the new `--header "Authorization: Bearer …"`.

</details>

<details>
<summary><b>The MCP connection fails, but <code>/healthz</code> works</b></summary>

Make sure the URL ends with `/mcp` and the header is included. Behind a proxy (like Render), Folio relaxes the MCP library's localhost protection automatically when it listens publicly; otherwise proxied requests would get 403.

</details>

<details>
<summary><b>I forgot my password</b></summary>

There's no email yet, so the server admin runs `folio admin reset-password --email you@…` (against the production database URL) and passes on the new password.

</details>

<details>
<summary><b>The Render build fails, or PDFs don't render</b></summary>

Check that the service's runtime is **Docker**. With the native Go runtime there's no Typst, and Render would try to build the wrong path.

</details>

<details>
<summary><b>Why is block order a special case on Postgres?</b></summary>

Folio orders blocks with short keys like `1`, `2`, `1i`, so inserting in the middle never renumbers anything. Those keys must sort byte by byte. Many Postgres servers sort text by language rules instead, which would quietly scramble notes, so the column is declared `COLLATE "C"` and a test checks it.

</details>

<details>
<summary><b>Can I print the notes?</b></summary>

Yes. Ask Claude to switch the notebook to **light mode** first. Dark pages look great on screen but use a lot of ink.

</details>

---

<sub>Built with [Typst](https://typst.app) (Apache-2.0), [D2](https://d2lang.com) (MPL-2.0), the [MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk), [pgx](https://github.com/jackc/pgx) and [modernc SQLite](https://gitlab.com/cznic/sqlite). Fonts are OFL-licensed: see [`fonts/LICENSES.md`](fonts/LICENSES.md).</sub>
