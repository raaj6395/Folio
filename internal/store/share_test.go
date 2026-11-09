package store

import (
	"errors"
	"testing"

	"github.com/raaj6395/folio/internal/model"
)

func TestShare(t *testing.T) {
	eachBackend(t, func(t *testing.T, s *Store) {
		alice, _ := s.CreateUser("alice@example.com", "x")
		bob, _ := s.CreateUser("bob@example.com", "x")
		nb, _ := s.CreateNotebook(alice.ID, Notebook{Title: "TCP", Theme: "clean", Paper: "plain", Mode: "dark"})
		if _, err := s.SharedNotebook(""); !errors.Is(err, ErrNotFound) {
			t.Fatal("an empty token must never match the unshared notebooks")
		}
		tok, err := s.Share(alice.ID, nb.ID)
		if err != nil || len(tok) != 22 {
			t.Fatalf("share: %q %v", tok, err)
		}
		if again, _ := s.Share(alice.ID, nb.ID); again != tok {
			t.Fatal("sharing twice must keep the same link")
		}
		if got, err := s.SharedNotebook(tok); err != nil || got.ID != nb.ID || got.UserID != alice.ID {
			t.Fatalf("resolve: %+v %v", got, err)
		}
		if _, err := s.Share(bob.ID, nb.ID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("bob shared alice's notebook: %v", err)
		}
		if err := s.Unshare(bob.ID, nb.ID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("bob unshared alice's notebook: %v", err)
		}

		// Edits and undo never touch the link.
		s.Mutate(alice.ID, nb.ID, nil, "rename", "x", func(n *Notebook, bs []model.Block) ([]model.Block, error) { n.Title = "TCP 2"; return bs, nil })
		s.Undo(alice.ID, nb.ID, nil)
		if got, _ := s.Notebook(alice.ID, nb.ID); got.ShareToken != tok || got.Version != 3 {
			t.Fatalf("undo changed sharing: %+v", got)
		}

		if err := s.Unshare(alice.ID, nb.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := s.SharedNotebook(tok); !errors.Is(err, ErrNotFound) {
			t.Fatal("unshared link still works")
		}
		tok2, _ := s.Share(alice.ID, nb.ID)
		if tok2 == tok {
			t.Fatal("sharing again must make a new link")
		}

		s.Delete(alice.ID, nb.ID)
		if _, err := s.SharedNotebook(tok2); !errors.Is(err, ErrNotFound) {
			t.Fatal("deleted notebook still shared")
		}
		s.Undelete(alice.ID, nb.ID)
		if _, err := s.SharedNotebook(tok2); !errors.Is(err, ErrNotFound) {
			t.Fatal("restoring must not bring the old link back")
		}

		tok3, _ := s.Share(alice.ID, nb.ID)
		s.DisableUser(alice.ID)
		if _, err := s.SharedNotebook(tok3); !errors.Is(err, ErrNotFound) {
			t.Fatal("a disabled account's links must stop working")
		}
	})
}
