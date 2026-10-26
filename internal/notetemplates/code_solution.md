---
name: Code solution
label: Code
use_when: a fix, how-to or implementation in a real stack (a stack trace or error, "how I fixed…", "implement X in Go", code from the user's repo)
required: Problem, Solution, How it works
density: compact
---
**Unit:** one solution = one H1, titled by the outcome ("Rate limiting a Go HTTP API", not "Rate limiter"). 1-2 pages.

## Sections
| # | H2 heading | What goes in it | Blocks |
|---|---|---|---|
| 0 | (under the H1) | Meta line: language/stack, tags, link to the issue, PR or docs | `paragraph` |
| 1 | Problem | What was wrong or needed: the symptom, and the exact error if there is one | `paragraph` + `code` (language "") for the error |
| 2 | Solution | The flow at a glance, then the code. One code block per file, captioned with the file path | `two_column` (half `diagram` + `numbered` steps), then `code` with caption |
| 3 | How it works | Why the fix works, tied to specific lines | `bullets` |
| 4 | Pitfalls and alternatives | What breaks it, other options and when to choose them | `two_column`: `callout` warning beside `bullets` of options; a `table` (Option, When, Trade-off) when there are 3+ options to weigh |
| 5 | (end) | One recall question | `qa` |

Variant: for an incident or debugging story, add an H2 "Root cause" between Problem and Solution.

## Rules
- Copy code from the user's repo exactly; cut irrelevant lines with `// …`. Each code block ≤ 60 lines.
- Never put secrets, tokens, passwords or internal hostnames on the page: replace them with placeholders.
- Don't claim the fix was tested unless the user said so.

## Skeleton
```json
[
{"type":"heading","level":1,"text":"Rate limiting a Go HTTP API"},
{"type":"paragraph","text":"**Stack:** Go, net/http · **Tags:** middleware, rate limiting · **Docs:** [golang.org/x/time/rate](https://pkg.go.dev/golang.org/x/time/rate)"},
{"type":"heading","level":2,"text":"Problem"},
{"type":"paragraph","text":"One client sending hundreds of requests a second slowed the API for everyone. We needed a per-client limit that answers **429 Too Many Requests**."},
{"type":"heading","level":2,"text":"Solution"},
{"type":"two_column","left":[{"type":"diagram","source":"direction: right\nclient -> limiter -> handler","size":"half"}],"right":[{"type":"numbered","items":[{"text":"Find the client by IP"},{"text":"Get or create its token bucket"},{"text":"No token left: reply 429"},{"text":"Otherwise call the next handler"}]}]},
{"type":"code","language":"go","code":"func RateLimit(next http.Handler) http.Handler {\n\tvar mu sync.Mutex\n\tlimiters := map[string]*rate.Limiter{}\n\treturn http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {\n\t\tip, _, _ := net.SplitHostPort(r.RemoteAddr)\n\t\tmu.Lock()\n\t\tl, ok := limiters[ip]\n\t\tif !ok {\n\t\t\tl = rate.NewLimiter(5, 10) // 5 req/s, bursts of 10\n\t\t\tlimiters[ip] = l\n\t\t}\n\t\tmu.Unlock()\n\t\tif !l.Allow() {\n\t\t\thttp.Error(w, \"too many requests\", http.StatusTooManyRequests)\n\t\t\treturn\n\t\t}\n\t\tnext.ServeHTTP(w, r)\n\t})\n}","caption":"middleware/ratelimit.go"},
{"type":"heading","level":2,"text":"How it works"},
{"type":"bullets","items":[{"text":"`rate.NewLimiter(5, 10)` is a token bucket: 5 tokens a second, up to 10 saved for bursts"},{"text":"`Allow()` takes a token without waiting, so a busy client gets 429 at once"},{"text":"The mutex guards the map; each limiter is already safe for concurrent use"}]},
{"type":"heading","level":2,"text":"Pitfalls and alternatives"},
{"type":"two_column","left":[{"type":"callout","variant":"warning","title":"Pitfalls","body":["The map never shrinks: evict idle clients in production.","Behind a proxy `RemoteAddr` is the proxy, so every client shares one bucket."]}],"right":[{"type":"bullets","items":[{"text":"**Several servers:** keep counters in Redis"},{"text":"**Managed edge:** limit at the API gateway"}]}]},
{"type":"qa","question":"Why does Allow() suit an API better than Wait()?","answer":"Wait() blocks the request until a token frees up, holding connections open; Allow() rejects at once with 429.","hide_answer":true}
]
```

## Checklist
- The title says the outcome, and the meta line links the source.
- Code blocks are captioned with file paths, with no secrets in them.
- "How it works" explains why, tied to specific lines.
- Pitfalls name what breaks in production.
