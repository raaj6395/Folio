## Page structure
- Each H1 section fits on one page. Start every H1 except the first on a new page (`{"type":"page_break"}` before it).
- Target 60–95% fill per page (`page_fill_pct`). A page under 50% that ends in a page_break or comes right before a new H1 is a problem: add content, arrange part of it in `two_column`, or merge sections.
- Content spilling a few percent (under ~15%) onto the next page is also a problem: tighten the section or shrink a diagram so it fits.
- Never let a diagram push the rest of its section onto the next page. If it doesn't fit, use size `half` inside a `two_column`.

## Spacing
- Every notebook has a **density** (`compact`, `comfortable` default, `airy`) that adds gaps automatically before H1/H2 and around tables, diagrams, callouts and Q&A cards. Don't add spacers before every H2 in those notebooks; that doubles the gap.
- Older notebooks show density `legacy` (tight spacing). Switch them to `comfortable` with `set_theme` instead of sprinkling spacers.
- Never place a heading, table, diagram and callout back-to-back with no space. Density handles this; in `legacy` notebooks insert a medium spacer.
- Use a `spacer` to fill a page evenly instead of leaving one large empty block at the bottom.

## Arrangement, not just a vertical stack
- Don't write everything as one long top-to-bottom sequence. When two things relate (a diagram and its explanation, pros and cons, steps and a tip), put them side by side with `two_column`.
- Good default: `two_column` with a `half` diagram on the left and numbered steps, bullets or a callout on the right. A column may start with a level-3 heading.
- Pair short callouts side by side instead of stacking them.
- Text is left-aligned. Tables align text left and numbers right automatically (override with `align`). Center only short labels and captions.

## Diagrams
- At most ~6 nodes; labels of 1–3 words.
- Chains of 4+ nodes use `direction: right`.
- Labels must render at 6pt or more (a warning tells you when they don't). Check with `preview_diagram` when unsure.
- A full-width diagram should not exceed ~40% of the page height. Prefer a `half` diagram inside `two_column`.
- A `half` diagram has room for about 3 nodes across. A chain of 4+ nodes is unreadable sideways at half width (labels under 6pt) and far too tall top-down, so show it full width with `direction: right` instead, with the explanation below or beside it in a `two_column`.

## Content density
- Short paragraphs (2–4 sentences). Tables for comparisons, lists for enumerations.
- End each section with 1–2 `qa` or `fill_in` blocks for recall.

## Working loop
1. Write a whole section per `append_blocks` call.
2. Read every write's `layout`, `warnings` and `layout_hints`, and fix the issues (`update_block`, `insert_blocks`, `move_blocks`) before moving on.
3. Before saying you're done, call `check_layout` and fix what it reports.
4. Then call `render_pages` once on the changed pages (at most 2) and look at them.
5. The user's own notebook guidelines (in `get_outline`, the first write's `notebook_guidelines`, or `get_notebook_guidelines`) override these defaults. When the user states a lasting preference, save it with `set_notebook_guidelines`.
