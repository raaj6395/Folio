package mcpserver

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/raaj6395/folio/internal/sim"
)

func demoBlock(t *testing.T, name string) map[string]any {
	t.Helper()
	for _, d := range sim.Demos() {
		if d.Name == name {
			var m map[string]any
			json.Unmarshal(d.JSON, &m)
			m["type"] = "system"
			return m
		}
	}
	t.Fatalf("no demo %s", name)
	return nil
}

func TestSystemNotebook(t *testing.T) {
	c := newClient(t)
	var nb NotebookInfo
	c.call("create_notebook", map[string]any{"title": "Read path", "format": "system"}, &nb)
	if nb.Format != "system" {
		t.Fatalf("format %q", nb.Format)
	}
	if msg := c.callErr("create_notebook", map[string]any{"title": "x", "format": "system", "template": "topic"}); !strings.Contains(msg, "no template") {
		t.Fatal(msg)
	}

	var res struct {
		MutationResult
		System *sim.Summary `json:"system"`
	}
	block := demoBlock(t, "read_path")
	c.call("append_blocks", map[string]any{"notebook_id": nb.ID, "blocks": []any{block}}, &res)
	if res.System == nil || res.System.Nodes != 6 || res.System.Scenarios != "5/5 pass" || len(res.Warnings) != 0 {
		t.Fatalf("write result: %+v %+v", res.MutationResult, res.System)
	}
	id := res.ChangedBlockIDs[0]

	var out SimulateOut
	c.call("simulate_system", map[string]any{"notebook_id": nb.ID, "request": "read", "kill": []string{"redis"}}, &out)
	if out.Status != 200 || !strings.Contains(strings.Join(out.Log, "\n"), "Redis is down") || !strings.Contains(strings.Join(out.Visited, ","), "pg") {
		t.Fatalf("simulate: %+v", out)
	}
	c.call("simulate_system", map[string]any{"notebook_id": nb.ID, "request": "read", "kill": []string{"pg"}, "warm": true}, &out)
	if out.Status != 200 {
		t.Fatalf("warm cache should survive Postgres down: %+v", out)
	}
	if msg := c.callErr("simulate_system", map[string]any{"notebook_id": nb.ID, "request": "read", "kill": []string{"mongo"}}); !strings.Contains(msg, "neither a node nor a group") {
		t.Fatal(msg)
	}

	// Format rules: one system block per system notebook, none anywhere else.
	if msg := c.callErr("append_blocks", map[string]any{"notebook_id": nb.ID, "blocks": blocksArg(`[{"type":"paragraph","text":"hi"}]`)}); !strings.Contains(msg, "exactly one block") {
		t.Fatal(msg)
	}
	if msg := c.callErr("set_theme", map[string]any{"notebook_id": nb.ID, "theme": "clean", "format": "a4"}); !strings.Contains(msg, "only goes in a system notebook") {
		t.Fatal(msg)
	}
	var a4 NotebookInfo
	c.call("create_notebook", map[string]any{"title": "Notes"}, &a4)
	if msg := c.callErr("append_blocks", map[string]any{"notebook_id": a4.ID, "blocks": []any{block}}); !strings.Contains(msg, "only goes in a system notebook") {
		t.Fatal(msg)
	}

	// Page tools refuse clearly; reading tools work.
	for _, tool := range []string{"render_pages", "check_layout", "get_layout_info", "inspect_render", "export_pdf"} {
		args := map[string]any{"notebook_id": nb.ID}
		if tool == "render_pages" {
			args["pages"] = []int{1}
		}
		if msg := c.callErr(tool, args); !strings.Contains(msg, "system notebook") {
			t.Fatalf("%s: %s", tool, msg)
		}
	}
	var outline Outline
	c.call("get_outline", map[string]any{"notebook_id": nb.ID}, &outline)
	if len(outline.Blocks) != 1 || !strings.Contains(outline.Blocks[0].Summary, "6 nodes") {
		t.Fatalf("outline: %+v", outline)
	}
	js, _ := json.Marshal(c.raw("read_blocks", map[string]any{"notebook_id": nb.ID, "block_ids": []string{id}}).StructuredContent)
	if !strings.Contains(string(js), `"engine":1`) || !strings.Contains(string(js), `"get_product"`) {
		t.Fatalf("read back: %s", js)
	}

	// A broken update is refused with a path, and nothing is written.
	bad := demoBlock(t, "read_path")
	bad["handlers"].(map[string]any)["get_product"].([]any)[1].(map[string]any)["call"] = "postgres"
	if msg := c.callErr("update_block", map[string]any{"notebook_id": nb.ID, "block_id": id, "block": bad}); !strings.Contains(msg, `handlers.get_product[1].call: unknown node "postgres"`) {
		t.Fatal(msg)
	}

	// A good update, then undo back to the read path.
	c.call("update_block", map[string]any{"notebook_id": nb.ID, "block_id": id, "block": demoBlock(t, "url_shortener")}, &res)
	if res.System.Nodes != 9 {
		t.Fatalf("update: %+v", res.System)
	}
	c.call("undo", map[string]any{"notebook_id": nb.ID}, &res)
	if res.System.Nodes != 6 {
		t.Fatalf("undo: %+v", res.System)
	}

	if text := c.raw("get_authoring_guide", map[string]any{"topic": "system"}).Content[0].(*mcp.TextContent).Text; !strings.HasPrefix(text, "# System notebooks") {
		t.Fatal("system guide topic")
	}
}
