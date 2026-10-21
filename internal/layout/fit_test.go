package layout

import (
	"testing"

	"github.com/raaj6395/folio/internal/diagram"
)

func TestFit(t *testing.T) {
	w, h, pt := Fit(&diagram.SVG{WidthPx: 200, HeightPx: 100}, "full")
	if w != round1(200*pxToMM*maxUpscale) || pt != round1(d2LabelPt*maxUpscale) || h != round1(w/2) {
		t.Fatalf("small diagram must not be blown up: %v %v %v", w, h, pt)
	}
	w, _, _ = Fit(&diagram.SVG{WidthPx: 2000, HeightPx: 500}, "half")
	if w != textWidthMM/2 {
		t.Fatalf("half width: %v", w)
	}
	_, h, _ = Fit(&diagram.SVG{WidthPx: 400, HeightPx: 2000}, "full")
	if h > maxDiagramHeight+0.1 {
		t.Fatalf("tall diagram not capped: %v", h)
	}
	_, _, pt = Fit(&diagram.SVG{WidthPx: 6000, HeightPx: 300}, "full")
	if pt >= minReadablePt {
		t.Fatalf("very wide diagram should be unreadable, got %vpt", pt)
	}
}
