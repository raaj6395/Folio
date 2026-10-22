package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/raaj6395/folio/internal/model"
)

var ErrEmailTaken = errors.New("email already registered")

type User struct {
	ID           string
	Email        string
	PasswordHash string
	CreatedAt    time.Time
	Disabled     bool
}

type Token struct {
	ID         string
	Name       string
	Last4      string
	CreatedAt  time.Time
	LastUsedAt time.Time
	Revoked    bool
}

const userCols = `id, email, password_hash, created_at, disabled_at`

func scanUser(r scanner) (User, error) {
	var u User
	var c string
	var disabled sql.NullString
	err := r.Scan(&u.ID, &u.Email, &u.PasswordHash, &c, &disabled)
	if errors.Is(err, sql.ErrNoRows) {
		return u, ErrNotFound
	}
	u.CreatedAt, u.Disabled = parseTS(c), disabled.Valid
	return u, err
}

func NormalizeEmail(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

func (s *Store) CreateUser(email, passwordHash string) (User, error) {
	u := User{ID: model.NewID("u_"), Email: NormalizeEmail(email), PasswordHash: passwordHash, CreatedAt: time.Now()}
	var n int
	if err := s.db.QueryRow(s.q(`SELECT count(*) FROM users WHERE email = ?`), u.Email).Scan(&n); err != nil {
		return u, err
	}
	if n > 0 {
		return u, ErrEmailTaken
	}
	if _, err := s.db.Exec(s.q(`INSERT INTO users (`+userCols+`) VALUES (?,?,?,?,NULL)`), u.ID, u.Email, u.PasswordHash, ts(u.CreatedAt)); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return u, ErrEmailTaken
		}
		return u, err
	}
	return u, nil
}

func (s *Store) EnsureLocalUser() error {
	_, err := s.db.Exec(s.q(`INSERT INTO users (`+userCols+`) VALUES (?,?,?,?,NULL) ON CONFLICT (id) DO NOTHING`), LocalUser, "local@folio.invalid", "!", ts(time.Now()))
	return err
}

func (s *Store) UserByEmail(email string) (User, error) {
	return scanUser(s.db.QueryRow(s.q(`SELECT `+userCols+` FROM users WHERE email = ?`), NormalizeEmail(email)))
}

func (s *Store) UserByID(id string) (User, error) {
	return scanUser(s.db.QueryRow(s.q(`SELECT `+userCols+` FROM users WHERE id = ?`), id))
}

func (s *Store) SetPassword(userID, passwordHash string) error {
	return s.execOne(`UPDATE users SET password_hash = ? WHERE id = ?`, passwordHash, userID)
}

func (s *Store) DisableUser(userID string) error {
	if err := s.execOne(`UPDATE users SET disabled_at = ? WHERE id = ? AND disabled_at IS NULL`, ts(time.Now()), userID); err != nil {
		return err
	}
	return s.DeleteUserSessions(userID)
}

func (s *Store) execOne(query string, args ...any) error {
	res, err := s.db.Exec(s.q(query), args...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) CreateSession(userID, idHash string, ttl time.Duration) error {
	now := time.Now()
	_, err := s.db.Exec(s.q(`INSERT INTO web_sessions (id_hash, user_id, created_at, expires_at) VALUES (?,?,?,?)`), idHash, userID, ts(now), ts(now.Add(ttl)))
	return err
}

func (s *Store) SessionUser(idHash string) (User, error) {
	u, err := scanUser(s.db.QueryRow(s.q(`SELECT u.id, u.email, u.password_hash, u.created_at, u.disabled_at FROM web_sessions w JOIN users u ON u.id = w.user_id WHERE w.id_hash = ? AND w.expires_at > ?`), idHash, ts(time.Now())))
	if err == nil && u.Disabled {
		return u, ErrNotFound
	}
	return u, err
}

func (s *Store) DeleteSession(idHash string) error {
	_, err := s.db.Exec(s.q(`DELETE FROM web_sessions WHERE id_hash = ?`), idHash)
	return err
}

func (s *Store) DeleteUserSessions(userID string) error {
	_, err := s.db.Exec(s.q(`DELETE FROM web_sessions WHERE user_id = ?`), userID)
	return err
}

func (s *Store) CreateToken(userID, name, tokenHash, last4 string) (Token, error) {
	t := Token{ID: model.NewID("t_"), Name: name, Last4: last4, CreatedAt: time.Now()}
	_, err := s.db.Exec(s.q(`INSERT INTO api_tokens (id, user_id, name, token_hash, last4, created_at) VALUES (?,?,?,?,?,?)`), t.ID, userID, name, tokenHash, last4, ts(t.CreatedAt))
	return t, err
}

func (s *Store) TokenUser(tokenHash string) (User, string, error) {
	var tokenID string
	var lastUsed sql.NullString
	row := s.db.QueryRow(s.q(`SELECT t.id, t.last_used_at, u.id, u.email, u.password_hash, u.created_at, u.disabled_at FROM api_tokens t JOIN users u ON u.id = t.user_id
		WHERE t.token_hash = ? AND t.revoked_at IS NULL AND (t.expires_at IS NULL OR t.expires_at > ?)`), tokenHash, ts(time.Now()))
	var u User
	var c string
	var disabled sql.NullString
	err := row.Scan(&tokenID, &lastUsed, &u.ID, &u.Email, &u.PasswordHash, &c, &disabled)
	if errors.Is(err, sql.ErrNoRows) {
		return u, "", ErrNotFound
	}
	if err != nil {
		return u, "", err
	}
	u.CreatedAt, u.Disabled = parseTS(c), disabled.Valid
	if u.Disabled {
		return u, "", ErrNotFound
	}
	if !lastUsed.Valid || time.Since(parseTS(lastUsed.String)) > time.Minute {
		s.db.Exec(s.q(`UPDATE api_tokens SET last_used_at = ? WHERE id = ?`), ts(time.Now()), tokenID)
	}
	return u, tokenID, nil
}

func (s *Store) Tokens(userID string) ([]Token, error) {
	rows, err := s.db.Query(s.q(`SELECT id, name, last4, created_at, last_used_at, revoked_at FROM api_tokens WHERE user_id = ? ORDER BY created_at DESC`), userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Token{}
	for rows.Next() {
		var t Token
		var c string
		var used, revoked sql.NullString
		if err := rows.Scan(&t.ID, &t.Name, &t.Last4, &c, &used, &revoked); err != nil {
			return nil, err
		}
		t.CreatedAt, t.Revoked = parseTS(c), revoked.Valid
		if used.Valid {
			t.LastUsedAt = parseTS(used.String)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) RevokeToken(userID, tokenID string) error {
	return s.execOne(`UPDATE api_tokens SET revoked_at = ? WHERE id = ? AND user_id = ? AND revoked_at IS NULL`, ts(time.Now()), tokenID, userID)
}

func (s *Store) AdoptOrphans(userID string) (int, error) {
	res, err := s.db.Exec(s.q(`UPDATE notebooks SET user_id = ? WHERE user_id IS NULL`), userID)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (s *Store) CountOrphans() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT count(*) FROM notebooks WHERE user_id IS NULL`).Scan(&n)
	return n, err
}

func (s *Store) RequireOwners() error {
	if !s.postgres {
		return fmt.Errorf("only supported on Postgres")
	}
	n, err := s.CountOrphans()
	if err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("%d notebook(s) have no owner; run `folio admin adopt` first", n)
	}
	_, err = s.db.Exec(`ALTER TABLE notebooks ALTER COLUMN user_id SET NOT NULL`)
	return err
}

func (s *Store) NotebookAdmin(id string) (Notebook, error) {
	return scanNotebook(s.db.QueryRow(s.q(`SELECT `+nbCols+` FROM notebooks WHERE id = ? AND deleted_at IS NULL`), id))
}

func (s *Store) Counts() (users, notebooks, orphans int, err error) {
	err = s.db.QueryRow(`SELECT (SELECT count(*) FROM users WHERE id <> 'u_local'), (SELECT count(*) FROM notebooks WHERE deleted_at IS NULL), (SELECT count(*) FROM notebooks WHERE user_id IS NULL AND deleted_at IS NULL)`).Scan(&users, &notebooks, &orphans)
	return
}
