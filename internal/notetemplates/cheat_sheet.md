---
name: Cheat sheet
label: Cheat sheet
use_when: dense revision on one or two pages ("cheat sheet", "one page", "quick revision", "exam tomorrow"), or a library or command reference (Git, Go channels, pandas)
max_pages: 2
density: compact
---
**Unit:** the whole notebook: one H1 for the subject, one H2 per area. 1 page, 2 at most.

## Sections
| Part | What goes in it | Blocks |
|---|---|---|
| Each H2 area | Facts as rows, not sentences | `table` (Term or command, What, Example), or `bullets` of one line each; pair small areas in a `two_column` |
| Code or commands | The 3-6 forms actually used | `code`, ≤ 10 lines each |
| Gotchas | The traps examiners and interviewers ask about | one `callout` warning, 3-5 lines |
| (end, if there's room) | One recall question | `qa` |

## Rules
- No paragraph longer than 2 sentences. No diagram bigger than size small, unless the whole topic is a diagram.
- If the user will print it, create it with mode "light".

## Skeleton
```json
[
{"type":"heading","level":1,"text":"Big-O cheat sheet"},
{"type":"heading","level":2,"text":"Data structures"},
{"type":"table","columns":["Structure","Access","Search","Insert","Delete"],"rows":[["Array","O(1)","O(n)","O(n)","O(n)"],["Linked list","O(n)","O(n)","O(1) at head","O(1) at head"],["Hash map","n/a","O(1) avg","O(1) avg","O(1) avg"],["Balanced BST","O(log n)","O(log n)","O(log n)","O(log n)"],["Binary heap","O(1) top","O(n)","O(log n)","O(log n)"]]},
{"type":"heading","level":2,"text":"Sorting"},
{"type":"table","columns":["Algorithm","Best","Average","Worst","Space","Stable"],"rows":[["Merge sort","n log n","n log n","n log n","n","yes"],["Quick sort","n log n","n log n","n²","log n","no"],["Heap sort","n log n","n log n","n log n","1","no"],["Counting sort","n + k","n + k","n + k","k","yes"]]},
{"type":"two_column","left":[{"type":"heading","level":3,"text":"Rules of thumb"},{"type":"bullets","items":[{"text":"Halving each step → O(log n)"},{"text":"Loop inside a loop over n → O(n²)"},{"text":"Sort first → at least O(n log n)"},{"text":"n ≤ 10^5 → aim for O(n log n)"}]}],"right":[{"type":"callout","variant":"warning","title":"Gotchas","body":["Hash map O(1) is average; the worst case is O(n).","Recursion depth counts as space.","Quick sort hits n² on sorted input with a naive pivot."]}]},
{"type":"qa","question":"Which sort is stable and always O(n log n)?","answer":"Merge sort.","hide_answer":true}
]
```

## Checklist
- It fits on 1-2 pages.
- Facts are table rows or one-line bullets, not prose.
- Gotchas are in one warning callout.
