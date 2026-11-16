package diagram

import (
	"fmt"
	"math"
	"strings"
)

// Finding is one problem found from geometry alone.
type Finding struct {
	Severity     string `json:"severity"` // error, warning
	Check        string `json:"check"`
	BlockID      string `json:"block_id,omitempty"`
	ElementID    string `json:"element_id,omitempty"`
	Page         int    `json:"page,omitempty"`
	Message      string `json:"message"`
	SuggestedFix string `json:"suggested_fix"`
}

// Placement is how a diagram sits on the page.
type Placement struct {
	MMPerUnit float64 // on-page mm per D2 unit
	AvailMM   float64 // width the diagram may use (0: skip the narrowness check)
}

const (
	overlapFrac  = 0.02 // node_overlap: shared area over the smaller box
	edgeTol      = 1.0  // edge_misroute: D2 units between an end point and its node's outline
	maxCrossings = 2
	minLabelPt   = 6.0
	narrowFactor = 3.0
)

func (b Box) right() float64  { return b.X + b.W }
func (b Box) bottom() float64 { return b.Y + b.H }

// overlap is the area two boxes share.
func overlap(a, b Box) float64 {
	w := math.Min(a.right(), b.right()) - math.Max(a.X, b.X)
	h := math.Min(a.bottom(), b.bottom()) - math.Max(a.Y, b.Y)
	if w <= 0 || h <= 0 {
		return 0
	}
	return w * h
}

func (b Box) inset(d float64) Box { return Box{b.X + d, b.Y + d, b.W - 2*d, b.H - 2*d} }

func (b Box) contains(o Box) bool {
	return o.X >= b.X-0.5 && o.Y >= b.Y-0.5 && o.right() <= b.right()+0.5 && o.bottom() <= b.bottom()+0.5
}

// related: one is the other, or contains it (D2 IDs nest with dots).
func related(a, b string) bool {
	return a == b || strings.HasPrefix(a, b+".") || strings.HasPrefix(b, a+".")
}

func pt(px, mmPerUnit float64) float64 { return px * mmPerUnit * 72 / 25.4 }

// Path is the drawn line: Bézier controls are sampled so curves are checked as polylines.
func (e Edge) Path() [][2]float64 {
	if !e.Curve || len(e.Points) < 4 || (len(e.Points)-1)%3 != 0 {
		return e.Points
	}
	out := [][2]float64{e.Points[0]}
	for i := 1; i+2 < len(e.Points); i += 3 {
		p0, c1, c2, p1 := e.Points[i-1], e.Points[i], e.Points[i+1], e.Points[i+2]
		for s := 1; s <= 8; s++ {
			t := float64(s) / 8
			u := 1 - t
			out = append(out, [2]float64{
				u*u*u*p0[0] + 3*u*u*t*c1[0] + 3*u*t*t*c2[0] + t*t*t*p1[0],
				u*u*u*p0[1] + 3*u*u*t*c1[1] + 3*u*t*t*c2[1] + t*t*t*p1[1],
			})
		}
	}
	return out
}

// segBox: does segment p-q pass through the inside of box b (Liang-Barsky clipping)?
func segBox(p, q [2]float64, b Box) bool {
	t0, t1 := 0.0, 1.0
	dx, dy := q[0]-p[0], q[1]-p[1]
	for _, c := range [][2]float64{{-dx, p[0] - b.X}, {dx, b.right() - p[0]}, {-dy, p[1] - b.Y}, {dy, b.bottom() - p[1]}} {
		if c[0] == 0 {
			if c[1] < 0 {
				return false
			}
			continue
		}
		t := c[1] / c[0]
		if c[0] < 0 {
			t0 = math.Max(t0, t)
		} else {
			t1 = math.Min(t1, t)
		}
		if t0 > t1 {
			return false
		}
	}
	return t1-t0 > 1e-9
}

func cross(o, a, b [2]float64) float64 { return (a[0]-o[0])*(b[1]-o[1]) - (a[1]-o[1])*(b[0]-o[0]) }

// segsCross: proper crossing only (touching end points don't count).
func segsCross(p1, p2, q1, q2 [2]float64) bool {
	d1, d2 := cross(q1, q2, p1), cross(q1, q2, p2)
	d3, d4 := cross(p1, p2, q1), cross(p1, p2, q2)
	return d1*d2 < -1e-9 && d3*d4 < -1e-9
}

// boundaryDist is how far p is from the outline of b (0 on it).
func boundaryDist(p [2]float64, b Box) float64 {
	dx := math.Max(math.Max(b.X-p[0], 0), p[0]-b.right())
	dy := math.Max(math.Max(b.Y-p[1], 0), p[1]-b.bottom())
	if dx > 0 || dy > 0 {
		return math.Hypot(dx, dy)
	}
	return math.Min(math.Min(p[0]-b.X, b.right()-p[0]), math.Min(p[1]-b.Y, b.bottom()-p[1]))
}

// rectangular shapes have their bounding box as outline; other end points only need to be near it.
var rectangular = map[string]bool{"rectangle": true, "square": true, "sql_table": true, "class": true, "page": true, "text": true, "code": true, "image": true}

func (g *Geometry) node(id string) *Node {
	for i := range g.Nodes {
		if g.Nodes[i].ID == id {
			return &g.Nodes[i]
		}
	}
	return nil
}

func name(n Node) string {
	if n.Label != "" {
		return fmt.Sprintf("%s (%q)", n.ID, strings.ReplaceAll(n.Label, "\n", " "))
	}
	return n.ID
}

// Inspect runs every geometry check on one diagram.
func Inspect(g *Geometry, pl Placement) []Finding {
	var fs []Finding
	add := func(sev, check, el, msg, fix string) {
		fs = append(fs, Finding{Severity: sev, Check: check, ElementID: el, Message: msg, SuggestedFix: fix})
	}
	ns := g.Nodes
	leaf := func(n Node) bool { return !n.Container }

	for i := range ns {
		for j := i + 1; j < len(ns); j++ {
			a, b := ns[i], ns[j]
			if related(a.ID, b.ID) {
				continue
			}
			if o := overlap(a.Box, b.Box); o > overlapFrac*math.Min(a.W*a.H, b.W*b.H) {
				add("error", "node_overlap", a.ID, fmt.Sprintf("%s and %s overlap (%.0f%% of the smaller one)", name(a), name(b), 100*o/math.Min(a.W*a.H, b.W*b.H)),
					"remove the fixed position/size that forces them together, or put them in different containers")
			}
		}
	}

	type lbl struct {
		owner, what string
		edge        bool
		box         Box
	}
	var labels []lbl
	for _, n := range ns {
		if n.LabelBox == nil {
			continue
		}
		labels = append(labels, lbl{n.ID, "label of " + n.ID, false, *n.LabelBox})
		// An outside label is D2's choice, unless it is wider than the node: then D2 pushed it out.
		if !n.Container && (!n.LabelOut && !n.Box.contains(*n.LabelBox) || n.LabelOut && n.LabelBox.W > n.W+edgeTol) {
			add("warning", "label_overflow", n.ID, fmt.Sprintf("the label of %s (%.0f×%.0f) does not fit inside its %.0f×%.0f box", name(n), n.LabelBox.W, n.LabelBox.H, n.W, n.H),
				"shorten the label, break it with \\n, or remove the fixed width/height")
		}
	}
	for _, e := range g.Edges {
		if e.LabelBox == nil {
			continue
		}
		labels = append(labels, lbl{e.ID, fmt.Sprintf("label %q of edge %s -> %s", e.Label, e.Src, e.Dst), true, *e.LabelBox})
		// The label sits on the line with an opaque background: longer than the line, it hides it.
		p := e.Path()
		length, dx, dy := 0.0, math.Abs(p[len(p)-1][0]-p[0][0]), math.Abs(p[len(p)-1][1]-p[0][1])
		for k := 1; k < len(p); k++ {
			length += math.Hypot(p[k][0]-p[k-1][0], p[k][1]-p[k-1][1])
		}
		if span := math.Hypot(dx, dy); span > 0 && length < (dx*e.LabelBox.W+dy*e.LabelBox.H)/span {
			add("warning", "label_overflow", e.ID, fmt.Sprintf("the label %q of edge %s -> %s is longer than the edge (%.0f units), so it hides the line and its arrow", e.Label, e.Src, e.Dst, length),
				"shorten the label, or give the nodes more room (grid-gap, a different direction)")
		}
	}
	for i, l := range labels {
		for _, n := range ns {
			// A node's label may sit on its containers and children; an edge label on containers.
			if n.Container && (l.edge || related(l.owner, n.ID)) || !l.edge && related(l.owner, n.ID) {
				continue
			}
			if overlap(l.box, n.Box) > 1 {
				add("warning", "label_overflow", l.owner, fmt.Sprintf("the %s overlaps %s", l.what, name(n)),
					"shorten the label, move it to the other connection, or give the nodes more room (grid-gap, a different direction)")
			}
		}
		for _, m := range labels[i+1:] {
			if l.owner != m.owner && overlap(l.box, m.box) > 1 {
				add("warning", "label_overflow", l.owner, fmt.Sprintf("the %s overlaps the %s", l.what, m.what), "shorten one of the labels or spread the nodes apart")
			}
		}
	}

	var crossings []string
	for i, e := range g.Edges {
		path := e.Path()
		src, dst := g.node(e.Src), g.node(e.Dst)
		if e.Sequence { // messages run between lifelines, away from the actor boxes
			src, dst, path = nil, nil, path[:1]
		}
		for k := 1; k < len(path); k++ {
			for _, n := range ns {
				if related(n.ID, e.Src) || related(n.ID, e.Dst) || n.Container && (strings.HasPrefix(e.Src, n.ID+".") || strings.HasPrefix(e.Dst, n.ID+".")) {
					continue
				}
				if segBox(path[k-1], path[k], n.Box.inset(edgeTol)) {
					check, msg := "edge_through_node", fmt.Sprintf("edge %s -> %s runs through %s", e.Src, e.Dst, name(n))
					if n.Grid != "" && dst != nil && dst.Grid == n.Grid {
						check, msg = "wrong_cell", fmt.Sprintf("edge %s -> %s crosses cell %s (row %d, col %d) first, so it reads as pointing at that cell", e.Src, e.Dst, name(n), n.Row, n.Col)
					}
					add("error", check, e.ID, msg, "connect the cells that are next to each other, reorder the grid, or route the edge around (a different direction or container)")
					k = len(path)
					break
				}
			}
		}
		for _, end := range []struct {
			n    *Node
			p    [2]float64
			what string
		}{{src, path[0], "starts"}, {dst, path[len(path)-1], "ends"}} {
			if end.n == nil {
				continue
			}
			d := boundaryDist(end.p, end.n.Box)
			bad := d > edgeTol
			if !rectangular[end.n.Shape] {
				bad = boundaryDist(end.p, end.n.Box.inset(-edgeTol)) > 0 && !end.n.Box.inset(-edgeTol).contains(Box{end.p[0], end.p[1], 0, 0})
			}
			if bad {
				add("error", "edge_misroute", e.ID, fmt.Sprintf("edge %s -> %s %s %.0f units away from the outline of %s", e.Src, e.Dst, end.what, d, name(*end.n)),
					"check the edge's source and target names; remove manual positions that pull nodes away")
			}
		}
		for _, f := range g.Edges[i+1:] {
			fp := f.Path()
			for a := 1; a < len(path); a++ {
				for b := 1; b < len(fp); b++ {
					if segsCross(path[a-1], path[a], fp[b-1], fp[b]) {
						crossings = append(crossings, e.Src+"->"+e.Dst+" × "+f.Src+"->"+f.Dst)
					}
				}
			}
		}
	}
	if len(crossings) > maxCrossings {
		add("warning", "edge_crossing", "", fmt.Sprintf("%d edge crossings: %s", len(crossings), strings.Join(crossings, "; ")),
			"reorder the nodes, change direction, or group related nodes in a container")
	}

	if pl.MMPerUnit > 0 {
		for _, n := range ns {
			if n.Label != "" && leaf(n) {
				if p := pt(float64(n.FontPx), pl.MMPerUnit); p < minLabelPt {
					add("warning", "tiny_text", n.ID, fmt.Sprintf("the label of %s prints at %.1fpt (under %.0fpt is unreadable)", name(n), p, minLabelPt),
						"make the diagram smaller in content (fewer nodes, direction: down) or larger on the page (size full)")
				}
			}
		}
		for _, e := range g.Edges {
			if e.Label != "" {
				if p := pt(float64(e.FontPx), pl.MMPerUnit); p < minLabelPt {
					add("warning", "tiny_text", e.ID, fmt.Sprintf("edge label %q prints at %.1fpt", e.Label, p), "shorten the diagram or use size full")
				}
			}
		}
		// A small wide diagram is fine; a portrait one under a third of its column is a strip.
		if wmm := g.W * pl.MMPerUnit; pl.AvailMM > 0 && wmm*narrowFactor < pl.AvailMM && g.H > g.W {
			add("warning", "aspect_extreme", "", fmt.Sprintf("the diagram is %.0fmm wide in a %.0fmm column: a tall thin strip", wmm, pl.AvailMM),
				`add "direction: right", or put it in a two_column next to its explanation`)
		}
	}

	fills := map[string]map[string]int{} // grid -> fill -> count: the most common fill is the plain cell
	for _, n := range ns {
		if n.Grid != "" {
			if fills[n.Grid] == nil {
				fills[n.Grid] = map[string]int{}
			}
			fills[n.Grid][n.Fill]++
		}
	}
	for _, n := range ns {
		if n.Grid == "" || n.Label != "" {
			continue
		}
		plain, best := "", 0
		for f, c := range fills[n.Grid] {
			if c > best || c == best && f < plain {
				plain, best = f, c
			}
		}
		if n.Fill != plain {
			add("warning", "empty_cell", n.ID, fmt.Sprintf("highlighted cell %s (row %d, col %d) has no text", n.ID, n.Row, n.Col), "label it, or drop the highlight")
		}
	}

	for _, n := range ns {
		if n.X < -edgeTol || n.Y < -edgeTol || n.right() > g.W+edgeTol || n.bottom() > g.H+edgeTol {
			add("error", "off_canvas", n.ID, fmt.Sprintf("%s lies outside the diagram's bounds", name(n)), "remove manual positions (top/left) or near: values that push it out")
		}
	}
	for _, e := range g.Edges {
		for _, p := range e.Points {
			if p[0] < -edgeTol || p[1] < -edgeTol || p[0] > g.W+edgeTol || p[1] > g.H+edgeTol {
				add("error", "off_canvas", e.ID, fmt.Sprintf("edge %s -> %s leaves the diagram's bounds", e.Src, e.Dst), "check the edge's endpoints")
				break
			}
		}
	}
	return fs
}
