package store

import (
	"database/sql"
	"errors"
	"fmt"
)

type rawNotebook struct {
	ID, Title, Theme, Paper, Mode, CreatedAt, UpdatedAt string
	Guidelines, Density, Template                       string
	Version                                             int
	DeletedAt                                           sql.NullString
}

type rawBlock struct {
	ID, Position, Type, Data, CreatedAt, UpdatedAt string
}

type rawSnapshot struct {
	Version, Base                                            int
	Blocks, Theme, Paper, Mode, ToolName, Summary, Timestamp string
	Guidelines, Density                                      sql.NullString
}

type ImportResult struct {
	ID     string
	Title  string
	Status string
}

func (s *Store) ImportFrom(src *Store, ownerID string, includeDeleted, dryRun bool) ([]ImportResult, error) {
	rows, err := src.db.Query(`SELECT id, title, theme, paper, mode, created_at, updated_at, version, deleted_at, guidelines, density, template FROM notebooks ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	var nbs []rawNotebook
	for rows.Next() {
		var n rawNotebook
		if err := rows.Scan(&n.ID, &n.Title, &n.Theme, &n.Paper, &n.Mode, &n.CreatedAt, &n.UpdatedAt, &n.Version, &n.DeletedAt, &n.Guidelines, &n.Density, &n.Template); err != nil {
			rows.Close()
			return nil, err
		}
		nbs = append(nbs, n)
	}
	rows.Close()
	var out []ImportResult
	for _, n := range nbs {
		r := ImportResult{ID: n.ID, Title: n.Title}
		var exists int
		if err := s.db.QueryRow(s.q(`SELECT count(*) FROM notebooks WHERE id = ?`), n.ID).Scan(&exists); err != nil {
			return out, err
		}
		switch {
		case n.DeletedAt.Valid && !includeDeleted:
			r.Status = "skipped (deleted)"
		case exists > 0:
			r.Status = "skipped (exists)"
		case dryRun:
			r.Status = "would import"
		default:
			if err := s.importOne(src, n, ownerID); err != nil {
				r.Status = "failed: " + err.Error()
			} else {
				r.Status = "imported"
			}
		}
		out = append(out, r)
	}
	return out, nil
}

func (s *Store) importOne(src *Store, n rawNotebook, ownerID string) error {
	var blocks []rawBlock
	rows, err := src.db.Query(`SELECT id, position, type, data, created_at, updated_at FROM blocks WHERE notebook_id = ?`, n.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var b rawBlock
		if err := rows.Scan(&b.ID, &b.Position, &b.Type, &b.Data, &b.CreatedAt, &b.UpdatedAt); err != nil {
			rows.Close()
			return err
		}
		blocks = append(blocks, b)
	}
	rows.Close()
	var snaps []rawSnapshot
	rows, err = src.db.Query(`SELECT version, base, blocks, theme, paper, mode, guidelines, density, tool_name, summary, timestamp FROM snapshots WHERE notebook_id = ?`, n.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var sn rawSnapshot
		if err := rows.Scan(&sn.Version, &sn.Base, &sn.Blocks, &sn.Theme, &sn.Paper, &sn.Mode, &sn.Guidelines, &sn.Density, &sn.ToolName, &sn.Summary, &sn.Timestamp); err != nil {
			rows.Close()
			return err
		}
		snaps = append(snaps, sn)
	}
	rows.Close()
	if len(snaps) == 0 {
		return errors.New("source notebook has no snapshots")
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	owner := sql.NullString{String: ownerID, Valid: ownerID != ""}
	if _, err := tx.Exec(s.q(`INSERT INTO notebooks (id, title, theme, paper, mode, created_at, updated_at, version, deleted_at, user_id, guidelines, density, template) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`),
		n.ID, n.Title, n.Theme, n.Paper, n.Mode, n.CreatedAt, n.UpdatedAt, n.Version, n.DeletedAt, owner, n.Guidelines, n.Density, n.Template); err != nil {
		return err
	}
	for _, b := range blocks {
		if _, err := tx.Exec(s.q(`INSERT INTO blocks (notebook_id, id, position, type, data, created_at, updated_at) VALUES (?,?,?,?,?,?,?)`),
			n.ID, b.ID, b.Position, b.Type, b.Data, b.CreatedAt, b.UpdatedAt); err != nil {
			return fmt.Errorf("block %s: %w", b.ID, err)
		}
	}
	for _, sn := range snaps {
		if _, err := tx.Exec(s.q(`INSERT INTO snapshots (notebook_id, version, base, blocks, theme, paper, mode, guidelines, density, tool_name, summary, timestamp) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`),
			n.ID, sn.Version, sn.Base, sn.Blocks, sn.Theme, sn.Paper, sn.Mode, sn.Guidelines, sn.Density, sn.ToolName, sn.Summary, sn.Timestamp); err != nil {
			return fmt.Errorf("snapshot v%d: %w", sn.Version, err)
		}
	}
	return tx.Commit()
}
