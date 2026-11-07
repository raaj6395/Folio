---
name: System design
label: Design
use_when: designing a system ("design a URL shortener", "system design of WhatsApp")
required: Functional, Non-functional, Estimates | Components, Data model | Trade-offs
---
**Unit:** one part of the system per H1, each on its own page:
1. `<System>: Requirements and estimates`: headings Functional, Non-functional, Estimates.
2. `<System>: Architecture`: headings Components, Data model.
3. `<System>: Deep dive`: heading Trade-offs, plus the 1-2 hardest parts (scaling, consistency, hot keys).

## Sections
| Part | What goes in it | Blocks |
|---|---|---|
| Functional / Non-functional | What it must do; scale, latency, availability, consistency | `two_column`: H3 + `bullets` on each side |
| Estimates | Traffic, storage and bandwidth, with the arithmetic | `table` (Quantity, Estimate, How) |
| Components | One full-width architecture diagram, then one line per component | `diagram` with direction: right (≤ 6 nodes; split if bigger) + `bullets` |
| Data model | Tables or keys with their fields and the main access pattern | `table` |
| Trade-offs | Each decision, why, and what it costs | `table` (Choice, Why, Cost) |
| (end of each part) | One recall question | `qa` |

## Rules
- Show the arithmetic in estimates (100M/month ≈ 40/s); round boldly.
- Name real technologies only where the choice matters, and say why.

## Skeleton
Part 1 (the other parts follow the same pattern):
```json
[
{"type":"heading","level":1,"text":"URL shortener: Requirements and estimates"},
{"type":"two_column","left":[{"type":"heading","level":3,"text":"Functional"},{"type":"bullets","items":[{"text":"Shorten a long URL to a 7-character code"},{"text":"Redirect a code to its URL"},{"text":"Optional expiry date"}]}],"right":[{"type":"heading","level":3,"text":"Non-functional"},{"type":"bullets","items":[{"text":"Redirects under 50 ms at p99"},{"text":"99.9% available; reads matter most"},{"text":"Codes must not be guessable in order"}]}]},
{"type":"heading","level":2,"text":"Estimates"},
{"type":"table","columns":["Quantity","Estimate","How"],"rows":[["New URLs","40 / s","100M a month ÷ 2.6M s"],["Redirects","4,000 / s","100 reads per write"],["Storage","3 TB","6B URLs in 5 years × 500 B"],["Code space","3.5 trillion","62^7 codes of 7 characters"]]},
{"type":"callout","variant":"key_point","title":"What the numbers say","body":["Reads dominate 100 to 1, so caching redirects matters more than write speed. 7 characters leave room for centuries of growth."]},
{"type":"qa","question":"Why 7 characters for the code?","answer":"62^7 ≈ 3.5 trillion codes, far more than the 6 billion needed in 5 years, and still short to type.","hide_answer":true}
]
```

## Checklist
- Each part has its required headings.
- Estimates show the arithmetic.
- Architecture is one readable diagram of at most 6 nodes.
- Every trade-off names its cost.
