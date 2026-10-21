package layout

import (
	"fmt"
	"slices"

	"github.com/raaj6395/folio/internal/model"
)

const contentBottomMM = pageHeightMM - marginTopMM

type mark struct {
	ID   string  `json:"id"`
	K    string  `json:"k"`
	Page int     `json:"page"`
	Y    float64 `json:"y"`
}

func round1(f float64) float64 { return float64(int(f*10+0.5)) / 10 }

func fillPct(mm float64) float64 {
	return round1(min(100, max(0, (mm-marginTopMM)/contentHeightMM*100)))
}

var unbreakable = []string{"callout", "code", "table", "diagram", "qa", "fill_in", "two_column"}

func analyze(marks []mark, blocks []model.Block) *Result {
	r := &Result{Blocks: []Pos{}, Warnings: []string{}}
	idx := map[string]int{}
	for _, b := range blocks {
		idx[b.ID] = len(r.Blocks)
		r.Blocks = append(r.Blocks, Pos{ID: b.ID, Type: b.Type})
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
			continue
		}
		p := &r.Blocks[i]
		if m.K == "s" {
			p.Page, p.YMM = m.Page, round1(mm)
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
	r.Warnings = warnings(r)
	return r
}

func warnings(r *Result) []string {
	ws := []string{}
	bs := r.Blocks
	for i, p := range bs {
		if p.Type == "heading" && i+1 < len(bs) && bs[i+1].Page > p.Page && bs[i+1].Type != "page_break" {
			ws = append(ws, fmt.Sprintf("heading %s is the last thing on page %d; its content starts on page %d", p.ID, p.Page, bs[i+1].Page))
		}
		if p.EndYMM > contentBottomMM+1 {
			ws = append(ws, fmt.Sprintf("block %s (%s) runs past the bottom of page %d; it is taller than a page, so split it into smaller blocks", p.ID, p.Type, p.EndPage))
		}
	}
	for pg := 1; pg < r.PageCount; pg++ {
		fill := r.PageFill[pg-1]
		lastOnPage := -1
		for i, p := range bs {
			if p.Page == pg || p.EndPage == pg {
				lastOnPage = i
			}
		}
		if lastOnPage >= 0 && bs[lastOnPage].Type == "page_break" {
			if fill < 30 {
				ws = append(ws, fmt.Sprintf("page %d is only %.0f%% full because page_break %s ends it; consider deleting that page_break", pg, fill, bs[lastOnPage].ID))
			}
			continue
		}
		if fill < 5 {
			ws = append(ws, fmt.Sprintf("page %d is almost empty (%.0f%% used) in the middle of the notebook; likely a bad break", pg, fill))
			continue
		}
		if fill >= 70 || lastOnPage+1 >= len(bs) {
			continue
		}
		next := bs[lastOnPage+1]
		h := next.EndYMM - next.YMM
		if next.EndPage > next.Page {
			h = contentBottomMM - next.YMM
		}
		if slices.Contains(unbreakable, next.Type) && next.Page == pg+1 {
			ws = append(ws, fmt.Sprintf("block %s (%s, %.0fmm tall) did not fit at the bottom of page %d and moved to page %d, leaving page %d %.0f%% empty; shorten it, split it, or move a smaller block before it", next.ID, next.Type, h, pg, pg+1, pg, 100-fill))
		}
	}
	return ws
}
