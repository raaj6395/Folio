package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/raaj6395/folio/internal/layout"
	"github.com/raaj6395/folio/internal/store"
)

type client struct {
	t  *testing.T
	cs *mcp.ClientSession
}

func newClient(t *testing.T) *client {
	home := t.TempDir()
	st, err := store.Open(filepath.Join(home, "folio.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	lb, err := layout.New(home)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	stp, ctp := mcp.NewInMemoryTransports()
	if _, err := New(st, lb, home).Connect(ctx, stp, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).Connect(ctx, ctp, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return &client{t, cs}
}

func (c *client) raw(name string, args any) *mcp.CallToolResult {
	c.t.Helper()
	res, err := c.cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		c.t.Fatalf("%s: %v", name, err)
	}
	return res
}

func (c *client) call(name string, args any, out any) {
	c.t.Helper()
	res := c.raw(name, args)
	if res.IsError {
		c.t.Fatalf("%s returned error: %s", name, res.Content[0].(*mcp.TextContent).Text)
	}
	if out != nil {
		js, _ := json.Marshal(res.StructuredContent)
		if err := json.Unmarshal(js, out); err != nil {
			c.t.Fatal(err)
		}
	}
}

func (c *client) callErr(name string, args any) string {
	c.t.Helper()
	res := c.raw(name, args)
	if !res.IsError {
		c.t.Fatalf("%s: expected an error", name)
	}
	return res.Content[0].(*mcp.TextContent).Text
}

func blocksArg(js string) []json.RawMessage {
	var bs []json.RawMessage
	if err := json.Unmarshal([]byte(js), &bs); err != nil {
		panic(err)
	}
	return bs
}

func TestM1Flow(t *testing.T) {
	c := newClient(t)
	var nb NotebookInfo
	c.call("create_notebook", map[string]any{"title": "REST APIs"}, &nb)
	if nb.Theme != "clean" || nb.Version != 1 {
		t.Fatalf("create: %+v", nb)
	}

	var res MutationResult
	c.call("append_blocks", map[string]any{"notebook_id": nb.ID, "blocks": blocksArg(`[
		{"type":"heading","level":1,"text":"REST"},
		{"type":"paragraph","text":"Resources and **verbs**."},
		{"type":"page_break"},
		{"type":"callout","variant":"tip","body":["Use nouns."]}
	]`)}, &res)
	if res.Version != 2 || len(res.ChangedBlockIDs) != 4 || res.Layout.PageCount != 2 || res.Layout.Blocks[3].Page != 2 {
		t.Fatalf("append: %+v", res)
	}

	msg := c.callErr("append_blocks", map[string]any{"notebook_id": nb.ID, "blocks": blocksArg(`[{"type":"paragraph","text":"ok"},{"type":"heading","level":7,"text":"x"}]`)})
	if !strings.Contains(msg, "block[1] (heading): level must be 1, 2 or 3") {
		t.Fatalf("validation message: %s", msg)
	}

	var out Outline
	c.call("get_outline", map[string]any{"notebook_id": nb.ID}, &out)
	if len(out.Blocks) != 4 || out.Version != 2 || out.Blocks[1].Summary != "Resources and verbs." || out.Blocks[3].Page != 2 {
		t.Fatalf("outline: %+v", out)
	}

	r := c.raw("render_pages", map[string]any{"notebook_id": nb.ID, "pages": []int{1, 2}})
	if r.IsError || len(r.Content) != 4 {
		t.Fatalf("render: %+v", r)
	}
	if img, ok := r.Content[1].(*mcp.ImageContent); !ok || !bytes.HasPrefix(img.Data, []byte("\x89PNG")) {
		t.Fatalf("render content: %T", r.Content[1])
	}
	if msg := c.callErr("render_pages", map[string]any{"notebook_id": nb.ID, "pages": []int{3}}); !strings.Contains(msg, "has 2 page(s)") {
		t.Fatal(msg)
	}

	var exp ExportOut
	c.call("export_pdf", map[string]any{"notebook_id": nb.ID}, &exp)
	if data, err := os.ReadFile(exp.Path); err != nil || !bytes.HasPrefix(data, []byte("%PDF")) || !strings.Contains(exp.Path, "rest-apis-") {
		t.Fatalf("export: %s %v", exp.Path, err)
	}

	var list map[string][]NotebookInfo
	c.call("list_notebooks", map[string]any{}, &list)
	if len(list["notebooks"]) != 1 || list["notebooks"][0].PageCount != 2 || list["notebooks"][0].BlockCount != 4 {
		t.Fatalf("list: %+v", list)
	}

	if msg := c.callErr("get_outline", map[string]any{"notebook_id": "n_nope"}); !strings.Contains(msg, "list_notebooks") {
		t.Fatal(msg)
	}
	g := c.raw("get_authoring_guide", map[string]any{})
	if !strings.Contains(g.Content[0].(*mcp.TextContent).Text, "Block types") {
		t.Fatal("guide missing")
	}
}

func (c *client) summaries(id string) string {
	var out Outline
	c.call("get_outline", map[string]any{"notebook_id": id}, &out)
	var s []string
	for _, b := range out.Blocks {
		s = append(s, b.Summary)
	}
	return strings.Join(s, "|")
}

func TestM2Editing(t *testing.T) {
	c := newClient(t)
	var nb NotebookInfo
	c.call("create_notebook", map[string]any{"title": "Edit"}, &nb)
	var res MutationResult
	c.call("append_blocks", map[string]any{"notebook_id": nb.ID, "blocks": blocksArg(`[
		{"type":"paragraph","text":"a"},{"type":"paragraph","text":"b"},{"type":"paragraph","text":"c"}]`)}, &res)
	a, b, cc := res.ChangedBlockIDs[0], res.ChangedBlockIDs[1], res.ChangedBlockIDs[2]

	c.call("insert_blocks", map[string]any{"notebook_id": nb.ID, "after_block_id": a, "blocks": blocksArg(`[{"type":"paragraph","text":"a2"}]`)}, &res)
	c.call("insert_blocks", map[string]any{"notebook_id": nb.ID, "after_block_id": "start", "blocks": blocksArg(`[{"type":"paragraph","text":"s"}]`)}, &res)
	if got := c.summaries(nb.ID); got != "s|a|a2|b|c" {
		t.Fatalf("after inserts: %s", got)
	}

	c.call("update_block", map[string]any{"notebook_id": nb.ID, "block_id": b, "block": json.RawMessage(`{"type":"heading","level":2,"text":"B"}`)}, &res)
	if got := c.summaries(nb.ID); got != "s|a|a2|H2 B|c" {
		t.Fatalf("after update: %s", got)
	}

	c.call("move_blocks", map[string]any{"notebook_id": nb.ID, "block_ids": []string{cc, a}, "after_block_id": "start"}, &res)
	if got := c.summaries(nb.ID); got != "a|c|s|a2|H2 B" {
		t.Fatalf("after move: %s", got)
	}
	if res.ChangedBlockIDs[0] != a {
		t.Fatalf("move should report document order: %v", res.ChangedBlockIDs)
	}

	c.call("delete_blocks", map[string]any{"notebook_id": nb.ID, "block_ids": []string{b}}, &res)
	if got := c.summaries(nb.ID); got != "a|c|s|a2" {
		t.Fatalf("after delete: %s", got)
	}
	if msg := c.callErr("delete_blocks", map[string]any{"notebook_id": nb.ID, "block_ids": []string{b}}); !strings.Contains(msg, "not found: "+b) {
		t.Fatal(msg)
	}

	var rb map[string][]ReadBlock
	c.call("read_blocks", map[string]any{"notebook_id": nb.ID, "from_block_id": cc, "count": 2}, &rb)
	if len(rb["blocks"]) != 2 || rb["blocks"][0].Block.ID != cc || rb["blocks"][0].Page != 1 {
		t.Fatalf("read: %+v", rb)
	}

	c.call("undo", map[string]any{"notebook_id": nb.ID}, &res)
	if got := c.summaries(nb.ID); got != "a|c|s|a2|H2 B" {
		t.Fatalf("undo delete: %s", got)
	}
	c.call("undo", map[string]any{"notebook_id": nb.ID}, &res)
	if got := c.summaries(nb.ID); got != "s|a|a2|H2 B|c" {
		t.Fatalf("undo move: %s", got)
	}

	var vs map[string][]map[string]any
	c.call("list_versions", map[string]any{"notebook_id": nb.ID, "limit": 3}, &vs)
	if len(vs["versions"]) != 3 || vs["versions"][0]["tool_name"] != "undo" {
		t.Fatalf("versions: %+v", vs)
	}
	c.call("restore_version", map[string]any{"notebook_id": nb.ID, "version": 2}, &res)
	if got := c.summaries(nb.ID); got != "a|b|c" {
		t.Fatalf("restore: %s", got)
	}

	stale := 1
	if msg := c.callErr("append_blocks", map[string]any{"notebook_id": nb.ID, "expected_version": stale, "blocks": blocksArg(`[{"type":"divider"}]`)}); !strings.Contains(msg, fmt.Sprintf("notebook is at version %d", res.Version)) {
		t.Fatal(msg)
	}
}
