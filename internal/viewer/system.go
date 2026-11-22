package viewer

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"

	"github.com/raaj6395/folio/internal/layout"
	"github.com/raaj6395/folio/internal/model"
	"github.com/raaj6395/folio/internal/sim"
	"github.com/raaj6395/folio/internal/store"
)

// system loads the notebook behind r as a system: its spec, or nil while it is still empty.
func (s *Server) system(r *http.Request) (store.Notebook, *sim.Spec, error) {
	nb, bs, err := s.load(r)
	if err == nil && nb.Format != "system" {
		err = store.ErrNotFound
	}
	if err != nil || len(bs) == 0 {
		return nb, nil, err
	}
	sp, _ := bs[0].SystemSpec()
	return nb, sp, nil
}

// systemLayout lays the system out with D2, cached per notebook by source and style.
func (s *Server) systemLayout(r *http.Request, nb store.Notebook, sp *sim.Spec) (*sim.Layout, string, error) {
	th, err := layout.ThemeByName(nb.Theme)
	if err != nil {
		return nil, "", err
	}
	return sim.Build(r.Context(), filepath.Join(s.lb.Dir(nb.ID), "system"), sp, th.D2For(nb.Mode))
}

func jsonError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// systemJSON is what the viewer draws: the spec and where D2 put every node and edge.
func (s *Server) systemJSON(w http.ResponseWriter, r *http.Request) {
	nb, sp, err := s.system(r)
	if err != nil {
		s.fail(w, err)
		return
	}
	out := map[string]any{"version": nb.Version, "title": nb.Title}
	if sp != nil {
		lay, _, err := s.systemLayout(r, nb, sp)
		if err != nil {
			s.fail(w, err)
			return
		}
		out["spec"], out["layout"] = sp, lay
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(out)
}

// maxRunBody bounds a run request: the action plus the viewer's state.
const maxRunBody = 64 << 10

// systemRun plays one action (send, kill, revive, flush, reset, scenario) against the state the
// viewer sends and returns the events to animate and the new state. Nothing is stored.
func (s *Server) systemRun(w http.ResponseWriter, r *http.Request) {
	var body struct {
		sim.Input
		State sim.State `json:"state"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRunBody)).Decode(&body); err != nil {
		jsonError(w, http.StatusBadRequest, "bad request: "+err.Error())
		return
	}
	_, sp, err := s.system(r)
	if errors.Is(err, store.ErrNotFound) {
		jsonError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	if sp == nil {
		jsonError(w, http.StatusConflict, "this system is still empty")
		return
	}
	res, err := sim.Run(sp, body.State, body.Input)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(res)
}

// systemThumb serves D2's own drawing of the system as the library thumbnail.
func (s *Server) systemThumb(w http.ResponseWriter, r *http.Request, nb store.Notebook, bs []model.Block) {
	if len(bs) == 0 {
		http.NotFound(w, r)
		return
	}
	sp, _ := bs[0].SystemSpec()
	_, path, err := s.systemLayout(r, nb, sp)
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
	w.Header().Set("Content-Security-Policy", "default-src 'none'; img-src data:; style-src 'unsafe-inline'")
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	w.Write(data)
}
