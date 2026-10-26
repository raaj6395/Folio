# Note templates: eval (2026-10-27)

Method (TEMPLATES_PLAN.md §8, §11.4): headless Claude Code (`claude -p`, model sonnet) against a local `folio mcp` built from branch `templates`, one empty `FOLIO_HOME` per run, `--strict-mcp-config` so only the local Folio tools exist, `--setting-sources project` so no user hooks or plugins. 18 prompts written the way a student would type them, 2 per template, none mentioning templates; they include the confusable pairs (sliding window vs LeetCode 76, TCP vs UDP vs "Explain TCP", a scheduling blog post vs scheduling lecture slides). Scored from each run's database with Folio's own layout build and template check.

## Results

| Metric | Target | Result |
|---|---|---|
| Right template, without asking | ≥ 14/15 | **25/25** (18 runs, then 7 re-runs) |
| Units with all required sections | ≥ 90% | **100%** (0 `missing_section`) |
| `check_layout` score | ≥ 85 median | **100** median; lowest 90 in round 1, 98 after the fix |
| Tool calls, same 4 prompts on `main` (no templates) vs this branch | no more than today | **main 12 / 14 / 11 / 8** vs **branch 6–7 / 7–10 / 9 / 6** (LC 76, rate limiter, TCP vs UDP, SQL cheat sheet): fewer on every prompt |
| Correct code (DSA solutions run) | all | LC 76 (Python): LeetCode examples + 3,000 random cases vs brute force; Two Sum (Go): all LeetCode examples. Both pass |
| Final reply names the template | — | 15/18 → **17/18** after the fix below |

Cost: about $12 for all 29 runs (18 + 7 re-runs + 4 controls).

| Prompt | Picked | Pages | Calls |
|---|---|---|---|
| Make Folio notes on LeetCode 76 Minimum Window Substring. | dsa_problem | 2 | 7 |
| Make Folio notes on Two Sum, with the solution in Go. | dsa_problem | 2 | 8 |
| Make Folio notes on sliding window. | dsa_pattern | 2 | 12 |
| Teach me the monotonic stack pattern in Folio notes. | dsa_pattern | 2 | 8 |
| Make Folio notes on how to add rate limiting to a Go HTTP API. | code_solution | 1 | 10 |
| My Express API failed with "No 'Access-Control-Allow-Origin' header…". I fixed it with cors(…). Make Folio notes on the fix. | code_solution | 2 | 8 |
| Summarise this article into Folio notes: martinfowler.com/articles/microservices.html | article | 2 | 9 |
| Summarise this blog post into Folio notes: ./post_scheduling.md | article | 2 | 7 |
| Make Folio notes from these lecture slides on CPU scheduling: ./slides_scheduling.md | lecture | 8 | 33 |
| Turn my week 5 OS slides (./slides_deadlock.md) into Folio notes. | lecture | 6 | 28 |
| TCP vs UDP, in Folio notes. | comparison | 2 | 9 |
| Make Folio notes on the difference between processes and threads. | comparison | 2 | 15 |
| Make a Folio cheat sheet for Go concurrency. | cheat_sheet | 2 | 9 |
| One-page Folio revision sheet on SQL joins for my exam tomorrow. | cheat_sheet | 1 | 6 |
| Make Folio notes: design a URL shortener. | system_design | 4 | 16 |
| System design of a chat app like WhatsApp, as Folio notes. | system_design | 4 | 20 |
| Explain TCP in Folio notes. | topic | 3 | 16 |
| Make Folio notes on how DNS works. | topic | 1 | 31 |

## What the eval changed
- **Page limits read as permission.** "Two Sum" skipped `check_layout` and left a near-empty second page, citing the template's "2 pages at most". Every blueprint now says: *page counts are limits, not permission; still run check_layout and fix every warning*, and the DSA unit says to page-break before Code "so neither page is a sliver". Re-run: score 100, `check_layout` called.
- **Naming the template** moved from the templates paragraph into workflow step 4 ("In your reply, name the template you used in one line"), where Claude follows it more reliably.

## Not covered
- Model: sonnet only. The claude.ai app's default model wasn't tested; if sonnet picks right, larger models should too, but that's unmeasured.
- One run per prompt (two for the 7 re-runs), so these are not variance estimates.
- `topic_dns` used 31 calls for one page, all layout polishing on the default template: the same behaviour as before templates, not caused by them.
