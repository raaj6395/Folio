package layout

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/raaj6395/folio/internal/model"
	"github.com/raaj6395/folio/internal/store"
)

type at struct {
	id, typ            string
	level, page, endPg int
	y, endY            float64
}

func layoutOf(density string, xs ...at) *Result {
	var bs []model.Block
	var marks []mark
	for _, x := range xs {
		b := model.Block{ID: x.id, Type: x.typ}
		if x.typ == "heading" {
			b.Data = json.RawMessage(fmt.Sprintf(`{"level":%d,"text":"x"}`, x.level))
		}
		bs = append(bs, b)
		end := x.endPg
		if end == 0 {
			end = x.page
		}
		marks = append(marks, mark{x.id, "s", x.page, pt(x.y), 0}, mark{x.id, "e", end, pt(x.endY), 0})
	}
	last := xs[len(xs)-1]
	end := max(last.endPg, last.page)
	marks = append(marks, mark{"_end", "s", end, pt(last.endY), 0})
	return analyze(marks, bs, density)
}

func has(r *Result, check string) bool {
	for _, is := range r.Issues {
		if is.Check == check {
			return true
		}
	}
	return false
}

func TestChecks(t *testing.T) {
	cases := []struct {
		name, check string
		want        bool
		r           *Result
	}{
		{"cramped H2 in legacy", "cramped", true, layoutOf("legacy",
			at{id: "p", typ: "paragraph", page: 1, y: 20, endY: 60}, at{id: "h", typ: "heading", level: 2, page: 1, y: 64.8, endY: 70})},
		{"roomy H2 in comfortable", "cramped", false, layoutOf("comfortable",
			at{id: "p", typ: "paragraph", page: 1, y: 20, endY: 60}, at{id: "h", typ: "heading", level: 2, page: 1, y: 69.7, endY: 75})},
		{"tight H2 in compact is a choice", "cramped", false, layoutOf("compact",
			at{id: "p", typ: "paragraph", page: 1, y: 20, endY: 60}, at{id: "h", typ: "heading", level: 2, page: 1, y: 64.1, endY: 70})},
		{"table right after a paragraph", "cramped", true, layoutOf("",
			at{id: "p", typ: "paragraph", page: 1, y: 20, endY: 60}, at{id: "t", typ: "table", page: 1, y: 64.4, endY: 90})},
		{"callout gap measured outside its padding", "cramped", false, layoutOf("comfortable",
			at{id: "p", typ: "paragraph", page: 1, y: 20, endY: 60}, at{id: "c", typ: "callout", page: 1, y: 68, endY: 90})},

		{"page ended early by page_break", "under_filled", true, layoutOf("comfortable",
			at{id: "a", typ: "paragraph", page: 1, y: 20, endY: 90}, at{id: "pb", typ: "page_break", page: 1, y: 90, endY: 90}, at{id: "z", typ: "paragraph", page: 2, y: 20, endY: 30})},
		{"page well filled before page_break", "under_filled", false, layoutOf("comfortable",
			at{id: "a", typ: "paragraph", page: 1, y: 20, endY: 170}, at{id: "pb", typ: "page_break", page: 1, y: 170, endY: 170}, at{id: "z", typ: "paragraph", page: 2, y: 20, endY: 30})},
		{"short page before a new H1", "under_filled", true, layoutOf("comfortable",
			at{id: "a", typ: "paragraph", page: 1, y: 20, endY: 90}, at{id: "h", typ: "heading", level: 1, page: 2, y: 20, endY: 30}, at{id: "z", typ: "paragraph", page: 2, y: 30, endY: 40})},

		{"section spills a few lines onto a new page", "spill_over", true, layoutOf("comfortable",
			at{id: "h1", typ: "heading", level: 1, page: 1, y: 20, endY: 30}, at{id: "p", typ: "paragraph", page: 1, endPg: 2, y: 30, endY: 40},
			at{id: "pb", typ: "page_break", page: 2, y: 40, endY: 40}, at{id: "h2", typ: "heading", level: 1, page: 3, y: 20, endY: 30}, at{id: "q", typ: "paragraph", page: 3, y: 30, endY: 200})},
		{"section ends with a real part of a page", "spill_over", false, layoutOf("comfortable",
			at{id: "h1", typ: "heading", level: 1, page: 1, y: 20, endY: 30}, at{id: "p", typ: "paragraph", page: 1, endPg: 2, y: 30, endY: 140},
			at{id: "pb", typ: "page_break", page: 2, y: 140, endY: 140}, at{id: "h2", typ: "heading", level: 1, page: 3, y: 20, endY: 30}, at{id: "q", typ: "paragraph", page: 3, y: 30, endY: 200})},

		{"diagram over 40% of the page", "oversized_diagram", true, layoutOf("comfortable",
			at{id: "d", typ: "diagram", page: 1, y: 20, endY: 130})},
		{"diagram under 40% of the page", "oversized_diagram", false, layoutOf("comfortable",
			at{id: "d", typ: "diagram", page: 1, y: 20, endY: 100})},

		{"long stack with a diagram", "monotone_stack", true, layoutOf("comfortable",
			at{id: "h", typ: "heading", level: 2, page: 1, y: 20, endY: 30}, at{id: "p", typ: "paragraph", page: 1, y: 40, endY: 60},
			at{id: "d", typ: "diagram", page: 1, y: 66, endY: 120}, at{id: "q", typ: "paragraph", page: 1, y: 126, endY: 150})},
		{"diagram already paired in two_column", "monotone_stack", false, layoutOf("comfortable",
			at{id: "h", typ: "heading", level: 2, page: 1, y: 20, endY: 30}, at{id: "p", typ: "paragraph", page: 1, y: 40, endY: 60},
			at{id: "tc", typ: "two_column", page: 1, y: 66, endY: 120}, at{id: "d", typ: "diagram", page: 1, y: 126, endY: 150}, at{id: "q", typ: "paragraph", page: 1, y: 156, endY: 170})},
		{"long stack without a diagram", "monotone_stack", false, layoutOf("comfortable",
			at{id: "h", typ: "heading", level: 2, page: 1, y: 20, endY: 30}, at{id: "p", typ: "paragraph", page: 1, y: 40, endY: 60},
			at{id: "t", typ: "table", page: 1, y: 66, endY: 120}, at{id: "q", typ: "paragraph", page: 1, y: 126, endY: 150})},

		{"heading stranded at the bottom", "orphan_heading", true, layoutOf("comfortable",
			at{id: "p", typ: "paragraph", page: 1, y: 20, endY: 260}, at{id: "h", typ: "heading", level: 2, page: 1, y: 266, endY: 275}, at{id: "q", typ: "paragraph", page: 2, y: 20, endY: 200})},
		{"heading with its content", "orphan_heading", false, layoutOf("comfortable",
			at{id: "h", typ: "heading", level: 2, page: 1, y: 20, endY: 30}, at{id: "q", typ: "paragraph", page: 1, y: 30, endY: 200})},
	}
	for _, c := range cases {
		if got := has(c.r, c.check); got != c.want {
			t.Errorf("%s: %s = %v, want %v; issues: %v", c.name, c.check, got, c.want, c.r.Warnings)
		}
	}
}

func TestReportAndHints(t *testing.T) {
	clean := layoutOf("comfortable", at{id: "h", typ: "heading", level: 1, page: 1, y: 20, endY: 30}, at{id: "q", typ: "paragraph", page: 1, y: 30, endY: 200})
	if score, pages := clean.Report(); score != 100 || len(pages) != 1 || len(pages[0].Issues) != 0 || len(clean.Hints()) != 0 {
		t.Fatalf("clean notebook: %d %+v", score, pages)
	}
	bad := layoutOf("legacy",
		at{id: "p", typ: "paragraph", page: 1, y: 20, endY: 60}, at{id: "h", typ: "heading", level: 2, page: 1, y: 64.8, endY: 70},
		at{id: "d", typ: "diagram", page: 1, y: 76, endY: 190}, at{id: "q", typ: "paragraph", page: 1, y: 196, endY: 200})
	score, pages := bad.Report()
	if score >= 80 || pages[0].Score != score || len(pages[0].Issues) < 3 {
		t.Fatalf("bad notebook: %d %+v", score, pages)
	}
	hints := bad.Hints()
	if len(hints) == 0 || len(hints) > MaxHints || !strings.HasPrefix(hints[0], "p1 cramped (h): switch this notebook to density") {
		t.Fatalf("hints: %q", hints)
	}
}

func TestDensityAndTableRendering(t *testing.T) {
	b := builder(t)
	bs := blocks(t,
		`{"type":"paragraph","text":"Intro paragraph."}`,
		`{"type":"heading","level":2,"text":"Section"}`,
		`{"type":"table","columns":["Plan","Price"],"rows":[["Free","$0"],["Pro","$12"]]}`,
		`{"type":"two_column","left":[{"type":"heading","level":3,"text":"Pros"},{"type":"paragraph","text":"fast"}],"right":[{"type":"paragraph","text":"slow"}]}`,
	)
	gap := map[string]float64{}
	for _, d := range []string{"legacy", "comfortable"} {
		r, err := b.Build(context.Background(), store.Notebook{ID: "n_dens_" + d, Title: "d", Theme: "clean", Paper: "plain", Mode: "light", Version: 1, Density: d}, bs)
		if err != nil {
			t.Fatal(err)
		}
		gap[d] = r.Blocks[1].YMM - r.Blocks[0].EndYMM
		if x := r.Blocks[2].XMM; x < 17.5 || x > 18.5 {
			t.Errorf("%s: table starts at x=%.1fmm, want the 18mm text edge (not centered)", d, x)
		}
		if d == "comfortable" && has(r, "cramped") {
			t.Errorf("comfortable notebook flagged as cramped: %v", r.Warnings)
		}
	}
	if gap["comfortable"] < MinHeadingGapMM || gap["comfortable"] < gap["legacy"]+3 {
		t.Fatalf("gap before H2: legacy %.1fmm, comfortable %.1fmm", gap["legacy"], gap["comfortable"])
	}
}

func TestUnreadableDiagramIssue(t *testing.T) {
	b := builder(t)
	var chain []string
	for i := range 14 {
		chain = append(chain, fmt.Sprintf("n%d -> n%d: step %d", i, i+1, i))
	}
	src, _ := json.Marshal(strings.Join(chain, "\n"))
	for _, c := range []struct {
		block string
		want  bool
	}{
		{`{"type":"diagram","size":"small","source":` + string(src) + `}`, true},
		{`{"type":"diagram","size":"half","source":"a -> b"}`, false},
	} {
		bs := blocks(t, c.block)
		r, err := b.Build(context.Background(), store.Notebook{ID: model.NewID("n_"), Title: "d", Theme: "clean", Paper: "plain", Mode: "light", Version: 1, Density: "comfortable"}, bs)
		if err != nil {
			t.Fatal(err)
		}
		if has(r, "unreadable_diagram") != c.want {
			t.Errorf("%.40s: unreadable_diagram want %v, issues %v", c.block, c.want, r.Warnings)
		}
		for _, is := range r.Issues {
			if is.Check == "unreadable_diagram" && is.Page != 1 {
				t.Errorf("unreadable_diagram issue has page %d, want 1", is.Page)
			}
		}
	}
}
