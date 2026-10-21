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
		{"h", "s", 1, pt(20)}, {"h", "e", 1, pt(30)},
		{"p", "s", 1, pt(30)}, {"p", "e", 1, pt(150)},
		{"c", "s", 2, pt(20)}, {"c", "e", 2, pt(120)},
		{"q", "s", 3, pt(20)}, {"q", "e", 3, pt(25)},
		{"_end", "s", 3, pt(25)},
	}
	r := analyze(marks, bs)
	if r.PageCount != 3 || r.PageFill[0] != fillPct(150) || r.PageFill[1] != fillPct(120) {
		t.Fatalf("fill: %+v", r.PageFill)
	}
	want := "block c (callout, 100mm tall) did not fit at the bottom of page 1"
	if len(r.Warnings) != 1 || !strings.Contains(r.Warnings[0], want) {
		t.Fatalf("warnings: %v", r.Warnings)
	}

	marks = []mark{{"h", "s", 1, pt(260)}, {"h", "e", 1, pt(270)}, {"p", "s", 2, pt(20)}, {"p", "e", 2, pt(22)}, {"c", "s", 3, pt(20)}, {"c", "e", 3, pt(290)}, {"_end", "s", 3, pt(290)}}
	r = analyze(marks, bs[:3])
	got := strings.Join(r.Warnings, "\n")
	for _, w := range []string{"heading h is the last thing on page 1", "page 2 is almost empty", "block c (callout) runs past the bottom of page 3"} {
		if !strings.Contains(got, w) {
			t.Errorf("missing warning %q in:\n%s", w, got)
		}
	}

	bs = []model.Block{{ID: "a", Type: "paragraph"}, {ID: "pb", Type: "page_break"}, {ID: "z", Type: "paragraph"}}
	marks = []mark{{"a", "s", 1, pt(20)}, {"a", "e", 1, pt(60)}, {"pb", "s", 1, pt(60)}, {"pb", "e", 1, pt(60)}, {"z", "s", 2, pt(20)}, {"z", "e", 2, pt(30)}, {"_end", "s", 2, pt(30)}}
	r = analyze(marks, bs)
	if len(r.Warnings) != 1 || !strings.Contains(r.Warnings[0], "page 1 is only 16% full because page_break pb ends it") {
		t.Fatalf("page_break warning: %v", r.Warnings)
	}
}
