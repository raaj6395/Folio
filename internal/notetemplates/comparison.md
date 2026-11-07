---
name: Comparison
label: Compare
use_when: two to five things compared ("X vs Y", "difference between", "compare", "when should I use X or Y")
required: At a glance, When to use
---
**Unit:** one comparison = one H1 titled `X vs Y`, 1 page. Up to 5 options (a table has at most 6 columns).

## Sections
| # | H2 heading | What goes in it | Blocks |
|---|---|---|---|
| 0 | (under the H1) | One sentence on what each option is | `two_column` of two `callout` definition |
| 1 | At a glance | 5-10 dimensions: speed, guarantees, cost, typical use… | `table`, first column = dimension |
| 2 | How they differ (optional) | Only when the difference is a mechanism: the same flow through each | `two_column` of two half `diagram` |
| 3 | When to use | A decision rule plus 2-3 real examples for each option | `bullets` or a `table` (Situation, Pick, Why) |
| 4 | (no heading) | The common myth or mix-up | `callout` warning |
| 5 | (end) | "Which would you pick for <scenario>, and why?" | `qa` |

## Rules
- Compare on the same dimensions for every option; no empty cells (write "n/a").
- The decision rule must let a reader choose without rereading the table.

## Skeleton
```json
[
{"type":"heading","level":1,"text":"TCP vs UDP"},
{"type":"two_column","left":[{"type":"callout","variant":"definition","title":"TCP","body":["A connection-based transport that delivers bytes reliably and in order."]}],"right":[{"type":"callout","variant":"definition","title":"UDP","body":["A connectionless transport that sends independent datagrams, best effort."]}]},
{"type":"heading","level":2,"text":"At a glance"},
{"type":"table","columns":["Dimension","TCP","UDP"],"rows":[["Connection","Handshake first","None"],["Delivery","Guaranteed, retransmits","Best effort"],["Order","Preserved","Not preserved"],["Header","20-60 bytes","8 bytes"],["Flow control","Yes","No"],["Typical use","Web, email, files","Video calls, games, DNS"]]},
{"type":"heading","level":2,"text":"When to use"},
{"type":"bullets","items":[{"text":"**Every byte must arrive, in order** (files, web pages, payments) → TCP"},{"text":"**Late data is useless** (live video, voice, game state) → UDP"},{"text":"**One small question and answer** (DNS lookups) → UDP, retrying in the app"}]},
{"type":"callout","variant":"warning","title":"Common confusion","body":["UDP isn't \"unreliable\" in practice: QUIC builds reliable, ordered streams on top of it."]},
{"type":"qa","question":"A multiplayer game sends player positions 30 times a second. TCP or UDP, and why?","answer":"UDP: a lost position is replaced by the next one, while TCP would stall newer updates behind the retransmission.","hide_answer":true}
]
```

## Checklist
- Every option is defined in one sentence.
- The table uses the same dimensions for every option.
- The decision rule chooses without the table.
- Ends with a scenario qa.
