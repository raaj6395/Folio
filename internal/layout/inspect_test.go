package layout

import (
	"testing"

	"github.com/raaj6395/folio/internal/model"
)

func TestBlockBoxes(t *testing.T) {
	bs := []model.Block{{ID: "p", Type: "paragraph"}, {ID: "c", Type: "callout"}, {ID: "t", Type: "table"}}
	m := func(id, k string, page int, ymm float64) mark {
		return mark{ID: id, K: k, Page: page, Y: pt(ymm), X: pt(18), Pt: 10.5}
	}
	marks := []mark{
		m("p", "s", 1, 20), m("p", "e", 1, 60),
		m("c", "s", 1, 50), m("c", "e", 1, 80), // starts inside p: they overlap
		m("t", "s", 1, 200), m("t", "e", 3, 100), // runs over three pages
	}
	r := &Result{PageCount: 3}
	inspectLayout(r, marks, bs, nil, false)
	if len(r.Boxes) != 5 {
		t.Fatalf("p, c and three parts of t: %+v", r.Boxes)
	}
	c := r.Boxes[1]
	if c.X != round1(18-insetXMM) || c.Y != round1(50-BoxInsetMM) || c.H != round1(30+2*BoxInsetMM) || c.W != textWidthMM || c.FontPt != 10.5 {
		t.Fatalf("a callout's box includes its padding: %+v", c)
	}
	mid, last := r.Boxes[3], r.Boxes[4]
	if mid.Page != 2 || mid.Part != "2/3" || mid.Y != marginTopMM || mid.H != contentHeightMM || last.Page != 3 || last.H != 100-marginTopMM {
		t.Fatalf("page parts: %+v %+v", mid, last)
	}
	if len(r.Findings) != 1 || r.Findings[0].Check != "block_overlap" || r.Findings[0].BlockID != "p" || r.Findings[0].ElementID != "c" {
		t.Fatalf("p and c overlap: %+v", r.Findings)
	}
	if is := r.ExtraIssues(); len(is) != 1 || is[0].Check != "block_overlap" || penalty["block_overlap"] == 0 {
		t.Fatalf("check_layout sees it: %+v", is)
	}

	// Two columns: children are half width and don't overlap each other or their parent.
	two := []model.Block{{ID: "two", Type: "two_column", Data: []byte(`{"left":[{"id":"l","type":"paragraph","text":"x"}],"right":[{"id":"r","type":"paragraph","text":"y"}]}`)}}
	marks = []mark{m("two", "s", 1, 20), m("l", "s", 1, 20), m("l", "e", 1, 40), {ID: "r", K: "s", Page: 1, Y: pt(20), X: pt(18 + 83.5 + 7)}, m("r", "e", 1, 40), m("two", "e", 1, 40)}
	r = &Result{PageCount: 1}
	inspectLayout(r, marks, two, nil, false)
	if len(r.Boxes) != 3 || r.Boxes[1].W != 83.5 || r.Boxes[1].Parent != "two" || len(r.Findings) != 0 {
		t.Fatalf("two_column: %+v %+v", r.Boxes, r.Findings)
	}
}
