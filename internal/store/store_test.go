package store

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/raaj6395/folio/internal/model"
)

func open(t *testing.T) *Store {
	s, err := Open(filepath.Join(t.TempDir(), "folio.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func para(text string) model.Block {
	b, _ := model.Parse(json.RawMessage(`{"type":"paragraph","text":"` + text + `"}`))
	b.ID = model.NewID("b_")
	return b
}

func appendText(t *testing.T, s *Store, id string, texts ...string) {
	t.Helper()
	_, _, err := s.Mutate(id, nil, "append_blocks", "append", func(nb *Notebook, bs []model.Block) ([]model.Block, error) {
		var add []model.Block
		for _, x := range texts {
			add = append(add, para(x))
		}
		return model.InsertAt(bs, len(bs), add), nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func texts(t *testing.T, s *Store, id string) string {
	t.Helper()
	bs, err := s.Blocks(id)
	if err != nil {
		t.Fatal(err)
	}
	out := ""
	for _, b := range bs {
		out += b.Summary()
	}
	return out
}

func TestMutateUndoRestore(t *testing.T) {
	s := open(t)
	nb, err := s.CreateNotebook("T", "clean", "plain")
	if err != nil {
		t.Fatal(err)
	}
	appendText(t, s, nb.ID, "a")
	appendText(t, s, nb.ID, "b", "c")
	if got := texts(t, s, nb.ID); got != "abc" {
		t.Fatalf("got %q", got)
	}
	if _, _, err := s.Undo(nb.ID, nil); err != nil {
		t.Fatal(err)
	}
	if got := texts(t, s, nb.ID); got != "a" {
		t.Fatalf("after undo got %q", got)
	}
	if _, _, err := s.Undo(nb.ID, nil); err != nil {
		t.Fatal(err)
	}
	if got := texts(t, s, nb.ID); got != "" {
		t.Fatalf("after 2nd undo got %q", got)
	}
	if _, _, err := s.Undo(nb.ID, nil); err == nil {
		t.Fatal("undo past the start should fail")
	}
	if _, _, err := s.Restore(nb.ID, 3, nil); err != nil {
		t.Fatal(err)
	}
	if got := texts(t, s, nb.ID); got != "abc" {
		t.Fatalf("after restore got %q", got)
	}
	if _, _, err := s.Undo(nb.ID, nil); err != nil {
		t.Fatal(err)
	}
	if got := texts(t, s, nb.ID); got != "" {
		t.Fatalf("undo of restore got %q", got)
	}
	vs, _ := s.Versions(nb.ID, 3)
	if len(vs) != 3 || vs[0].Version != 7 || vs[0].ToolName != "undo" {
		t.Fatalf("versions: %+v", vs)
	}
}

func TestExpectedVersionConflict(t *testing.T) {
	s := open(t)
	nb, _ := s.CreateNotebook("T", "clean", "plain")
	stale := 0
	_, _, err := s.Mutate(nb.ID, &stale, "x", "x", func(nb *Notebook, bs []model.Block) ([]model.Block, error) { return bs, nil })
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("got %v", err)
	}
}

func TestSnapshotPruning(t *testing.T) {
	s := open(t)
	nb, _ := s.CreateNotebook("T", "clean", "plain")
	for range keepSnapshots + 10 {
		appendText(t, s, nb.ID, "x")
	}
	var n, min int
	s.db.QueryRow(`SELECT count(*), min(version) FROM snapshots WHERE notebook_id = ?`, nb.ID).Scan(&n, &min)
	if n != keepSnapshots || min != 12 {
		t.Fatalf("snapshots: count %d min %d", n, min)
	}
}

func TestSoftDelete(t *testing.T) {
	s := open(t)
	nb, _ := s.CreateNotebook("T", "clean", "plain")
	if err := s.Delete(nb.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Notebook(nb.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v", err)
	}
	if nbs, _ := s.Notebooks(); len(nbs) != 0 {
		t.Fatal("deleted notebook still listed")
	}
}
