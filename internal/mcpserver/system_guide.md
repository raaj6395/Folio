# System notebooks

A system notebook is an interactive system design. The user opens it in the viewer, picks a request and sends it, kills and revives nodes, and watches each request travel hop by hop with a log line per step. Folio lays it out and runs it; you only write the spec.

## Workflow

1. `create_notebook` with `format: "system"` (no template).
2. `append_blocks` with exactly one block: `{"type": "system", ...spec}`. Folio validates the spec fully: structure, references, loops, every request with each node down, and every scenario's `expect`. Fix what the error says and write again.
3. Check behaviour with `simulate_system` (a request, optionally with `kill` and `warm`). Then `get_viewer_url` and give the user the link.
4. To change the system: `read_blocks`, then `update_block` with the whole new spec. The open viewer updates live. `undo` works as usual.

There are no pages: `check_layout`, `render_pages`, `inspect_render` and `export_pdf` don't apply.

## Example

```json
{"type": "system",
 "nodes": [
  {"id": "client", "kind": "client", "label": "Browser"},
  {"id": "lb", "kind": "load_balancer", "label": "Load Balancer", "pool": ["app1", "app2"]},
  {"id": "app1", "kind": "service", "label": "App Server 1", "group": "Zone A"},
  {"id": "app2", "kind": "service", "label": "App Server 2", "group": "Zone B"},
  {"id": "redis", "kind": "cache", "label": "Redis",
   "about": "Keeps hot products in memory.", "when_down": "Reads fall back to Postgres and get slower."},
  {"id": "pg", "kind": "database", "label": "Postgres", "ms": 30}
 ],
 "handlers": {
  "get_product": [
   {"call": "redis", "op": "get", "key": "product:{key}", "on": {"hit": "return", "down": "next"}},
   {"call": "pg", "op": "read", "key": "product:{key}"},
   {"call": "redis", "op": "set", "key": "product:{key}", "async": true}
  ]
 },
 "requests": [
  {"id": "read", "label": "GET /products/{key}", "to": "lb", "handler": "get_product", "keys": ["7", "9"]}
 ],
 "scenarios": [
  {"label": "Redis is down", "kill": ["redis"], "request": "read",
   "expect": {"status": 200, "visits": ["pg"]},
   "explain": "Cache-aside falls back to the database: slower, but it still works."}
 ]}
```

## How it works

- **No coordinates and no links.** Folio lays the system out left to right (`"direction": "down"` for top to bottom). Connections come from `pool`, `call`, `publish`, `consumers` and `replicas`. Optional `"links": [{"from", "to", "label"}]` only labels an existing connection (e.g. "gRPC").
- **Nodes have behaviour; you don't script failure paths.** Each node kind behaves in a fixed way, and services run handlers: step lists with a branch per outcome. Kill any node or combination of nodes and the engine works out what happens. A failure travels back up as an HTTP status (cascading failure).
- Every request carries a **key** (the id it is about). `{key}` in labels, step keys and `say` is replaced by it.

## Nodes

| Field | Meaning |
|---|---|
| `id` | lowercase letters, digits, `_` |
| `kind` | `client`, `load_balancer`, `service`, `cache`, `database`, `queue` |
| `label` | ≤ 40 chars, defaults to id |
| `icon` | optional: client, phone, load_balancer, gateway, server, worker, cache, database, queue, cdn, storage, search, auth, external |
| `group` | zone, tier or region; drawn as a box, and the user (or a scenario) can kill the whole group |
| `ms` | processing time; defaults: client 0, load_balancer 1, service 5, cache 1, database 20, queue 2 |
| `about`, `when_down` | shown when the user clicks the node; write them for a student |
| `pool` | load_balancer: the services it spreads requests over |
| `strategy` | load_balancer: `round_robin` (default) or `hash` (by key; killing a member re-maps keys) |
| `replicas` | database: read replicas. Reads go round robin to live replicas, then the primary; writes go to the primary |
| `consumers`, `handler` | queue: the services that take its messages, and the handler they run |
| `persistent` | cache: keep keys when killed (by default a killed cache comes back empty) |

## Steps

| Step | Meaning |
|---|---|
| `{"call": "<cache or database>", "op": "...", "key": "..."}` | cache ops `get` `set` `del` `setnx` `incr` (incr needs `"limit": n`); database ops `read` `write`. key defaults to the request key |
| `{"call": "<service or load_balancer>", "handler": "<name>"}` | run that handler there (through the pool for a load balancer) |
| `{"publish": "<queue>", "key": "..."}` | queue a message; a live consumer runs the queue's handler later. With no live consumer it waits until one is revived |
| `{"parallel": [[steps], [steps]]}` | branches at the same time; fails if any branch fails |

Modifiers on call and publish: `"async": true` (don't wait; no `on`), `"retry": n` (0-5; retries connection failures and 5xx), `"on": {outcome: action}`, `"say": "log line"`, and `"force": "<outcome>"` to fix the outcome of logic the engine doesn't model (a call to an outside API, a matching algorithm); a forced call to a service needs no handler.

An action is `"next"`, `"return"` (stop, success), `"fail"` (stop, error) or a list of steps (run them, then continue). To undo something and then fail (release a lock when payment fails), end the list with a step whose `on` maps `ok` to `"fail"`.

## Outcomes

| Target | Outcomes | If not in `on` | Status when it fails the request |
|---|---|---|---|
| service | ok, error, down | ok → next, else fail | its own error status / 502 |
| load_balancer | ok, error, unavailable, down | ok → next, else fail | 503 if no live server |
| cache get | hit, miss, down | hit/miss → next | 502 |
| cache setnx | ok, taken, down | taken → fail | 409 |
| cache incr | ok, limited, down | limited → fail | 429 |
| cache set/del, database, publish | ok, down | ok → next | 502 |

## Requests and scenarios

- Request: `id`, `label`, `to` (a load balancer or service), `handler`, `keys` (1-10). `from` is needed only with several clients.
- Scenario: a ready-made experiment the user can click. `label`, `kill` (nodes or groups), `request`, optional `key`, `warm` (send once first so caches are warm), `expect` (`status`, `visits`, `avoids`), `explain`. Add 3-5: the most instructive failures of the design. Expects are checked on every write.

## Good systems

- 5-15 nodes. Model what the lesson is about; leave out the rest.
- Use groups for zones or worker pools, and at least two instances wherever redundancy is the point.
- Write `about` and `when_down` for every node that matters, in plain words.
- Scenario `explain` says what the student should notice and why.
