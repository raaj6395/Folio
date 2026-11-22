package sim

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/raaj6395/folio/internal/diagram"
)

// Card size the viewer draws each node at; D2 lays the cards out at exactly this size.
const (
	CardW = 224
	CardH = 68
)

// Layout is where the viewer draws everything, in px from the top left.
type Layout struct {
	W      float64 `json:"w"`
	H      float64 `json:"h"`
	Nodes  []Box   `json:"nodes"`
	Groups []Box   `json:"groups"`
	Edges  []Line  `json:"edges"`
}

type Box struct {
	ID string  `json:"id"` // node id, or group name
	X  float64 `json:"x"`
	Y  float64 `json:"y"`
	W  float64 `json:"w"`
	H  float64 `json:"h"`
}

type Line struct {
	Edge
	Points [][2]float64 `json:"points"`
	Curve  bool         `json:"curve,omitempty"` // points are cubic Bézier controls: p0, (c1, c2, p)…
}

var d2Shape = map[string]string{"database": "cylinder", "queue": "queue", "client": "oval"}

// D2 is the diagram source for a system. Node ids get an n_ prefix so none collides with a D2
// keyword (a node called "label" or "direction"); groups become containers g0, g1, … .
func D2(sp *Spec, edges []Edge) string {
	var b strings.Builder
	dir := sp.Direction
	if dir == "" {
		dir = "right"
	}
	fmt.Fprintf(&b, "direction: %s\n", dir)
	path := map[string]string{}
	groups := map[string]string{}
	var order []string
	for _, n := range sp.Nodes {
		if n.Group != "" {
			if _, ok := groups[n.Group]; !ok {
				groups[n.Group] = "g" + strconv.Itoa(len(groups))
				order = append(order, n.Group)
			}
			path[n.ID] = groups[n.Group] + ".n_" + n.ID
		} else {
			path[n.ID] = "n_" + n.ID
		}
	}
	shape := func(n Node) string {
		s := fmt.Sprintf("%s: %s {width: %d; height: %d", "n_"+n.ID, strconv.Quote(n.Name()), CardW, CardH)
		if sh := d2Shape[n.Kind]; sh != "" {
			s += "; shape: " + sh
		}
		return s + "}\n"
	}
	for _, g := range order {
		fmt.Fprintf(&b, "%s: %s {\n", groups[g], strconv.Quote(g))
		for _, n := range sp.Nodes {
			if n.Group == g {
				b.WriteString("  " + shape(n))
			}
		}
		b.WriteString("}\n")
	}
	for _, n := range sp.Nodes {
		if n.Group == "" {
			b.WriteString(shape(n))
		}
	}
	for _, e := range edges {
		fmt.Fprintf(&b, "%s -> %s", path[e.From], path[e.To])
		label := e.Label
		if e.Replicate && label == "" {
			label = "replicates"
		}
		if label != "" {
			fmt.Fprintf(&b, ": %s", strconv.Quote(label))
		}
		if e.Replicate {
			b.WriteString(" {style.stroke-dash: 4}")
		}
		b.WriteString("\n")
	}
	return b.String()
}

// Build lays the system out with D2 (cached in dir by source and style). It returns the layout
// and the path of D2's own SVG, which serves as the library thumbnail.
func Build(ctx context.Context, dir string, sp *Spec, st diagram.Style) (*Layout, string, error) {
	edges, _, err := Walk(sp)
	if err != nil {
		return nil, "", err
	}
	path, svg, err := diagram.Cached(ctx, dir, D2(sp, edges), st)
	if err != nil {
		return nil, "", err
	}
	g := svg.Geom
	out := &Layout{W: g.W, H: g.H, Nodes: []Box{}, Groups: []Box{}, Edges: []Line{}}
	groupName := map[string]string{} // D2 container id → group name, numbered like D2() does
	for _, n := range sp.Nodes {
		if n.Group == "" {
			continue
		}
		if !hasValue(groupName, n.Group) {
			groupName["g"+strconv.Itoa(len(groupName))] = n.Group
		}
	}
	for _, n := range g.Nodes {
		if id, ok := nodeID(n.ID); ok {
			out.Nodes = append(out.Nodes, Box{id, n.X, n.Y, n.W, n.H})
		} else if name, ok := groupName[n.ID]; ok {
			out.Groups = append(out.Groups, Box{name, n.X, n.Y, n.W, n.H})
		}
	}
	drawn := map[string]diagram.Edge{}
	for _, e := range g.Edges {
		from, _ := nodeID(e.Src)
		to, _ := nodeID(e.Dst)
		drawn[from+">"+to] = e
	}
	for _, e := range edges {
		if d, ok := drawn[e.From+">"+e.To]; ok {
			out.Edges = append(out.Edges, Line{Edge: e, Points: d.Points, Curve: d.Curve})
		}
	}
	return out, path, nil
}

// nodeID turns a D2 path like g0.n_app1 back into the node id app1.
func nodeID(path string) (string, bool) {
	return strings.CutPrefix(path[strings.LastIndex(path, ".")+1:], "n_")
}

func hasValue(m map[string]string, v string) bool {
	for _, x := range m {
		if x == v {
			return true
		}
	}
	return false
}
