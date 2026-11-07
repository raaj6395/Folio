---
name: DSA pattern
label: Pattern
use_when: one algorithm pattern or technique named without a specific problem ("sliding window", "monotonic stack", "union-find")
required: Signals, Template, Variations, Problems
density: compact
---
**Unit:** one pattern = one H1. 1-2 pages. It's the index for that pattern: detailed problems go in dsa_problem notebooks, which can share a folder like `DSA/Sliding window`.

## Sections
| # | H2 heading | What goes in it | Blocks |
|---|---|---|---|
| 0 | (under the H1) | The idea in one sentence and the complexity it usually gives | `callout` key_point |
| 1 | Signals | Words and constraints in a problem that point to this pattern ("contiguous", "longest/shortest", n ≤ 10^5) | `bullets`, beside the pitfalls in a `two_column` |
| 2 | Template | A parameterised skeleton, not a solution: the loop, the invariant, and where the problem-specific part goes. State the invariant in one sentence | `code` (≤ 25 lines) + `paragraph` |
| 3 | How it moves (optional) | The state on a tiny example | `two_column`: half `diagram` or `table` + `numbered` steps |
| 4 | Variations | What changes in the template for each variant | `table` (Variation, Change, Example) |
| 5 | Problems | 5-10 problems from easy to hard, each with its one-line twist | `table` (Problem, Level, Twist) |
| 6 | (end) | "Which signal tells you this pattern?", "What's the invariant?" | `qa` with hide_answer true |

## Rules
- Problem names and numbers must be real. If unsure of a number, give the name only.
- The Template must run in the chosen language once the placeholder is filled in. Python unless the user or the notebook guidelines say otherwise.

## Skeleton
```json
[
{"type":"heading","level":1,"text":"Sliding window"},
{"type":"callout","variant":"key_point","title":"Idea","body":["Keep a contiguous window and move both ends forward only, so each element enters and leaves once: O(n) instead of O(n²)."]},
{"type":"two_column","left":[{"type":"heading","level":3,"text":"Signals"},{"type":"bullets","items":[{"text":"*Contiguous* subarray or substring"},{"text":"Longest, shortest or count of windows"},{"text":"The condition stays true when the window shrinks"}]}],"right":[{"type":"callout","variant":"warning","title":"Pitfalls","body":["Negative numbers break sum windows: shrinking no longer lowers the sum.","Update the answer after the window is valid again, not before."]}]},
{"type":"heading","level":2,"text":"Template"},
{"type":"code","language":"python","code":"def solve(items):\n    state = ...          # what the window holds\n    left = best = 0\n    for right, x in enumerate(items):\n        add(state, x)\n        while not valid(state):\n            remove(state, items[left])\n            left += 1\n        best = max(best, right - left + 1)\n    return best"},
{"type":"paragraph","text":"**Invariant:** after the inner loop, `items[left..right]` is the longest valid window that ends at `right`."},
{"type":"heading","level":2,"text":"Variations"},
{"type":"table","columns":["Variation","Change","Example"],"rows":[["Fixed size k","Remove `items[right-k]` each step, no while loop","Max sum of k elements"],["Shortest valid","Shrink while valid and record inside the loop","Minimum Window Substring"],["Count windows","Add `right - left + 1` each step","Subarrays with product < k"]]},
{"type":"heading","level":2,"text":"Problems"},
{"type":"table","columns":["Problem","Level","Twist"],"rows":[["643. Maximum Average Subarray I","Easy","Fixed window"],["3. Longest Substring Without Repeating","Medium","Jump left past the repeat"],["424. Longest Repeating Character Replacement","Medium","Valid while size - top count ≤ k"],["713. Subarray Product Less Than K","Medium","Count windows"],["76. Minimum Window Substring","Hard","Shortest valid window"]]},
{"type":"qa","question":"Why is a sliding window O(n) despite the nested loop?","answer":"`left` only moves forward, so the inner loop runs at most n times in total.","hide_answer":true}
]
```

## Checklist
- The signals are things you can spot in a problem statement.
- The template is a skeleton with placeholders, and the invariant is stated.
- Every problem in the table is real, ordered easy to hard, with its twist.
- Ends with a hidden-answer qa.
