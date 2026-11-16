package diagram

import (
	"context"
	"os"
	"strings"
	"testing"
)

func geom(t *testing.T, src string) *SVG {
	t.Helper()
	svg, err := Render(context.Background(), src, Style{Font: "Inter"})
	if err != nil {
		t.Fatal(err)
	}
	return svg
}

func fixture(t *testing.T, name string) *SVG {
	t.Helper()
	src, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return geom(t, string(src))
}

// full is a diagram drawn at its natural size capped to the 174mm column, as a full-size block.
func full(s *SVG) Placement {
	w := min(174, s.WidthPx*25.4/96*0.9)
	return Placement{MMPerUnit: w / s.WidthPx, AvailMM: 174}
}

func checks(fs []Finding) map[string]int {
	out := map[string]int{}
	for _, f := range fs {
		out[f.Check]++
	}
	return out
}

func TestGridFixture(t *testing.T) {
	s := fixture(t, "grid_ok.d2")
	g := s.Geom
	cell := g.node("g.c11")
	if cell == nil || cell.Grid != "g" || cell.Row != 1 || cell.Col != 1 || cell.Label != "(i, j)" {
		t.Fatalf("grid cell: %+v", cell)
	}
	if len(g.Edges) != 3 || !g.Edges[0].Straight || g.Edges[2].Straight {
		t.Fatalf("edges: %+v", g.Edges)
	}
	if fs := Inspect(g, full(s)); len(fs) != 0 {
		t.Fatalf("a clean grid has no findings: %+v", fs)
	}
	got := ASCII(g, 80, 30)
	if os.Getenv("FOLIO_UPDATE_GOLDEN") != "" {
		os.WriteFile("testdata/grid_ok.ascii", []byte(got+"\n"), 0o644)
	}
	want, _ := os.ReadFile("testdata/grid_ok.ascii")
	if got+"\n" != string(want) {
		t.Fatalf("ASCII changed (FOLIO_UPDATE_GOLDEN=1 to accept):\n%s", got)
	}
}

func TestWrongCell(t *testing.T) {
	s := fixture(t, "grid_wrong.d2")
	fs := Inspect(s.Geom, full(s))
	c := checks(fs)
	if c["wrong_cell"] != 1 {
		t.Fatalf("c10 -> c12 crosses c11: %+v", fs)
	}
	if !strings.Contains(fs[0].Message, "g.c11") {
		t.Fatalf("names the cell it seems to point at: %s", fs[0].Message)
	}
}

// D2's layouts never overlap nodes on their own, so this fixture is built by hand.
func TestNodeOverlap(t *testing.T) {
	g := &Geometry{W: 300, H: 200, Nodes: []Node{
		{ID: "a", Label: "A", Shape: "rectangle", Box: Box{10, 10, 120, 60}, FontPx: 16},
		{ID: "b", Label: "B", Shape: "rectangle", Box: Box{60, 40, 120, 60}, FontPx: 16},
		{ID: "c", Label: "C", Shape: "rectangle", Box: Box{200, 120, 60, 40}, FontPx: 16},
	}}
	if c := checks(Inspect(g, Placement{})); c["node_overlap"] != 1 || len(c) != 1 {
		t.Fatalf("a and b overlap, c is apart: %v", c)
	}
	g.Nodes[1].Box = Box{129.5, 10, 120, 60} // shares a 0.5-unit strip: under 2% of a's area
	if c := checks(Inspect(g, Placement{})); c["node_overlap"] != 0 {
		t.Fatalf("touching boxes are fine: %v", c)
	}
}

func TestLongLabelInSmallNode(t *testing.T) {
	s := geom(t, `a: "a label far too long for this node, which is only forty units wide and thirty tall" {width: 40; height: 30}`)
	small := Placement{MMPerUnit: 0.35 * 174 / s.WidthPx, AvailMM: 0.35 * 174}
	c := checks(Inspect(s.Geom, small))
	if c["label_overflow"] != 1 || c["tiny_text"] != 1 {
		t.Fatalf("want label_overflow and tiny_text: %v", c)
	}
}

// Sequence messages run between lifelines, nowhere near the actor boxes: not misrouted.
func TestSequenceDiagram(t *testing.T) {
	for _, src := range []string{
		"shape: sequence_diagram\nclient -> server: GET /users/42\nserver -> db: SELECT\ndb -> server: row\nserver -> client: 200 OK",
		"api: {shape: sequence_diagram; a -> b: call; b -> a: reply}\nx -> api",
	} {
		s := geom(t, src)
		if fs := Inspect(s.Geom, full(s)); len(fs) != 0 {
			t.Fatalf("%q: %+v", src, fs)
		}
		if !s.Geom.Edges[0].Sequence {
			t.Fatalf("message not marked: %+v", s.Geom.Edges[0])
		}
	}
}

func TestCellsAndBounds(t *testing.T) {
	cell := func(id, label, fill string, x float64) Node {
		return Node{ID: "g." + id, Label: label, Shape: "rectangle", Fill: fill, Grid: "g", Box: Box{x, 10, 40, 40}}
	}
	g := &Geometry{W: 300, H: 100, Nodes: []Node{
		{ID: "g", Shape: "rectangle", Container: true, Box: Box{0, 0, 210, 60}},
		cell("a", "start", "#2e7d32", 10), cell("b", "", "B5", 60), cell("c", "", "B5", 110), cell("d", "", "#f9a825", 160),
		{ID: "x", Shape: "rectangle", Box: Box{280, 80, 40, 40}},
	}}
	fs := Inspect(g, Placement{})
	if c := checks(fs); c["empty_cell"] != 1 || c["off_canvas"] != 1 || len(c) != 2 {
		t.Fatalf("d is highlighted but empty; x pokes out: %+v", fs)
	}
	if fs[0].ElementID != "g.d" || fs[1].ElementID != "x" {
		t.Fatalf("elements: %+v", fs)
	}
}

func TestEdgeChecks(t *testing.T) {
	g := &Geometry{W: 400, H: 300, Nodes: []Node{
		{ID: "a", Shape: "rectangle", Box: Box{10, 10, 50, 40}},
		{ID: "b", Shape: "rectangle", Box: Box{300, 10, 50, 40}},
		{ID: "m", Shape: "rectangle", Box: Box{150, 0, 50, 60}},
	}, Edges: []Edge{
		{ID: "e1", Src: "a", Dst: "b", Points: [][2]float64{{60, 30}, {300, 30}}},
		{ID: "e2", Src: "a", Dst: "b", Points: [][2]float64{{60, 30}, {290, 200}}},
		{ID: "e3", Src: "a", Dst: "b", Points: [][2]float64{{60, 30}, {420, 30}}},
	}}
	c := checks(Inspect(g, Placement{}))
	if c["edge_through_node"] != 2 || c["edge_misroute"] != 2 || c["off_canvas"] != 1 {
		t.Fatalf("e1 and e3 cross m, e2 misses b, e3 ends past b and off the canvas: %v", c)
	}
	for i := range 3 {
		y := float64(80 + 40*i)
		g.Edges = append(g.Edges, Edge{ID: "v", Src: "x", Dst: "y", Points: [][2]float64{{100, y - 30}, {100, y + 30}}},
			Edge{ID: "h", Src: "x", Dst: "y", Points: [][2]float64{{80, y}, {120, y}}})
	}
	if c := checks(Inspect(g, Placement{})); c["edge_crossing"] != 1 {
		t.Fatalf("three crossings are reported once: %v", c)
	}
}
