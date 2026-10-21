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
	if _, err := s.db.Exec(`TRUNCATE notebooks, blocks, snapshots`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
