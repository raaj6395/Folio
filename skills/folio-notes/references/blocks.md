# Folio block reference

Send blocks as JSON objects. Do not include an `id` on new blocks; Folio assigns `b_xxxxxxxx` and returns it. Unknown fields are rejected, and if one block in a batch is invalid, nothing is written.

## Limits

| Block | Limit |
|---|---|
| paragraph `text` | 2000 characters |
| bullets / numbered | 40 items total, nesting depth 3 |
| code | 80 lines |
| table | 2–6 columns, at most 25 rows, every row the same length as `columns` |
| fill_in `lines` | 1–15 |
| append / insert | 50 blocks per call |
| guidelines | 2000 characters |

## One example of each block

```json
{"type": "heading", "level": 1, "text": "REST APIs"}
{"type": "paragraph", "text": "REST maps **resources** to URLs. The client sends a verb; the server returns a representation."}
{"type": "bullets", "items": [{"text": "Stateless", "children": [{"text": "No session stored on the server"}]}, {"text": "Cacheable"}]}
{"type": "numbered", "items": [{"text": "Client sends the request"}, {"text": "Server returns a representation"}]}
{"type": "callout", "variant": "definition", "title": "Idempotent", "body": ["Doing it twice has the same effect as doing it once."]}
{"type": "code", "language": "http", "code": "GET /users/42 HTTP/1.1\nAccept: application/json", "caption": "A simple GET"}
{"type": "table", "columns": ["Method", "Idempotent", "Safe"], "rows": [["GET", "yes", "yes"], ["POST", "no", "no"]], "caption": "HTTP method properties"}
{"type": "qa", "question": "Why is PUT idempotent?", "answer": "Repeating it sets the same state, so nothing further changes.", "hide_answer": true}
{"type": "fill_in", "prompt": "Explain statelessness in your own words.", "lines": 3}
{"type": "spacer", "size": "medium"}
{"type": "diagram", "source": "direction: right\nclient -> api: request\napi -> db: query", "size": "half", "caption": "Request path"}
{"type": "divider"}
{"type": "page_break"}
```

Callout `variant` is one of `note`, `tip`, `warning`, `definition`, `key_point`. `body` is a list of paragraph strings. `title` is optional.

`spacer` size is `small`, `medium`, or `large`. Density already spaces headings, tables, diagrams, callouts, and Q&A, so a spacer is for a hole density does not cover, not a rhythm device.

`qa` with `hide_answer: true` prints the answer upside down at the bottom of the page so the student can try first.

Tables align text left and number columns right on their own. Pass `align` (one of `left`, `center`, `right` per column) only to override that. Center only short labels.

Inside `two_column`, each side needs at least one block. Allowed types: level-3 heading, paragraph, bullets, numbered, callout, code, diagram (`half` or `small` only), and qa. No page breaks, no nested columns, no tables.

```json
{"type": "two_column",
 "left": [{"type": "diagram", "source": "direction: right\nclient -> api -> db", "size": "half"}],
 "right": [
   {"type": "heading", "level": 3, "text": "What happens"},
   {"type": "numbered", "items": [{"text": "The client sends a request"}, {"text": "The API queries the database"}]}
 ]}
```

A second `two_column` is how two short callouts sit side by side:

```json
{"type": "two_column",
 "left": [{"type": "callout", "variant": "key_point", "title": "Remember", "body": ["PUT replaces the resource. POST adds one."]}],
 "right": [{"type": "callout", "variant": "warning", "title": "Easy to mix up", "body": ["POST is not idempotent. A retry can create a second row."]}]}
```

## Diagrams (D2)

You describe the graph. Folio lays it out. In JSON, newlines inside `source` are `\n`.

- Node: `api` or `api: API gateway`.
- Shapes: `{shape: cylinder}`, and also `queue`, `person`, `cloud`, `hexagon`, `oval`, `diamond`.
- Edge: `client -> api: HTTPS`. Also `<-`, `<->`, `--`.
- Group: `backend: { api; worker }`, then refer to `backend.api`.
- `direction: right` for a chain. The default is downward, which gets very tall past 3 nodes.

Flow:

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

Sequence (a handshake, a request/response, anything that is ordered in time):

```
shape: sequence_diagram
client: Client
server: Server
client -> server: POST /orders
server -> client: 201 Created
```

Keep labels to 1–3 words. About 6 nodes maximum. A bigger idea is two diagrams, not one crowded one.

## A finished one-page section

This is the shape to copy. Swap the subject; keep the rhythm: title, one paragraph, diagram beside steps, a comparison, two short callouts, one self-test, then a page break when another H1 follows.

```json
[
  {"type": "heading", "level": 1, "text": "TCP handshake"},
  {"type": "paragraph", "text": "TCP opens a connection with a ==three-way handshake== before any data is sent. Each side picks a sequence number so later bytes can be acknowledged."},
  {"type": "two_column",
   "left": [{"type": "diagram", "size": "half", "caption": "SYN, SYN-ACK, ACK",
             "source": "shape: sequence_diagram\nclient: Client\nserver: Server\nclient -> server: SYN\nserver -> client: SYN-ACK\nclient -> server: ACK"}],
   "right": [
     {"type": "heading", "level": 3, "text": "The three steps"},
     {"type": "numbered", "items": [
       {"text": "Client sends SYN with its initial sequence number"},
       {"text": "Server replies SYN-ACK and sends its own number"},
       {"text": "Client sends ACK. The connection is open"}
     ]}
   ]},
  {"type": "table", "caption": "What each flag means",
   "columns": ["Flag", "Who sends it", "Meaning"],
   "rows": [["SYN", "opener", "I want to connect"], ["ACK", "either", "I got your number"]]},
  {"type": "two_column",
   "left": [{"type": "callout", "variant": "key_point", "title": "Remember", "body": ["No application data is sent until the third segment."]}],
   "right": [{"type": "callout", "variant": "warning", "title": "Half-open", "body": ["If the final ACK is lost, the server is left waiting."]}]},
  {"type": "qa", "hide_answer": true,
   "question": "Why does the handshake need a third segment?",
   "answer": "So the client can acknowledge the server's sequence number. Two segments would leave one direction unconfirmed."}
]
```
