---
name: DSA problem
label: DSA
use_when: one coding problem (a LeetCode/GFG/Codeforces link or number, "given an array…", "solve")
required: Problem, Approach, Code, Complexity
density: compact
---
**Unit:** one problem = one H1, titled `<number>. <name>` when it has a number. Aim for 1 page. If it needs 2, put a page_break before "Code" so neither page is a sliver. A notebook can hold many problems, one H1 each.

## Sections
| # | H2 heading | What goes in it | Blocks |
|---|---|---|---|
| 0 | (under the H1) | Meta line: difficulty, pattern(s), source link | `paragraph`: `**Difficulty:** Medium · **Pattern:** Sliding window · **Source:** [LeetCode 3](url)` |
| 1 | Problem | The task in 1-3 sentences in your own words, plus 1-2 examples. Constraints only if they decide the approach (n ≤ 10^5 → O(n log n)) | `paragraph` + `table` (Input, Output, Why) |
| 2 | Approach | Brute force (1-2 sentences + its complexity) beside the optimal idea, then the one-sentence insight that makes it work. Graph, tree, two-pointer and DP problems: add a small diagram or a state table | `two_column` (H3 Brute force / H3 Optimal), then `callout` key_point titled "Key insight"; optional `diagram` size half |
| 3 | Code | The optimal solution only, ≤ 40 lines, idiomatic. Comment only the non-obvious line | `code` |
| 4 | Dry run | Trace example 1, one row per step, ≤ 8 rows | `table` |
| 5 | Complexity (and edge cases) | Time and space, each with the reason. Edge cases that apply: empty, one element, duplicates, negatives, overflow | `two_column` of two `bullets` |
| 6 | Mistakes (optional) | Only what the user said about their attempt, or the classic trap of this problem | `callout` warning |
| 7 | (end) | 1-2 recall questions: "Which signal points to <pattern>?", "What's the invariant?" | `qa` with hide_answer true |

## Rules
- Never invent the statement or constraints. If no link or text was given and the problem isn't well known, ask for it.
- The code must be correct: trace it against the examples before writing it. Python unless the user or the notebook guidelines name a language.
- Name the pattern with its standard name (sliding window, two pointers, BFS, topological sort, monotonic stack, DP on subsequences…), so notebooks can be filed by pattern.

## Skeleton
```json
[
{"type":"heading","level":1,"text":"3. Longest Substring Without Repeating Characters"},
{"type":"paragraph","text":"**Difficulty:** Medium · **Pattern:** Sliding window · **Source:** [LeetCode 3](https://leetcode.com/problems/longest-substring-without-repeating-characters/)"},
{"type":"heading","level":2,"text":"Problem"},
{"type":"paragraph","text":"Given a string, return the length of the longest substring with no repeated character."},
{"type":"table","columns":["Input","Output","Why"],"rows":[["\"abcabcbb\"","3","\"abc\""],["\"bbbbb\"","1","\"b\""]]},
{"type":"heading","level":2,"text":"Approach"},
{"type":"two_column","left":[{"type":"heading","level":3,"text":"Brute force"},{"type":"paragraph","text":"Check every substring for repeats: O(n³)."}],"right":[{"type":"heading","level":3,"text":"Optimal"},{"type":"paragraph","text":"Grow the window right; when a repeat enters, jump left past its last index."}]},
{"type":"callout","variant":"key_point","title":"Key insight","body":["A window with no repeats stays valid when it shrinks, so each index enters and leaves once."]},
{"type":"heading","level":2,"text":"Code"},
{"type":"code","language":"python","code":"def length_of_longest_substring(s):\n    last = {}\n    left = best = 0\n    for right, ch in enumerate(s):\n        if last.get(ch, -1) >= left:\n            left = last[ch] + 1  # jump past the repeat\n        last[ch] = right\n        best = max(best, right - left + 1)\n    return best"},
{"type":"heading","level":2,"text":"Dry run"},
{"type":"table","columns":["right","ch","left","window","best"],"rows":[["0","a","0","a","1"],["1","b","0","ab","2"],["2","c","0","abc","3"],["3","a","1","bca","3"]],"caption":"s = \"abcabcbb\""},
{"type":"heading","level":2,"text":"Complexity and edge cases"},
{"type":"two_column","left":[{"type":"bullets","items":[{"text":"**Time:** O(n), each index enters and leaves once"},{"text":"**Space:** O(min(n, alphabet))"}]}],"right":[{"type":"bullets","items":[{"text":"Empty string → 0"},{"text":"All characters the same → 1"}]}]},
{"type":"qa","question":"Which signal in a problem points to a sliding window?","answer":"Longest or shortest *contiguous* range, with a condition that stays true when the range shrinks.","hide_answer":true}
]
```

## Checklist
- The pattern is named in the meta line.
- The key insight is one sentence.
- The code is the optimal solution, ≤ 40 lines, and the dry run matches it.
- Time and space each have a reason.
- The problem ends with a hidden-answer qa.
