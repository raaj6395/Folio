package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"

	"github.com/raaj6395/folio/internal/model"
)

const keepSnapshots = 200

var pgMigrations = []string{
	`CREATE TABLE notebooks (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		theme TEXT NOT NULL,
		paper TEXT NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		version INTEGER NOT NULL,
		deleted_at TEXT,
		mode TEXT NOT NULL DEFAULT 'light'
	);
	CREATE TABLE blocks (
		notebook_id TEXT NOT NULL REFERENCES notebooks(id),
		id TEXT NOT NULL,
		position TEXT COLLATE "C" NOT NULL,
		type TEXT NOT NULL,
		data TEXT NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		PRIMARY KEY (notebook_id, id)
	);
	CREATE INDEX blocks_order ON blocks(notebook_id, position);
	CREATE TABLE snapshots (
		notebook_id TEXT NOT NULL REFERENCES notebooks(id),
		version INTEGER NOT NULL,
		base INTEGER NOT NULL,
		blocks TEXT NOT NULL,
		theme TEXT NOT NULL,
		paper TEXT NOT NULL,
		mode TEXT NOT NULL DEFAULT 'light',
		tool_name TEXT NOT NULL,
		summary TEXT NOT NULL,
		timestamp TEXT NOT NULL,
		PRIMARY KEY (notebook_id, version)
	)`,
}

var migrations = []string{
	`CREATE TABLE notebooks (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		theme TEXT NOT NULL,
		paper TEXT NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		version INTEGER NOT NULL,
		deleted_at TEXT
	);
	CREATE TABLE blocks (
		notebook_id TEXT NOT NULL REFERENCES notebooks(id),
		id TEXT NOT NULL,
		position TEXT NOT NULL,
		type TEXT NOT NULL,
		data TEXT NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		PRIMARY KEY (notebook_id, id)
	);
	CREATE INDEX blocks_order ON blocks(notebook_id, position);
	CREATE TABLE snapshots (
		notebook_id TEXT NOT NULL REFERENCES notebooks(id),
		version INTEGER NOT NULL,
		base INTEGER NOT NULL,
		blocks TEXT NOT NULL,
		theme TEXT NOT NULL,
		paper TEXT NOT NULL,
		tool_name TEXT NOT NULL,
		summary TEXT NOT NULL,
		timestamp TEXT NOT NULL,
		PRIMARY KEY (notebook_id, version)
	);`,
	`ALTER TABLE notebooks ADD COLUMN mode TEXT NOT NULL DEFAULT 'light';
	ALTER TABLE snapshots ADD COLUMN mode TEXT NOT NULL DEFAULT 'light';`,
}

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("version conflict")
)

type Notebook struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Theme     string    `json:"theme"`
	Paper     string    `json:"paper"`
	Mode      string    `json:"mode"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Version   int       `json:"version"`
}

type Snapshot struct {
	Version   int       `json:"version"`
	ToolName  string    `json:"tool_name"`
	Summary   string    `json:"summary"`
	Timestamp time.Time `json:"timestamp"`
}

type Store struct {
	db       *sql.DB
	postgres bool
}

func IsPostgres(dsn string) bool {
	return strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://")
}

func Open(dsn string) (*Store, error) {
	if IsPostgres(dsn) {
		return openPostgres(dsn)
	}
	db, err := sql.Open("sqlite", "file:"+dsn+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_txlock=immediate")
	if err != nil {
		return nil, err
	}
	var v int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		db.Close()
		return nil, fmt.Errorf("opening %s: %w", dsn, err)
	}
	for ; v < len(migrations); v++ {
		if _, err := db.Exec(migrations[v] + fmt.Sprintf(";PRAGMA user_version=%d", v+1)); err != nil {
			db.Close()
			return nil, fmt.Errorf("migration %d: %w", v+1, err)
		}
	}
	return &Store{db: db}, nil
}

func openPostgres(dsn string) (*Store, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(10)
	fail := func(err error) (*Store, error) {
		db.Close()
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return fail(fmt.Errorf("connecting to Postgres: %w", err))
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY)`); err != nil {
		return fail(err)
	}
	var v int
	if err := db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&v); err != nil {
		return fail(err)
	}
	for ; v < len(pgMigrations); v++ {
		tx, err := db.Begin()
		if err != nil {
			return fail(err)
		}
		for _, stmt := range strings.Split(pgMigrations[v], ";") {
			if strings.TrimSpace(stmt) == "" {
				continue
			}
			if _, err := tx.Exec(stmt); err != nil {
				tx.Rollback()
				return fail(fmt.Errorf("postgres migration %d: %w", v+1, err))
			}
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (version) VALUES ($1)`, v+1); err != nil {
			tx.Rollback()
			return fail(err)
		}
		if err := tx.Commit(); err != nil {
			return fail(err)
		}
	}
	return &Store{db: db, postgres: true}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Backend() string {
	if s.postgres {
		return "postgres"
	}
	return "sqlite"
}

func (s *Store) Ping() error { return s.db.Ping() }

func (s *Store) q(query string) string {
	if !s.postgres {
		return query
	}
	var b strings.Builder
	n := 0
	for _, c := range query {
		if c == '?' {
			n++
			fmt.Fprintf(&b, "$%d", n)
			continue
		}
		b.WriteRune(c)
	}
	return b.String()
}

const tsLayout = "2006-01-02T15:04:05.000000000Z07:00"

func ts(t time.Time) string { return t.UTC().Format(tsLayout) }

func parseTS(s string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, s)
	return t
}

type scanner interface{ Scan(...any) error }

func scanNotebook(r scanner) (Notebook, error) {
	var nb Notebook
	var c, u string
	err := r.Scan(&nb.ID, &nb.Title, &nb.Theme, &nb.Paper, &c, &u, &nb.Version, &nb.Mode)
	if errors.Is(err, sql.ErrNoRows) {
		return nb, ErrNotFound
	}
	nb.CreatedAt, nb.UpdatedAt = parseTS(c), parseTS(u)
	return nb, err
}

const nbCols = `id, title, theme, paper, created_at, updated_at, version, mode`

func (s *Store) CreateNotebook(title, theme, paper, mode string) (Notebook, error) {
	now := time.Now()
	nb := Notebook{ID: model.NewID("n_"), Title: title, Theme: theme, Paper: paper, Mode: mode, CreatedAt: now, UpdatedAt: now, Version: 1}
	tx, err := s.db.Begin()
	if err != nil {
		return nb, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(s.q(`INSERT INTO notebooks (`+nbCols+`) VALUES (?,?,?,?,?,?,?,?)`), nb.ID, title, theme, paper, ts(now), ts(now), 1, mode); err != nil {
		return nb, err
	}
	if err := s.snapshot(tx, nb, nil, 1, "create_notebook", "created notebook"); err != nil {
		return nb, err
	}
	return nb, tx.Commit()
}

func (s *Store) Notebook(id string) (Notebook, error) {
	return scanNotebook(s.db.QueryRow(s.q(`SELECT `+nbCols+` FROM notebooks WHERE id = ? AND deleted_at IS NULL`), id))
}

func (s *Store) Notebooks() ([]Notebook, error) {
	rows, err := s.db.Query(`SELECT ` + nbCols + ` FROM notebooks WHERE deleted_at IS NULL ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	nbs := []Notebook{}
	for rows.Next() {
		nb, err := scanNotebook(rows)
		if err != nil {
			return nil, err
		}
		nbs = append(nbs, nb)
	}
	return nbs, rows.Err()
}

func (s *Store) Delete(id string) error {
	res, err := s.db.Exec(s.q(`UPDATE notebooks SET deleted_at = ? WHERE id = ? AND deleted_at IS NULL`), ts(time.Now()), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) Blocks(id string) ([]model.Block, error) {
	return s.blocks(s.db, id)
}

type querier interface {
	Query(string, ...any) (*sql.Rows, error)
}

func (s *Store) blocks(q querier, id string) ([]model.Block, error) {
	rows, err := q.Query(s.q(`SELECT id, position, type, data, created_at, updated_at FROM blocks WHERE notebook_id = ? ORDER BY position`), id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Block{}
	for rows.Next() {
		var b model.Block
		var data, c, u string
		if err := rows.Scan(&b.ID, &b.Position, &b.Type, &data, &c, &u); err != nil {
			return nil, err
		}
		b.Data, b.CreatedAt, b.UpdatedAt = json.RawMessage(data), parseTS(c), parseTS(u)
		out = append(out, b)
	}
	return out, rows.Err()
}

type MutateFunc func(nb *Notebook, blocks []model.Block) ([]model.Block, error)

func (s *Store) Mutate(id string, expected *int, tool, summary string, fn MutateFunc) (Notebook, []model.Block, error) {
	return s.mutate(id, expected, tool, summary, 0, fn)
}

func (s *Store) mutate(id string, expected *int, tool, summary string, base int, fn MutateFunc) (Notebook, []model.Block, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return Notebook{}, nil, err
	}
	defer tx.Rollback()
	lock := ""
	if s.postgres {
		lock = " FOR UPDATE"
	}
	nb, err := scanNotebook(tx.QueryRow(s.q(`SELECT `+nbCols+` FROM notebooks WHERE id = ? AND deleted_at IS NULL`+lock), id))
	if err != nil {
		return nb, nil, err
	}
	if expected != nil && *expected != nb.Version {
		return nb, nil, fmt.Errorf("%w: expected_version %d but notebook is at version %d; re-read it with get_outline and retry", ErrConflict, *expected, nb.Version)
	}
	old, err := s.blocks(tx, id)
	if err != nil {
		return nb, nil, err
	}
	next, err := fn(&nb, slices.Clone(old))
	if err != nil {
		return nb, nil, err
	}
	now := time.Now()
	if err := s.writeDiff(tx, id, old, next, now); err != nil {
		return nb, nil, err
	}
	nb.Version++
	nb.UpdatedAt = now
	if _, err := tx.Exec(s.q(`UPDATE notebooks SET title = ?, theme = ?, paper = ?, mode = ?, updated_at = ?, version = ? WHERE id = ?`), nb.Title, nb.Theme, nb.Paper, nb.Mode, ts(now), nb.Version, id); err != nil {
		return nb, nil, err
	}
	if base == 0 {
		base = nb.Version
	}
	if err := s.snapshot(tx, nb, next, base, tool, summary); err != nil {
		return nb, nil, err
	}
	return nb, next, tx.Commit()
}

func (s *Store) writeDiff(tx *sql.Tx, id string, old, next []model.Block, now time.Time) error {
	prev := map[string]model.Block{}
	for _, b := range old {
		prev[b.ID] = b
	}
	seen := map[string]bool{}
	for i := range next {
		b := &next[i]
		if b.ID == "" || b.Position == "" || seen[b.ID] {
			return fmt.Errorf("internal error: block %d has missing or duplicate id/position", i)
		}
		seen[b.ID] = true
		p, existed := prev[b.ID]
		if existed && p.Position == b.Position && p.Type == b.Type && string(p.Data) == string(b.Data) {
			continue
		}
		if !existed {
			b.CreatedAt = now
		}
		b.UpdatedAt = now
		if _, err := tx.Exec(s.q(`INSERT INTO blocks (notebook_id, id, position, type, data, created_at, updated_at) VALUES (?,?,?,?,?,?,?)
			ON CONFLICT (notebook_id, id) DO UPDATE SET position = excluded.position, type = excluded.type, data = excluded.data, updated_at = excluded.updated_at`),
			id, b.ID, b.Position, b.Type, string(b.Data), ts(b.CreatedAt), ts(now)); err != nil {
			return err
		}
	}
	for _, b := range old {
		if !seen[b.ID] {
			if _, err := tx.Exec(s.q(`DELETE FROM blocks WHERE notebook_id = ? AND id = ?`), id, b.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

type snapBlock struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Position string          `json:"position"`
	Data     json.RawMessage `json:"data"`
}

func (s *Store) snapshot(tx *sql.Tx, nb Notebook, bs []model.Block, base int, tool, summary string) error {
	sb := []snapBlock{}
	for _, b := range bs {
		sb = append(sb, snapBlock{b.ID, b.Type, b.Position, b.Data})
	}
	js, _ := json.Marshal(sb)
	if _, err := tx.Exec(s.q(`INSERT INTO snapshots (notebook_id, version, base, blocks, theme, paper, mode, tool_name, summary, timestamp) VALUES (?,?,?,?,?,?,?,?,?,?)`),
		nb.ID, nb.Version, base, string(js), nb.Theme, nb.Paper, nb.Mode, tool, summary, ts(time.Now())); err != nil {
		return err
	}
	_, err := tx.Exec(s.q(`DELETE FROM snapshots WHERE notebook_id = ? AND version <= ?`), nb.ID, nb.Version-keepSnapshots)
	return err
}

func (s *Store) Versions(id string, limit int) ([]Snapshot, error) {
	rows, err := s.db.Query(s.q(`SELECT version, tool_name, summary, timestamp FROM snapshots WHERE notebook_id = ? ORDER BY version DESC LIMIT ?`), id, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Snapshot{}
	for rows.Next() {
		var sn Snapshot
		var t string
		if err := rows.Scan(&sn.Version, &sn.ToolName, &sn.Summary, &t); err != nil {
			return nil, err
		}
		sn.Timestamp = parseTS(t)
		out = append(out, sn)
	}
	return out, rows.Err()
}

func (s *Store) Restore(id string, version int, expected *int) (Notebook, []model.Block, error) {
	return s.restore(id, version, expected, "restore_version", false)
}

func (s *Store) restore(id string, version int, expected *int, tool string, inheritBase bool) (Notebook, []model.Block, error) {
	var js, theme, paper, mode string
	var base int
	err := s.db.QueryRow(s.q(`SELECT blocks, theme, paper, mode, base FROM snapshots WHERE notebook_id = ? AND version = ?`), id, version).Scan(&js, &theme, &paper, &mode, &base)
	if errors.Is(err, sql.ErrNoRows) {
		return Notebook{}, nil, fmt.Errorf("%w: version %d of notebook %s (only the last %d versions are kept; see list_versions)", ErrNotFound, version, id, keepSnapshots)
	}
	if err != nil {
		return Notebook{}, nil, err
	}
	var sb []snapBlock
	json.Unmarshal([]byte(js), &sb)
	restored := make([]model.Block, len(sb))
	for i, b := range sb {
		restored[i] = model.Block{ID: b.ID, Type: b.Type, Position: b.Position, Data: b.Data}
	}
	if !inheritBase {
		base = 0
	}
	return s.mutate(id, expected, tool, fmt.Sprintf("restored version %d", version), base, func(nb *Notebook, _ []model.Block) ([]model.Block, error) {
		nb.Theme, nb.Paper, nb.Mode = theme, paper, mode
		return restored, nil
	})
}

func (s *Store) Undo(id string, expected *int) (Notebook, []model.Block, error) {
	var base int
	err := s.db.QueryRow(s.q(`SELECT s.base FROM snapshots s JOIN notebooks n ON n.id = s.notebook_id AND n.version = s.version WHERE n.id = ?`), id).Scan(&base)
	if errors.Is(err, sql.ErrNoRows) {
		return Notebook{}, nil, ErrNotFound
	}
	if err != nil {
		return Notebook{}, nil, err
	}
	if base <= 1 {
		return Notebook{}, nil, errors.New("nothing to undo: the notebook is at its original state")
	}
	return s.restore(id, base-1, expected, "undo", true)
}
