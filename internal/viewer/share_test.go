package viewer

import (
	"bufio"
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

// shareSite has alice (two notebooks) and bob; requests carry the user in X-As, none = anonymous.
func shareSite(t *testing.T) (st *store.Store, url string, alice, bob string, nb, other store.Notebook) {
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
	nb, _ = st.CreateNotebook(a.ID, store.Notebook{Title: "Shared TCP", Theme: "clean", Paper: "plain", Mode: "light"})
	other, _ = st.CreateNotebook(a.ID, store.Notebook{Title: "Private diary", Theme: "clean", Paper: "plain", Mode: "light"})
	for _, id := range []string{nb.ID, other.ID} {
		st.Mutate(a.ID, id, nil, "append_blocks", "x", func(n *store.Notebook, bs []model.Block) ([]model.Block, error) {
			blk, _ := model.Parse(json.RawMessage(`{"type":"paragraph","text":"hello"}`))
			blk.ID = model.NewID("b_")
			return append(bs, blk), nil
		})
	}
	user := func(r *http.Request) (string, string) { return r.Header.Get("X-As"), "" }
	srv := httptest.NewServer(New(st, lb, user, true))
	t.Cleanup(srv.Close)
	return st, srv.URL, a.ID, b.ID, nb, other
}

func do(t *testing.T, method, url, as, ip string) (int, http.Header, string) {
	t.Helper()
	req, _ := http.NewRequest(method, url, nil)
	if as != "" {
		req.Header.Set("X-As", as)
	}
	req.Header.Set("X-Forwarded-For", ip)
	resp, err := (&http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header, string(b)
}

func TestShareLinks(t *testing.T) {
	st, url, alice, bob, nb, other := shareSite(t)

	if code, _, _ := do(t, "POST", url+"/n/"+nb.ID+"/share", bob, "1.1.1.1"); code != 404 {
		t.Fatalf("bob shared alice's notebook: %d", code)
	}
	if code, _, _ := do(t, "POST", url+"/n/"+nb.ID+"/share", "", "1.1.1.1"); code != 401 {
		t.Fatalf("anonymous share: %d", code)
	}
	code, _, body := do(t, "POST", url+"/n/"+nb.ID+"/share", alice, "1.1.1.1")
	var out struct{ Path string }
	json.Unmarshal([]byte(body), &out)
	if code != 200 || !strings.HasPrefix(out.Path, "/s/") {
		t.Fatalf("share: %d %s", code, body)
	}
	link := url + out.Path

	// The owner sees the link state; the dashboard marks the card.
	if _, _, page := do(t, "GET", url+"/n/"+nb.ID, alice, "1.1.1.1"); !strings.Contains(page, `data-path="`+out.Path+`"`) {
		t.Fatal("owner page must know the share link")
	}
	if _, _, dash := do(t, "GET", url+"/", alice, "1.1.1.1"); strings.Count(dash, ">Shared</span>") != 1 {
		t.Fatal("exactly one card is marked Shared")
	}

	// A visitor: no login, read-only, this notebook only.
	code, h, page := do(t, "GET", link, "", "2.2.2.2")
	if code != 200 || !strings.Contains(page, "Shared TCP") || !strings.Contains(page, "Read-only") {
		t.Fatalf("visitor page: %d", code)
	}
	if h.Get("X-Robots-Tag") != "noindex" || h.Get("Referrer-Policy") != "no-referrer" || !strings.Contains(page, `content="noindex"`) {
		t.Fatalf("share pages must not be indexed or leak the link: %v", h)
	}
	for _, leak := range []string{"/n/", "shareBtn", "Private diary", "alice@", "Back to notebooks"} {
		if strings.Contains(page, leak) {
			t.Errorf("visitor page contains %q", leak)
		}
	}
	for path, prefix := range map[string]string{"/page/1.png": "\x89PNG", "/thumb.png": "\x89PNG", "/pdf": "%PDF"} {
		if code, _, b := do(t, "GET", link+path, "", "2.2.2.2"); code != 200 || !strings.HasPrefix(b, prefix) {
			t.Errorf("visitor %s: %d", path, code)
		}
	}
	if code, _, _ := do(t, "GET", link+"/canvas.svg", "", "2.2.2.2"); code != 404 {
		t.Errorf("an A4 notebook has no canvas SVG: %d", code)
	}
	// Nothing else is reachable: no writes under /s/, and owner routes still need a login.
	for _, p := range []string{"", "/share", "/unshare", "/rename", "/delete"} {
		if code, _, _ := do(t, "POST", link+p, "", "2.2.2.2"); code != 404 && code != 405 {
			t.Errorf("POST %s under a share link: %d", p, code)
		}
	}
	for _, p := range []string{"/n/" + other.ID, "/n/" + nb.ID, "/", "/n/" + other.ID + "/page/1.png"} {
		if code, _, _ := do(t, "GET", url+p, "", "2.2.2.2"); code != 303 && code != 401 {
			t.Errorf("visitor reached %s: %d", p, code)
		}
	}

	// Live updates, and stopping sharing ends an open stream.
	req, _ := http.NewRequest("GET", link+"/events", nil)
	req.Header.Set("X-Forwarded-For", "2.2.2.2")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(nil, 1<<20)
	next := func() string {
		for sc.Scan() {
			if l := sc.Text(); strings.HasPrefix(l, "data: ") || strings.HasPrefix(l, "event: ") {
				return l
			}
		}
		return ""
	}
	if l := next(); !strings.Contains(l, `"title":"Shared TCP"`) {
		t.Fatalf("first event: %.80s", l)
	}
	if code, _, _ := do(t, "POST", url+"/n/"+nb.ID+"/unshare", alice, "1.1.1.1"); code != 204 {
		t.Fatalf("unshare: %d", code)
	}
	done := make(chan string, 1)
	go func() { done <- next() }()
	select {
	case l := <-done:
		if l != "event: gone" {
			t.Fatalf("after unshare: %q", l)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("an open share page must learn the link was turned off")
	}
	for _, p := range []string{"", "/page/1.png", "/pdf", "/thumb.png", "/events"} {
		if code, _, b := do(t, "GET", link+p, "", "2.2.2.2"); code != 404 || (p == "" && !strings.Contains(b, "This link doesn")) {
			t.Errorf("after unshare %s: %d", p, code)
		}
	}

	// Sharing again makes a new link; the old one stays dead.
	tok, _ := st.Share(alice, nb.ID)
	if "/s/"+tok == out.Path {
		t.Fatal("re-share reused the old token")
	}
	if code, _, _ := do(t, "GET", url+"/s/"+tok, "", "2.2.2.2"); code != 200 {
		t.Fatalf("new link: %d", code)
	}
}

func TestShareCanvasAndLimit(t *testing.T) {
	st, url, alice, _, nb, _ := shareSite(t)
	st.Mutate(alice, nb.ID, nil, "set_theme", "x", func(n *store.Notebook, bs []model.Block) ([]model.Block, error) {
		n.Format = "canvas"
		return bs, nil
	})
	tok, _ := st.Share(alice, nb.ID)
	link := url + "/s/" + tok
	if code, _, page := do(t, "GET", link, "", "3.3.3.3"); code != 200 || !strings.Contains(page, "canvas-view") || !strings.Contains(page, `const base = "/s/`+tok+`"`) {
		t.Fatalf("canvas share page: %d %s", code, page[strings.Index(page, "const base"):][:60])
	}
	if code, h, b := do(t, "GET", link+"/canvas.svg", "", "3.3.3.3"); code != 200 || !strings.HasPrefix(h.Get("Content-Type"), "image/svg") || !strings.Contains(b, "<svg") {
		t.Fatalf("canvas SVG: %d", code)
	}
	for i := 0; i < 118; i++ {
		do(t, "GET", link+"/thumb.png", "", "3.3.3.3")
	}
	if code, _, _ := do(t, "GET", link+"/thumb.png", "", "3.3.3.3"); code != 429 {
		t.Fatalf("request 121 in a minute: %d", code)
	}
	if code, _, _ := do(t, "GET", link+"/thumb.png", "", "4.4.4.4"); code != 200 {
		t.Fatalf("other visitors are unaffected: %d", code)
	}
}
