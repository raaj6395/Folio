---
name: Lecture
label: Lecture
use_when: notes from a course lecture (slides, a course PDF or transcript, "lecture 3", "week 3", "class notes")
required: Key terms, Summary
---
**Unit:** one lecture = several H1s, each on its own page:
1. `<Course> L<n>: <title>`, with a meta line (course, lecturer, date, source file) and a numbered outline of what was taught.
2. One H1 per main concept, in the order taught, each shaped like a topic page (definition, how it works, example, qa).
3. `<Course> L<n>: Review`, the only page with the headings "Key terms" and "Summary".

## Review page sections
| # | H2 heading | What goes in it | Blocks |
|---|---|---|---|
| 1 | Key terms | Every term the lecture defined, one line each | `table` (Term, Meaning), ≤ 25 rows |
| 2 | Summary | 3-5 sentences in plain words | `callout` key_point or `paragraph` |
| 3 | Likely exam questions | What the lecturer stressed, repeated or called important | 3-5 `qa` with hide_answer true; pair them in a `two_column` |
| 4 | Ask the lecturer (optional) | What the slides left unclear | `fill_in`, 3 lines |

## Rules
- Follow the source. Don't add material the lecture didn't cover except to explain a step, and mark it "(added)".
- Keep the lecturer's own examples: exams reuse them.
- Use the headings "Key terms" and "Summary" only on the Review page.

## Skeleton
The Review page (the concept pages follow the topic template's shape):
```json
[
{"type":"heading","level":1,"text":"Operating Systems L4: Review"},
{"type":"heading","level":2,"text":"Key terms"},
{"type":"table","columns":["Term","Meaning"],"rows":[["Process","A running program with its own address space"],["Thread","A unit of execution inside a process; threads share memory"],["Context switch","Saving one task's registers and loading another's"],["PCB","The kernel's record of a process: state, registers, open files"],["Scheduler","Picks which ready task runs next"]]},
{"type":"heading","level":2,"text":"Summary"},
{"type":"callout","variant":"key_point","title":"In plain words","body":["A process is a program plus its own memory; threads are cheaper workers sharing that memory. The kernel tracks each process in a PCB and switches between them, and every switch costs saved and restored registers."]},
{"type":"heading","level":2,"text":"Likely exam questions"},
{"type":"two_column","left":[{"type":"qa","question":"What do threads of one process share, and what do they not?","answer":"They share the address space and open files; each has its own registers and stack.","hide_answer":true}],"right":[{"type":"qa","question":"Why is a context switch between threads cheaper than between processes?","answer":"The address space stays the same, so no page-table switch or TLB flush.","hide_answer":true}]},
{"type":"fill_in","prompt":"Ask the lecturer:","lines":3}
]
```

## Checklist
- Concept pages follow the order the lecture taught them.
- The Review page has Key terms, Summary and likely exam questions.
- Anything not from the lecture is marked "(added)".
