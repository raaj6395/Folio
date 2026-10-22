package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/raaj6395/folio/internal/model"
)

type OAuthClient struct {
	ID           string
	Name         string
	RedirectURIs []string
	Kind         string
	ExpiresAt    time.Time
}

type Grant struct {
	ID         string
	ClientID   string
	ClientName string
	CreatedAt  time.Time
	LastUsedAt time.Time
}

type Code struct {
	ClientID, UserID, RedirectURI, Challenge, Resource string
}

func (s *Store) SaveOAuthClient(c OAuthClient) error {
	uris, _ := json.Marshal(c.RedirectURIs)
	var exp sql.NullString
	if !c.ExpiresAt.IsZero() {
		exp = sql.NullString{String: ts(c.ExpiresAt), Valid: true}
	}
	_, err := s.db.Exec(s.q(`INSERT INTO oauth_clients (client_id, client_name, redirect_uris, kind, created_at, expires_at) VALUES (?,?,?,?,?,?)
		ON CONFLICT (client_id) DO UPDATE SET client_name = excluded.client_name, redirect_uris = excluded.redirect_uris, expires_at = excluded.expires_at`),
		c.ID, c.Name, string(uris), c.Kind, ts(time.Now()), exp)
	return err
}

func (s *Store) OAuthClient(id string) (OAuthClient, error) {
	var c OAuthClient
	var uris string
	var exp sql.NullString
	err := s.db.QueryRow(s.q(`SELECT client_id, client_name, redirect_uris, kind, expires_at FROM oauth_clients WHERE client_id = ?`), id).Scan(&c.ID, &c.Name, &uris, &c.Kind, &exp)
	if errors.Is(err, sql.ErrNoRows) {
		return c, ErrNotFound
	}
	if err != nil {
		return c, err
	}
	json.Unmarshal([]byte(uris), &c.RedirectURIs)
	if exp.Valid {
		c.ExpiresAt = parseTS(exp.String)
	}
	return c, nil
}

func (s *Store) CreateCode(codeHash string, c Code, ttl time.Duration) error {
	_, err := s.db.Exec(s.q(`INSERT INTO oauth_codes (code_hash, client_id, user_id, redirect_uri, code_challenge, resource, expires_at) VALUES (?,?,?,?,?,?,?)`),
		codeHash, c.ClientID, c.UserID, c.RedirectURI, c.Challenge, c.Resource, ts(time.Now().Add(ttl)))
	return err
}

func (s *Store) ConsumeCode(codeHash string) (Code, error) {
	var c Code
	res, err := s.db.Exec(s.q(`UPDATE oauth_codes SET used = 1 WHERE code_hash = ? AND used = 0 AND expires_at > ?`), codeHash, ts(time.Now()))
	if err != nil {
		return c, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return c, ErrNotFound
	}
	err = s.db.QueryRow(s.q(`SELECT client_id, user_id, redirect_uri, code_challenge, resource FROM oauth_codes WHERE code_hash = ?`), codeHash).Scan(&c.ClientID, &c.UserID, &c.RedirectURI, &c.Challenge, &c.Resource)
	return c, err
}

func (s *Store) CreateGrant(userID, clientID, clientName, refreshHash string) (Grant, error) {
	g := Grant{ID: model.NewID("g_"), ClientID: clientID, ClientName: clientName, CreatedAt: time.Now()}
	_, err := s.db.Exec(s.q(`INSERT INTO oauth_grants (id, user_id, client_id, client_name, refresh_hash, created_at) VALUES (?,?,?,?,?,?)`),
		g.ID, userID, clientID, clientName, refreshHash, ts(g.CreatedAt))
	return g, err
}

func (s *Store) RotateRefresh(oldHash, newHash, clientID string) (userID, grantID string, err error) {
	err = s.db.QueryRow(s.q(`SELECT g.id, g.user_id FROM oauth_grants g JOIN users u ON u.id = g.user_id
		WHERE g.refresh_hash = ? AND g.client_id = ? AND g.revoked_at IS NULL AND u.disabled_at IS NULL`), oldHash, clientID).Scan(&grantID, &userID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", ErrNotFound
	}
	if err != nil {
		return "", "", err
	}
	res, err := s.db.Exec(s.q(`UPDATE oauth_grants SET refresh_hash = ?, last_used_at = ? WHERE id = ? AND refresh_hash = ?`), newHash, ts(time.Now()), grantID, oldHash)
	if err != nil {
		return "", "", err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return "", "", ErrNotFound
	}
	return userID, grantID, nil
}

func (s *Store) CreateAccessToken(userID, grantID, tokenHash, last4 string, ttl time.Duration) (time.Time, error) {
	exp := time.Now().Add(ttl)
	_, err := s.db.Exec(s.q(`INSERT INTO api_tokens (id, user_id, name, token_hash, last4, created_at, expires_at, grant_id) VALUES (?,?,?,?,?,?,?,?)`),
		model.NewID("t_"), userID, "oauth", tokenHash, last4, ts(time.Now()), ts(exp), grantID)
	return exp, err
}

func (s *Store) Grants(userID string) ([]Grant, error) {
	rows, err := s.db.Query(s.q(`SELECT id, client_id, client_name, created_at, last_used_at FROM oauth_grants WHERE user_id = ? AND revoked_at IS NULL ORDER BY created_at DESC`), userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Grant{}
	for rows.Next() {
		var g Grant
		var c string
		var used sql.NullString
		if err := rows.Scan(&g.ID, &g.ClientID, &g.ClientName, &c, &used); err != nil {
			return nil, err
		}
		g.CreatedAt = parseTS(c)
		if used.Valid {
			g.LastUsedAt = parseTS(used.String)
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Store) RevokeGrant(userID, grantID string) error {
	if err := s.execOne(`UPDATE oauth_grants SET revoked_at = ? WHERE id = ? AND user_id = ? AND revoked_at IS NULL`, ts(time.Now()), grantID, userID); err != nil {
		return err
	}
	_, err := s.db.Exec(s.q(`UPDATE api_tokens SET revoked_at = ? WHERE grant_id = ? AND revoked_at IS NULL`), ts(time.Now()), grantID)
	return err
}
