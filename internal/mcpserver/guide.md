# Folio authoring guide

Folio is a typesetter. You write **blocks**; Folio flows them onto A4 pages. You never place anything with coordinates.

## Workflow
1. `create_notebook` (or `list_notebooks` to continue one).
2. Plan an outline: 1 H1 per major topic, H2 per subtopic.
3. `append_blocks` one section at a time (up to 50 blocks per call).
4. Read the result each write returns: `page_count`, the page each new block landed on, `last_page_fill_pct` and `warnings`.
5. `render_pages` on 1–2 pages to see them. Fix what looks wrong.
6. `export_pdf` when done.

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
{"type": "divider"}
{"type": "page_break"}
```
Callout variants: `note`, `tip`, `warning`, `definition`, `key_point`.

Limits: paragraph ≤ 2000 chars; lists ≤ 40 items and depth ≤ 3; code ≤ 80 lines; tables 2–6 columns, ≤ 25 rows; fill_in 1–15 lines.

`qa` with `hide_answer: true` prints the answer upside down at the bottom of the page.

## Style for good notes
- Short paragraphs: 2–4 sentences.
- One idea per callout. Use `definition` for terms, `key_point` for the thing to remember, `warning` for pitfalls.
- Prefer lists over long prose for enumerations, and tables for comparisons.
- End each section with 1–3 `qa` or `fill_in` blocks for active recall.
- Use `==highlight==` sparingly, for the single most important phrase in a section.
- Aim for dense but breathable pages: about one H2 section per half page.
