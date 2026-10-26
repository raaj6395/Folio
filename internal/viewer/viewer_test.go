package viewer

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/raaj6395/folio/internal/layout"
	"github.com/raaj6395/folio/internal/model"
	"github.com/raaj6395/folio/internal/store"
)

func addPara(t *testing.T, st *store.Store, id, text string) {
	t.Helper()
	addBlock(t, st, id, `{"type":"paragraph","text":"`+text+`"}`)
}

func addBlock(t *testing.T, st *store.Store, id, raw string) {
	t.Helper()
	_, _, err := st.Mutate(store.LocalUser, id, nil, "append_blocks", "x", func(nb *store.Notebook, bs []model.Block) ([]model.Block, error) {
		b, _ := model.Parse(json.RawMessage(raw))
		b.ID = model.NewID("b_")
		return model.InsertAt(bs, len(bs), []model.Block{b}), nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestViewer(t *testing.T) {
	home := t.TempDir()
	st, err := store.Open(filepath.Join(home, "folio.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	lb, _ := layout.New(home)
	nb, _ := st.CreateNotebook(store.LocalUser, store.Notebook{Title: "Viewer <test>", Theme: "clean", Paper: "plain", Mode: "light"})
	addPara(t, st, nb.ID, "first")
	srv := httptest.NewServer(New(st, lb, Local, false))
	defer srv.Close()

	body := get(t, srv.URL+"/")
	if !strings.Contains(body, "/n/"+nb.ID) || !strings.Contains(body, "Viewer &lt;test&gt;") {
		t.Fatal("index missing notebook or not escaped")
	}
	if css := get(t, srv.URL+"/style.css"); !strings.Contains(css, `[data-theme="dark"]`) {
		t.Fatal("stylesheet missing dark tokens")
	}
	if js, svg := get(t, srv.URL+"/app.js"), get(t, srv.URL+"/logo.svg"); !strings.Contains(js, "folio.applyView") || !strings.HasPrefix(svg, "<svg") {
		t.Fatal("shared script or logo not served")
	}
	if page := get(t, srv.URL+"/n/"+nb.ID); !strings.Contains(page, `const notebookMode = "light"`) || !strings.Contains(page, `href="/style.css"`) {
		t.Fatal("notebook page must start in the notebook's mode and use the shared stylesheet")
	}
	if png := get(t, srv.URL+"/n/"+nb.ID+"/page/1.png"); !strings.HasPrefix(png, "\x89PNG") {
		t.Fatal("page is not a PNG")
	}
	if png := get(t, srv.URL+"/n/"+nb.ID+"/thumb.png"); !strings.HasPrefix(png, "\x89PNG") {
		t.Fatal("thumbnail is not a PNG")
	}
	if pdf := get(t, srv.URL+"/n/"+nb.ID+"/pdf"); !strings.HasPrefix(pdf, "%PDF") {
		t.Fatal("not a PDF")
	}
	darkPNG := get(t, srv.URL+"/n/"+nb.ID+"/page/1.png?mode=dark")
	lightPNG := get(t, srv.URL+"/n/"+nb.ID+"/page/1.png")
	if darkPNG == lightPNG || !strings.HasPrefix(darkPNG, "\x89PNG") {
		t.Fatal("?mode=dark must render a different page")
	}
	if n, _ := st.Notebook(store.LocalUser, nb.ID); n.Mode != "light" || n.Version != 2 {
		t.Fatalf("view override must not change the notebook: %+v", n)
	}
	if resp, _ := http.Get(srv.URL + "/n/" + nb.ID + "/page/9.png"); resp.StatusCode != 404 {
		t.Fatalf("missing page status %d", resp.StatusCode)
	}

	resp, err := http.Get(srv.URL + "/n/" + nb.ID + "/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	events := make(chan State, 4)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for sc.Scan() {
			if data, ok := strings.CutPrefix(sc.Text(), "data: "); ok {
				var s State
				json.Unmarshal([]byte(data), &s)
				events <- s
			}
		}
	}()
	first := <-events
	if first.Version != 2 || first.PageCount != 1 {
		t.Fatalf("first event: %+v", first)
	}
	for v, want := range map[string]string{first.Pages[0]: "private, max-age=31536000, immutable", "stale": "no-store"} {
		resp, err := http.Get(srv.URL + "/n/" + nb.ID + "/page/1.png?v=" + v)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if got := resp.Header.Get("Cache-Control"); got != want {
			t.Fatalf("page image with v=%s: Cache-Control %q, want %q", v, got, want)
		}
	}
	start := time.Now()
	addBlock(t, st, nb.ID, `{"type":"heading","level":2,"text":"**Second** part"}`)
	select {
	case next := <-events:
		if next.Version != 3 || next.Pages[0] == first.Pages[0] {
			t.Fatalf("update event: %+v", next)
		}
		if len(first.Outline) != 0 || len(next.Outline) != 1 || next.Outline[0].Text != "Second part" || next.Outline[0].Level != 2 || next.Outline[0].Page != 1 {
			t.Fatalf("outline: %+v", next.Outline)
		}
		t.Logf("update seen after %v", time.Since(start))
	case <-time.After(5 * time.Second):
		t.Fatal("no SSE event after mutation")
	}

	post := func(path string, form url.Values) (int, string) {
		resp, err := http.PostForm(srv.URL+path, form)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	if code, body := post("/n/"+nb.ID+"/rename", url.Values{"title": {"  Renamed  "}}); code != 200 || !strings.Contains(body, ">Renamed</a>") {
		t.Fatalf("rename: %d", code)
	}
	if code, _ := post("/n/"+nb.ID+"/rename", url.Values{"title": {" "}}); code != 400 {
		t.Fatalf("empty title: %d", code)
	}
	key := `data-key="` + nb.ID + `:`
	if code, body := post("/n/"+nb.ID+"/delete", nil); code != 200 || strings.Contains(body, key) {
		t.Fatalf("delete: %d", code)
	}
	if resp, _ := http.Get(srv.URL + "/n/" + nb.ID); resp.StatusCode != 404 {
		t.Fatalf("deleted notebook: %d", resp.StatusCode)
	}
	if code, body := post("/n/"+nb.ID+"/restore", nil); code != 200 || !strings.Contains(body, key) {
		t.Fatalf("restore: %d", code)
	}
	resp, err = http.Get(srv.URL + "/no-such-page")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if b, _ := io.ReadAll(resp.Body); resp.StatusCode != 404 || !strings.Contains(string(b), "Page not found") {
		t.Fatalf("styled 404: %d", resp.StatusCode)
	}
}

func get(t *testing.T, url string) string {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var b bytes.Buffer
	io.Copy(&b, resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("%s: %d %s", url, resp.StatusCode, b.String())
	}
	return b.String()
}
