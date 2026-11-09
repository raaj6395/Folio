package viewer

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/raaj6395/folio/internal/account"
	"github.com/raaj6395/folio/internal/layout"
	"github.com/raaj6395/folio/internal/model"
	"github.com/raaj6395/folio/internal/notetemplates"
	"github.com/raaj6395/folio/internal/store"
)

const (
	PPI         = 110
	ThumbPPI    = 72
	DefaultPort = 7420
)

//go:embed index.html
var indexHTML string

//go:embed notebook.html
var notebookHTML string

//go:embed canvas.html
var canvasHTML string

//go:embed style.css
var styleCSS string

//go:embed app.js
var appJS string

//go:embed logo.svg
var logoSVG string

//go:embed error.html
var errorHTML string

var (
	indexTmpl    = template.Must(template.New("index").Parse(indexHTML))
	notebookTmpl = template.Must(template.New("notebook").Parse(notebookHTML))
	canvasTmpl   = template.Must(template.New("canvas").Parse(canvasHTML))
	errorTmpl    = template.Must(template.New("error").Parse(errorHTML))
)

type Server struct {
	st     *store.Store
	lb     *layout.Builder
	user   func(*http.Request) (id, email string)
	auth   bool
	visits *account.Limiter // share-link requests per IP
}

type uidKey struct{}

func uid(r *http.Request) string { return r.Context().Value(uidKey{}).(string) }

func Local(*http.Request) (string, string) { return store.LocalUser, "" }

func New(st *store.Store, lb *layout.Builder, user func(*http.Request) (id, email string), auth bool) *http.ServeMux {
	s := &Server{st: st, lb: lb, user: user, auth: auth, visits: account.NewLimiter(120, time.Minute)}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.require(true, s.index))
	asset := func(path, contentType, body string) {
		mux.HandleFunc("GET "+path, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", contentType)
			fmt.Fprint(w, body)
		})
	}
	asset("/style.css", "text/css; charset=utf-8", styleCSS)
	asset("/app.js", "text/javascript; charset=utf-8", appJS)
	asset("/logo.svg", "image/svg+xml", logoSVG)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "folio viewer ok") })
	mux.HandleFunc("GET /n/{id}", s.require(true, s.notebook))
	mux.HandleFunc("GET /n/{id}/events", s.require(false, s.events))
	mux.HandleFunc("GET /n/{id}/page/{file}", s.require(false, s.page))
	mux.HandleFunc("GET /n/{id}/pdf", s.require(true, s.pdf))
	mux.HandleFunc("GET /n/{id}/thumb.png", s.require(false, s.thumb))
	mux.HandleFunc("GET /n/{id}/canvas.svg", s.require(false, s.svg))
	// Share links: read-only routes for one notebook, no login. Only these GETs exist under /s/.
	mux.HandleFunc("GET /s/{token}", s.shared(s.notebook))
	mux.HandleFunc("GET /s/{token}/events", s.shared(s.events))
	mux.HandleFunc("GET /s/{token}/page/{file}", s.shared(s.page))
	mux.HandleFunc("GET /s/{token}/pdf", s.shared(s.pdf))
	mux.HandleFunc("GET /s/{token}/thumb.png", s.shared(s.thumb))
	mux.HandleFunc("GET /s/{token}/canvas.svg", s.shared(s.svg))
	cop := http.NewCrossOriginProtection()
	mux.Handle("POST /n/{id}/share", cop.Handler(s.require(false, s.share)))
	mux.Handle("POST /n/{id}/unshare", cop.Handler(s.require(false, s.unshare)))
	mux.Handle("POST /n/{id}/rename", cop.Handler(s.require(true, s.rename)))
	mux.Handle("POST /n/{id}/delete", cop.Handler(s.require(true, s.del)))
	mux.Handle("POST /n/{id}/restore", cop.Handler(s.require(true, s.restore)))
	mux.Handle("POST /n/{id}/move", cop.Handler(s.require(true, s.move)))
	mux.HandleFunc("GET /f/{id}", s.require(true, s.folder))
	mux.Handle("POST /folders", cop.Handler(s.require(true, s.createFolder)))
	mux.Handle("POST /f/{id}/rename", cop.Handler(s.require(true, s.renameFolder)))
	mux.Handle("POST /f/{id}/delete", cop.Handler(s.require(true, s.deleteFolder)))
	mux.Handle("POST /f/{id}/move", cop.Handler(s.require(true, s.moveFolder)))
	mux.Handle("POST /f/{id}/color", cop.Handler(s.require(true, s.colorFolder)))
	mux.Handle("POST /n/{id}/favorite", cop.Handler(s.require(true, s.favorite)))
	mux.Handle("POST /notebooks/move", cop.Handler(s.require(true, s.moveMany)))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		renderError(w, http.StatusNotFound, "Page not found", "There's nothing at this address.")
	})
	return mux
}

func (s *Server) require(redirect bool, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, email := s.user(r)
		if id == "" {
			if redirect {
				http.Redirect(w, r, "/login?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
				return
			}
			http.Error(w, "login required", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), uidKey{}, id)
		ctx = context.WithValue(ctx, emailKey{}, email)
		h(w, r.WithContext(ctx))
	}
}

type emailKey struct{}

type shareKey struct{}

// token is the share token the request came in through ("" for the owner's own view).
func token(r *http.Request) string { t, _ := r.Context().Value(shareKey{}).(string); return t }

// shared serves one of the read handlers for the notebook behind a share link, as its owner sees it.
// The visitor never names a notebook: the token picks exactly one, and only read routes use this.
func (s *Server) shared(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Robots-Tag", "noindex")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if !s.visits.Allow(account.ClientIP(r)) {
			http.Error(w, "too many requests; wait a minute", http.StatusTooManyRequests)
			return
		}
		t := r.PathValue("token")
		nb, err := s.st.SharedNotebook(t)
		if err != nil {
			if !errors.Is(err, store.ErrNotFound) {
				s.fail(w, err)
				return
			}
			renderError(w, http.StatusNotFound, "This link doesn't work", "The owner stopped sharing this notebook, or deleted it.")
			return
		}
		ctx := context.WithValue(r.Context(), uidKey{}, nb.UserID)
		r = r.WithContext(context.WithValue(ctx, shareKey{}, t))
		r.SetPathValue("id", nb.ID)
		h(w, r)
	}
}

// current re-reads the notebook; behind a share link it also checks the link is still on.
func (s *Server) current(r *http.Request) (store.Notebook, error) {
	if t := token(r); t != "" {
		return s.st.SharedNotebook(t)
	}
	return s.st.Notebook(uid(r), r.PathValue("id"))
}

// page is what the notebook and canvas templates render.
type page struct {
	store.Notebook
	Base      string // URL prefix of this notebook's routes: /n/{id} or /s/{token}
	Shared    bool   // a visitor through a share link
	SharePath string // the owner's share link path, "" when not shared
}

func (s *Server) share(w http.ResponseWriter, r *http.Request) {
	t, err := s.st.Share(uid(r), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"path": "/s/" + t})
}

func (s *Server) unshare(w http.ResponseWriter, r *http.Request) {
	if err := s.st.Unshare(uid(r), r.PathValue("id")); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type State struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Theme     string    `json:"theme"`
	Mode      string    `json:"mode"`
	Version   int       `json:"version"`
	PageCount int       `json:"page_count"`
	UpdatedAt time.Time `json:"updated_at"`
	Pages     []string  `json:"pages"`
	Outline   []Heading `json:"outline"`
	Format    string    `json:"format"`
	CanvasMM  []float64 `json:"canvas_mm,omitempty"`
	Blocks    []Mark    `json:"blocks,omitempty"`
}

// Mark is where a top-level canvas block sits, with a hash of its content so the viewer can tell
// which block changed and pan to it.
type Mark struct {
	ID   string  `json:"id"`
	XMM  float64 `json:"x_mm"`
	YMM  float64 `json:"y_mm"`
	Hash string  `json:"h"`
}

type Heading struct {
	Text  string  `json:"text"`
	Level int     `json:"level"`
	Page  int     `json:"page"`
	YMM   float64 `json:"y_mm"`
	XMM   float64 `json:"x_mm"`
}

func outline(bs []model.Block, r *layout.Result) []Heading {
	out := []Heading{}
	for i, p := range r.Blocks {
		var h model.Heading
		if p.Type != "heading" || json.Unmarshal(bs[i].Data, &h) != nil {
			continue
		}
		var text strings.Builder
		for _, sp := range model.ParseInline(h.Text) {
			text.WriteString(sp.Text)
		}
		out = append(out, Heading{Text: text.String(), Level: h.Level, Page: p.Page, YMM: p.YMM, XMM: p.XMM})
	}
	return out
}

func viewMode(r *http.Request, nb *store.Notebook) {
	if m := r.URL.Query().Get("mode"); slices.Contains(layout.Modes, m) {
		nb.Mode = m
	}
}

func (s *Server) load(r *http.Request) (store.Notebook, []model.Block, error) {
	nb, err := s.st.Notebook(uid(r), r.PathValue("id"))
	if err != nil {
		return nb, nil, err
	}
	viewMode(r, &nb)
	bs, err := s.st.Blocks(uid(r), nb.ID)
	return nb, bs, err
}

func (s *Server) state(r *http.Request) (State, error) {
	nb, bs, err := s.load(r)
	if err != nil {
		return State{}, err
	}
	ctx := r.Context()
	res, err := s.lb.Build(ctx, nb, bs)
	if err != nil {
		return State{}, err
	}
	st := State{ID: nb.ID, Title: nb.Title, Theme: nb.Theme, Mode: nb.Mode, Version: nb.Version, PageCount: res.PageCount, UpdatedAt: nb.UpdatedAt, Pages: res.Fingerprints(nb, bs), Outline: outline(bs, res), Format: nb.Format}
	if nb.Format == "canvas" {
		st.CanvasMM = []float64{res.CanvasWMM, res.CanvasHMM}
		for i, p := range res.Blocks {
			sum := sha256.Sum256(append([]byte(bs[i].Type), bs[i].Data...))
			st.Blocks = append(st.Blocks, Mark{p.ID, p.XMM, p.YMM, hex.EncodeToString(sum[:6])})
		}
	}
	return st, nil
}

func renderError(w http.ResponseWriter, status int, title, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	errorTmpl.Execute(w, map[string]any{"Status": status, "Title": title, "Message": msg})
}

func (s *Server) fail(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrInvalid) || errors.Is(err, store.ErrQuota) {
		_, msg, ok := strings.Cut(err.Error(), ": ")
		if !ok || msg == "" {
			msg = err.Error()
		}
		renderError(w, http.StatusBadRequest, "Can't do that", strings.ToUpper(msg[:1])+msg[1:]+".")
		return
	}
	if errors.Is(err, store.ErrNotFound) && strings.Contains(err.Error(), "folder") {
		renderError(w, http.StatusNotFound, "Folder not found", "It may have been deleted, or it belongs to another account.")
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		renderError(w, http.StatusNotFound, "Notebook not found", "It may have been deleted, or it belongs to another account.")
		return
	}
	slog.Error("viewer", "err", err)
	renderError(w, http.StatusInternalServerError, "Something went wrong", "Folio couldn't load this. Try again in a moment.")
}

// card is a notebook on the dashboard with the path of its folder ("" at the top level).
type card struct {
	store.Notebook
	Path    string
	Movable bool
}

// Kind is the card's template pill, e.g. "DSA" ("" for none).
func (c card) Kind() string { return notetemplates.Label(c.Template) }

type treeItem struct{ ID, Label, Path string }

func folderURL(id string) string {
	if id == "" {
		return "/"
	}
	return "/f/" + id
}

// safeBack keeps redirects on dashboard pages.
func safeBack(back string) string {
	if rest, ok := strings.CutPrefix(back, "/f/"); ok && rest != "" && !strings.ContainsAny(rest, "/\\?#") {
		return back
	}
	return "/"
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) { s.library(w, r, "") }

func (s *Server) folder(w http.ResponseWriter, r *http.Request) { s.library(w, r, r.PathValue("id")) }

func (s *Server) library(w http.ResponseWriter, r *http.Request, folderID string) {
	nbs, err := s.st.Notebooks(uid(r))
	if err != nil {
		s.fail(w, err)
		return
	}
	folders, err := s.st.Folders(uid(r))
	if err != nil {
		s.fail(w, err)
		return
	}
	byID := map[string]store.Folder{}
	for _, f := range folders {
		byID[f.ID] = f
	}
	here, ok := byID[folderID]
	if folderID != "" && !ok {
		renderError(w, http.StatusNotFound, "Folder not found", "It may have been deleted, or it belongs to another account.")
		return
	}
	var crumbs, subs []store.Folder
	for f, ok := here, ok; ok; f, ok = byID[f.ParentID] {
		crumbs = append([]store.Folder{f}, crumbs...)
	}
	tree := []treeItem{{"", "Notebooks (top level)", ""}}
	for _, f := range folders {
		if f.ParentID == folderID {
			subs = append(subs, f)
		}
		tree = append(tree, treeItem{f.ID, strings.Repeat("\u00a0\u00a0\u00a0", f.Depth) + f.Name, f.Path})
	}
	var inHere, all, favs []card
	for _, nb := range nbs {
		c := card{nb, byID[nb.FolderID].Path, len(folders) > 0}
		all = append(all, c)
		if nb.Favorite {
			favs = append(favs, c)
		}
		if nb.FolderID == folderID {
			inHere = append(inHere, card{nb, "", c.Movable})
		}
	}
	data := map[string]any{"Notebooks": inHere, "All": all, "Folders": subs, "HasFolders": len(folders) > 0, "Tree": tree, "Crumbs": crumbs,
		"CanNest": !ok || here.Depth < store.MaxFolderDepth, "Search": len(folders) > 0 || len(nbs) > 6, "Auth": s.auth}
	if ok {
		data["Here"], data["Up"], data["UpName"] = here, folderURL(here.ParentID), "Notebooks"
		if p, has := byID[here.ParentID]; has {
			data["UpName"] = p.Name
		}
	} else {
		data["Favorites"] = favs
		if len(folders) > 0 {
			data["Recent"] = all[:min(4, len(all))]
		}
	}
	data["Colors"] = store.FolderColors
	data["Email"], _ = r.Context().Value(emailKey{}).(string)
	if s.auth && len(nbs) == 0 {
		connected, err := s.st.HasConnection(uid(r))
		if err != nil {
			slog.Error("has connection", "err", err)
		}
		data["Connected"] = connected
	}
	w.Header().Set("Cache-Control", "no-store")
	indexTmpl.Execute(w, data)
}

func (s *Server) createFolder(w http.ResponseWriter, r *http.Request) {
	if _, err := s.st.CreateFolder(uid(r), r.FormValue("parent_id"), r.FormValue("name")); err != nil {
		s.fail(w, err)
		return
	}
	http.Redirect(w, r, safeBack(r.FormValue("back")), http.StatusSeeOther)
}

func (s *Server) renameFolder(w http.ResponseWriter, r *http.Request) {
	if err := s.st.RenameFolder(uid(r), r.PathValue("id"), r.FormValue("name")); err != nil {
		s.fail(w, err)
		return
	}
	http.Redirect(w, r, safeBack(r.FormValue("back")), http.StatusSeeOther)
}

func (s *Server) deleteFolder(w http.ResponseWriter, r *http.Request) {
	f, err := s.st.DeleteFolder(uid(r), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	http.Redirect(w, r, folderURL(f.ParentID)+"?"+url.Values{"folder_deleted": {f.Name}}.Encode(), http.StatusSeeOther)
}

func (s *Server) moveFolder(w http.ResponseWriter, r *http.Request) {
	if err := s.st.MoveFolder(uid(r), r.PathValue("id"), r.FormValue("parent_id")); err != nil {
		s.fail(w, err)
		return
	}
	http.Redirect(w, r, safeBack(r.FormValue("back")), http.StatusSeeOther)
}

func (s *Server) colorFolder(w http.ResponseWriter, r *http.Request) {
	if err := s.st.SetFolderColor(uid(r), r.PathValue("id"), r.FormValue("color")); err != nil {
		s.fail(w, err)
		return
	}
	http.Redirect(w, r, safeBack(r.FormValue("back")), http.StatusSeeOther)
}

func (s *Server) favorite(w http.ResponseWriter, r *http.Request) {
	if err := s.st.SetFavorite(uid(r), r.PathValue("id"), r.FormValue("on") == "1"); err != nil {
		s.fail(w, err)
		return
	}
	http.Redirect(w, r, safeBack(r.FormValue("back")), http.StatusSeeOther)
}

func (s *Server) moveMany(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	if _, err := s.st.MoveNotebooks(uid(r), r.PostForm["id"], r.FormValue("folder_id")); err != nil {
		s.fail(w, err)
		return
	}
	http.Redirect(w, r, safeBack(r.FormValue("back")), http.StatusSeeOther)
}

func (s *Server) move(w http.ResponseWriter, r *http.Request) {
	if err := s.st.MoveNotebook(uid(r), r.PathValue("id"), r.FormValue("folder_id")); err != nil {
		s.fail(w, err)
		return
	}
	http.Redirect(w, r, safeBack(r.FormValue("back")), http.StatusSeeOther)
}

func (s *Server) notebook(w http.ResponseWriter, r *http.Request) {
	nb, err := s.current(r)
	if err != nil {
		s.fail(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	p := page{Notebook: nb, Base: "/n/" + nb.ID}
	if t := token(r); t != "" {
		p.Base, p.Shared = "/s/"+t, true
	} else if nb.ShareToken != "" {
		p.SharePath = "/s/" + nb.ShareToken
	}
	if nb.Format == "canvas" {
		canvasTmpl.Execute(w, p)
		return
	}
	notebookTmpl.Execute(w, p)
}

// svg serves a canvas as one SVG: the viewer zooms it without blurring, and it is the SVG download.
func (s *Server) svg(w http.ResponseWriter, r *http.Request) {
	nb, bs, err := s.load(r)
	if err == nil && nb.Format != "canvas" {
		err = store.ErrNotFound
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	res, err := s.lb.Build(r.Context(), nb, bs)
	if err != nil {
		s.fail(w, err)
		return
	}
	path, err := s.lb.SVG(r.Context(), nb, bs)
	if err != nil {
		s.fail(w, err)
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		s.fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	// Opened directly in a tab, the SVG is a document: let it show its embedded images and nothing else.
	w.Header().Set("Content-Security-Policy", "default-src 'none'; img-src data:; style-src 'unsafe-inline'")
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	if r.URL.Query().Get("v") != res.Fingerprints(nb, bs)[0] {
		w.Header().Set("Cache-Control", "no-store")
	}
	if r.URL.Query().Has("download") {
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.svg"`, strings.ReplaceAll(nb.Title, `"`, "")))
	}
	w.Write(data)
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	last := -1
	var retryAt time.Time
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	beat := time.NewTicker(15 * time.Second)
	defer beat.Stop()
	for {
		if nb, err := s.current(r); err != nil {
			fmt.Fprintf(w, "event: gone\ndata: {}\n\n")
			fl.Flush()
			return
		} else if nb.Version != last && time.Now().After(retryAt) {
			st, err := s.state(r)
			if err != nil {
				// A failed build can hold the shared layout lock for up to 20s; back off instead of retrying every tick.
				slog.Error("viewer state", "err", err)
				retryAt = time.Now().Add(10 * time.Second)
			} else {
				js, _ := json.Marshal(st)
				fmt.Fprintf(w, "data: %s\n\n", js)
				fl.Flush()
				last = nb.Version
			}
		}
		select {
		case <-r.Context().Done():
			return
		case <-beat.C:
			fmt.Fprint(w, ": keepalive\n\n")
			fl.Flush()
		case <-tick.C:
		}
	}
}

func (s *Server) page(w http.ResponseWriter, r *http.Request) {
	n, err := strconv.Atoi(strings.TrimSuffix(r.PathValue("file"), ".png"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	s.png(w, r, n, PPI, true)
}

func (s *Server) thumb(w http.ResponseWriter, r *http.Request) { s.png(w, r, 1, ThumbPPI, false) }

// Long edge of a canvas PNG: a canvas can be metres wide, so its PNGs are capped by pixels, not ppi.
const (
	canvasThumbPx = 600
	canvasPagePx  = 1568
)

func (s *Server) png(w http.ResponseWriter, r *http.Request, n, ppi int, all bool) {
	nb, bs, err := s.load(r)
	if err != nil {
		s.fail(w, err)
		return
	}
	res, err := s.lb.Build(r.Context(), nb, bs)
	if err != nil {
		s.fail(w, err)
		return
	}
	if n < 1 || n > res.PageCount {
		http.NotFound(w, r)
		return
	}
	pages := []int{n}
	if all {
		pages = make([]int, res.PageCount)
		for i := range pages {
			pages[i] = i + 1
		}
	}
	if nb.Format == "canvas" {
		maxPx := canvasThumbPx
		if all {
			maxPx = canvasPagePx
		}
		ppi = res.CanvasPPI(ppi, maxPx)
	}
	pngs, err := s.lb.RenderPNG(r.Context(), nb, bs, pages, ppi)
	if err != nil {
		s.fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	if all && r.URL.Query().Get("v") != res.Fingerprints(nb, bs)[n-1] {
		w.Header().Set("Cache-Control", "no-store")
	}
	w.Write(pngs[slices.Index(pages, n)])
}

func (s *Server) rename(w http.ResponseWriter, r *http.Request) {
	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" || len(title) > 200 {
		renderError(w, http.StatusBadRequest, "Can't rename", "A title must be 1 to 200 characters.")
		return
	}
	_, _, err := s.st.Mutate(uid(r), r.PathValue("id"), nil, "rename_notebook", "renamed to "+title+" in the web app",
		func(nb *store.Notebook, bs []model.Block) ([]model.Block, error) {
			nb.Title = title
			return bs, nil
		})
	if err != nil {
		s.fail(w, err)
		return
	}
	http.Redirect(w, r, safeBack(r.FormValue("back")), http.StatusSeeOther)
}

func (s *Server) del(w http.ResponseWriter, r *http.Request) {
	nb, err := s.st.Notebook(uid(r), r.PathValue("id"))
	if err == nil {
		err = s.st.Delete(uid(r), nb.ID)
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	http.Redirect(w, r, folderURL(nb.FolderID)+"?"+url.Values{"deleted": {nb.ID}, "title": {nb.Title}}.Encode(), http.StatusSeeOther)
}

func (s *Server) restore(w http.ResponseWriter, r *http.Request) {
	if err := s.st.Undelete(uid(r), r.PathValue("id")); err != nil {
		s.fail(w, err)
		return
	}
	nb, _ := s.st.Notebook(uid(r), r.PathValue("id"))
	http.Redirect(w, r, folderURL(nb.FolderID), http.StatusSeeOther)
}

func (s *Server) pdf(w http.ResponseWriter, r *http.Request) {
	nb, bs, err := s.load(r)
	if err != nil {
		s.fail(w, err)
		return
	}
	path, err := s.lb.PDF(r.Context(), nb, bs)
	if err != nil {
		s.fail(w, err)
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		s.fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.pdf"`, strings.ReplaceAll(nb.Title, `"`, "")))
	w.Write(data)
}

func Running(port int) bool {
	c := http.Client{Timeout: 300 * time.Millisecond}
	resp, err := c.Get(fmt.Sprintf("http://127.0.0.1:%d/healthz", port))
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}
