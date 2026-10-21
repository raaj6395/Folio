package httpserver

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/raaj6395/folio/internal/layout"
	"github.com/raaj6395/folio/internal/store"
)

func TestIsLoopback(t *testing.T) {
	for addr, want := range map[string]bool{
		"127.0.0.1:8080": true, "localhost:8080": true, "[::1]:8080": true,
		"0.0.0.0:8080": false, ":8080": false, "192.168.1.5:8080": false, "example.com:80": false,
	} {
		if got := IsLoopback(addr); got != want {
			t.Errorf("IsLoopback(%q) = %v, want %v", addr, got, want)
		}
	}
}

func TestMCPOverHTTP(t *testing.T) {
	t.Run("sqlite", func(t *testing.T) {
		st, err := store.Open(filepath.Join(t.TempDir(), "folio.db"))
		if err != nil {
			t.Fatal(err)
		}
		defer st.Close()
		run(t, st)
	})
	t.Run("postgres", func(t *testing.T) { run(t, store.OpenTestPostgres(t)) })
}

func run(t *testing.T, st *store.Store) {
	home := t.TempDir()
	lb, err := layout.New(home)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(nil)
	srv.Start()
	defer srv.Close()
	srv.Config.Handler = Handler(st, lb, home, srv.URL)

	ctx := context.Background()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).Connect(ctx, &mcp.StreamableClientTransport{Endpoint: srv.URL + "/mcp"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	call := func(name string, args any, out any) *mcp.CallToolResult {
		t.Helper()
		res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if res.IsError {
			t.Fatalf("%s: %s", name, res.Content[0].(*mcp.TextContent).Text)
		}
		if out != nil {
			js, _ := json.Marshal(res.StructuredContent)
			json.Unmarshal(js, out)
		}
		return res
	}

	var nb struct{ ID, Mode string }
	call("create_notebook", map[string]any{"title": "HTTP notes"}, &nb)
	if nb.Mode != "dark" {
		t.Fatalf("create: %+v", nb)
	}
	var blocks []json.RawMessage
	json.Unmarshal([]byte(`[{"type":"heading","level":1,"text":"DNS"},{"type":"paragraph","text":"Names to **addresses**."},{"type":"diagram","source":"client -> resolver -> root"}]`), &blocks)
	var res struct {
		Version int `json:"version"`
		Layout  struct {
			PageCount int `json:"page_count"`
		} `json:"layout"`
	}
	call("append_blocks", map[string]any{"notebook_id": nb.ID, "blocks": blocks}, &res)
	if res.Version != 2 || res.Layout.PageCount != 1 {
		t.Fatalf("append: %+v", res)
	}
	var out struct{ Blocks []struct{ Summary string } }
	call("get_outline", map[string]any{"notebook_id": nb.ID}, &out)
	if len(out.Blocks) != 3 || out.Blocks[1].Summary != "Names to addresses." {
		t.Fatalf("outline: %+v", out)
	}
	r := call("render_pages", map[string]any{"notebook_id": nb.ID, "pages": []int{1}}, nil)
	if img, ok := r.Content[1].(*mcp.ImageContent); !ok || !strings.HasPrefix(string(img.Data), "\x89PNG") {
		t.Fatal("render_pages did not return a PNG")
	}
	var exp struct{ Path, URL string }
	call("export_pdf", map[string]any{"notebook_id": nb.ID, "path": filepath.Join(home, "x.pdf")}, &exp)
	if exp.URL != srv.URL+"/n/"+nb.ID+"/pdf" {
		t.Fatalf("export url: %+v", exp)
	}
	if body := get(t, exp.URL); !strings.HasPrefix(body, "%PDF") {
		t.Fatal("export URL did not serve a PDF")
	}
	var v struct {
		URL     string
		Running bool
	}
	call("get_viewer_url", map[string]any{"notebook_id": nb.ID}, &v)
	if v.URL != srv.URL+"/n/"+nb.ID || !v.Running {
		t.Fatalf("viewer url: %+v", v)
	}
	if body := get(t, v.URL); !strings.Contains(body, "HTTP notes") {
		t.Fatal("viewer page missing notebook")
	}
	if body := get(t, srv.URL+"/healthz"); !strings.Contains(body, "folio ok ("+st.Backend()+")") {
		t.Fatalf("healthz: %s", body)
	}
}

func get(t *testing.T, url string) string {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("GET %s: %d %s", url, resp.StatusCode, b)
	}
	return string(b)
}
