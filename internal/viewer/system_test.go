package viewer

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/raaj6395/folio/internal/layout"
	"github.com/raaj6395/folio/internal/model"
	"github.com/raaj6395/folio/internal/sim"
	"github.com/raaj6395/folio/internal/store"
)

func systemSite(t *testing.T) (st *store.Store, url, alice, bob string, nb store.Notebook) {
	t.Helper()
	home := t.TempDir()
	st, err := store.Open(filepath.Join(home, "folio.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	lb, _ := layout.New(home)
	a, _ := st.CreateUser("alice@example.com", "x")
	b, _ := st.CreateUser("bob@example.com", "x")
	nb, _ = st.CreateNotebook(a.ID, store.Notebook{Title: "Read path", Theme: "clean", Paper: "plain", Mode: "dark", Format: "system"})
	raw := sim.Demos()[0].JSON // read_path
	var m map[string]any
	json.Unmarshal(raw, &m)
	m["type"] = "system"
	js, _ := json.Marshal(m)
	if _, _, err := st.Mutate(a.ID, nb.ID, nil, "append_blocks", "x", func(n *store.Notebook, bs []model.Block) ([]model.Block, error) {
		blk, err := model.Parse(js)
		blk.ID = model.NewID("b_")
		return model.InsertAt(bs, len(bs), []model.Block{blk}), err
	}); err != nil {
		t.Fatal(err)
	}
	user := func(r *http.Request) (string, string) { return r.Header.Get("X-As"), "" }
	srv := httptest.NewServer(New(st, lb, user, true))
	t.Cleanup(srv.Close)
	return st, srv.URL, a.ID, b.ID, nb
}

func run(t *testing.T, url, as string, body any) (int, map[string]any) {
	t.Helper()
	js, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(js))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-As", as)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	b, _ := io.ReadAll(resp.Body)
	json.Unmarshal(b, &out)
	return resp.StatusCode, out
}

func TestSystemViewer(t *testing.T) {
	st, url, alice, bob, nb := systemSite(t)
	base := url + "/n/" + nb.ID

	code, _, page := do(t, "GET", base, alice, "1.1.1.1")
	if code != 200 || !strings.Contains(page, `src="/system.js"`) || strings.Contains(page, "/pdf") {
		t.Fatalf("page: %d", code)
	}
	if code, hdr, js := do(t, "GET", url+"/system.js", "", "1.1.1.1"); code != 200 || !strings.Contains(hdr.Get("Content-Type"), "javascript") || !strings.Contains(js, "animateMotion") {
		t.Fatalf("system.js: %d", code)
	}

	code, _, body := do(t, "GET", base+"/system.json", alice, "1.1.1.1")
	var sys struct {
		Version int
		Spec    sim.Spec
		Layout  sim.Layout
	}
	json.Unmarshal([]byte(body), &sys)
	if code != 200 || len(sys.Spec.Nodes) != 6 || len(sys.Layout.Nodes) != 6 || len(sys.Layout.Edges) == 0 {
		t.Fatalf("system.json: %d %s", code, body[:min(200, len(body))])
	}
	if code, _, _ := do(t, "GET", base+"/system.json", bob, "1.1.1.1"); code != 404 {
		t.Fatalf("bob read alice's system: %d", code)
	}

	// Run: the owner plays; state goes in and comes back; bob and bad input are refused.
	code, res := run(t, base+"/system/run", alice, map[string]any{"action": "send", "request": "read", "key": "7", "state": map[string]any{"down": []string{"redis"}}})
	if code != 200 || res["status"].(float64) != 200 || len(res["events"].([]any)) == 0 {
		t.Fatalf("run: %d %v", code, res)
	}
	if down := res["state"].(map[string]any)["down"].([]any); len(down) != 1 || down[0] != "redis" {
		t.Fatalf("state round trip: %v", down)
	}
	if code, _ := run(t, base+"/system/run", bob, map[string]any{"action": "reset"}); code != 404 {
		t.Fatalf("bob ran alice's system: %d", code)
	}
	if code, res := run(t, base+"/system/run", alice, map[string]any{"action": "send", "request": "nope"}); code != 400 || !strings.Contains(res["error"].(string), "unknown request") {
		t.Fatalf("bad request: %d %v", code, res)
	}
	big := strings.Repeat("x", maxRunBody+10)
	if code, _ := run(t, base+"/system/run", alice, map[string]any{"action": "reset", "pad": big}); code != 400 {
		t.Fatalf("oversized body: %d", code)
	}

	// No pages: PDF and page PNGs are 404; the thumbnail is D2's SVG; SSE reports the version.
	if code, _, _ := do(t, "GET", base+"/pdf", alice, "1.1.1.1"); code != 404 {
		t.Fatalf("pdf: %d", code)
	}
	if code, _, _ := do(t, "GET", base+"/page/1.png", alice, "1.1.1.1"); code != 404 {
		t.Fatalf("page png: %d", code)
	}
	if code, hdr, svg := do(t, "GET", base+"/thumb.png?v=2", alice, "1.1.1.1"); code != 200 || hdr.Get("Content-Type") != "image/svg+xml" || !strings.Contains(svg, "<svg") {
		t.Fatalf("thumb: %d %s", code, hdr.Get("Content-Type"))
	}
	if _, _, lib := do(t, "GET", url+"/", alice, "1.1.1.1"); !strings.Contains(lib, `class="nb system"`) || !strings.Contains(lib, ">System<") {
		t.Fatal("library card should be marked System")
	}

	// A share link can play but reaches nothing else.
	tok, err := st.Share(alice, nb.ID)
	if err != nil {
		t.Fatal(err)
	}
	link := url + "/s/" + tok
	if code, _, page := do(t, "GET", link, "", "2.2.2.2"); code != 200 || !strings.Contains(page, "Read-only") {
		t.Fatalf("shared page: %d", code)
	}
	if code, res := run(t, link+"/system/run", "", map[string]any{"action": "scenario", "scenario": 1}); code != 200 || res["status"].(float64) != 200 {
		t.Fatalf("shared run: %d %v", code, res)
	}
	st.Unshare(alice, nb.ID)
	if code, _ := run(t, link+"/system/run", "", map[string]any{"action": "reset"}); code != 404 {
		t.Fatalf("run after unshare: %d", code)
	}

	// Cross-origin POSTs are refused like every other write route.
	req, _ := http.NewRequest("POST", base+"/system/run", strings.NewReader(`{"action":"reset"}`))
	req.Header.Set("X-As", alice)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-site run: %d", resp.StatusCode)
	}
}
