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

	"github.com/raaj6395/folio/internal/layout"
	"github.com/raaj6395/folio/internal/model"
	"github.com/raaj6395/folio/internal/store"
)

const (
	PPI         = 110
	DefaultPort = 7420
)

//go:embed index.html
var indexHTML string

//go:embed notebook.html
var notebookHTML string

//go:embed style.css
var styleCSS string

var (
	indexTmpl    = template.Must(template.New("index").Parse(indexHTML))
	notebookTmpl = template.Must(template.New("notebook").Parse(notebookHTML))
)

type Server struct {
	st   *store.Store
	lb   *layout.Builder
	user func(*http.Request) (id, email string)
	auth bool
}

type uidKey struct{}

func uid(r *http.Request) string { return r.Context().Value(uidKey{}).(string) }

func Local(*http.Request) (string, string) { return store.LocalUser, "" }

func New(st *store.Store, lb *layout.Builder, user func(*http.Request) (id, email string), auth bool) *http.ServeMux {
	s := &Server{st: st, lb: lb, user: user, auth: auth}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.require(true, s.index))
	mux.HandleFunc("GET /style.css", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		fmt.Fprint(w, styleCSS)
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "folio viewer ok") })
	mux.HandleFunc("GET /n/{id}", s.require(true, s.notebook))
	mux.HandleFunc("GET /n/{id}/events", s.require(false, s.events))
	mux.HandleFunc("GET /n/{id}/page/{file}", s.require(false, s.page))
	mux.HandleFunc("GET /n/{id}/pdf", s.require(true, s.pdf))
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

type State struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Theme     string    `json:"theme"`
	Mode      string    `json:"mode"`
	Version   int       `json:"version"`
	PageCount int       `json:"page_count"`
	UpdatedAt time.Time `json:"updated_at"`
	Pages     []string  `json:"pages"`
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
	return State{ID: nb.ID, Title: nb.Title, Theme: nb.Theme, Mode: nb.Mode, Version: nb.Version, PageCount: res.PageCount, UpdatedAt: nb.UpdatedAt, Pages: signatures(nb, bs, res)}, nil
}

func signatures(nb store.Notebook, bs []model.Block, r *layout.Result) []string {
	hs := make([]*strings.Builder, r.PageCount)
	for i := range hs {
		hs[i] = &strings.Builder{}
		fmt.Fprintf(hs[i], "%s|%s|%s|%s|%d|", nb.Title, nb.Theme, nb.Paper, nb.Mode, r.PageCount)
	}
	for i, p := range r.Blocks {
		for pg := max(p.Page, 1); pg <= min(max(p.EndPage, p.Page), r.PageCount); pg++ {
			fmt.Fprintf(hs[pg-1], "%s:%s:%.1f;", p.ID, bs[i].Data, p.YMM)
		}
	}
	out := make([]string, r.PageCount)
	for i, h := range hs {
		sum := sha256.Sum256([]byte(h.String()))
		out[i] = hex.EncodeToString(sum[:6])
	}
	return out
}

func (s *Server) fail(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, "notebook not found", http.StatusNotFound)
		return
	}
	slog.Error("viewer", "err", err)
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	nbs, err := s.st.Notebooks(uid(r))
	if err != nil {
		s.fail(w, err)
		return
	}
	email, _ := r.Context().Value(emailKey{}).(string)
	w.Header().Set("Cache-Control", "no-store")
	indexTmpl.Execute(w, map[string]any{"Notebooks": nbs, "Email": email, "Auth": s.auth})
}

func (s *Server) notebook(w http.ResponseWriter, r *http.Request) {
	nb, err := s.st.Notebook(uid(r), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	notebookTmpl.Execute(w, nb)
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	id := r.PathValue("id")
	last := -1
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	beat := time.NewTicker(15 * time.Second)
	defer beat.Stop()
	for {
		if nb, err := s.st.Notebook(uid(r), id); err != nil {
			fmt.Fprintf(w, "event: gone\ndata: {}\n\n")
			fl.Flush()
			return
		} else if nb.Version != last {
			st, err := s.state(r)
			if err != nil {
				slog.Error("viewer state", "err", err)
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
	all := make([]int, res.PageCount)
	for i := range all {
		all[i] = i + 1
	}
	pngs, err := s.lb.RenderPNG(r.Context(), nb, bs, all, PPI)
	if err != nil {
		s.fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	w.Write(pngs[n-1])
}

func (s *Server) pdf(w http.ResponseWriter, r *http.Request) {
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
	data, err := os.ReadFile(res.PDF)
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
