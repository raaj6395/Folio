package store

import (
	"path/filepath"
	"testing"
)

func TestImportFrom(t *testing.T) {
	src, err := Open(filepath.Join(t.TempDir(), "src.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	a, _ := src.CreateNotebook(LocalUser, Notebook{Title: "A", Theme: "clean", Paper: "plain", Mode: "dark", Density: "airy", Template: "article"})
	src.db.Exec(`UPDATE notebooks SET guidelines = 'Short paragraphs.' WHERE id = ?`, a.ID)
	appendText(t, src, a.ID, "x")
	appendText(t, src, a.ID, "y", "z")
	gone, _ := src.CreateNotebook(LocalUser, Notebook{Title: "Gone", Theme: "clean", Paper: "plain", Mode: "light"})
	src.Delete(LocalUser, gone.ID)

	eachBackend(t, func(t *testing.T, dst *Store) {
		dry, err := dst.ImportFrom(src, LocalUser, false, true)
		if err != nil || len(dry) != 2 || dry[0].Status != "would import" || dry[1].Status != "skipped (deleted)" {
			t.Fatalf("dry run: %+v %v", dry, err)
		}
		if nbs, _ := dst.Notebooks(LocalUser); len(nbs) != 0 {
			t.Fatal("dry run wrote data")
		}
		res, err := dst.ImportFrom(src, LocalUser, false, false)
		if err != nil || res[0].Status != "imported" {
			t.Fatalf("import: %+v %v", res, err)
		}
		got, err := dst.Notebook(LocalUser, a.ID)
		if err != nil || got.Version != 3 || got.Mode != "dark" || got.Title != "A" || got.Density != "airy" || got.Guidelines != "Short paragraphs." || got.Template != "article" {
			t.Fatalf("notebook: %+v %v", got, err)
		}
		if txt := texts(t, dst, a.ID); txt != "xyz" {
			t.Fatalf("blocks: %q", txt)
		}
		if _, _, err := dst.Undo(LocalUser, a.ID, nil); err != nil {
			t.Fatal(err)
		}
		if txt := texts(t, dst, a.ID); txt != "x" {
			t.Fatalf("undo after import: %q", txt)
		}
		again, _ := dst.ImportFrom(src, LocalUser, false, false)
		if again[0].Status != "skipped (exists)" {
			t.Fatalf("second run must skip: %+v", again)
		}
	})
}
