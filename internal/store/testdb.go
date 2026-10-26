package store

import (
	"os"
	"testing"
)

func OpenTestPostgres(t testing.TB) *Store {
	t.Helper()
	dsn := os.Getenv("FOLIO_TEST_DB_URL")
	if dsn == "" {
		t.Skip("FOLIO_TEST_DB_URL not set; skipping Postgres")
	}
	s, err := Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`TRUNCATE notebooks, folders, blocks, snapshots, api_tokens, web_sessions, oauth_codes, oauth_grants, oauth_clients, users; ALTER TABLE notebooks ALTER COLUMN user_id DROP NOT NULL`); err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureLocalUser(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
