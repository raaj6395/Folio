package viewer

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/raaj6395/folio/internal/layout"
	"github.com/raaj6395/folio/internal/store"
)

func TestFolderViews(t *testing.T) {
	home := t.TempDir()
	st, err := store.Open(filepath.Join(home, "folio.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	lb, _ := layout.New(home)
	srv := httptest.NewServer(New(st, lb, Local, false))
	defer srv.Close()
	c := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	post := func(path string, form url.Values) *http.Response {
		t.Helper()
		resp, err := c.PostForm(srv.URL+path, form)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp
	}

	loose, _ := st.CreateNotebook(store.LocalUser, store.Notebook{Title: "Loose notes", Theme: "clean", Paper: "plain", Mode: "dark"})
	if body := get(t, srv.URL+"/"); strings.Contains(body, "<h2>Recent</h2>") || strings.Contains(body, "data-move=") {
		t.Error("without folders the dashboard must look as before")
	}

	if resp := post("/folders", url.Values{"name": {"Semester 1"}, "back": {"/"}}); resp.StatusCode != 303 || resp.Header.Get("Location") != "/" {
		t.Fatalf("create folder: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	sem, _ := st.EnsureFolderPath(store.LocalUser, "Semester 1")
	post("/folders", url.Values{"name": {"Networks"}, "parent_id": {sem.ID}, "back": {"/f/" + sem.ID}})
	net, _ := st.EnsureFolderPath(store.LocalUser, "Semester 1/Networks")
	if resp := post("/folders", url.Values{"name": {"networks"}, "parent_id": {sem.ID}}); resp.StatusCode != 400 {
		t.Errorf("duplicate folder: %d", resp.StatusCode)
	}
	if resp := post("/folders", url.Values{"name": {"Other"}, "back": {"//evil.example"}}); resp.Header.Get("Location") != "/" {
		t.Errorf("open redirect via back: %s", resp.Header.Get("Location"))
	}

	empty := get(t, srv.URL+"/f/"+net.ID)
	if !strings.Contains(empty, "This folder is empty") || !strings.Contains(empty, `href="/f/`+sem.ID+`" data-drop="`+sem.ID+`">Semester 1</a>`) || !strings.Contains(empty, `aria-current="page">Networks`) {
		t.Fatalf("empty folder page: %.600s", empty)
	}

	post("/n/"+loose.ID+"/move", url.Values{"folder_id": {net.ID}, "back": {"/"}})
	if n, _ := st.Notebook(store.LocalUser, loose.ID); n.FolderID != net.ID {
		t.Fatal("move didn't file the notebook")
	}
	top := get(t, srv.URL+"/")
	if !strings.Contains(top, "<h2>Recent</h2>") || !strings.Contains(top, "/f/"+sem.ID) || !strings.Contains(top, "Semester 1/Networks") || !strings.Contains(top, `id="results"`) {
		t.Errorf("top level: %.600s", top)
	}
	if body := get(t, srv.URL+"/f/"+sem.ID); !strings.Contains(body, "/f/"+net.ID) || !strings.Contains(body, "1 notebook") {
		t.Error("parent folder must show the sub-folder and its count")
	}
	if body := get(t, srv.URL+"/n/"+loose.ID); !strings.Contains(body, `href="/f/`+net.ID+`"`) {
		t.Error("notebook back button must return to its folder")
	}
	if resp, _ := http.Get(srv.URL + "/f/f_nothere"); resp.StatusCode != 404 {
		t.Errorf("unknown folder: %d", resp.StatusCode)
	}

	resp := post("/f/"+net.ID+"/delete", nil)
	if resp.StatusCode != 303 || !strings.HasPrefix(resp.Header.Get("Location"), "/f/"+sem.ID+"?folder_deleted=") {
		t.Errorf("delete folder: %s", resp.Header.Get("Location"))
	}
	if n, err := st.Notebook(store.LocalUser, loose.ID); err != nil || n.FolderID != sem.ID {
		t.Error("deleting a folder must move its notebook up, not delete it")
	}

	// F3: favourites, colours, moving folders, moving several notebooks.
	post("/n/"+loose.ID+"/favorite", url.Values{"on": {"1"}})
	if n, _ := st.Notebook(store.LocalUser, loose.ID); !n.Favorite {
		t.Error("favourite")
	}
	if top := get(t, srv.URL+"/"); !strings.Contains(top, "<h2>Favourites</h2>") {
		t.Error("top level must list favourites")
	}
	post("/f/"+sem.ID+"/color", url.Values{"color": {"teal"}})
	if top := get(t, srv.URL+"/"); !strings.Contains(top, "nb folder c-teal") {
		t.Error("folder colour not shown")
	}
	if resp := post("/f/"+sem.ID+"/color", url.Values{"color": {"url(evil)"}}); resp.StatusCode != 400 {
		t.Errorf("bad colour: %d", resp.StatusCode)
	}
	arch, _ := st.EnsureFolderPath(store.LocalUser, "Archive")
	if resp := post("/f/"+arch.ID+"/move", url.Values{"parent_id": {sem.ID}, "back": {"/"}}); resp.StatusCode != 303 {
		t.Errorf("move folder: %d", resp.StatusCode)
	}
	if resp := post("/f/"+sem.ID+"/move", url.Values{"parent_id": {arch.ID}}); resp.StatusCode != 400 {
		t.Errorf("folder into its own child: %d", resp.StatusCode)
	}
	n1, _ := st.CreateNotebook(store.LocalUser, store.Notebook{Title: "One", Theme: "clean", Paper: "plain", Mode: "dark"})
	n2, _ := st.CreateNotebook(store.LocalUser, store.Notebook{Title: "Two", Theme: "clean", Paper: "plain", Mode: "dark"})
	post("/notebooks/move", url.Values{"id": {n1.ID, n2.ID}, "folder_id": {arch.ID}})
	for _, id := range []string{n1.ID, n2.ID} {
		if n, _ := st.Notebook(store.LocalUser, id); n.FolderID != arch.ID {
			t.Error("bulk move")
		}
	}
	if body := get(t, srv.URL+"/f/"+arch.ID); !strings.Contains(body, `id="selectBtn"`) || !strings.Contains(body, `class="pick"`) || !strings.Contains(body, `data-drag-nb="`+n1.ID+`"`) {
		t.Error("folder page must offer select and drag")
	}

	req, _ := http.NewRequest("POST", srv.URL+"/folders", strings.NewReader("name=Forged"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	if resp, _ := c.Do(req); resp.StatusCode != http.StatusForbidden {
		t.Errorf("cross-site folder POST: %d", resp.StatusCode)
	}
	resp, _ = http.Get(srv.URL + "/")
	b, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(b), "Forged") {
		t.Error("cross-site POST created a folder")
	}
}

func TestTemplatePill(t *testing.T) {
	home := t.TempDir()
	st, err := store.Open(filepath.Join(home, "folio.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	lb, _ := layout.New(home)
	srv := httptest.NewServer(New(st, lb, Local, false))
	defer srv.Close()
	st.CreateNotebook(store.LocalUser, store.Notebook{Title: "Two Sum", Theme: "clean", Paper: "plain", Mode: "dark", Template: "dsa_problem"})
	st.CreateNotebook(store.LocalUser, store.Notebook{Title: "DNS", Theme: "clean", Paper: "plain", Mode: "dark", Template: "topic"})
	body := get(t, srv.URL+"/")
	if !strings.Contains(body, `data-kind="DSA"`) || !strings.Contains(body, `<span class="pill" title="Template">DSA</span>`) {
		t.Fatalf("templated card shows its pill and search key: %.2000s", body)
	}
	if strings.Count(body, `title="Template"`) != 1 {
		t.Fatal("topic notebooks get no pill")
	}
}
