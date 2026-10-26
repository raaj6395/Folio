---
name: Article
label: Article
use_when: notes on something someone else wrote or said (a blog post, paper, talk, video or book chapter; "summarise this article")
required: Key ideas, My take
---
**Unit:** one source = one H1, titled with the source's title. 1-2 pages.

## Sections
| # | H2 heading | What goes in it | Blocks |
|---|---|---|---|
| 0 | (under the H1) | Author · publication · date · link | `paragraph` |
| 1 | (no heading) | TL;DR: 1-2 sentences in plain words | `callout` key_point titled "TL;DR" |
| 2 | Key ideas | 3-7 points, each starting with a bold lead phrase | `numbered` |
| 3 | How it works (if the source explains a mechanism) | The mechanism, or two contrasting terms | `two_column`: half `diagram` + `paragraph`, or two definition callouts |
| 4 | Details worth keeping (optional) | Numbers, definitions, one quote | `table`, `callout` definition, `callout` note for a quote with attribution |
| 5 | My take | What the user said about it. If they said nothing, blank lines for them to write on | `bullets`, or `fill_in` (3-4 lines) |
| 6 | (end) | 1-2 recall questions | `qa` |

Variant: for a paper, add H2 "Method" and "Results" after Key ideas.

## Rules
- Quote only verbatim text from content the user gave you, with attribution. Never paraphrase inside quotation marks.
- Never invent the user's opinion: without one, "My take" is a fill_in.
- If you couldn't read the source (paywall, video), say so in the meta line and use only what the user provided.

## Skeleton
```json
[
{"type":"heading","level":1,"text":"Out of the Tar Pit"},
{"type":"paragraph","text":"**Authors:** Ben Moseley, Peter Marks · **Published:** 2006 · **Source:** [paper (PDF)](https://curtclifton.net/papers/MoseleyMarks06a.pdf)"},
{"type":"callout","variant":"key_point","title":"TL;DR","body":["Most of what makes software hard is complexity we add ourselves, mainly mutable state. Keep only the essential state and logic, and push everything else to the edges."]},
{"type":"heading","level":2,"text":"Key ideas"},
{"type":"numbered","items":[{"text":"**Complexity is the root problem**: it makes systems hard to understand, change and test."},{"text":"**State is the biggest source**: each piece of mutable state multiplies the cases you must reason about."},{"text":"**Control flow is the second**: fixing an order the problem doesn't require adds complexity."},{"text":"**Separate essential from accidental**: model the essential logic relationally and keep performance tricks apart."}]},
{"type":"heading","level":2,"text":"How it works"},
{"type":"two_column","left":[{"type":"callout","variant":"definition","title":"Essential complexity","body":["Complexity the user's problem really has."]}],"right":[{"type":"callout","variant":"definition","title":"Accidental complexity","body":["Everything we add on top: state we didn't need, ordering, caching."]}]},
{"type":"heading","level":2,"text":"My take"},
{"type":"fill_in","prompt":"Where does accidental state creep into the code I work on?","lines":3},
{"type":"qa","question":"Which two sources of complexity do the authors rank highest?","answer":"Mutable state first, then control (ordering).","hide_answer":true}
]
```

## Checklist
- The meta line links the source; the TL;DR is in plain words.
- Every key idea starts with a bold lead phrase.
- Quotes are verbatim and attributed.
- "My take" is the user's own, or a fill_in.
