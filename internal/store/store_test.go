package store

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/raaj6395/folio/internal/model"
)

func eachBackend(t *testing.T, fn func(t *testing.T, s *Store)) {
	t.Run("sqlite", func(t *testing.T) {
		s, err := Open(filepath.Join(t.TempDir(), "folio.db"))
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		fn(t, s)
	})
	t.Run("postgres", func(t *testing.T) {
		s := OpenTestPostgres(t)
		fn(t, s)
	})
}

func para(text string) model.Block {
	b, _ := model.Parse(json.RawMessage(`{"type":"paragraph","text":"` + text + `"}`))
	b.ID = model.NewID("b_")
	return b
}

func appendText(t *testing.T, s *Store, id string, texts ...string) {
	t.Helper()
	_, _, err := s.Mutate(LocalUser, id, nil, "append_blocks", "append", func(nb *Notebook, bs []model.Block) ([]model.Block, error) {
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
	bs, err := s.Blocks(LocalUser, id)
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
	eachBackend(t, func(t *testing.T, s *Store) {
		nb, err := s.CreateNotebook(LocalUser, "T", "clean", "plain", "light", "", "")
		if err != nil {
			t.Fatal(err)
		}
		appendText(t, s, nb.ID, "a")
		appendText(t, s, nb.ID, "b", "c")
		if got := texts(t, s, nb.ID); got != "abc" {
			t.Fatalf("got %q", got)
		}
		if _, _, err := s.Undo(LocalUser, nb.ID, nil); err != nil {
			t.Fatal(err)
		}
		if got := texts(t, s, nb.ID); got != "a" {
			t.Fatalf("after undo got %q", got)
		}
		if _, _, err := s.Undo(LocalUser, nb.ID, nil); err != nil {
			t.Fatal(err)
		}
		if got := texts(t, s, nb.ID); got != "" {
			t.Fatalf("after 2nd undo got %q", got)
		}
		if _, _, err := s.Undo(LocalUser, nb.ID, nil); err == nil {
			t.Fatal("undo past the start should fail")
		}
		if _, _, err := s.Restore(LocalUser, nb.ID, 3, nil); err != nil {
			t.Fatal(err)
		}
		if got := texts(t, s, nb.ID); got != "abc" {
			t.Fatalf("after restore got %q", got)
		}
		if _, _, err := s.Undo(LocalUser, nb.ID, nil); err != nil {
			t.Fatal(err)
		}
		if got := texts(t, s, nb.ID); got != "" {
			t.Fatalf("undo of restore got %q", got)
		}
		vs, _ := s.Versions(LocalUser, nb.ID, 3)
		if len(vs) != 3 || vs[0].Version != 7 || vs[0].ToolName != "undo" {
			t.Fatalf("versions: %+v", vs)
		}
	})
}

func TestGuidelinesAndDensityAreVersioned(t *testing.T) {
	eachBackend(t, func(t *testing.T, s *Store) {
		nb, _ := s.CreateNotebook(LocalUser, "T", "clean", "plain", "light", "", "")
		if nb.Density != DefaultDensity {
			t.Fatalf("new notebook density: %q", nb.Density)
		}
		set := func(g, d string) {
			t.Helper()
			if _, _, err := s.Mutate(LocalUser, nb.ID, nil, "x", "x", func(n *Notebook, bs []model.Block) ([]model.Block, error) {
				n.Guidelines, n.Density = g, d
				return bs, nil
			}); err != nil {
				t.Fatal(err)
			}
		}
		set("Use British spelling.", "airy")
		set("Use British spelling. One H1 per page.", "compact")
		if _, _, err := s.Undo(LocalUser, nb.ID, nil); err != nil {
			t.Fatal(err)
		}
		if got, _ := s.Notebook(LocalUser, nb.ID); got.Guidelines != "Use British spelling." || got.Density != "airy" {
			t.Fatalf("after undo: %q %q", got.Guidelines, got.Density)
		}
		s.db.Exec(s.q(`UPDATE snapshots SET guidelines = NULL, density = NULL WHERE notebook_id = ? AND version = 1`), nb.ID)
		if _, _, err := s.Restore(LocalUser, nb.ID, 1, nil); err != nil {
			t.Fatal(err)
		}
		if got, _ := s.Notebook(LocalUser, nb.ID); got.Guidelines != "Use British spelling." || got.Density != "airy" {
			t.Fatalf("restoring a snapshot from before the feature must keep current values: %q %q", got.Guidelines, got.Density)
		}
	})
}

func TestExpectedVersionConflict(t *testing.T) {
	eachBackend(t, func(t *testing.T, s *Store) {
		nb, _ := s.CreateNotebook(LocalUser, "T", "clean", "plain", "light", "", "")
		stale := 0
		_, _, err := s.Mutate(LocalUser, nb.ID, &stale, "x", "x", func(nb *Notebook, bs []model.Block) ([]model.Block, error) { return bs, nil })
		if !errors.Is(err, ErrConflict) {
			t.Fatalf("got %v", err)
		}
	})
}

func TestSnapshotPruning(t *testing.T) {
	eachBackend(t, func(t *testing.T, s *Store) {
		nb, _ := s.CreateNotebook(LocalUser, "T", "clean", "plain", "light", "", "")
		for range keepSnapshots + 10 {
			appendText(t, s, nb.ID, "x")
		}
		var n, min int
		s.db.QueryRow(s.q(`SELECT count(*), min(version) FROM snapshots WHERE notebook_id = ?`), nb.ID).Scan(&n, &min)
		if n != keepSnapshots || min != 12 {
			t.Fatalf("snapshots: count %d min %d", n, min)
		}
	})
}

func TestSoftDelete(t *testing.T) {
	eachBackend(t, func(t *testing.T, s *Store) {
		nb, _ := s.CreateNotebook(LocalUser, "T", "clean", "plain", "light", "", "")
		if err := s.Delete(LocalUser, nb.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Notebook(LocalUser, nb.ID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("got %v", err)
		}
		if nbs, _ := s.Notebooks(LocalUser); len(nbs) != 0 {
			t.Fatal("deleted notebook still listed")
		}
	})
}

func TestPositionsSortByteOrder(t *testing.T) {
	eachBackend(t, func(t *testing.T, s *Store) {
		nb, _ := s.CreateNotebook(LocalUser, "T", "clean", "plain", "light", "", "")
		_, _, err := s.Mutate(LocalUser, nb.ID, nil, "x", "x", func(_ *Notebook, _ []model.Block) ([]model.Block, error) {
			var bs []model.Block
			for _, pos := range []string{"z1", "a", "Z", "a0b", "_"} {
				b := para(pos)
				b.Position = pos
				bs = append(bs, b)
			}
			return bs, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if got := texts(t, s, nb.ID); got != "Z_aa0bz1" {
			t.Fatalf("blocks must sort by byte order, got %q", got)
		}
	})
}

func TestPostgresPositionCollationIsC(t *testing.T) {
	s := OpenTestPostgres(t)
	var coll string
	if err := s.db.QueryRow(`SELECT collation_name FROM information_schema.columns WHERE table_name = 'blocks' AND column_name = 'position'`).Scan(&coll); err != nil {
		t.Fatal(err)
	}
	if coll != "C" {
		t.Fatalf(`blocks.position must use COLLATE "C" (byte order), got %q`, coll)
	}
}
