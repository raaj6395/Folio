# Folio: AI-Authored A4 Notes via MCP

## Build Specification and Instructions for Claude Code

> **Read this entire file before doing anything.** It is the single source of truth for what to build, how to build it, and how to record progress. If anything here is unclear or seems wrong, stop and ask. Do not guess.

---

## 0. Instructions to Claude Code (read first, follow always)

### 0.1 Session start ritual (every session, no exceptions)
1. Read `docs/SPEC.md` (this file, copied into the repo in task M0.1).
2. Read `PROGRESS.md` completely.
3. Read `DECISIONS.md`.
4. Run the full test suite (`go test ./...`) and note the result.
5. In one short paragraph, tell the user:
   - where the project stands
   - what you will work on next
   - any blockers or failing tests you found
6. Only then start work.

### 0.2 Working rules
- **Work in small, verifiable tasks.** Each task in Section 14 has an ID (e.g. `M1.3`). Work on one task at a time.
- **A task is "done" only when** its code is written, its tests pass, and its acceptance check has actually been run and observed to pass. Never mark a task done on the assumption that it works.
- **Never guess at the cause of a failure.** When something fails:
  1. Reproduce it.
  2. Read the actual error output.
  3. Isolate it step by step until you find the real cause.
  4. Then fix it.

  If you cannot find or fix the cause, say so clearly, record it under "Blockers" in `PROGRESS.md`, and ask the user. Do not present a hypothesis as if it were confirmed.
- **Verify library APIs; don't rely on memory.** Library APIs change. This applies especially to the MCP Go SDK, the Typst CLI flags and query syntax, and the D2 Go API. Before using one, check the current docs, the module source (`go doc`), or `--help` output. Pin versions in `go.mod` and record them in `DECISIONS.md`.
- **Don't silently change the spec.** If you need to deviate (a library doesn't support something, a better approach exists):
  1. Write the deviation and its reason in `DECISIONS.md`.
  2. If it changes user-visible behaviour or the tool interface, ask first.
- **Keep dependencies minimal.** Every new dependency needs a one-line justification in `DECISIONS.md`.
- **Commit after every completed task.** Use the format `M1.3: add update_block tool`. Do not commit with failing tests unless the commit message says `WIP` and `PROGRESS.md` explains why.
- **Update `PROGRESS.md` continuously.** Update it after each task, not just at session end (see Section 15). If the session dies mid-task, the next session must be able to resume from `PROGRESS.md` alone.

### 0.3 Session end ritual
1. Make sure the tree is either committed or clearly marked WIP.
2. Update `PROGRESS.md`:
   - current status
   - a session log entry
   - next steps
   - blockers
3. Tell the user in a short paragraph:
   - what was completed
   - what's next
   - anything they need to do (install something, make a decision, test something manually)

---

## 1. Product Overview

**Folio** is a local application that lets an AI model (Claude Code, or any MCP client) author good-looking notes on a sequence of **A4 pages**. The user says "make notes on REST APIs", and the AI produces structured pages that look like a real study notebook:
- headings and explanations
- boxed definitions, callouts and warnings
- tables
- code snippets
- diagrams

The AI reads the current state of a notebook, adds or edits content, renders pages to see how they look, and fixes layout problems itself.

### 1.1 What the user gets
- Notebooks stored locally, each a sequence of A4 pages.
- A PDF export of any notebook.
- A live web viewer that shows pages updating as the AI writes.
- Themes: a clean typeset look, and a handwritten-notebook look (handwriting font, ruled or dotted paper).
- Revision-friendly blocks: Q&A cards and "fill in yourself" blank spaces, so notes can be used for active recall and not just passive reading.

### 1.2 Primary use cases
1. "Make 2 pages of revision notes on REST APIs."
2. "Add a diagram of the request lifecycle after the HTTP methods section."
3. "Turn what we discussed in this session into notes."
4. "Make a cheat sheet for consistent hashing with blanks I can fill in."
5. "Page 3 looks cramped. Fix it."

---

## 2. Core Design Principles (non-negotiable)

1. **The AI is the author; Folio is the typesetter.** The AI never supplies x/y coordinates in the main (flow) mode. It supplies *structured semantic content* (blocks). Folio's layout engine measures text, flows content across A4 pages, handles page breaks and places diagrams.
2. **Diagrams are described, not drawn.** The AI writes a graph description (D2 language). Folio lays it out automatically.
3. **The document model is the source of truth.** Pages, PDFs and PNGs are derived artifacts, regenerated from the model.
4. **Every block has a stable ID.** This lets the AI make small, targeted edits ("update block `b_7f2a`") instead of rewriting whole documents.
5. **The AI must be able to see its output.** A render tool returns page images so the model can inspect and correct layout. This feedback loop is essential to quality.
6. **Writes return cheap feedback.** Every mutating tool returns:
   - a compact layout summary (page count, where the changed blocks landed)
   - warnings (overflow, orphaned headings, oversized diagrams)

   The AI should rarely need to request images just to know the basics.
7. **Token efficiency matters.** Outlines are compact. Images are rendered only for requested pages, at modest resolution by default.
8. **Safety of content.** User/AI text must never be interpreted as Typst code (no injection). See Section 6.2.

---

## 3. Tech Stack

| Concern | Choice | Notes |
|---|---|---|
| Language | **Go** (latest stable) | Single static binary. |
| MCP | **Official MCP Go SDK** (`github.com/modelcontextprotocol/go-sdk`) | Verify the current API and version before use. stdio transport. |
| Storage | **SQLite** in WAL mode | Use a pure-Go driver (e.g. `modernc.org/sqlite`) to avoid CGO, unless there is a good reason. Record the choice. |
| Layout / PDF | **Typst CLI** (external binary) | Compiles to PDF and PNG. Must be installed on the machine; detect it at startup and give a clear error if missing. |
| Diagrams | **D2** (Go library, `oss.terrastruct.com/d2`) | Renders to SVG in-process. Supports sketch mode for the handwritten theme. Verify the current API. |
| Viewer | Minimal web app served by Go (`net/http`) | Plain HTML/JS, no framework needed. Shows per-page PNG/SVG images. Live updates via Server-Sent Events. |
| Fonts | Bundled OFL-licensed fonts | Clean theme: e.g. Inter + JetBrains Mono. Handwritten theme: e.g. Caveat or Patrick Hand + a mono font. Verify licenses and record them in `fonts/LICENSES.md`. |

If any choice proves unworkable, follow the deviation rule in Section 0.2.

---

## 4. Process Model

One binary, `folio`, with subcommands:

- **`folio mcp`**: the MCP server over stdio. Claude Code spawns this process. It reads and writes the shared SQLite DB and renders via Typst.
- **`folio serve [--port 7420]`**: long-running local web viewer. It reads the same DB. It detects changes by watching a `notebooks.version` counter (poll every ~500ms, or use file-change notification on the DB) and pushes SSE events to open browser tabs.
- **`folio export <notebook-id> [-o file.pdf]`**: CLI export, mainly for testing.
- **`folio doctor`**: checks that Typst is installed (and which version), the fonts are present, the DB is reachable and the data dir is writable. Prints clear pass/fail lines.

Both `mcp` and `serve` may run at the same time. SQLite WAL mode plus short transactions make this safe. All writes go through the MCP process. The viewer is read-only in v1.

### 4.1 Data directory
Default: `~/.folio/`. It can be overridden with the `FOLIO_HOME` env var.
```
~/.folio/
  folio.db                 # SQLite
  build/<notebook-id>/     # generated .typ, doc.json, diagrams/*.svg, pages/*.png, out.pdf
  exports/                 # user-requested PDF exports
  logs/folio.log           # structured logs (never write logs to stdout in mcp mode!)
```
**Critical:** in `mcp` mode, stdout is the MCP protocol channel. All logging must go to stderr or the log file.

---

## 5. Document Model

### 5.1 Entities
- **Notebook**: fields:
  - `id`
  - `title`
  - `theme`
  - `paper` (`plain` | `ruled` | `dotted` | `grid`)
  - `created_at`, `updated_at`
  - `version` (integer, incremented on every mutation)
- **Block**: fields:
  - `id` (format: `b_` + 8 random base32 chars, unique within the notebook)
  - `notebook_id`
  - `position`: ordering key. Use fractional indexing or a float/string order key, so inserts don't renumber everything. Record the choice.
  - `type`
  - `data`: JSON, schema depends on type
  - `created_at`, `updated_at`
- **Version snapshot**: on every mutating call, store:
  - `notebook_id`
  - `version`
  - full ordered block list JSON
  - `tool_name`
  - `timestamp`
  - `summary`: a short description of the change

  This enables undo/restore. Keep the last 200 snapshots per notebook and prune older ones.

### 5.2 Inline text format
Text fields support a **restricted inline markdown subset** only:
- `**bold**`
- `*italic*`
- `` `inline code` ``
- `[link text](url)`
- `==highlight==`

Nothing else; no block-level markdown inside text fields. Go parses this into a span list and validates it. Unclosed markers are treated as literal text (never error on them).

### 5.3 Block types

All blocks share `id` and `type`. Type-specific `data` fields:

| Type | Fields | Rules |
|---|---|---|
| `heading` | `level` (1–3), `text` | Level 1 = section title. A heading must never be the last thing on a page (keep-with-next). |
| `paragraph` | `text` | Max 2,000 chars. |
| `bullets` | `items`: list of `{text, children?}` | Max nesting depth 3. Max 40 items total. |
| `numbered` | same as `bullets` | |
| `callout` | `variant` (`note` \| `tip` \| `warning` \| `definition` \| `key_point`), `title?`, `body` (list of paragraphs, inline text) | Rendered as a bordered/tinted box with a variant-specific accent. Should not split across pages if under ~40% of a page tall. |
| `code` | `language`, `code`, `caption?` | Monospace with syntax highlighting (Typst raw blocks support this). Max 80 lines; longer code returns an error suggesting the AI split it. |
| `table` | `columns` (list of header strings), `rows` (list of lists of inline text), `caption?` | 2–6 columns, max 25 rows. Every row must have exactly `len(columns)` cells. |
| `diagram` | `source` (D2), `caption?`, `size` (`full` \| `half` \| `small`) | Validated by compiling D2. D2 compile errors are returned verbatim to the AI so it can fix them. |
| `qa` | `question`, `answer`, `hide_answer` (bool) | Revision card. If `hide_answer`, render the answer upside down at the bottom of the page, or render an empty answer area (pick one, record it). |
| `fill_in` | `prompt`, `lines` (1–15) | Prompt text followed by N blank ruled lines for the user to write in by hand. |
| `two_column` | `left` (list of blocks), `right` (list of blocks) | Nested blocks may not themselves be `two_column`. Allowed nested types: paragraph, bullets, numbered, callout, code, diagram (small/half), qa. **Milestone 2.** |
| `divider` | none | Horizontal rule. |
| `page_break` | none | Forces a new page. |
| `spacer` | `size` (`small` \| `medium` \| `large`) | Vertical space. |

Nested blocks inside `two_column` also get IDs and can be addressed directly by edit tools.

### 5.4 Validation
- Validate every incoming block fully before writing anything. A call with 10 blocks where 1 is invalid writes nothing.
- Errors are returned to the AI and must be **specific and actionable**. Example: `block[3] (table): row 2 has 4 cells, expected 3 (columns: Method, Idempotent, Safe)`.
- Unknown fields produce an error, not silent dropping. This helps the AI learn the schema.

---

## 6. Layout Pipeline

### 6.1 Overview
```
blocks (SQLite)
  → doc.json (normalized blocks + parsed inline spans + theme config)
  → diagrams rendered to SVG files (D2)
  → main.typ (a FIXED template that reads doc.json)
  → typst compile → out.pdf
  → typst compile --format png → pages/page-{n}.png (on demand)
  → typst query → block positions (layout info)
```

### 6.2 No string-built Typst (injection safety)
Do **not** generate Typst markup by concatenating user text. Instead:
- Write a fixed, version-controlled Typst template, `templates/main.typ`. It loads `doc.json` with Typst's `json()` function and renders each block by type.
- All user/AI text reaches Typst only as JSON string data, rendered as text. It is never evaluated as markup.
- Inline spans (bold/italic/code/link/highlight) are pre-parsed in Go and passed as structured span arrays. The template renders them.

This makes the AI unable to break the layout or inject Typst code, however strange its text is.

### 6.3 Themes
Each theme is defined in a `templates/themes/<name>.typ` file (or as a JSON config consumed by `main.typ`). It sets:
- page size A4 (210 × 297 mm) and margins (default 18mm sides, 20mm top/bottom)
- body, heading and mono fonts, plus font sizes
- colors for headings, callout variants, code background and table header
- paper background: plain, ruled lines, dot grid or square grid (drawn by Typst as a page background), optionally with a red margin line for the notebook look
- header/footer: notebook title (small, top) and page number (bottom)
- the D2 style to use for diagrams: theme ID, and sketch mode on/off

Themes required:
- **`clean`** (default): sans body, crisp boxes, plain paper.
- **`notebook`**: handwriting font, ruled paper, red margin line, D2 sketch mode, slightly irregular callout borders if feasible.
- **`dotgrid`**: clean fonts on dot-grid paper (bullet-journal look).

### 6.4 Pagination rules
These are implemented in the template.
- Headings keep with the next block (no heading at the bottom of a page).
- Callouts, tables under ~40% of a page, diagrams, `qa` and `fill_in` blocks do not split across pages.
- Long tables may split, repeating the header row.
- Code blocks may split only if longer than ~40% of a page.

### 6.5 Layout info extraction
Folio must report where each block landed, without the AI requesting images. Approach (verify against current Typst docs):
- In the template, at the start of each block, emit a `metadata` element carrying the block ID, labelled `<blockpos>`. Use a `context` expression with `here().position()` so the page number and y-position are captured.
- Emit a final marker after the last block, to measure how full the last page is.
- Run `typst query main.typ "<blockpos>" --field value` (check exact syntax) to get JSON. Parse it to build:
  - per-block page number and y-position (mm)
  - total page count
  - last-page fill percentage
- Also detect and return **warnings**:
  - a diagram rendered smaller than a readable threshold
  - a page that is more than 95% whitespace in the middle of the doc (likely a bad break)
  - a block that forced an otherwise mostly-empty page

### 6.6 Build caching and performance
- Rebuild only on change: key the build on notebook `version` and theme.
- Cache D2 SVGs by hash of (source + diagram style). Re-render only changed diagrams.
- PNG rendering is on demand, per page, at a requested PPI. Default 80 PPI for AI inspection; viewer uses ~110. Cache by (version, page, ppi).
- Target: a 10-page notebook rebuilds (PDF + layout info) in under 1.5 s on a laptop. Measure and record actual numbers in `PROGRESS.md`.
- Typst invocations need a timeout (e.g. 20 s) and captured stderr. On a compile error, return the error to the caller. A compile error from the fixed template is a bug in Folio, not the AI's fault, so log it loudly.

---

## 7. Diagram Pipeline

- The AI supplies D2 source in `diagram.source`.
- Folio compiles it with the D2 Go library to SVG, using the theme's D2 style (theme ID, sketch mode, font matching the notes where possible).
- Validation happens at write time. If D2 fails to compile, the write is rejected and D2's error message is returned verbatim, with the line number.
- Size mapping:
  - `full`: up to the full text width
  - `half`: 50% of the text width, centered
  - `small`: 35% of the text width

  Height is constrained so that no diagram exceeds ~60% of the page height. Scale down proportionally and emit a warning if scaling would make text unreadably small.
- Also expose a `preview_diagram` tool, so the AI can check a diagram before inserting it.

---

## 8. MCP Server Specification

### 8.1 General
- Transport: stdio.
- Server name: `folio`. Server instructions (sent at initialize) must briefly explain:
  - the flow-layout model ("you write blocks, Folio lays them out; never think in coordinates")
  - that block IDs are stable
  - the recommended workflow: outline → write → check layout → render a page or two → fix
- Expose an MCP **resource** (and also a tool, `get_authoring_guide`, for clients that don't read resources). It returns a concise authoring guide covering:
  - all block types with one example each
  - D2 syntax basics with 2 examples (flow diagram, sequence diagram)
  - style guidance for good notes:
    - short paragraphs
    - one idea per callout
    - prefer tables for comparisons
    - a diagram for every process or architecture
    - end sections with 1–3 `qa` or `fill_in` blocks for active recall
    - aim for dense but breathable pages
- All tool inputs are described with JSON Schema, including descriptions on every field. These descriptions are what the AI sees, so write them carefully.

### 8.2 Tools

Every mutating tool:
- accepts optional `expected_version` (int). If it is given and doesn't match the current notebook version, reject with a conflict error that includes the current version. This is optimistic concurrency.
- returns `{ version, changed_block_ids, layout: {page_count, blocks: [{id, page}] for changed blocks, last_page_fill_pct}, warnings: [...] }`.
- creates a version snapshot.

| Tool | Input | Output | Notes |
|---|---|---|---|
| `list_notebooks` | none | `[{id, title, theme, page_count, block_count, updated_at}]` | |
| `create_notebook` | `title`, `theme?`, `paper?` | notebook info | |
| `rename_notebook` | `notebook_id`, `title` | notebook info | |
| `delete_notebook` | `notebook_id`, `confirm: true` | ok | Soft delete (move to trash table). |
| `get_outline` | `notebook_id` | List of `{id, type, page, summary}`. Summary is ≤ 80 chars: heading text, first words of a paragraph, "table: 3×5 Method/Idempotent/Safe", "diagram: 6 nodes". Also page_count, version, theme. | The main "read current state" tool. Must stay compact even for 30-page notebooks. |
| `read_blocks` | `notebook_id`, and either `block_ids` or `{from_block_id, count}` or `{page}` | Full block data | |
| `append_blocks` | `notebook_id`, `blocks[]` (max 50) | standard mutation result, with the new IDs in order | |
| `insert_blocks` | `notebook_id`, `after_block_id` (or `"start"`), `blocks[]` | standard | |
| `update_block` | `notebook_id`, `block_id`, `block` (full replacement of `data`; type may change) | standard | |
| `delete_blocks` | `notebook_id`, `block_ids[]` | standard | |
| `move_blocks` | `notebook_id`, `block_ids[]`, `after_block_id` | standard | Keeps relative order of moved blocks. |
| `get_layout_info` | `notebook_id` | Full per-block page/y info, page fill per page, warnings | |
| `render_pages` | `notebook_id`, `pages[]` (max 4 per call), `ppi?` (default 80, max 150) | MCP image content (PNG) for each page, plus text noting the page numbers | The visual feedback loop. Pages are 1-indexed. |
| `preview_diagram` | `source`, `theme?` | PNG image of the diagram or the D2 error | Does not modify any notebook. |
| `set_theme` | `notebook_id`, `theme`, `paper?` | standard | |
| `list_themes` | none | Themes with a one-line description | |
| `export_pdf` | `notebook_id`, `path?` | Absolute path of the PDF | Default path `~/.folio/exports/<slug>-<date>.pdf`. |
| `list_versions` | `notebook_id`, `limit?` | `[{version, tool_name, summary, timestamp}]` | |
| `undo` | `notebook_id` | standard (restores the previous snapshot) | |
| `restore_version` | `notebook_id`, `version` | standard | |
| `get_authoring_guide` | none | Guide text | |
| `get_viewer_url` | `notebook_id` | `http://localhost:7420/n/<id>` and whether the viewer appears to be running | |

### 8.3 Error format
Errors are returned as MCP tool errors (`isError: true`) with a message that is:
- **specific**: what is wrong and where
- **actionable**: how to fix it
- **bounded**: never dump a full stack trace to the AI; log those to the file instead

---

## 9. Viewer (`folio serve`)

- `GET /`: list of notebooks.
- `GET /n/<id>`: page view. A vertical scroll of A4 pages with a shadow, like a PDF reader. Images come from `GET /n/<id>/page/<n>.png?v=<version>`.
- `GET /n/<id>/events`: SSE stream that emits `{version, page_count}` whenever the notebook changes. The page reloads only the changed or added page images, keeping scroll position.
- `GET /n/<id>/pdf`: download the PDF.
- A small toolbar with:
  - notebook title
  - version number
  - "last updated X seconds ago"
  - theme name
  - download PDF button
- Highlight recently changed pages briefly (e.g. a 2-second outline flash). This makes watching the AI write satisfying.
- Bind to `127.0.0.1` only.

---

## 10. Claude Code Integration

- Document in `README.md` how to register the server with Claude Code (verify current syntax via `claude mcp add --help`), e.g. registering the `folio` binary with the `mcp` argument at user scope.
- Provide an example `CLAUDE.md` snippet the user can add to their other projects, describing when to use Folio and the recommended workflow.
- Provide 3 example prompts in the README (REST APIs notes, a consistent hashing cheat sheet with blanks, a "session to notes" request).

---

## 11. Repository Structure

```
folio/
  cmd/folio/main.go            # subcommand dispatch
  internal/model/              # block types, validation, inline parser, ordering keys
  internal/store/              # SQLite schema, migrations, queries, snapshots
  internal/layout/             # doc.json generation, typst invocation, query parsing, caching
  internal/diagram/            # D2 rendering + cache
  internal/mcpserver/          # tool definitions, handlers, guide resource
  internal/viewer/             # HTTP server, SSE, static assets
  internal/doctor/             # environment checks
  templates/main.typ           # fixed Typst template
  templates/themes/*.typ
  fonts/                       # bundled fonts + LICENSES.md
  testdata/                    # fixture notebooks, golden files
  docs/SPEC.md                 # this file
  PROGRESS.md
  DECISIONS.md
  README.md
  Makefile                     # build, test, lint, run-viewer, kitchen-sink
```

---

## 12. Testing Strategy

- **Unit tests:**
  - inline markdown parser (including malformed input)
  - every block validator, with valid and invalid cases and checks on error message content
  - ordering keys
  - snapshot/undo logic
- **Template tests:** for each block type and each theme, compile a fixture notebook. Assert that Typst succeeds, the PDF exists and the page count is as expected.
- **Layout info tests:** a fixture with a forced `page_break` must report the correct page for blocks on either side.
- **MCP integration tests:** use the SDK's in-memory or pipe transport to drive the server like a client:
  1. create a notebook
  2. append blocks
  3. get the outline
  4. update and delete
  5. undo
  6. render a page and check that an image is returned
- **Injection test:** text containing Typst syntax (`#`, `$`, `[`, `]`, `\`, `#import`, `#eval(...)`, `<label>`, `@ref`) renders literally and compiles fine.
- **Kitchen sink:** `make kitchen-sink` builds `testdata/kitchen_sink.json` (every block type, long content, edge cases) into a PDF in all three themes. A human checks these PDFs visually at each milestone. Record in `PROGRESS.md` when it was last checked and what issues were seen.
- **Performance check:** time a 10-page and a 30-page rebuild and record the numbers.

---

## 13. Out of Scope (for now)

- Cloud sync, accounts, multi-user collaboration.
- Editing in the viewer (read-only in v1).
- Importing existing PDFs or handwriting.
- Free-form placement (see Milestone 4, which is optional).
- Image blocks from the web (local image files may be added later).
- Mobile apps.

---

## 14. Milestones and Tasks

Work in order. Each milestone ends with an **acceptance check** that must be run and pass before moving on.

### Milestone 0: Foundations
- **M0.1** Initialize the Go module and repo structure (Section 11). Copy this spec to `docs/SPEC.md`. Create `PROGRESS.md` (template in Section 15) and `DECISIONS.md`.
- **M0.2** Implement `folio doctor`: Typst presence and version, data dir writable.
- **M0.3** Verify and record the versions and APIs of the MCP Go SDK, D2 and Typst in `DECISIONS.md`. Include short notes on the exact APIs and commands you will use (e.g. how tools are registered, how image content is returned, D2 compile-to-SVG call, typst query syntax).
- **M0.4** Spike: a minimal MCP server with one `ping` tool, registered in Claude Code and called successfully. Document the registration steps in the README.

**Acceptance:**
- `folio doctor` passes.
- Claude Code can call `ping`.

### Milestone 1: Markdown-ish to PDF loop (thin vertical slice)
- **M1.1** SQLite schema and migrations: notebooks, blocks, snapshots.
- **M1.2** Model and validation for `heading`, `paragraph`, `bullets`, `numbered`, `code`, `callout`, `divider`, `page_break`. Plus the inline parser.
- **M1.3** `templates/main.typ` rendering those types from `doc.json`, `clean` theme only.
- **M1.4** Layout package: generate doc.json, compile the PDF, render the PNG for a page, with timeout and error capture.
- **M1.5** MCP tools:
  - `create_notebook`
  - `list_notebooks`
  - `append_blocks`
  - `get_outline` (without page numbers yet)
  - `render_pages`
  - `export_pdf`
  - `get_authoring_guide` (first draft)
- **M1.6** Injection-safety test passes.

**Acceptance:** from Claude Code, the prompt "make 2 pages of notes on REST APIs using folio" produces a sensible PDF. Claude can render a page and see it. Record observations in `PROGRESS.md`.

### Milestone 2: Editing and layout awareness
- **M2.1** Stable IDs and fractional ordering. Implement:
  - `insert_blocks`
  - `update_block`
  - `delete_blocks`
  - `move_blocks`
  - `read_blocks`
- **M2.2** Version snapshots: `list_versions`, `undo`, `restore_version`, `expected_version` conflicts.
- **M2.3** Layout info via typst query (Section 6.5). Add page numbers to `get_outline` and the mutation results. Add `get_layout_info`.
- **M2.4** Pagination rules (Section 6.4) and warnings.
- **M2.5** Add `table`, `qa`, `fill_in` and `spacer` blocks.
- **M2.6** Build caching (Section 6.6) and a performance measurement.

**Acceptance:**
- Claude can say "add a comparison table of HTTP methods after the methods section", and the table lands in the right place.
- Undo restores the previous state.
- Page numbers in the outline match the actual PDF.

### Milestone 3: Diagrams, themes, viewer
- **M3.1** D2 integration: the `diagram` block, validation with error passthrough, SVG cache, size mapping, `preview_diagram`.
- **M3.2** `notebook` and `dotgrid` themes, paper backgrounds, `set_theme`, `list_themes`, bundled fonts with licenses.
- **M3.3** `two_column` block.
- **M3.4** `folio serve`: page view, SSE live updates, changed-page highlight, PDF download. Add the `get_viewer_url` tool.
- **M3.5** Finalize the authoring guide with D2 examples and style guidance. Tune it by actually generating 3 notebooks (REST APIs, consistent hashing, Kafka basics) and revising the guide based on what went wrong.
- **M3.6** README polish:
  - setup
  - Claude Code registration
  - example prompts
  - a `CLAUDE.md` snippet
  - screenshots

**Acceptance:**
- The kitchen sink renders well in all 3 themes (human-checked).
- The viewer updates live while Claude writes.
- A notes session on a new topic needs no manual fixes to be readable.

### Milestone 4 (optional, ask the user before starting): Free-form layer
- A per-page overlay layer of free-positioned elements: sticky notes, arrows between blocks, margin notes.
- A `get_free_regions(page)` tool that computes empty rectangles (in mm) from layout info.
- Placement tools that reject overlaps server-side.
- Plan this in `DECISIONS.md` and get the user's approval before building.

---

## 15. Progress Tracking Protocol

### 15.1 Files
- **`PROGRESS.md`**: current state, task checklist, session log, blockers, next steps. Always kept current.
- **`DECISIONS.md`**: append-only log of decisions and deviations. Each entry has:
  - date
  - decision
  - reason
  - alternatives considered
  - whether the user approved it (if it affected behaviour or interface)

### 15.2 When to update `PROGRESS.md`
- When starting a task: mark it `[~]` (in progress).
- When finishing a task: mark it `[x]`, with the commit hash and a one-line note of how it was verified.
- When hitting a blocker: add it under Blockers immediately, with what was tried and the exact error.
- At session end: add a session log entry.

### 15.3 `PROGRESS.md` template (create exactly this structure)

```markdown
# Folio — Progress

**Last updated:** YYYY-MM-DD HH:MM
**Current milestone:** M1
**Current task:** M1.3 — Typst template for core blocks
**Overall status:** On track | Blocked | Needs user input

## Status summary
(2–4 sentences: what works right now end-to-end, what doesn't yet.)

## How to verify the current state
(Exact commands to build, test, and try the latest working feature.)

## Task checklist
Legend: [ ] todo · [~] in progress · [x] done · [!] blocked

### Milestone 0 — Foundations
- [x] M0.1 Repo init — `a1b2c3d` — verified: `go build ./...` ok
- [~] M0.2 folio doctor
...

(One line per task from Section 14 of the spec, all milestones listed from the start.)

## Blockers
- (none) | M2.3: typst query returns empty for <blockpos>. Tried: X, Y. Exact error: `...`. Next idea: Z. Needs user? yes/no

## Open questions for the user
- ...

## Known issues / tech debt
- ...

## Performance measurements
| Date | Notebook | Pages | Rebuild time | Notes |

## Kitchen sink visual check
| Date | Theme | Result | Issues seen |

## Session log
### YYYY-MM-DD — Session N
- Did: ...
- Verified: ...
- Problems: ...
- Next: ...
```

### 15.4 `DECISIONS.md` entry template

```markdown
## YYYY-MM-DD — <short title>
- **Decision:** ...
- **Why:** ...
- **Alternatives considered:** ...
- **Affects interface/behaviour?** yes/no — **User approved?** yes/no/n.a.
```

---

## 16. Definition of Done (whole project, Milestones 0–3)

- [ ] `folio doctor` passes on a fresh machine following the README.
- [ ] Claude Code can create, read, edit, render and export notebooks using only the MCP tools.
- [ ] Every block type works in all three themes. The kitchen sink is visually checked.
- [ ] No Typst injection is possible (test passes).
- [ ] Undo/restore works. Concurrent edit conflicts are detected.
- [ ] The viewer shows live updates.
- [ ] 10-page rebuild under ~1.5 s (or the actual number is recorded with an explanation).
- [ ] `PROGRESS.md` is complete, and `DECISIONS.md` records every pinned version and deviation.
- [ ] README lets someone else set it up from scratch.
