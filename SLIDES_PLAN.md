# Folio Slides — plan: make PowerPoint decks with Claude

> **Status: DRAFT 2026-10-22, researched, not built.** We agree on Section 9, then build phase by phase.

---

## 1. The idea in one paragraph

Today Folio turns *"make notes on X"* into typeset A4 pages. The same engine can turn **"make a 10-slide deck on X"** into a **presentation**:
- a live preview in the viewer,
- a PDF you can present from anywhere,
- **a real, editable `.pptx`** that opens in PowerPoint, Keynote and Google Slides.

Claude writes the slides; Folio designs, checks and exports them.

```
You: "Make a 10-slide deck on REST vs gRPC for my team meeting"
 └─► Claude plans slides ─► Folio lays them out on 16:9 ─► checks every slide fits
                                       │
                                       ├─► live preview in the viewer
                                       ├─► PDF (present anywhere)
                                       └─► .pptx (edit in PowerPoint / Keynote / Google Slides)
```

---

## 2. Research findings (what decides the design)

| Question | Finding | What it means for us |
|---|---|---|
| Can't Claude already make PPTs? | **Yes.** Since Oct 2025, Claude.ai builds `.pptx` with code execution, and there's a pptx skill for Claude Code and a PowerPoint add-in. Reviewers note AI decks tend to put **too much text on slides** and need cleanup. | Folio must add what's missing: consistent themes, a **check that every slide fits**, a live preview, undo/versions, edit-slide-by-ID, and working in *any* MCP client (Claude Code needs no code execution). |
| Can our engine (Typst) export PPTX? | **No.** Typst outputs PDF, PNG and SVG. Slide packages exist (Touying, Polylux), but PPTX export isn't part of core Typst. | Typst stays the **layout + preview + PDF** engine. A separate **PPTX writer** is needed. |
| Which PPTX writer? | **PptxGenJS:** MIT, ~6.3k stars, runs in Node with no browser; tables, charts, images incl. SVG, speaker notes, masters; works in PowerPoint, Keynote and others. **goppt** (Go): pure Go with a broad feature list, but v1.0 is only from Apr 2026 and its license is listed inconsistently (Apache-2.0 vs MIT). Other Go libs are small or unclear. **unioffice** has commercial licensing. | **Recommend PptxGenJS**, run like we run Typst (a subprocess), behind one Go interface so goppt could replace it later. |
| Marp / "render each slide as a picture"? | Marp's normal PPTX export puts **one picture per slide (not editable)**; its editable mode is experimental, needs LibreOffice, and drops notes. | Pictures-in-PPTX is the cheap trap. **We produce native text boxes, bullets and tables that people can edit.** |
| Diagrams inside PPTX | PowerPoint supports SVG images. **Google Slides does not import SVG** (convert to PNG). | Embed diagrams as **high-resolution PNG** (works everywhere). SVG with a PNG fallback is an optional later upgrade for PowerPoint. |
| Will text look the same in PowerPoint as in our preview? | Only if the fonts match. Our themes use Inter, which most PowerPoint / Keynote / Slides installs don't have; missing fonts get substituted, changing line breaks so text can overflow. | Slides use **metric-compatible fonts**: preview with **Arimo** (same widths as **Arial**) and **Cousine** (same as **Courier New**), both Apache-2.0. The PPTX declares Arial / Courier New, which every app has. Line breaks then match our measurements. |

---

## 3. What users get (features)

### Must have (v1)
1. **"Make a deck" from Claude:** `create_deck`, then slides are written like notebook blocks. 16:9 by default.
2. **Slide layouts** (Claude picks one per slide):
   | Layout | Contents |
   |---|---|
   | `title` | Big title, subtitle (deck opener) |
   | `section` | Section divider with a short line |
   | `content` | Title + body (bullets, paragraph, table, code, callout or one diagram) |
   | `two_column` | Title + left + right (diagram ↔ explanation, pros ↔ cons, before ↔ after) |
   | `statement` | One big sentence or number + small caption ("70% faster") |
   | `closing` | Thanks / Q&A / key takeaways |
3. **Speaker notes** on every slide (`notes`). Detail goes in the notes, not on the slide.
4. **Fits-on-slide guarantee:** Folio measures every text region. Text auto-shrinks down to a minimum size (body 18pt, title 28pt); below that, the slide is flagged with a concrete fix. **No text ever runs off a slide.**
5. **Slide checks** (like `check_layout` for notebooks, scored 0–100):
   - text overflow,
   - more than 6 bullets,
   - more than ~40 words of body text,
   - a title longer than 2 lines,
   - an unreadable diagram (labels below 12pt at slide size),
   - an empty region,
   - several slides in a row with the same layout ("monotone deck").
6. **Live preview** in the viewer: slides at 16:9, contents sidebar = slide titles, live updates while Claude writes.
7. **Exports:** **PDF** (present from any laptop or phone) and **PPTX** (editable): `export_pptx` and a **Download PPTX** button.
8. **Themes:** the existing clean / notebook / dotgrid look (colors, accents) adapted to slides, in dark or light.
9. **Undo / versions / edit by ID:** for free, because slides are blocks: *"change slide 4's title"*, *"undo"*, *"move the summary slide to the end"*.

### Should have (v2)
10. **Presenter mode** in the viewer: full screen, arrow keys, speaker notes + timer on a second window.
11. **Charts:** a `chart` block (bar / line / pie from a small data table), exported as **native, editable PowerPoint charts**.
12. **Notebook → deck:** `create_deck_from_notebook`, where each H1 becomes a section slide and each H2 becomes 1–2 content slides with notes.
13. **Code with syntax colors** inside the PPTX (colored text runs).
14. **4:3** slide size option.

### Later / not doing now
- Images uploaded by the user (needs file upload and storage); animations and transitions; direct Google Slides export (needs Google OAuth); custom brand templates (masters uploaded by the user).

---

## 4. How it fits Folio's design (reuse first)

**A deck is a notebook with `kind = "deck"`, whose top-level blocks are `slide` blocks.** Everything that exists keeps working:
- storage,
- versions and undo,
- block IDs and editing tools,
- ownership and isolation,
- the viewer with live updates,
- thumbnails,
- the account and OAuth layers.

### 4.1 The `slide` block
```json
{"type": "slide", "layout": "two_column", "title": "How a call flows",
 "left":  [{"type": "diagram", "source": "direction: right\nclient -> api -> db", "size": "half"}],
 "right": [{"type": "numbered", "items": [{"text": "Client sends a request"}, {"text": "API queries the DB"}]}],
 "notes": "Point out that gRPC keeps one HTTP/2 connection open."}
```
- **Fields by layout:**
  - `title`: `title`, `subtitle`
  - `section`: `title`, `subtitle?`
  - `content`: `title`, `body: [blocks]`
  - `two_column`: `title`, `left`, `right`
  - `statement`: `text`, `caption?`
  - `closing`: `title`, `body?`
- **Allowed inside a slide:** paragraph, bullets, numbered, callout, code, table, diagram (and chart in v2). Not allowed: headings, page_break, spacer, fill_in, qa, nested two_column.
- **IDs:** child blocks get their own IDs, exactly like `two_column` today, so `update_block` can change one bullet list on slide 7.
- **Kinds can't mix:** in a deck, top-level blocks must be `slide`; in a notebook, `slide` isn't allowed. The error messages say so clearly.

### 4.2 One geometry, two renderers
- Slide size is **338.67 × 190.5 mm** (13.333 × 7.5 in, PowerPoint's default widescreen).
- Each layout's regions (title box, body box, left/right boxes) are defined **once, in Go**.
- **Typst** gets the regions in `doc.json`, places the content in them, and **measures** it. If it doesn't fit, it shrinks the font step by step to the minimum, then reports the size it used, or an overflow.
- **The PPTX writer** gets **the same boxes plus the font sizes Typst chose**, and writes native text boxes, bullet paragraphs, tables and PNG diagrams at those exact positions. Because the fonts are metric-compatible, PowerPoint breaks lines where Typst did.
- PowerPoint's own "shrink text on overflow" is switched on as a safety net for text the user edits later.

### 4.3 Pipeline
```
slides (blocks, in Postgres)
   └─► layout (Go): regions per layout ─► doc.json
          └─► Typst slides template ─► positions + chosen font sizes + overflow report
                 ├─► PNG previews / PDF  (viewer, render_pages, export_pdf)
                 └─► deck.json (boxes, runs, tables, diagram PNGs, notes, theme colors)
                        └─► pptx writer (PptxGenJS, Node) ─► deck.pptx
```

### 4.4 MCP tools
- **New:**
  - `create_deck(title, theme?, mode?)`
  - `export_pptx(notebook_id)` returns a download URL, like `export_pdf` on the hosted server.
  - v2: `create_deck_from_notebook`.
- **Reused as is:**
  - `append_blocks`, `insert_blocks`, `update_block`, `move_blocks` and `delete_blocks`, with slides as blocks;
  - `get_outline`, which shows slide number, layout and title;
  - `render_pages`, which renders slides;
  - `check_layout`, which runs the slide checks on decks;
  - `export_pdf`, `undo`, `list_versions`, `set_theme`.
- **Guidance:**
  - The authoring guide gets a **"Slide rules"** section: one idea per slide, at most 6 bullets, about 40 words maximum, titles of 8 words or fewer, details in `notes`, a diagram or table in at least every third slide, and a section slide every 3–5 slides.
  - The server instructions get 3–4 lines about decks.

### 4.5 Viewer and web
- Pages render at 16:9 for decks; the page aspect becomes per-notebook instead of fixed A4.
- The contents sidebar lists slide titles.
- A **Download PPTX** button sits next to Download PDF.
- Dashboard cards show a "Deck" pill and a 16:9 thumbnail.

---

## 5. Build phases

| Phase | Delivers | Main work | Rough effort |
|---|---|---|---|
| **S1: Decks you can present** | `create_deck`, `slide` block with 6 layouts, 16:9 Typst template with fixed regions + auto-shrink + overflow report, slide checks, viewer at 16:9, PDF export, slide rules in the guide | `kind` column + migration; model `slide` + validation; `templates/slides.typ`; layout analysis for slides; viewer aspect; tools | 2–3 days |
| **S2: Real PowerPoint** | `export_pptx` + Download PPTX: editable text, bullets, tables, notes, theme colors, PNG diagrams, metric-compatible fonts | `deck.json` builder from the Typst measurements; `pptx/` Node script with PptxGenJS (pinned); Docker: add Node + script; HTTP route; tests | 2 days |
| **S3: Presenting & richer slides** | Presenter mode, charts (native in PPTX), notebook → deck, code colors, 4:3 | viewer presenter page; `chart` block (Typst preview + PPTX chart); conversion tool | 3+ days |

**Done means, for each phase:**
- tests pass on SQLite and Postgres;
- the isolation matrix covers the new tools;
- a sample deck is rendered and screenshotted;
- for S2, the `.pptx` is opened and checked in **PowerPoint, Keynote, Google Slides and LibreOffice**, recorded in a compatibility table.

---

## 6. How we'll test the PPTX
1. **Structure test (automatic):** unzip the generated `.pptx` and check the slide count, that every title and bullet text is present, that notes are present, that table cells match, and that images exist and are referenced.
2. **Geometry test (automatic):** text box positions in the PPTX equal the regions from Go; font sizes equal what Typst chose.
3. **Round-trip check (automatic, if available):** convert the PPTX to PDF with LibreOffice in CI (optional job; LibreOffice is too heavy for the production image) and compare the page count.
4. **Compatibility (manual, once per release):** open it in PowerPoint (Mac / Windows / web), Keynote and Google Slides, then check text fits, nothing is missing, it's editable, and notes are visible.

---

## 7. Risks and how we handle them

| Risk | Impact | Mitigation |
|---|---|---|
| Text breaks differently in PowerPoint than in our preview | Overflowing text in the exported deck | Metric-compatible fonts; keep ~10% spare room per region; PowerPoint shrink-on-overflow as a safety net |
| Google Slides import quirks | Missing diagrams | PNG only (no SVG); test imports every release |
| Node in the Docker image | ~+50 MB image; another runtime to patch | Pinned Node LTS + pinned PptxGenJS; the script is tiny; the interface allows swapping to pure Go (goppt) later |
| Slow server (free tier, 0.1 CPU) | Slide edits re-render slowly; PPTX export adds a Node start (~0.2 s) | Decks are short (10–20 pages); the performance plan's fixes apply here too; exports are rare |
| Scope creep (animations, templates, images) | Delays | Listed as "later" in Section 3 |
| Claude's built-in PPTX feature competes | "Why use Folio?" | Our edge: decks that **always fit**, themes, live preview, edits by ID with undo, Claude Code support, and the same content as notes or slides |

---

## 8. Example: what Claude would send

```json
[
 {"type":"slide","layout":"title","title":"REST vs gRPC","subtitle":"Choosing an API style · Team sync"},
 {"type":"slide","layout":"content","title":"Two ways services talk",
  "body":[{"type":"bullets","items":[{"text":"**REST**: resources as JSON over HTTP"},{"text":"**gRPC**: typed functions, Protobuf over HTTP/2"}]}],
  "notes":"Keep it simple; most of the team knows REST already."},
 {"type":"slide","layout":"two_column","title":"How a call flows",
  "left":[{"type":"diagram","source":"direction: right\nclient -> api -> db","size":"half"}],
  "right":[{"type":"numbered","items":[{"text":"Client sends a request"},{"text":"API runs the logic"},{"text":"DB returns rows"}]}]},
 {"type":"slide","layout":"content","title":"Side by side",
  "body":[{"type":"table","columns":["","REST","gRPC"],"rows":[["Payload","JSON","Protobuf"],["Streaming","Limited","Built in"],["Browser support","Native","Via proxy"]]}]},
 {"type":"slide","layout":"statement","text":"Public API → REST.  Internal services → gRPC.","caption":"Our rule of thumb"},
 {"type":"slide","layout":"closing","title":"Questions?"}
]
```

---

## 9. Decisions for you
1. **PPTX engine:** PptxGenJS via Node (proven, my pick) or pure Go (goppt: no Node, but young and with an unclear license)?
2. **Slide fonts:** Arial-compatible (Arimo) for exact PowerPoint fidelity (my pick), or keep Inter (prettier, but PowerPoint will substitute it and may overflow)?
3. **Scope of the first release:** S1 + S2 together (PDF and PPTX), or ship S1 first (decks + PDF) and add PPTX right after?
4. **Notebook → deck conversion:** v1 or v2?
5. **Hosting:** decks add CPU work. Should the performance plan come first, or in parallel?

---

Sources:
- [Anthropic: Claude can create files (Oct 2025)](https://anthropic.com/news/create-files)
- [Claude PPTX skill guide 2026](https://smartscope.blog/en/generative-ai/claude/claude-pptx-skill-practical-guide/)
- [PptxGenJS on GitHub (MIT, Node)](https://github.com/gitbrent/PptxGenJS)
- [goppt / gopresentation (Go)](https://pkg.go.dev/github.com/casibase/goppt)
- [unioffice (Go)](https://github.com/unidoc/unioffice)
- [Touying for Typst](https://maxbtroeger.substack.com/p/touying-an-extensible-presentation)
- [Marp CLI PPTX export notes](https://open-design.ai/blog/marp/)
- [Google Slides and SVG (third-party guide)](https://plusdocs.com/blog/guide-how-to-import-an-svg-into-google-slides)
- [PowerPoint SVG support](https://slidemodel.com/svg-in-powerpoint/)
