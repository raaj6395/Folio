package sim

import (
	"context"
	"testing"

	"github.com/raaj6395/folio/internal/diagram"
)

func TestLayoutCoversEveryNodeAndEdge(t *testing.T) {
	for _, d := range Demos() {
		sp := demo(t, d.Name)
		lay, svg, err := Build(context.Background(), t.TempDir(), sp, diagram.Style{Font: "Inter"})
		if err != nil {
			t.Fatal(d.Name, err)
		}
		edges, _, _ := Walk(sp)
		if len(lay.Nodes) != len(sp.Nodes) || len(lay.Edges) != len(edges) || svg == "" {
			t.Fatalf("%s: %d/%d nodes, %d/%d edges", d.Name, len(lay.Nodes), len(sp.Nodes), len(lay.Edges), len(edges))
		}
		for _, e := range lay.Edges {
			if len(e.Points) < 2 {
				t.Fatalf("%s: edge %s→%s has no route", d.Name, e.From, e.To)
			}
		}
		for _, b := range lay.Nodes {
			if b.W != CardW || b.H != CardH || b.X < 0 || b.Y < 0 || b.X+b.W > lay.W || b.Y+b.H > lay.H {
				t.Fatalf("%s: node %s box %+v outside %vx%v", d.Name, b.ID, b, lay.W, lay.H)
			}
		}
		groups := map[string]bool{}
		for _, n := range sp.Nodes {
			if n.Group != "" {
				groups[n.Group] = true
			}
		}
		if len(lay.Groups) != len(groups) {
			t.Fatalf("%s: %d groups drawn, %d in spec", d.Name, len(lay.Groups), len(groups))
		}
		t.Logf("%s: %.0fx%.0f, first edge curve=%v points=%d", d.Name, lay.W, lay.H, lay.Edges[0].Curve, len(lay.Edges[0].Points))
	}
}
