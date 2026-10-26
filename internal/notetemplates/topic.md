---
name: Topic
use_when: anything else: one concept or subject explained (the default)
---
**Unit:** one concept = one H1, 1 page. This is Folio's default shape for notes that don't fit another template.

## Sections
| Part | What goes in it | Blocks |
|---|---|---|
| Definition | One or two sentences | `callout` definition |
| Why it matters | The problem it solves | `paragraph` |
| How it works | The mechanism in steps beside a small diagram | `two_column`: half `diagram` + `numbered` |
| Example | A concrete case | `code`, `table` or `paragraph` |
| Comparison (if there's a sibling concept) | How it differs | `table` |
| Pitfalls | The common mistake | `callout` warning |
| (end) | 1-2 recall questions | `qa` or `fill_in` |

## Skeleton
```json
[
{"type":"heading","level":1,"text":"DNS resolution"},
{"type":"callout","variant":"definition","title":"DNS","body":["The Internet's phone book: it turns names like example.com into IP addresses."]},
{"type":"paragraph","text":"Computers connect by IP address, people remember names. DNS answers the lookup in milliseconds and caches the answer at every step."},
{"type":"heading","level":2,"text":"How it works"},
{"type":"two_column","left":[{"type":"diagram","source":"browser -> resolver: example.com?\nresolver -> servers: ask","size":"half"}],"right":[{"type":"numbered","items":[{"text":"The browser asks a recursive resolver"},{"text":"The resolver asks a root server, then the .com server"},{"text":"The authoritative server returns the IP"},{"text":"Every step caches it for the record's TTL"}]}]},
{"type":"heading","level":2,"text":"Common records"},
{"type":"table","columns":["Record","Maps","Example use"],"rows":[["A","Name → IPv4 address","Website address"],["CNAME","Name → another name","www → the main domain"],["MX","Domain → mail server","Where email goes"]]},
{"type":"callout","variant":"warning","title":"Pitfall","body":["Changing a record isn't instant: old answers live in caches until their TTL expires."]},
{"type":"qa","question":"Why can a DNS change take hours to reach everyone?","answer":"Resolvers keep the old answer until its TTL expires.","hide_answer":true}
]
```

## Checklist
- Opens with a definition; the mechanism has a diagram beside its steps.
- Ends with a recall question.
