# Folio authoring guide

Folio is a typesetter. You write **blocks**; Folio flows them onto A4 pages. You never place anything with coordinates.

## Workflow
1. `create_notebook` (or `list_notebooks` to continue one).
2. Plan an outline: 1 H1 per major topic, H2 per subtopic.
3. `append_blocks` one section at a time (up to 50 blocks per call).
4. Read the result each write returns: `page_count`, the page each new block landed on, `last_page_fill_pct` and `warnings`.
5. `render_pages` on 1–2 pages to see them. Fix what looks wrong.
6. `export_pdf` when done.

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
{"type": "qa", "question": "Why is PUT idempotent?", "answer": "It sets the resource to the given state; repeating it changes nothing.", "hide_answer": true}
{"type": "fill_in", "prompt": "Explain statelessness in your own words.", "lines": 3}
{"type": "spacer", "size": "medium"}
{"type": "diagram", "source": "client -> api: request\napi -> db: query", "size": "half", "caption": "Request path"}
{"type": "two_column", "left": [{"type": "paragraph", "text": "**Pros**: fast lookups"}], "right": [{"type": "paragraph", "text": "**Cons**: rebalancing"}]}
{"type": "divider"}
{"type": "page_break"}
```
Callout variants: `note`, `tip`, `warning`, `definition`, `key_point`.

Limits: paragraph ≤ 2000 chars; lists ≤ 40 items and depth ≤ 3; code ≤ 80 lines; tables 2–6 columns, ≤ 25 rows; fill_in 1–15 lines.

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
- At most ~8 nodes per diagram. Split bigger ideas into two diagrams.
- A chain of 4+ nodes drawn top-down gets very tall: add `direction: right`, or use size `half`.
- Keep labels to 1–3 words; put explanations in the paragraph next to it.
- Use `preview_diagram` when unsure; it returns D2 errors with line numbers.
- In JSON, newlines inside `source` are `\n`.

## Style for good notes
- Short paragraphs: 2–4 sentences.
- One idea per callout. Use `definition` for terms, `key_point` for the thing to remember, `warning` for pitfalls.
- Prefer lists over long prose for enumerations, and tables for comparisons.
- End each section with 1–3 `qa` or `fill_in` blocks for active recall.
- Use `==highlight==` sparingly, for the single most important phrase in a section.
- Every process or architecture gets a diagram.
- Aim for dense but breathable pages: about one H2 section per half page.
- Don't add `page_break` unless a new H1 really should start a fresh page; Folio already keeps headings with their content and avoids splitting boxes.

## Checking your work
- Each write returns `layout.page_count`, `page_fill_pct` per page, `last_page_free_mm` (space left on the last page; a page holds 257mm of content), the page and height of each changed block, and `warnings`. Use these to budget pages: e.g. a 60mm diagram won't fit in 40mm of free space.
- Act on warnings: a mostly empty page, a near-empty last page (content spilled over), a heading alone at the bottom, an unreadable diagram. Warnings don't catch everything, so still render a page or two.
- Folio keeps short lists (under a quarter page), callouts, tables, code, Q&A, fill-ins and diagrams on one page when they fit, and keeps headings with what follows. You rarely need `page_break` to fix splits.
- `get_layout_info` gives per-page fill without images.
- `render_pages` (1–4 pages, default 80 ppi) shows the actual pages. Look at 1–2 pages after a big write.
- Edit with `update_block`, `insert_blocks`, `move_blocks`, `delete_blocks` using IDs from `get_outline`. `undo` reverts the last change.
- `get_viewer_url` gives a link where the user can watch pages update live.
