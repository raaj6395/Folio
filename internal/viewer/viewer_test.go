package viewer

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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
	_, _, err := st.Mutate(id, nil, "append_blocks", "x", func(nb *store.Notebook, bs []model.Block) ([]model.Block, error) {
		b, _ := model.Parse(json.RawMessage(`{"type":"paragraph","text":"` + text + `"}`))
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
	nb, _ := st.CreateNotebook("Viewer <test>", "clean", "plain", "light")
	addPara(t, st, nb.ID, "first")
	srv := httptest.NewServer(New(st, lb))
	defer srv.Close()

	body := get(t, srv.URL+"/")
	if !strings.Contains(body, "/n/"+nb.ID) || !strings.Contains(body, "Viewer &lt;test&gt;") {
		t.Fatal("index missing notebook or not escaped")
	}
	if png := get(t, srv.URL+"/n/"+nb.ID+"/page/1.png"); !strings.HasPrefix(png, "\x89PNG") {
		t.Fatal("page is not a PNG")
	}
	if pdf := get(t, srv.URL+"/n/"+nb.ID+"/pdf"); !strings.HasPrefix(pdf, "%PDF") {
		t.Fatal("not a PDF")
	}
	darkPNG := get(t, srv.URL+"/n/"+nb.ID+"/page/1.png?mode=dark")
	lightPNG := get(t, srv.URL+"/n/"+nb.ID+"/page/1.png")
	if darkPNG == lightPNG || !strings.HasPrefix(darkPNG, "\x89PNG") {
		t.Fatal("?mode=dark must render a different page")
	}
	if n, _ := st.Notebook(nb.ID); n.Mode != "light" || n.Version != 2 {
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
	start := time.Now()
	addPara(t, st, nb.ID, "second")
	select {
	case next := <-events:
		if next.Version != 3 || next.Pages[0] == first.Pages[0] {
			t.Fatalf("update event: %+v", next)
		}
		t.Logf("update seen after %v", time.Since(start))
	case <-time.After(5 * time.Second):
		t.Fatal("no SSE event after mutation")
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
