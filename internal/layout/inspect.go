package layout

import (
	"fmt"
	"slices"

	"github.com/raaj6395/folio/internal/diagram"
	"github.com/raaj6395/folio/internal/model"
)

type Finding = diagram.Finding

const (
	ptToMM      = 25.4 / 72
	columnGapMM = 7.0
	insetXMM    = 10 * ptToMM // callout and qa padding (10pt × 8pt)
)

// BlockBox is one block's box on one page, in mm from the page's top left. A block that runs
// over several pages has one box per page.
type BlockBox struct {
	ID     string    `json:"id"`
	Type   string    `json:"type"`
	Parent string    `json:"parent,omitempty"`
	Page   int       `json:"page"`
	X      float64   `json:"x"`
	Y      float64   `json:"y"`
	W      float64   `json:"w"`
	H      float64   `json:"h"`
	FontPt float64   `json:"font_pt,omitempty"`
	Part   string    `json:"part,omitempty"` // "2/3" when the block spans pages
	Code   *CodeInfo `json:"code,omitempty"`
}

type CodeInfo struct {
	Lines          int     `json:"lines"`
	WidestLineMM   float64 `json:"widest_line_mm"`   // unwrapped; longer lines wrap at spaces
	LongestTokenMM float64 `json:"longest_token_mm"` // can't wrap: clipped when wider than the box
	AvailMM        float64 `json:"available_mm"`
	Clipped        bool    `json:"clipped"`
}

// Placed is a diagram where it landed: its image box in mm and its D2 geometry.
type Placed struct {
	BlockID   string            `json:"block_id"`
	Page      int               `json:"page"`
	X         float64           `json:"x"`
	Y         float64           `json:"y"`
	W         float64           `json:"w"`
	H         float64           `json:"h"`
	MMPerUnit float64           `json:"mm_per_unit"`
	Geom      *diagram.Geometry `json:"geometry"`
}

type blockInfo struct{ typ, parent string }

func blockIndex(blocks []model.Block) map[string]blockInfo {
	idx := map[string]blockInfo{}
	for _, b := range blocks {
		idx[b.ID] = blockInfo{b.Type, ""}
		for _, k := range b.Children() {
			idx[k.ID] = blockInfo{k.Type, b.ID}
		}
	}
	return idx
}

func columnMM(parent string) float64 {
	if parent != "" {
		return (textWidthMM - columnGapMM) / 2
	}
	return textWidthMM
}

// inspectLayout measures block boxes, places diagrams and runs the geometry checks.
func inspectLayout(r *Result, marks []mark, blocks []model.Block, geoms map[string]*diagram.Geometry, canvas bool) {
	idx := blockIndex(blocks)
	starts, ends, code := map[string]mark{}, map[string]mark{}, map[string]mark{}
	var order []string
	for _, m := range marks {
		switch m.K {
		case "s":
			if _, seen := starts[m.ID]; !seen && m.ID != "_end" {
				starts[m.ID] = m
				order = append(order, m.ID)
			}
		case "e":
			ends[m.ID] = m
		case "code":
			code[m.ID] = m
		}
	}
	r.Boxes = nil
	for _, id := range order {
		s, e := starts[id], ends[id]
		bi, ok := idx[id]
		if !ok || bi.typ == "page_break" {
			continue
		}
		x, y, endY := s.X*ptToMM, s.Y*ptToMM, e.Y*ptToMM
		if insetTypes[bi.typ] {
			x, y, endY = x-insetXMM, y-BoxInsetMM, endY+BoxInsetMM
		}
		box := BlockBox{ID: id, Type: bi.typ, Parent: bi.parent, X: round1(x), W: round1(columnMM(bi.parent)), FontPt: round1(s.Pt)}
		if c, ok := code[id]; ok {
			box.Code = &CodeInfo{Lines: c.Lines, WidestLineMM: round1(c.Widest * ptToMM), LongestTokenMM: round1(c.Need * ptToMM), AvailMM: round1(c.Have * ptToMM), Clipped: c.Need > c.Have+1}
		}
		last := max(e.Page, s.Page)
		for pg := s.Page; pg <= last; pg++ {
			b := box
			b.Page = pg
			top, bottom := marginTopMM, contentBottomMM
			if pg == s.Page {
				top = y
			}
			if pg == last {
				bottom = endY
			}
			if canvas {
				bottom = endY
			}
			b.Y, b.H = round1(top), round1(max(0, bottom-top))
			if last > s.Page {
				b.Part = fmt.Sprintf("%d/%d", pg-s.Page+1, last-s.Page+1)
			}
			r.Boxes = append(r.Boxes, b)
		}
	}

	r.Findings = nil
	for i, a := range r.Boxes {
		for _, b := range r.Boxes[i+1:] {
			if a.Page != b.Page || a.ID == b.ID || a.ID == b.Parent || b.ID == a.Parent || a.H == 0 || b.H == 0 {
				continue
			}
			ow := min(a.X+a.W, b.X+b.W) - max(a.X, b.X)
			oh := min(a.Y+a.H, b.Y+b.H) - max(a.Y, b.Y)
			if ow > 0.5 && oh > 0.5 {
				r.Findings = append(r.Findings, Finding{Severity: "error", Check: "block_overlap", BlockID: a.ID, ElementID: b.ID, Page: a.Page,
					Message:      fmt.Sprintf("blocks %s (%s) and %s (%s) overlap by %.0f×%.0f mm on page %d", a.ID, a.Type, b.ID, b.Type, ow, oh, a.Page),
					SuggestedFix: "this is a renderer problem: report it, and meanwhile separate the blocks with a spacer"})
			}
		}
	}

	r.Diagrams = nil
	for _, m := range marks {
		g := geoms[m.ID]
		if m.K != "img" || g == nil || g.W == 0 {
			continue
		}
		wmm := m.W * ptToMM
		p := Placed{BlockID: m.ID, Page: m.Page, X: round1(m.X * ptToMM), Y: round1(m.Y * ptToMM), W: round1(wmm), H: round1(g.H * wmm / g.W), MMPerUnit: wmm / g.W, Geom: g}
		for i := range g.Nodes {
			g.Nodes[i].FontPt = round1(float64(g.Nodes[i].FontPx) * p.MMPerUnit / ptToMM)
		}
		for i := range g.Edges {
			g.Edges[i].FontPt = round1(float64(g.Edges[i].FontPx) * p.MMPerUnit / ptToMM)
		}
		avail := columnMM(idx[m.ID].parent)
		if canvas || max(p.H, r.Height(m.ID)) > OversizedDiagramFrac*contentHeightMM { // too tall is already oversized_diagram
			avail = 0
		}
		for _, f := range diagram.Inspect(g, diagram.Placement{MMPerUnit: p.MMPerUnit, AvailMM: avail}) {
			f.BlockID, f.Page = m.ID, m.Page
			f.Message = "diagram " + m.ID + ": " + f.Message
			r.Findings = append(r.Findings, f)
		}
		r.Diagrams = append(r.Diagrams, p)
	}
}

// ExtraIssues are the inspection findings (diagram geometry, block overlaps) as layout issues,
// for check_layout's report and score.
func (r *Result) ExtraIssues() []Issue {
	var is []Issue
	for _, f := range r.Findings {
		is = append(is, Issue{f.Check, f.Page, []string{f.BlockID}, f.Message, f.SuggestedFix})
	}
	return is
}

// Inspection names: check_layout's issue → the inspect_render check it is.
var inspectName = map[string]string{"overflow": "page_overflow", "wide_table": "clipped", "wide_code": "clipped", "oversized_diagram": "aspect_extreme"}

var errorChecks = []string{"page_overflow", "clipped", "block_overlap", "node_overlap", "edge_through_node", "wrong_cell", "edge_misroute", "off_canvas"}

func toFinding(is Issue) Finding {
	f := Finding{Check: is.Check, Page: is.Page, Message: is.Problem, SuggestedFix: is.Fix, Severity: "warning"}
	if n, ok := inspectName[f.Check]; ok {
		f.Check = n
	}
	if slices.Contains(errorChecks, f.Check) {
		f.Severity = "error"
	}
	if len(is.Blocks) > 0 {
		f.BlockID = is.Blocks[0]
	}
	return f
}

type Inspection struct {
	Version  int                 `json:"version"`
	Score    int                 `json:"score"`
	Pages    []PageInspection    `json:"pages"`
	Diagrams []DiagramInspection `json:"diagrams"`
	Findings []Finding           `json:"findings"`
	Trimmed  string              `json:"trimmed,omitempty"`
}

type PageInspection struct {
	Page    int        `json:"page"`
	FillPct float64    `json:"fill_pct"`
	Summary []string   `json:"summary,omitempty"` // detail "summary": one line per block
	Blocks  []BlockBox `json:"blocks,omitempty"`  // detail "full"
}

type DiagramInspection struct {
	BlockID string         `json:"block_id"`
	Page    int            `json:"page"`
	BoxMM   [4]float64     `json:"box_mm"` // x, y, w, h of the image on the page
	Canvas  CanvasInfo     `json:"canvas"`
	Counts  string         `json:"counts"`
	Nodes   []diagram.Node `json:"nodes,omitempty"` // detail "full"
	Edges   []diagram.Edge `json:"edges,omitempty"`
	ASCII   string         `json:"ascii"`
}

type CanvasInfo struct {
	W         float64 `json:"w"` // D2 units
	H         float64 `json:"h"`
	MMPerUnit float64 `json:"mm_per_unit"`
	WMM       float64 `json:"w_mm"`
	HMM       float64 `json:"h_mm"`
}

const asciiCols, asciiRows = 80, 30

// Inspect is the text-only view of the render: block boxes, diagram geometry with an ASCII
// map, and every finding. issues are the layout issues (check_layout's, template ones included).
// pages and ids narrow it down; nil means everything.
func (r *Result) Inspect(issues []Issue, score int, pages []int, ids []string, full bool) Inspection {
	want := func(page int, id, parent string) bool {
		return (len(pages) == 0 || slices.Contains(pages, page)) && (len(ids) == 0 || slices.Contains(ids, id) || parent != "" && slices.Contains(ids, parent))
	}
	out := Inspection{Score: score, Pages: []PageInspection{}, Diagrams: []DiagramInspection{}, Findings: []Finding{}}
	for pg := 1; pg <= r.PageCount; pg++ {
		if len(pages) > 0 && !slices.Contains(pages, pg) {
			continue
		}
		p := PageInspection{Page: pg, FillPct: r.PageFill[pg-1]}
		for _, b := range r.Boxes {
			if b.Page != pg || !want(pg, b.ID, b.Parent) {
				continue
			}
			if full {
				p.Blocks = append(p.Blocks, b)
				continue
			}
			line := fmt.Sprintf("%s %s: x %.0f y %.1f w %.0f h %.1f mm, %.1fpt", b.ID, b.Type, b.X, b.Y, b.W, b.H, b.FontPt)
			if b.Parent != "" {
				line += ", in " + b.Parent
			}
			if b.Part != "" {
				line += ", part " + b.Part
			}
			if c := b.Code; c != nil {
				line += fmt.Sprintf(", %d lines, widest %.0fmm of %.0fmm", c.Lines, c.WidestLineMM, c.AvailMM)
				if c.Clipped {
					line += ", CLIPPED"
				}
			}
			p.Summary = append(p.Summary, line)
		}
		out.Pages = append(out.Pages, p)
	}
	for _, d := range r.Diagrams {
		if !want(d.Page, d.BlockID, "") {
			continue
		}
		g := d.Geom
		di := DiagramInspection{BlockID: d.BlockID, Page: d.Page, BoxMM: [4]float64{d.X, d.Y, d.W, d.H},
			Canvas: CanvasInfo{g.W, g.H, round3(d.MMPerUnit), d.W, d.H},
			Counts: fmt.Sprintf("%d nodes, %d edges", len(g.Nodes), len(g.Edges)), ASCII: diagram.ASCII(g, asciiCols, asciiRows)}
		if full {
			di.Nodes, di.Edges = g.Nodes, g.Edges
		}
		out.Diagrams = append(out.Diagrams, di)
	}
	all := slices.Clone(r.Findings)
	for _, is := range issues {
		all = append(all, toFinding(is))
	}
	for _, f := range all {
		if want(f.Page, f.BlockID, "") || f.BlockID == "" && (len(pages) == 0 || slices.Contains(pages, f.Page)) && len(ids) == 0 {
			out.Findings = append(out.Findings, f)
		}
	}
	return out
}

func round3(f float64) float64 { return float64(int(f*1000+0.5)) / 1000 }
