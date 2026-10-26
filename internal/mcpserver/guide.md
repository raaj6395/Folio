# Folio authoring guide

Folio is a typesetter. You write **blocks**; Folio flows them onto A4 pages. You never place anything with coordinates.

## Workflow
1. `create_notebook` (or `list_notebooks` to continue one). New notebooks use density `comfortable`. If the note is a kind listed under **Templates** at the end, call `get_authoring_guide` with `template` for its blueprint and pass the same `template` to `create_notebook`. If the user names a folder or subject ("in my Networks folder"), pass `folder`; call `list_folders` first if unsure and reuse an existing folder rather than making a near-duplicate.
2. Plan an outline: one page-sized H1 per major topic, H2 per subtopic.
3. `append_blocks` a whole section at a time (up to 50 blocks per call).
4. Read what each write returns: `page_count`, the page each new block landed on, `page_fill_pct`, `warnings` and `layout_hints`. Fix issues before moving on.
5. Before finishing: `check_layout`, fix what it reports, then `render_pages` once on the changed pages (at most 2).
6. `export_pdf` when the user wants the file.

The **Layout rules** at the end of this guide say how to arrange pages. The user's `notebook_guidelines` override them.

## Dark and light mode
Every theme comes in dark (the default) and light. Create in light mode only when the user asks for it: `create_notebook` with `"mode": "light"`. Switch later with `set_theme` (`"mode": "light"` or `"dark"`); the content doesn't change.

## Inline text
Text fields accept only: `**bold**`, `*italic*`, `` `code` ``, `[text](url)`, `==highlight==`. Anything else is printed literally, so you never need to escape characters.

## Block types (one example each)
```json
{"type": "heading", "level": 1, "text": "REST APIs"}
{"type": "paragraph", "text": "REST maps **resources** to URLs and uses HTTP verbs to act on them."}
{"type": "bullets", "items": [{"text": "Stateless", "children": [{"text": "No session on the server"}]}, {"text": "Cacheable"}]}
{"type": "numbered", "items": [{"text": "Client sends request"}, {"text": "Server returns representation"}]}
{"type": "callout", "variant": "definition", "title": "Idempotent", "body": ["Doing it twice has the same effect as doing it once."]}
{"type": "code", "language": "http", "code": "GET /users/42 HTTP/1.1\nAccept: application/json", "caption": "A simple GET"}
{"type": "table", "columns": ["Method", "Idempotent", "Safe"], "rows": [["GET", "yes", "yes"], ["POST", "no", "no"]], "caption": "HTTP method properties"}
{"type": "table", "columns": ["Plan", "Price", "Seats"], "rows": [["Free", "$0", "1"], ["Team", "$12", "10"]], "align": ["left", "right", "center"]}
{"type": "qa", "question": "Why is PUT idempotent?", "answer": "It sets the resource to the given state; repeating it changes nothing.", "hide_answer": true}
{"type": "fill_in", "prompt": "Explain statelessness in your own words.", "lines": 3}
{"type": "spacer", "size": "medium"}
{"type": "diagram", "source": "client -> api: request\napi -> db: query", "size": "half", "caption": "Request path"}
{"type": "two_column", "left": [{"type": "heading", "level": 3, "text": "Pros"}, {"type": "paragraph", "text": "Fast lookups"}], "right": [{"type": "heading", "level": 3, "text": "Cons"}, {"type": "paragraph", "text": "Rebalancing"}]}
{"type": "two_column", "left": [{"type": "diagram", "source": "direction: right\nclient -> api -> db", "size": "half"}], "right": [{"type": "numbered", "items": [{"text": "Client sends a request"}, {"text": "API queries the DB"}]}]}
{"type": "divider"}
{"type": "page_break"}
```
Callout variants: `note`, `tip`, `warning`, `definition`, `key_point`.

Limits: paragraph ≤ 2000 chars; lists ≤ 40 items and depth ≤ 3; code ≤ 80 lines; tables 2–6 columns, ≤ 25 rows; fill_in 1–15 lines.

Tables align text columns left and number columns right by default; `align` (one of left/center/right per column) overrides that. Inside `two_column` you may use level-3 headings, paragraphs, lists, callouts, code, `half`/`small` diagrams and Q&A.

`qa` with `hide_answer: true` prints the answer upside down at the bottom of the page.

## Diagrams (D2)
Describe the graph; Folio lays it out. Basics:
- Node: `api` or with a label `api: API gateway`. Shapes: `db: Users DB {shape: cylinder}`, also `queue`, `person`, `cloud`, `hexagon`, `oval`, `diamond`.
- Edge: `client -> api: HTTPS` (also `<-`, `<->`, `--`).
- Group: `backend: { api; worker }`, then refer to `backend.api`.
- Layout direction: `direction: right` (default is down).

Flow diagram:
```
direction: right
client: Browser {shape: person}
lb: Load balancer
api: API
db: Postgres {shape: cylinder}
client -> lb: HTTPS
lb -> api
api -> db: SQL
```

Sequence diagram:
```
shape: sequence_diagram
client: Client
server: Server
client -> server: POST /orders
server -> client: 201 Created
```

Tips:
- At most ~6 nodes per diagram. Split bigger ideas into two diagrams.
- A chain of 4+ nodes drawn top-down gets very tall: add `direction: right`, or use size `half` inside a `two_column`.
- Keep labels to 1–3 words; put explanations in the paragraph next to it.
- Use `preview_diagram` when unsure; it returns D2 errors with line numbers.
- In JSON, newlines inside `source` are `\n`.

## Style for good notes
- One idea per callout. Use `definition` for terms, `key_point` for the thing to remember, `warning` for pitfalls.
- Use `==highlight==` sparingly, for the single most important phrase in a section.
- Every process or architecture gets a diagram.
- Page structure, spacing, arrangement and density: see **Layout rules** below.

## Checking your work
- Each write returns `layout.page_count`, `page_fill_pct` per page, `last_page_free_mm` (space left on the last page; a page holds 257mm of content), the page and height of each changed block, and `warnings`. Use these to budget pages: e.g. a 60mm diagram won't fit in 40mm of free space.
- Act on `warnings` and `layout_hints`: each hint names the page, the check, the block IDs and a concrete fix.
- `check_layout` scores the notebook (0–100) and lists every issue per page; call it before saying you're done.
- Folio keeps short lists (under a quarter page), callouts, tables, code, Q&A, fill-ins and diagrams on one page when they fit, and keeps headings with what follows. Use `page_break` to start H1 sections on a new page, not to patch splits.
- `get_layout_info` gives per-page fill and block positions without images.
- `render_pages` (1–4 pages, default 80 ppi) shows the actual pages. Render the changed pages once at the end.
- Edit with `update_block`, `insert_blocks`, `move_blocks`, `delete_blocks` using IDs from `get_outline`. `undo` reverts the last change.
- `get_viewer_url` gives a link where the user can watch pages update live.
