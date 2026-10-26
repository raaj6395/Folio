package store

import (
	"errors"
	"testing"

	"github.com/raaj6395/folio/internal/model"
)

func TestFolders(t *testing.T) {
	eachBackend(t, func(t *testing.T, s *Store) {
		a, _ := s.CreateUser("a@example.com", "h")
		b, _ := s.CreateUser("b@example.com", "h")

		lab, err := s.EnsureFolderPath(a.ID, " semester 1 / Networks/Labs ")
		if err != nil || lab.Path != "semester 1/Networks/Labs" || lab.Depth != 3 {
			t.Fatalf("ensure: %+v %v", lab, err)
		}
		again, _ := s.EnsureFolderPath(a.ID, "SEMESTER 1/networks/labs")
		if again.ID != lab.ID {
			t.Fatal("ensure must reuse folders whatever the case")
		}
		net, _ := s.EnsureFolderPath(a.ID, "Semester 1/Networks")
		sem, _ := s.EnsureFolderPath(a.ID, "Semester 1")

		for name, err := range map[string]error{
			"too deep":  first(s.EnsureFolderPath(a.ID, "Semester 1/Networks/Labs/Week 1")),
			"too deep2": first(s.CreateFolder(a.ID, lab.ID, "Week 1")),
			"duplicate": first(s.CreateFolder(a.ID, sem.ID, "NETWORKS")),
			"slash":     first(s.CreateFolder(a.ID, "", "a/b")),
			"empty":     first(s.CreateFolder(a.ID, "", "   ")),
		} {
			if !errors.Is(err, ErrInvalid) {
				t.Errorf("%s: %v", name, err)
			}
		}
		if _, err := s.CreateFolder(b.ID, sem.ID, "Inside alice's"); !errors.Is(err, ErrNotFound) {
			t.Errorf("bob created a folder inside alice's: %v", err)
		}
		if err := s.RenameFolder(b.ID, sem.ID, "pwned"); !errors.Is(err, ErrNotFound) {
			t.Errorf("bob renamed alice's folder: %v", err)
		}
		if _, err := s.DeleteFolder(b.ID, sem.ID); !errors.Is(err, ErrNotFound) {
			t.Errorf("bob deleted alice's folder: %v", err)
		}
		if bf, _ := s.Folders(b.ID); len(bf) != 0 {
			t.Errorf("bob sees folders: %+v", bf)
		}

		nb, err := s.CreateNotebook(a.ID, "TCP", "clean", "plain", "dark", "", lab.ID)
		if err != nil || nb.FolderID != lab.ID {
			t.Fatalf("create in folder: %v", err)
		}
		if _, err := s.CreateNotebook(b.ID, "x", "clean", "plain", "dark", "", lab.ID); !errors.Is(err, ErrNotFound) {
			t.Errorf("bob created a notebook in alice's folder: %v", err)
		}
		bobNB, _ := s.CreateNotebook(b.ID, "bob", "clean", "plain", "dark", "", "")
		if err := s.MoveNotebook(b.ID, bobNB.ID, sem.ID); !errors.Is(err, ErrNotFound) {
			t.Errorf("bob moved his notebook into alice's folder: %v", err)
		}
		if err := s.MoveNotebook(b.ID, nb.ID, ""); !errors.Is(err, ErrNotFound) {
			t.Errorf("bob moved alice's notebook: %v", err)
		}

		// Moving isn't an edit: version, updated_at and history stay put, and undo doesn't undo it.
		s.Mutate(a.ID, nb.ID, nil, "append_blocks", "x", func(_ *Notebook, bs []model.Block) ([]model.Block, error) {
			return model.InsertAt(bs, 0, []model.Block{para("one")}), nil
		})
		before, _ := s.Notebook(a.ID, nb.ID)
		if err := s.MoveNotebook(a.ID, nb.ID, net.ID); err != nil {
			t.Fatal(err)
		}
		after, _ := s.Notebook(a.ID, nb.ID)
		vs, _ := s.Versions(a.ID, nb.ID, 10)
		if after.Version != before.Version || !after.UpdatedAt.Equal(before.UpdatedAt) || len(vs) != 2 || after.FolderID != net.ID {
			t.Errorf("move changed the notebook: %+v → %+v, %d versions", before, after, len(vs))
		}
		s.Undo(a.ID, nb.ID, nil)
		if u, _ := s.Notebook(a.ID, nb.ID); u.FolderID != net.ID {
			t.Error("undo undid a move")
		}

		fs, _ := s.Folders(a.ID)
		if len(fs) != 3 || fs[0].ID != sem.ID || fs[1].ID != net.ID || fs[2].ID != lab.ID || fs[0].Count != 1 || fs[1].Count != 1 || fs[2].Count != 0 {
			t.Fatalf("tree order and counts: %+v", fs)
		}

		// Deleting a folder moves its contents up and never deletes a notebook.
		s.CreateFolder(a.ID, sem.ID, "Labs")
		if _, err := s.DeleteFolder(a.ID, net.ID); err != nil {
			t.Fatal(err)
		}
		if u, _ := s.Notebook(a.ID, nb.ID); u.FolderID != sem.ID {
			t.Errorf("notebook went to %q, want the parent", u.FolderID)
		}
		fs, _ = s.Folders(a.ID)
		var paths []string
		for _, f := range fs {
			paths = append(paths, f.Path)
		}
		if len(paths) != 3 || paths[1] != "semester 1/Labs" || paths[2] != "semester 1/Labs (2)" {
			t.Errorf("after delete: %v", paths)
		}
	})
}

func first[T any](_ T, err error) error { return err }

func TestFolderPolish(t *testing.T) {
	eachBackend(t, func(t *testing.T, s *Store) {
		a, _ := s.CreateUser("a@example.com", "h")
		b, _ := s.CreateUser("b@example.com", "h")
		x, _ := s.EnsureFolderPath(a.ID, "X")
		xy, _ := s.EnsureFolderPath(a.ID, "X/Y")
		z, _ := s.EnsureFolderPath(a.ID, "Z")
		zz, _ := s.EnsureFolderPath(a.ID, "Z/Z2/Z3")

		// Move folders: no cycles, depth checked including the moved subtree, owner only.
		if err := s.MoveFolder(a.ID, x.ID, xy.ID); !errors.Is(err, ErrInvalid) {
			t.Errorf("folder into its own child: %v", err)
		}
		if err := s.MoveFolder(a.ID, x.ID, x.ID); !errors.Is(err, ErrInvalid) {
			t.Errorf("folder into itself: %v", err)
		}
		if err := s.MoveFolder(a.ID, x.ID, z.ID); err != nil {
			t.Errorf("X (2 levels) into Z: %v", err)
		}
		if err := s.MoveFolder(a.ID, x.ID, zz.ParentID); !errors.Is(err, ErrInvalid) {
			t.Errorf("X/Y under Z/Z2 would be 4 deep: %v", err)
		}
		if err := s.MoveFolder(b.ID, x.ID, ""); !errors.Is(err, ErrNotFound) {
			t.Errorf("bob moved alice's folder: %v", err)
		}
		if err := s.MoveFolder(a.ID, x.ID, ""); err != nil {
			t.Fatal(err)
		}
		if fs, _ := s.Folders(a.ID); fs[0].Path != "X" || fs[1].Path != "X/Y" {
			t.Errorf("after moving back: %+v", fs)
		}

		// Colours: validated, owner only.
		if err := s.SetFolderColor(a.ID, x.ID, "green"); err != nil {
			t.Fatal(err)
		}
		if err := s.SetFolderColor(a.ID, x.ID, "chartreuse"); !errors.Is(err, ErrInvalid) {
			t.Errorf("bad colour: %v", err)
		}
		if err := s.SetFolderColor(b.ID, x.ID, "red"); !errors.Is(err, ErrNotFound) {
			t.Errorf("bob coloured alice's folder: %v", err)
		}
		if fs, _ := s.Folders(a.ID); fs[0].Color != "green" {
			t.Errorf("colour: %+v", fs[0])
		}

		// Favourites don't touch version or updated_at; owner only.
		nb, _ := s.CreateNotebook(a.ID, "n", "clean", "plain", "dark", "", "")
		if err := s.SetFavorite(a.ID, nb.ID, true); err != nil {
			t.Fatal(err)
		}
		if got, _ := s.Notebook(a.ID, nb.ID); !got.Favorite || got.Version != nb.Version || !got.UpdatedAt.Equal(nb.UpdatedAt) {
			t.Errorf("favourite: %+v", got)
		}
		if err := s.SetFavorite(b.ID, nb.ID, false); !errors.Is(err, ErrNotFound) {
			t.Errorf("bob unstarred alice's notebook: %v", err)
		}
		s.SetFavorite(a.ID, nb.ID, false)
		if got, _ := s.Notebook(a.ID, nb.ID); got.Favorite {
			t.Error("unstar")
		}

		// Bulk move skips IDs that aren't the user's.
		nb2, _ := s.CreateNotebook(a.ID, "n2", "clean", "plain", "dark", "", "")
		bobs, _ := s.CreateNotebook(b.ID, "bob", "clean", "plain", "dark", "", "")
		n, err := s.MoveNotebooks(a.ID, []string{nb.ID, nb2.ID, bobs.ID, "n_missing"}, xy.ID)
		if err != nil || n != 2 {
			t.Fatalf("bulk move: %d %v", n, err)
		}
		if got, _ := s.Notebook(b.ID, bobs.ID); got.FolderID != "" {
			t.Error("alice moved bob's notebook")
		}
		if _, err := s.MoveNotebooks(b.ID, []string{bobs.ID}, xy.ID); !errors.Is(err, ErrNotFound) {
			t.Errorf("bob bulk-moved into alice's folder: %v", err)
		}
	})
}
