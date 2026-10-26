package layout

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/raaj6395/folio/internal/model"
)

const contentBottomMM = pageHeightMM - marginTopMM

type mark struct {
	ID   string  `json:"id"`
	K    string  `json:"k"`
	Page int     `json:"page"`
	Y    float64 `json:"y"`
	X    float64 `json:"x"`
}

func round1(f float64) float64 { return float64(int(f*10+0.5)) / 10 }

func fillPct(mm float64) float64 {
	return round1(min(100, max(0, (mm-marginTopMM)/contentHeightMM*100)))
}

var unbreakable = []string{"callout", "code", "table", "diagram", "qa", "fill_in", "two_column", "bullets", "numbered"}

func analyze(marks []mark, blocks []model.Block, density string) *Result {
	r := &Result{Blocks: []Pos{}, Warnings: []string{}}
	idx := map[string]int{}
	for _, b := range blocks {
		idx[b.ID] = len(r.Blocks)
		p := Pos{ID: b.ID, Type: b.Type}
		if b.Type == "heading" {
			var h model.Heading
			json.Unmarshal(b.Data, &h)
			p.Level = h.Level
		}
		r.Blocks = append(r.Blocks, p)
	}
	var endPage int
	var endY float64
	for _, m := range marks {
		mm := m.Y * 25.4 / 72
		if m.ID == "_end" {
			endPage, endY = m.Page, mm
			continue
		}
		i, ok := idx[m.ID]
		if !ok {
			if m.K == "s" {
				r.Nested = append(r.Nested, Pos{ID: m.ID, Page: m.Page, YMM: round1(mm)})
			}
			continue
		}
		p := &r.Blocks[i]
		if m.K == "s" {
			p.Page, p.YMM, p.XMM = m.Page, round1(mm), round1(m.X*25.4/72)
		} else {
			p.EndPage, p.EndYMM = m.Page, round1(mm)
		}
	}
	r.PageCount = endPage
	r.PageFill = make([]float64, endPage)
	for _, p := range r.Blocks {
		if p.Page == 0 || p.Type == "page_break" {
			continue
		}
		for pg := p.Page; pg < p.EndPage && pg <= endPage; pg++ {
			r.PageFill[pg-1] = 100
		}
		if p.EndPage >= 1 && p.EndPage <= endPage {
			r.PageFill[p.EndPage-1] = max(r.PageFill[p.EndPage-1], fillPct(p.EndYMM))
		}
	}
	if endPage > 0 {
		r.LastPageFill = r.PageFill[endPage-1]
		if r.LastPageFill == 0 {
			r.LastPageFill = fillPct(endY)
		}
	}
	r.Issues = checks(r, density)
	r.Warnings = warningsOf(r.Issues)
	return r
}

type Issue struct {
	Check   string   `json:"check"`
	Page    int      `json:"page"`
	Blocks  []string `json:"blocks,omitempty"`
	Problem string   `json:"problem"`
	Fix     string   `json:"fix"`
}

func (i Issue) String() string { return i.Problem + "; " + i.Fix }

func warningsOf(issues []Issue) []string {
	ws := []string{}
	for _, i := range issues {
		ws = append(ws, i.String())
	}
	return ws
}

func visibleGap(p, n Pos) float64 {
	g := n.YMM - p.EndYMM
	if insetTypes[p.Type] {
		g -= BoxInsetMM
	}
	if insetTypes[n.Type] {
		g -= BoxInsetMM
	}
	return g
}

func checks(r *Result, density string) []Issue {
	var is []Issue
	add := func(check string, page int, blocks []string, problem, fix string) {
		is = append(is, Issue{check, page, blocks, problem, fix})
	}
	bs := r.Blocks
	for i, p := range bs {
		if p.Type == "heading" && i+1 < len(bs) && bs[i+1].Page > p.Page && bs[i+1].Type != "page_break" {
			add("orphan_heading", p.Page, []string{p.ID}, fmt.Sprintf("heading %s is the last thing on page %d; its content starts on page %d", p.ID, p.Page, bs[i+1].Page),
				"keep it with its content: shorten something above it, or insert a page_break before it")
		}
		if p.EndYMM > contentBottomMM+1 {
			add("overflow", p.EndPage, []string{p.ID}, fmt.Sprintf("block %s (%s) runs past the bottom of page %d", p.ID, p.Type, p.EndPage),
				"it is taller than a page, so split it into smaller blocks")
		}
	}
	is = append(is, crampedIssues(bs, density)...)
	is = append(is, spillIssues(r)...)
	for pg := 1; pg < r.PageCount; pg++ {
		fill := r.PageFill[pg-1]
		lastOnPage := -1
		for i, p := range bs {
			if p.Page == pg || p.EndPage == pg {
				lastOnPage = i
			}
		}
		if lastOnPage >= 0 && bs[lastOnPage].Type == "page_break" {
			if fill < UnderFilledPct {
				add("under_filled", pg, []string{bs[lastOnPage].ID}, fmt.Sprintf("page %d is only %.0f%% full because page_break %s ends it", pg, fill, bs[lastOnPage].ID),
					"add content, move a block from the next section here, convert part of it to two_column, or delete that page_break")
			}
			continue
		}
		if fill < AlmostEmptyPct {
			add("almost_empty", pg, nil, fmt.Sprintf("page %d is almost empty (%.0f%% used) in the middle of the notebook", pg, fill),
				"likely a bad break: check for a page_break or an oversized block right after it")
			continue
		}
		if lastOnPage+1 >= len(bs) {
			continue
		}
		next := bs[lastOnPage+1]
		if fill < UnderFilledPct && next.Type == "heading" && next.Level == 1 && next.Page == pg+1 {
			add("under_filled", pg, []string{next.ID}, fmt.Sprintf("page %d is only %.0f%% full and the next page starts H1 %s", pg, fill, next.ID),
				"add content, convert part of the section to two_column, or merge it with the next section")
			continue
		}
		if fill >= PushedBlockFillPct {
			continue
		}
		h := next.EndYMM - next.YMM
		if next.EndPage > next.Page {
			h = contentBottomMM - next.YMM
		}
		if slices.Contains(unbreakable, next.Type) && next.Page == pg+1 {
			add("pushed_block", pg, []string{next.ID}, fmt.Sprintf("block %s (%s, %.0fmm tall) did not fit at the bottom of page %d and moved to page %d, leaving page %d %.0f%% empty", next.ID, next.Type, h, pg, pg+1, pg, 100-fill),
				"shorten it, split it, or move a smaller block before it")
		}
	}
	for _, p := range bs {
		if p.Type == "diagram" && p.Page == p.EndPage && p.EndYMM-p.YMM > OversizedDiagramFrac*contentHeightMM {
			add("oversized_diagram", p.Page, []string{p.ID}, fmt.Sprintf("diagram %s is %.0fmm tall, %.0f%% of the page", p.ID, p.EndYMM-p.YMM, (p.EndYMM-p.YMM)/contentHeightMM*100),
				`use size "half" inside a two_column next to its explanation, or add "direction: right" to the source`)
		}
	}
	is = append(is, monotoneIssues(r)...)
	return is
}

func crampedIssues(bs []Pos, density string) []Issue {
	if density == "compact" {
		return nil
	}
	var is []Issue
	byPage := map[int][]string{}
	var pages []int
	for i := 1; i < len(bs); i++ {
		p, n := bs[i-1], bs[i]
		if n.Page == 0 || p.EndPage != n.Page || crampedSkipPrev[p.Type] {
			continue
		}
		min := MinBoxGapMM
		switch {
		case n.Type == "heading" && n.Level == 2:
			min = MinHeadingGapMM
		case !crampedNext[n.Type]:
			continue
		}
		if visibleGap(p, n) < min {
			if byPage[n.Page] == nil {
				pages = append(pages, n.Page)
			}
			byPage[n.Page] = append(byPage[n.Page], n.ID)
		}
	}
	fix := `insert {"type":"spacer","size":"medium"} before them`
	if density == "" || density == "legacy" {
		fix = `switch this notebook to density "comfortable" with set_theme (it adds the gaps automatically), or ` + fix
	}
	for _, pg := range pages {
		ids := byPage[pg]
		is = append(is, Issue{"cramped", pg, ids, fmt.Sprintf("page %d: %d element(s) start with almost no space above them (%s)", pg, len(ids), strings.Join(ids, ", ")), fix})
	}
	return is
}

func spillIssues(r *Result) []Issue {
	var is []Issue
	bs := r.Blocks
	start := 0
	for i := 0; i <= len(bs); i++ {
		if i < len(bs) && !(bs[i].Type == "heading" && bs[i].Level == 1 && i > start) {
			continue
		}
		sec := bs[start:i]
		start = i
		first, last := -1, -1
		for j, p := range sec {
			if p.Page == 0 || p.Type == "page_break" {
				continue
			}
			if first < 0 {
				first = j
			}
			last = j
		}
		if first < 0 || sec[last].EndPage <= sec[first].Page {
			continue
		}
		lp := sec[last].EndPage
		part := fillPct(sec[last].EndYMM)
		if part >= SpillOverPct || startsAfterBreak(bs, lp) {
			continue
		}
		if i == len(bs) && lp == r.PageCount {
			is = append(is, Issue{"spill_over", lp, []string{sec[last].ID}, fmt.Sprintf("the last page (%d) is only %.0f%% full: a little content spilled over", lp, part),
				"trim or tighten something earlier to save a page, shrink a diagram, or keep writing"})
			continue
		}
		head := sec[first].ID
		is = append(is, Issue{"spill_over", lp, []string{head}, fmt.Sprintf("section %s spills only %.0f%% onto page %d", head, part, lp),
			"tighten spacing or shrink a diagram in that section so it fits on its own page"})
	}
	return is
}

func monotoneIssues(r *Result) []Issue {
	var is []Issue
	for pg := 1; pg <= r.PageCount; pg++ {
		run, best, paired := 0, 0, false
		var diagrams []string
		for _, p := range r.Blocks {
			if p.Page != pg || stackSkip[p.Type] {
				continue
			}
			if p.Type == "two_column" {
				paired = true
				run = 0
				continue
			}
			if p.Type == "diagram" {
				diagrams = append(diagrams, p.ID)
			}
			run++
			best = max(best, run)
		}
		if !paired && len(diagrams) > 0 && best >= MonotoneRun {
			is = append(is, Issue{"monotone_stack", pg, diagrams, fmt.Sprintf("page %d is one long top-to-bottom stack of %d full-width blocks", pg, best),
				`pair the diagram with its explanation in a two_column (diagram size "half" on one side, steps, bullets or a callout on the other)`})
		}
	}
	return is
}

type PageReport struct {
	Page    int     `json:"page"`
	FillPct float64 `json:"fill_pct"`
	Score   int     `json:"score"`
	Issues  []Issue `json:"issues"`
}

func (r *Result) Report() (int, []PageReport) {
	pages := make([]PageReport, r.PageCount)
	for i := range pages {
		pages[i] = PageReport{Page: i + 1, FillPct: r.PageFill[i], Issues: []Issue{}}
	}
	for _, is := range r.Issues {
		if is.Page >= 1 && is.Page <= r.PageCount {
			pages[is.Page-1].Issues = append(pages[is.Page-1].Issues, is)
		}
	}
	total := 0
	for i := range pages {
		s := 100.0
		for _, is := range pages[i].Issues {
			s -= penalty[is.Check]
		}
		if i+1 < r.PageCount && pages[i].FillPct < TargetFillPct {
			s -= (TargetFillPct - pages[i].FillPct) / 3
		}
		pages[i].Score = int(max(0, min(100, s)) + 0.5)
		total += pages[i].Score
	}
	if len(pages) == 0 {
		return 100, pages
	}
	return (total + len(pages)/2) / len(pages), pages
}

// With returns a copy of r with extra issues listed first, so they win the hint slots;
// r itself may be cached and stays untouched. Checks without a penalty don't lower the score.
func (r *Result) With(extra []Issue) *Result {
	if len(extra) == 0 {
		return r
	}
	c := *r
	c.Issues = slices.Concat(extra, r.Issues)
	c.Warnings = slices.Concat(warningsOf(extra), r.Warnings)
	return &c
}

func (r *Result) Hints() []string {
	var hs []string
	for _, is := range r.Issues {
		if len(hs) == MaxHints {
			break
		}
		h := fmt.Sprintf("p%d %s: %s", is.Page, is.Check, is.Fix)
		if len(is.Blocks) > 0 {
			h = fmt.Sprintf("p%d %s (%s): %s", is.Page, is.Check, strings.Join(is.Blocks, ", "), is.Fix)
		}
		hs = append(hs, h)
	}
	return hs
}

func startsAfterBreak(bs []Pos, page int) bool {
	for i, p := range bs {
		if p.Page == page && p.Type != "page_break" {
			return i > 0 && bs[i-1].Type == "page_break"
		}
	}
	return false
}
