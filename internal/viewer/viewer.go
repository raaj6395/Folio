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
	"os"
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

var (
	indexTmpl    = template.Must(template.New("index").Parse(indexHTML))
	notebookTmpl = template.Must(template.New("notebook").Parse(notebookHTML))
)

type Server struct {
	st *store.Store
	lb *layout.Builder
}

func New(st *store.Store, lb *layout.Builder) http.Handler {
	s := &Server{st, lb}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.index)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "folio viewer ok") })
	mux.HandleFunc("GET /n/{id}", s.notebook)
	mux.HandleFunc("GET /n/{id}/events", s.events)
	mux.HandleFunc("GET /n/{id}/page/{file}", s.page)
	mux.HandleFunc("GET /n/{id}/pdf", s.pdf)
	return mux
}

type State struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Theme     string    `json:"theme"`
	Version   int       `json:"version"`
	PageCount int       `json:"page_count"`
	UpdatedAt time.Time `json:"updated_at"`
	Pages     []string  `json:"pages"`
}

func (s *Server) state(ctx context.Context, id string) (State, error) {
	nb, err := s.st.Notebook(id)
	if err != nil {
		return State{}, err
	}
	bs, err := s.st.Blocks(id)
	if err != nil {
		return State{}, err
	}
	r, err := s.lb.Build(ctx, nb, bs)
	if err != nil {
		return State{}, err
	}
	return State{ID: nb.ID, Title: nb.Title, Theme: nb.Theme, Version: nb.Version, PageCount: r.PageCount, UpdatedAt: nb.UpdatedAt, Pages: signatures(nb, bs, r)}, nil
}

func signatures(nb store.Notebook, bs []model.Block, r *layout.Result) []string {
	hs := make([]*strings.Builder, r.PageCount)
	for i := range hs {
		hs[i] = &strings.Builder{}
		fmt.Fprintf(hs[i], "%s|%s|%s|%d|", nb.Title, nb.Theme, nb.Paper, r.PageCount)
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
	nbs, err := s.st.Notebooks()
	if err != nil {
		s.fail(w, err)
		return
	}
	indexTmpl.Execute(w, nbs)
}

func (s *Server) notebook(w http.ResponseWriter, r *http.Request) {
	nb, err := s.st.Notebook(r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
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
		if nb, err := s.st.Notebook(id); err != nil {
			fmt.Fprintf(w, "event: gone\ndata: {}\n\n")
			fl.Flush()
			return
		} else if nb.Version != last {
			st, err := s.state(r.Context(), id)
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
	id := r.PathValue("id")
	nb, err := s.st.Notebook(id)
	if err != nil {
		s.fail(w, err)
		return
	}
	bs, err := s.st.Blocks(id)
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
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Write(pngs[n-1])
}

func (s *Server) pdf(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	nb, err := s.st.Notebook(id)
	if err != nil {
		s.fail(w, err)
		return
	}
	bs, err := s.st.Blocks(id)
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
