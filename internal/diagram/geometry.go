package diagram

import (
	"math"
	"slices"
	"strings"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/label"
	"oss.terrastruct.com/d2/lib/shape"
)

// Geometry is where D2 put everything, in D2 units (SVG px at 96 dpi), measured from the
// top left of the rendered SVG (its viewBox, padding included).
type Geometry struct {
	W     float64 `json:"w"`
	H     float64 `json:"h"`
	Nodes []Node  `json:"nodes"`
	Edges []Edge  `json:"edges"`
}

type Box struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

type Node struct {
	ID    string `json:"id"`
	Label string `json:"label,omitempty"`
	Shape string `json:"shape"`
	Box
	Fill      string  `json:"fill,omitempty"`
	Stroke    string  `json:"stroke,omitempty"`
	FontPx    int     `json:"font_px"`
	FontPt    float64 `json:"font_size_pt_on_page,omitempty"` // set once the diagram is placed
	LabelBox  *Box    `json:"label_box,omitempty"`
	LabelOut  bool    `json:"label_outside,omitempty"` // D2 places the label outside or on the border on purpose
	Container bool    `json:"container,omitempty"`
	Grid      string  `json:"grid,omitempty"` // a grid cell: the grid container's ID
	Row       int     `json:"row,omitempty"`  // 0-based, in a grid or a sql_table
	Col       int     `json:"col,omitempty"`
	Cells     []Cell  `json:"cells,omitempty"` // sql_table rows
}

type Cell struct {
	Row  int    `json:"row"`
	Col  int    `json:"col"`
	Text string `json:"text"`
	Box
}

type Edge struct {
	ID       string       `json:"id"`
	Src      string       `json:"src"`
	Dst      string       `json:"dst"`
	Label    string       `json:"label,omitempty"`
	Points   [][2]float64 `json:"points"`
	Curve    bool         `json:"curve,omitempty"`    // points are cubic Bézier controls: p0, (c1, c2, p)…
	Straight bool         `json:"axis_aligned"`       // every segment is horizontal or vertical
	Sequence bool         `json:"sequence,omitempty"` // a message between lifelines in a sequence diagram
	FontPx   int          `json:"font_px,omitempty"`
	FontPt   float64      `json:"font_size_pt_on_page,omitempty"`
	Stroke   int          `json:"stroke_width"`
	LabelBox *Box         `json:"label_box,omitempty"`
}

func r1(f float64) float64 { return math.Round(f*10) / 10 }

func (b Box) round() Box { return Box{r1(b.X), r1(b.Y), r1(b.W), r1(b.H)} }

// geometry reads positions from a compiled diagram; ox, oy is the SVG viewBox origin.
func geometry(d *d2target.Diagram, g *d2graph.Graph, ox, oy, w, h float64) *Geometry {
	out := &Geometry{W: r1(w), H: r1(h), Nodes: []Node{}, Edges: []Edge{}}
	grids := map[string]string{} // cell ID -> grid ID
	var seqs []string            // sequence diagram IDs ("" is the whole board)
	if g != nil {
		for _, o := range append([]*d2graph.Object{g.Root}, g.Objects...) {
			if o.IsSequenceDiagram() {
				seqs = append(seqs, o.AbsID())
			}
		}
		for _, o := range g.Objects {
			if o.IsGridDiagram() {
				for _, c := range o.ChildrenArray {
					grids[c.AbsID()] = o.AbsID()
				}
			}
		}
	}
	parents := map[string]bool{}
	for _, s := range d.Shapes {
		if i := strings.LastIndex(s.ID, "."); i > 0 {
			parents[s.ID[:i]] = true
		}
	}
	for _, s := range d.Shapes {
		px, py := float64(s.Pos.X), float64(s.Pos.Y)
		n := Node{ID: s.ID, Label: strings.TrimSpace(s.Label), Shape: s.Type, Fill: s.Fill, Stroke: s.Stroke, FontPx: s.FontSize,
			Box: Box{px - ox, py - oy, float64(s.Width), float64(s.Height)}.round(), Container: parents[s.ID], Grid: grids[s.ID]}
		if n.Shape == "" {
			n.Shape = "rectangle"
		}
		if n.Label != "" && s.LabelWidth > 0 {
			pos := label.FromString(s.LabelPosition)
			if pos == label.Unset {
				pos = label.InsideMiddleCenter
			}
			sh := shape.NewShape(d2target.DSL_SHAPE_TO_SHAPE_TYPE[s.Type], geo.NewBox(geo.NewPoint(px, py), float64(s.Width), float64(s.Height)))
			box := sh.GetInnerBox()
			if pos.IsOutside() {
				box = sh.GetBox()
			}
			tl := pos.GetPointOnBox(box, label.PADDING, float64(s.LabelWidth), float64(s.LabelHeight))
			lb := Box{tl.X - ox, tl.Y - oy, float64(s.LabelWidth), float64(s.LabelHeight)}.round()
			n.LabelBox, n.LabelOut = &lb, pos.IsOutside() || pos.IsBorder()
		}
		if s.Type == d2target.ShapeSQLTable && len(s.SQLTable.Columns) > 0 {
			rowH := n.H / float64(len(s.SQLTable.Columns)+1)
			for i, c := range s.SQLTable.Columns {
				text := strings.TrimSpace(c.Name.Label + " " + c.Type.Label + " " + strings.Join(c.Constraint, " "))
				n.Cells = append(n.Cells, Cell{Row: i + 1, Text: text, Box: Box{n.X, n.Y + rowH*float64(i+1), n.W, rowH}.round()})
			}
		}
		out.Nodes = append(out.Nodes, n)
	}
	gridCells(out.Nodes)
	for _, c := range d.Connections {
		e := Edge{ID: c.ID, Src: c.Src, Dst: c.Dst, Label: strings.TrimSpace(c.Label), Curve: c.IsCurve, FontPx: c.FontSize, Stroke: c.StrokeWidth, Straight: true}
		for _, sq := range seqs {
			e.Sequence = e.Sequence || sq == "" || strings.HasPrefix(c.Src, sq+".")
		}
		for _, p := range c.Route {
			e.Points = append(e.Points, [2]float64{r1(p.X - ox), r1(p.Y - oy)})
		}
		for i := 1; i < len(e.Points); i++ {
			if a, b := e.Points[i-1], e.Points[i]; math.Abs(a[0]-b[0]) > 1 && math.Abs(a[1]-b[1]) > 1 {
				e.Straight = false
			}
		}
		if e.Label != "" && c.LabelWidth > 0 && len(c.Route) > 1 {
			tl := c.GetLabelTopLeft()
			lb := Box{tl.X - ox, tl.Y - oy, float64(c.LabelWidth), float64(c.LabelHeight)}.round()
			e.LabelBox = &lb
		}
		out.Edges = append(out.Edges, e)
	}
	return out
}

// gridCells numbers each grid's cells by row and column from their positions.
func gridCells(ns []Node) {
	byGrid := map[string][]int{}
	for i, n := range ns {
		if n.Grid != "" {
			byGrid[n.Grid] = append(byGrid[n.Grid], i)
		}
	}
	for _, idx := range byGrid {
		var xs, ys []float64
		for _, i := range idx {
			xs, ys = append(xs, ns[i].X), append(ys, ns[i].Y)
		}
		rank := func(vs []float64, v float64) int {
			slices.Sort(vs)
			vs = slices.CompactFunc(vs, func(a, b float64) bool { return math.Abs(a-b) < 2 })
			return slices.IndexFunc(vs, func(x float64) bool { return math.Abs(x-v) < 2 })
		}
		for _, i := range idx {
			ns[i].Row, ns[i].Col = rank(slices.Clone(ys), ns[i].Y), rank(slices.Clone(xs), ns[i].X)
		}
	}
}
