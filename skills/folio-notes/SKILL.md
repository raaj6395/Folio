---
name: folio-notes
description: Write, revise, and lay out study notes as real Folio notebook pages through the Folio MCP connector. Use this whenever the user asks for notes, revision notes, a cheat sheet, a study page, a summary to print or study from, a flowchart, mind map or architecture map on a canvas, or to add, fix, restyle, undo, or export a Folio notebook — even if they only say "make notes on X", "turn this into notes", or "notes for my exam" and never name Folio. Plan the pages, write the blocks, and fix the layout yourself. Do not use this for Apple Notes, a markdown file, a Google Doc, or a slide deck.
compatibility: Requires the Folio MCP connector to be connected in this chat. Tools are create_notebook, append_blocks, check_layout, and the other Folio tools.
---

# Folio notes

Folio typesets study notes onto A4 pages, or onto one big canvas for flowcharts and maps. You send structured blocks. Folio measures them, flows them, and tells you what is wrong. You never place anything with coordinates, and you never paste the finished notes into the chat. The notebook is the deliverable.

The user should not have to review a draft, pick a layout, or tell you a page looks empty. Decide, write, read the layout report, fix it, and hand them a link.

## Before you write

Confirm the Folio tools are available (`list_notebooks`, `create_notebook`, `append_blocks`, and the rest). If they are not, stop and tell the user to connect the Folio connector, then start a new chat. Do not imitate the notes as markdown.

Call `list_folders` and `list_notebooks` before creating anything.

- Continue an existing notebook when the user says to add, update, fix, or extend one, or when a notebook with the same topic already exists and they did not ask for a new one. `get_outline` first. Make a small edit with `insert_blocks`, `update_block`, `move_blocks`, or `delete_blocks`. Do not rewrite the notebook.
- Create a new notebook when the topic is new or they asked for a new one.
- Reuse a folder that already matches the subject. Pass `folder` only when they named a folder, course, or subject, or an existing folder clearly matches. Do not invent a folder they did not ask for. Paths look like `Semester 1/Networks`, at most 3 levels. Matching ignores case.
- If `get_outline` returns `notebook_guidelines`, those preferences override this skill. When the user states a lasting preference ("always British spelling", "one idea per box", "exam notes stay on one page"), save it with `set_notebook_guidelines` (max 2000 characters) so the next session follows it.

## Decide these yourself

Do not ask about theme, length, or structure. Pick them and mention the choice in one sentence at the end.

| They say | You pick |
|---|---|
| nothing about look | theme `clean`, paper `plain`, mode `dark`, density `comfortable` |
| flowchart, canvas, whiteboard, mind map, architecture map, "too big for a page" | `format: canvas`: one big diagram at natural size plus short notes, no page breaks (see "Canvas" in the authoring guide) |
| handwritten, notebook, sketch | theme `notebook` (ruled paper, handwriting, sketch diagrams) |
| bullet journal, dots | theme `dotgrid` (dotted paper) |
| print, light mode, white page | mode `light`. Mention that dark pages use a lot of ink |
| tighter, fit more | density `compact` |
| more space, airy | density `airy` |
| a page count | that many H1 sections, one page each |
| "cheat sheet" | 1 page: tables, callouts, almost no prose |
| "revision notes" or "make notes on X" with no length | 2 pages for one mechanism, 3–4 for a broad topic. Cover the ideas that matter; name what you left out |
| a lecture, chat, or pasted text | notes of *that* material only. Do not add chapters they did not give you |

A broad ask ("notes on operating systems") becomes the few mechanisms worth revising, not a textbook. Say which ones you covered.

`list_themes` only if you are unsure a name is valid. Themes are `clean`, `notebook`, `dotgrid`. Papers are `plain`, `ruled`, `dotted`, `grid`. Modes are `dark`, `light`. Densities are `compact`, `comfortable`, `airy`.

## What a good page is

One H1 is one page and one idea a student can revise in a few minutes. A page holds 257mm of content. Aim for 60–95% full.

Build each page in this order:

1. **H1** — the topic, not "Introduction" or "Overview".
2. **One short paragraph** — what it is, in 2–4 sentences. Define the term before you use it.
3. **A `two_column`** — a `half` diagram on one side, the steps, bullets, or a definition on the other. Any process, handshake, architecture, lifecycle, or data flow gets a diagram. A column may start with a level-3 heading.
4. **A table** when two or more things are being compared. Lists are for steps and enumerations, not for comparisons.
5. **Paired callouts** in another `two_column` when you have two short ones (a `definition` beside a `key_point`, or a `tip` beside a `warning`). One idea per callout.
6. **Recall** — 1 or 2 `qa` blocks with `hide_answer: true`, or one `fill_in` (3 lines) when they should explain it in their own words. The question tests the key point.
7. **`page_break`** before the next H1. The first H1 does not get one.

Use `==highlight==` once per section, on the phrase that is the answer. Inline text only supports `**bold**`, `*italic*`, `` `code` ``, `[text](url)`, and `==highlight==`. Anything else is printed as-is, so there is nothing to escape.

Write like study notes: concrete, with one worked example (a request, a number, a small input). Skip conclusions, motivational lines, and filler. Code only when the topic is code, and keep it short.

Block fields, limits, and D2 diagram patterns are in [references/blocks.md](references/blocks.md). Read that file before the first `append_blocks` of the session.

## Writing loop

1. `create_notebook` with the title, theme, paper, mode, density, and folder. Title is the topic ("TCP handshake"), not "Notes".
2. Plan the H1s silently. Do not show an outline and wait.
3. `append_blocks` one whole section at a time, at most 50 blocks per call. Pass `expected_version` from the previous result so a second writer cannot overwrite you.
4. Read the result before the next section: `layout.page_fill_pct`, `layout.last_page_free_mm`, `warnings`, and `layout_hints`. Each hint looks like `p2 under_filled (b_xxxxxxxx): add content...`. Fix that page before starting the next H1.
5. If a warning says the change was saved but layout is still computing, do not send the same blocks again. Call `get_layout_info` after a few seconds, then continue.
6. If a write is rejected, fix the reported block and resend only that call. A version mismatch means someone else wrote: `get_outline`, then retry once with the new version.
7. When the sections are in, `check_layout`. Fix what it reports. Two repair passes is enough.
8. `render_pages` once, on the pages you changed, at most 2 pages, default 80 ppi. Look at the images. Fix clipped text, a colliding diagram, or a heading stranded at the bottom. Do not render the whole notebook, and do not render again unless that fix was large. Rendering is limited to about 30 calls a minute.
9. `get_viewer_url`. If they asked for a file or to print, `export_pdf` and include that URL too.

`preview_diagram` before inserting a diagram with more than 4 nodes, a sequence, or any source you are not sure compiles. It returns the D2 error with a line number and does not touch the notebook.

To change one block, `read_blocks` (by id, by page, or from an id) and `update_block` with the full new block. The id stays. Nested blocks inside a `two_column` can be updated, deleted, and read, but `insert_blocks` and `move_blocks` only accept top-level ids. `"start"` means the beginning.

`undo` reverts the last change. `list_versions` then `restore_version` for an older one. `delete_notebook` only when they clearly asked to delete it, and only with `confirm: true`.

`set_theme` changes theme, paper, mode, or density. Pass the current theme even if you only want to switch density or dark/light. An older notebook whose density is `legacy` should be switched to `comfortable` instead of padding it with spacers.

## How to fix a hint

The hint names the check, the page, the block ids, and a fix. Do that fix. Do not ask the user which one they prefer.

| Check | What you do |
|---|---|
| `under_filled` | The page is under 50% before a new H1 or a page break. Add a comparison, a second recall question, or a worked example, or put part of the page in `two_column` so it uses the width. Merging two thin sections is better than a blank page. |
| `spill_over` | A few percent fell onto the next page. Shorten a paragraph, shrink the diagram (`half` inside `two_column`, or `direction: right`), or drop a spacer. |
| `almost_empty` | A near-blank page in the middle. Remove the `page_break` that caused it, or move content onto it. |
| `orphan_heading` | A heading is the last thing on its page. Insert a `page_break` before it, or shorten whatever is above it. |
| `pushed_block` | A box, table, diagram, or code block did not fit and jumped to the next page. Shorten it, split the table, or move a smaller block ahead of it. |
| `oversized_diagram` | Taller than about 40% of the page. Use size `half` inside a `two_column`, or add `direction: right`. |
| `unreadable_diagram` | Labels would render under 6pt. Fewer nodes (about 6 max), shorter labels (1–3 words), `direction: right`, or size `full`. Put the explanation in the paragraph, not in the node. |
| `monotone_stack` | A long vertical stack with a diagram and no side-by-side layout. Pair the diagram with its steps in a `two_column`. |
| `cramped` | On `comfortable` or `airy`, density already adds gaps, so this usually means blocks that the density does not space. Insert one medium spacer, or switch a `legacy` notebook to `comfortable` with `set_theme`. Do not put a spacer before every H2. |
| `overflow` | One block is taller than a page. Split it. |

A full-width chain of 4 or more nodes is too tall drawn downward and unreadable at half width. Draw it full width with `direction: right`, and put the explanation under it or beside it. A `half` diagram has room for about 3 nodes across.

`get_layout_info` is the cheap way to see fill and positions when you do not need an image. `check_layout` is the score (0–100) and the full issue list. Ship when the score is in the 90s, or when two repair passes did not move a remaining issue. Mention that leftover issue in the reply. Do not keep looping.

## What you say when you are done

A short reply, not the notes:

- Notebook title, folder if it has one, theme and mode, page count, layout score.
- The viewer URL, and the PDF URL if you exported one.
- One sentence on what the pages cover, including anything you deliberately left out.
- One sentence if you saved a guideline or changed an existing notebook instead of creating one.

Do not paste the block JSON or rewrite the notes as bullets in the chat.

## Editing in plain words

These are the requests you handle without a follow-up question:

- "Add a comparison of HTTP methods after the methods section." — `get_outline`, `insert_blocks` after that section's last block.
- "Page 3 looks cramped." — `check_layout` on that page, then the matching fix above.
- "Make the spacing airy." / "Switch to light mode." — `set_theme`, passing the current theme.
- "Undo that." / "Go back to version 4." — `undo` or `restore_version`.
- "Put it in my Networks folder." — `move_notebook`.
- "Always keep one idea per box." — `set_notebook_guidelines`.
