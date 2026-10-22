package store

import (
	"errors"
	"testing"
	"time"

	"github.com/raaj6395/folio/internal/model"
)

func TestIsolationBetweenUsers(t *testing.T) {
	eachBackend(t, func(t *testing.T, s *Store) {
		a, err := s.CreateUser("A@Example.com", "h")
		if err != nil {
			t.Fatal(err)
		}
		b, _ := s.CreateUser("b@example.com", "h")
		nb, err := s.CreateNotebook(a.ID, "secret", "clean", "plain", "dark", "")
		if err != nil {
			t.Fatal(err)
		}
		_, _, err = s.Mutate(a.ID, nb.ID, nil, "x", "x", func(_ *Notebook, bs []model.Block) ([]model.Block, error) {
			return model.InsertAt(bs, 0, []model.Block{para("a1")}), nil
		})
		if err != nil {
			t.Fatal(err)
		}
		noop := func(_ *Notebook, bs []model.Block) ([]model.Block, error) { return bs, nil }
		checks := map[string]error{}
		_, checks["Notebook"] = s.Notebook(b.ID, nb.ID)
		_, checks["Blocks"] = s.Blocks(b.ID, nb.ID)
		_, _, checks["Mutate"] = s.Mutate(b.ID, nb.ID, nil, "x", "x", noop)
		_, _, checks["Undo"] = s.Undo(b.ID, nb.ID, nil)
		_, _, checks["Restore"] = s.Restore(b.ID, nb.ID, 1, nil)
		_, checks["Versions"] = s.Versions(b.ID, nb.ID, 5)
		checks["Delete"] = s.Delete(b.ID, nb.ID)
		for name, err := range checks {
			if !errors.Is(err, ErrNotFound) {
				t.Errorf("%s as another user: got %v, want ErrNotFound", name, err)
			}
		}
		if nbs, _ := s.Notebooks(b.ID); len(nbs) != 0 {
			t.Errorf("user B lists A's notebooks: %v", nbs)
		}
		if nbs, _ := s.Notebooks(a.ID); len(nbs) != 1 {
			t.Errorf("user A should see 1 notebook, got %d", len(nbs))
		}
		if got, _ := s.Blocks(a.ID, nb.ID); len(got) != 1 {
			t.Error("owner lost access")
		}
		if _, err := s.CreateUser("a@example.COM ", "h"); !errors.Is(err, ErrEmailTaken) {
			t.Errorf("duplicate email (case-insensitive): %v", err)
		}
	})
}

func TestOrphansInvisibleUntilAdopted(t *testing.T) {
	eachBackend(t, func(t *testing.T, s *Store) {
		u, _ := s.CreateUser("owner@example.com", "h")
		s.db.Exec(s.q(`INSERT INTO notebooks (id, title, theme, paper, mode, created_at, updated_at, version) VALUES ('n_orphan01','o','clean','plain','dark','x','x',1)`))
		if _, err := s.Notebook(u.ID, "n_orphan01"); !errors.Is(err, ErrNotFound) {
			t.Fatal("orphan visible before adopt")
		}
		if n, _ := s.CountOrphans(); n != 1 {
			t.Fatalf("orphans: %d", n)
		}
		if n, err := s.AdoptOrphans(u.ID); err != nil || n != 1 {
			t.Fatalf("adopt: %d %v", n, err)
		}
		if _, err := s.Notebook(u.ID, "n_orphan01"); err != nil {
			t.Fatalf("after adopt: %v", err)
		}
		if s.postgres {
			if err := s.RequireOwners(); err != nil {
				t.Fatal(err)
			}
		}
	})
}

func TestSessionsAndTokens(t *testing.T) {
	eachBackend(t, func(t *testing.T, s *Store) {
		u, _ := s.CreateUser("u@example.com", "h")
		if err := s.CreateSession(u.ID, "sess-hash", time.Hour); err != nil {
			t.Fatal(err)
		}
		if got, err := s.SessionUser("sess-hash"); err != nil || got.ID != u.ID {
			t.Fatalf("session: %v %v", got, err)
		}
		s.CreateSession(u.ID, "old-hash", -time.Minute)
		if _, err := s.SessionUser("old-hash"); !errors.Is(err, ErrNotFound) {
			t.Error("expired session accepted")
		}
		tok, err := s.CreateToken(u.ID, "laptop", "tok-hash", "abcd")
		if err != nil {
			t.Fatal(err)
		}
		if got, _, err := s.TokenUser("tok-hash"); err != nil || got.ID != u.ID {
			t.Fatalf("token: %v %v", got, err)
		}
		other, _ := s.CreateUser("x@example.com", "h")
		if err := s.RevokeToken(other.ID, tok.ID); !errors.Is(err, ErrNotFound) {
			t.Error("another user revoked my token")
		}
		if err := s.RevokeToken(u.ID, tok.ID); err != nil {
			t.Fatal(err)
		}
		if _, _, err := s.TokenUser("tok-hash"); !errors.Is(err, ErrNotFound) {
			t.Error("revoked token still works")
		}
		s.CreateToken(u.ID, "second", "tok2", "wxyz")
		if err := s.DisableUser(u.ID); err != nil {
			t.Fatal(err)
		}
		if _, _, err := s.TokenUser("tok2"); !errors.Is(err, ErrNotFound) {
			t.Error("disabled user's token still works")
		}
		if _, err := s.SessionUser("sess-hash"); !errors.Is(err, ErrNotFound) {
			t.Error("disabled user's session still works")
		}
	})
}

func TestQuotas(t *testing.T) {
	eachBackend(t, func(t *testing.T, s *Store) {
		old := MaxNotebooks
		MaxNotebooks = 2
		defer func() { MaxNotebooks = old }()
		u, _ := s.CreateUser("q@example.com", "h")
		s.CreateNotebook(u.ID, "1", "clean", "plain", "dark", "")
		s.CreateNotebook(u.ID, "2", "clean", "plain", "dark", "")
		if _, err := s.CreateNotebook(u.ID, "3", "clean", "plain", "dark", ""); !errors.Is(err, ErrQuota) {
			t.Fatalf("notebook quota: %v", err)
		}
	})
}
