package mcpserver

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/raaj6395/folio/internal/layout"
)

func (c *client) inspect(args map[string]any) layout.Inspection {
	c.t.Helper()
	res := c.raw("inspect_render", args)
	text := res.Content[0].(*mcp.TextContent).Text
	if res.IsError {
		c.t.Fatalf("inspect_render: %s", text)
	}
	var out layout.Inspection
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		c.t.Fatal(err)
	}
	return out
}

func findings(in layout.Inspection, check string) []layout.Finding {
	var out []layout.Finding
	for _, f := range in.Findings {
		if f.Check == check {
			out = append(out, f)
		}
	}
	return out
}

// Page 1 of "110. Minimum Path Sum" (n_yaltnucn), whose grid diagram has edge labels wider
// than the 30-unit gaps they sit in.
func TestInspectRenderMinPathSum(t *testing.T) {
	c := newClient(t)
	page1, err := os.ReadFile("testdata/minpath_page1.json")
	if err != nil {
		t.Fatal(err)
	}
	var nb NotebookInfo
	c.call("create_notebook", map[string]any{"title": "110. Minimum Path Sum", "density": "compact"}, &nb)
	var res MutationResult
	c.call("append_blocks", map[string]any{"notebook_id": nb.ID, "blocks": blocksArg(string(page1))}, &res)
	dia := res.ChangedBlockIDs[8]

	sum := c.inspect(map[string]any{"notebook_id": nb.ID})
	if len(sum.Diagrams) != 1 || sum.Diagrams[0].BlockID != dia || sum.Diagrams[0].Counts != "10 nodes, 5 edges" || sum.Diagrams[0].Nodes != nil {
		t.Fatalf("summary diagram: %+v", sum.Diagrams)
	}
	if a := sum.Diagrams[0].ASCII; !strings.Contains(a, "(i, j)") || !strings.Contains(a, "(i-1, j)") || !strings.Contains(a, "|****|") {
		t.Fatalf("ascii:\n%s", a)
	}
	// "path 1" and "from left" overlap both their cells and hide their 30-unit edges;
	// the dp label hides the short diagonal edge. "path 2" and "from top" fit their gaps.
	var msgs []string
	for _, f := range findings(sum, "label_overflow") {
		msgs = append(msgs, f.Message)
	}
	labels := strings.Join(msgs, "\n")
	if n := len(msgs); n != 7 || !strings.Contains(labels, `"path 1" of edge g.c00 -> g.c01 overlaps g.c00`) ||
		!strings.Contains(labels, `"from left" of edge g.c10 -> g.c11 is longer than the edge`) || !strings.Contains(labels, `"dp[i][j] = min cost (i,j) → end" of edge g.c11 -> g.c22 is longer`) ||
		strings.Contains(labels, "path 2") || strings.Contains(labels, "from top") || findings(sum, "label_overflow")[0].BlockID != dia {
		t.Fatalf("label findings (%d): %s", n, labels)
	}
	if len(findings(sum, "wrong_cell"))+len(findings(sum, "edge_misroute"))+len(findings(sum, "node_overlap")) != 0 {
		t.Fatalf("the grid's edges are routed right: %+v", sum.Findings)
	}
	if !strings.HasPrefix(sum.Pages[0].Summary[0], res.ChangedBlockIDs[0]+" heading: x 18 y 20.") || !strings.Contains(sum.Pages[0].Summary[0], "20.0pt") {
		t.Fatalf("heading line: %v", sum.Pages[0].Summary)
	}

	full := c.inspect(map[string]any{"notebook_id": nb.ID, "block_ids": []string{dia}, "detail": "full"})
	if len(full.Pages[0].Blocks) != 1 || full.Pages[0].Blocks[0].ID != dia || full.Pages[0].Summary != nil {
		t.Fatalf("block_ids narrows the boxes: %+v", full.Pages)
	}
	d := full.Diagrams[0]
	var cell *struct{ row, col int }
	for _, n := range d.Nodes {
		if n.ID == "g.c11" {
			cell = &struct{ row, col int }{n.Row, n.Col}
			if n.FontPt < 6 || n.LabelBox == nil || n.W == 0 {
				t.Fatalf("node: %+v", n)
			}
		}
	}
	if cell == nil || cell.row != 1 || cell.col != 1 || len(d.Edges) != 5 || d.Canvas.MMPerUnit == 0 || d.BoxMM[2] > 174 {
		t.Fatalf("full diagram: %+v", d)
	}

	var check CheckOut
	c.call("check_layout", map[string]any{"notebook_id": nb.ID}, &check)
	if check.Score >= 100 || !strings.Contains(js(check), "label_overflow") {
		t.Fatalf("check_layout counts diagram findings: %+v", check)
	}
	var info LayoutInfo
	c.call("get_layout_info", map[string]any{"notebook_id": nb.ID}, &info)
	if strings.Contains(js(info), "label_overflow") || strings.Contains(js(info), "boxes") {
		t.Fatal("get_layout_info is unchanged")
	}

	r := c.raw("render_pages", map[string]any{"notebook_id": nb.ID, "pages": []int{1}, "include_geometry": true})
	if last := r.Content[len(r.Content)-1].(*mcp.TextContent).Text; len(r.Content) != 3 || !strings.Contains(last, "label_overflow") {
		t.Fatalf("render_pages include_geometry: %d parts", len(r.Content))
	}
	if r := c.raw("render_pages", map[string]any{"notebook_id": nb.ID, "pages": []int{1}}); len(r.Content) != 2 {
		t.Fatal("render_pages without the flag is unchanged")
	}
}

func js(v any) string { b, _ := json.Marshal(v); return string(b) }

// Fixture 5: a table too wide for the page, and code with a token too long to wrap.
func TestInspectRenderClipped(t *testing.T) {
	c := newClient(t)
	var nb NotebookInfo
	c.call("create_notebook", map[string]any{"title": "Clip"}, &nb)
	cols, row := []string{}, []string{}
	for range 12 {
		cols, row = append(cols, "Characteristically"), append(row, "Characteristically")
	}
	table, _ := json.Marshal(map[string]any{"type": "table", "columns": cols, "rows": [][]string{row}})
	code := `{"type":"code","language":"go","code":"x := \"` + strings.Repeat("a", 140) + `\"\nfmt.Println(x)"}`
	var res MutationResult
	c.call("append_blocks", map[string]any{"notebook_id": nb.ID, "blocks": blocksArg(`[` + string(table) + `,` + code + `,{"type":"code","code":"short and fine"}]`)}, &res)
	in := c.inspect(map[string]any{"notebook_id": nb.ID})
	clipped := findings(in, "clipped")
	if len(clipped) != 2 || clipped[0].BlockID != res.ChangedBlockIDs[0] || clipped[1].BlockID != res.ChangedBlockIDs[1] || clipped[0].Severity != "error" {
		t.Fatalf("table and long code are clipped: %+v", in.Findings)
	}
	full := c.inspect(map[string]any{"notebook_id": nb.ID, "detail": "full"})
	for _, b := range full.Pages[0].Blocks {
		if b.Type == "code" && (b.Code == nil || b.Code.Lines != 2 && b.Code.Lines != 1 || b.Code.Clipped != (b.ID == res.ChangedBlockIDs[1])) {
			t.Fatalf("code info: %+v", b.Code)
		}
	}
}
