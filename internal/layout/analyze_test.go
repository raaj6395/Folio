package layout

import (
	"strings"
	"testing"

	"github.com/raaj6395/folio/internal/model"
)

func pt(mm float64) float64 { return mm * 72 / 25.4 }

func TestWarnings(t *testing.T) {
	bs := []model.Block{{ID: "h", Type: "heading"}, {ID: "p", Type: "paragraph"}, {ID: "c", Type: "callout"}, {ID: "q", Type: "paragraph"}}
	marks := []mark{
		{"h", "s", 1, pt(20), 0}, {"h", "e", 1, pt(30), 0},
		{"p", "s", 1, pt(30), 0}, {"p", "e", 1, pt(150), 0},
		{"c", "s", 2, pt(20), 0}, {"c", "e", 2, pt(120), 0},
		{"q", "s", 3, pt(20), 0}, {"q", "e", 3, pt(25), 0},
		{"_end", "s", 3, pt(25), 0},
	}
	r := analyze(marks, bs, "")
	if r.PageCount != 3 || r.PageFill[0] != fillPct(150) || r.PageFill[1] != fillPct(120) {
		t.Fatalf("fill: %+v", r.PageFill)
	}
	want := "block c (callout, 100mm tall) did not fit at the bottom of page 1"
	if len(r.Warnings) != 2 || !strings.Contains(r.Warnings[1], want) || !strings.Contains(r.Warnings[0], "last page (3) is only 2% full") {
		t.Fatalf("warnings: %v", r.Warnings)
	}

	marks = []mark{{"h", "s", 1, pt(260), 0}, {"h", "e", 1, pt(270), 0}, {"p", "s", 2, pt(20), 0}, {"p", "e", 2, pt(22), 0}, {"c", "s", 3, pt(20), 0}, {"c", "e", 3, pt(290), 0}, {"_end", "s", 3, pt(290), 0}}
	r = analyze(marks, bs[:3], "")
	got := strings.Join(r.Warnings, "\n")
	for _, w := range []string{"heading h is the last thing on page 1", "page 2 is almost empty", "block c (callout) runs past the bottom of page 3"} {
		if !strings.Contains(got, w) {
			t.Errorf("missing warning %q in:\n%s", w, got)
		}
	}

	bs = []model.Block{{ID: "a", Type: "paragraph"}, {ID: "pb", Type: "page_break"}, {ID: "z", Type: "paragraph"}}
	marks = []mark{{"a", "s", 1, pt(20), 0}, {"a", "e", 1, pt(60), 0}, {"pb", "s", 1, pt(60), 0}, {"pb", "e", 1, pt(60), 0}, {"z", "s", 2, pt(20), 0}, {"z", "e", 2, pt(30), 0}, {"_end", "s", 2, pt(30), 0}}
	r = analyze(marks, bs, "")
	if len(r.Warnings) != 1 || !strings.Contains(r.Warnings[0], "page 1 is only 16% full because page_break pb ends it") {
		t.Fatalf("page_break warning: %v", r.Warnings)
	}
}

func TestWithLeavesCachedResultAlone(t *testing.T) {
	r := &Result{PageCount: 1, PageFill: []float64{90}, Issues: make([]Issue, 1, 4), Warnings: make([]string, 1, 4)}
	r.Issues[0] = Issue{Check: "cramped", Page: 1, Fix: "space it"}
	r.Warnings[0] = r.Issues[0].String()
	c := r.With([]Issue{{Check: "missing_section", Page: 1, Fix: "add it"}})
	if len(r.Issues) != 1 || len(r.Warnings) != 1 || cap(r.Issues) != 4 || r.Issues[:2][1].Check != "" {
		t.Fatalf("cached result changed: %+v", r)
	}
	if h := c.Hints(); len(h) != 2 || !strings.HasPrefix(h[0], "p1 missing_section") || len(c.Warnings) != 2 {
		t.Fatalf("extra issues come first: %v %v", h, c.Warnings)
	}
	if s, _ := c.Report(); s != 100-int(penalty["cramped"]) {
		t.Fatalf("checks without a penalty don't change the score: %d", s)
	}
	if r.With(nil) != r {
		t.Fatal("no extra issues: same result")
	}
}
