package store

import (
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/raaj6395/folio/internal/model"
)

var ErrInvalid = errors.New("invalid")

var FolderColors = []string{"blue", "green", "amber", "red", "purple", "teal"}

const (
	MaxFolders     = 200
	MaxFolderDepth = 3
	maxFolderName  = 80
)

type Folder struct {
	ID       string
	ParentID string
	Name     string
	Color    string // one of FolderColors; "" = default
	Path     string // "Semester 1/Networks"
	Depth    int    // 1 = top level
	Count    int    // live notebooks inside, sub-folders included
}

// folderTree is all of one user's folders by ID; users have at most MaxFolders, so it's always loaded whole.
type folderTree map[string]*Folder

func (s *Store) loadFolders(q querier, userID string) (folderTree, error) {
	rows, err := q.Query(s.q(`SELECT id, parent_id, name, color FROM folders WHERE user_id = ?`), userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	t := folderTree{}
	for rows.Next() {
		var f Folder
		var parent sql.NullString
		if err := rows.Scan(&f.ID, &parent, &f.Name, &f.Color); err != nil {
			return nil, err
		}
		f.ParentID = parent.String
		t[f.ID] = &f
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, f := range t {
		t.fill(f)
	}
	return t, nil
}

func (t folderTree) fill(f *Folder) {
	if f.Depth > 0 {
		return
	}
	f.Path, f.Depth = f.Name, 1
	if p := t[f.ParentID]; p != nil {
		t.fill(p)
		f.Path, f.Depth = p.Path+"/"+f.Name, p.Depth+1
	}
}

func (t folderTree) child(parentID, name string) *Folder {
	for _, f := range t {
		if f.ParentID == parentID && strings.EqualFold(f.Name, name) {
			return f
		}
	}
	return nil
}

// height is how many levels of sub-folders sit below id.
func (t folderTree) height(id string) int {
	h := 0
	for _, f := range t {
		if f.ParentID == id {
			h = max(h, 1+t.height(f.ID))
		}
	}
	return h
}

func cleanFolderName(name string) (string, error) {
	name = strings.Join(strings.Fields(name), " ")
	switch {
	case name == "" || utf8.RuneCountInString(name) > maxFolderName:
		return "", fmt.Errorf("%w: a folder name must be 1 to %d characters", ErrInvalid, maxFolderName)
	case strings.Contains(name, "/"):
		return "", fmt.Errorf(`%w: a folder name can't contain "/"`, ErrInvalid)
	}
	return name, nil
}

// folderTx runs fn in a transaction holding the user's folder tree. On Postgres the user row is
// locked, so concurrent folder changes by one user run one at a time (SQLite transactions already do).
func (s *Store) folderTx(userID string, fn func(tx *sql.Tx, t folderTree) error) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if s.postgres {
		if _, err := tx.Exec(`SELECT id FROM users WHERE id = $1 FOR UPDATE`, userID); err != nil {
			return err
		}
	}
	t, err := s.loadFolders(tx, userID)
	if err != nil {
		return err
	}
	if err := fn(tx, t); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) addFolder(tx *sql.Tx, t folderTree, userID, parentID, name string) (*Folder, error) {
	name, err := cleanFolderName(name)
	if err != nil {
		return nil, err
	}
	if len(t) >= MaxFolders {
		return nil, fmt.Errorf("%w: you already have %d folders (the limit is %d)", ErrQuota, len(t), MaxFolders)
	}
	parent := sql.NullString{String: parentID, Valid: parentID != ""}
	if parentID != "" && t[parentID] == nil {
		return nil, fmt.Errorf("%w: folder %s", ErrNotFound, parentID)
	}
	if parentID != "" && t[parentID].Depth >= MaxFolderDepth {
		return nil, fmt.Errorf("%w: folders can be at most %d levels deep", ErrInvalid, MaxFolderDepth)
	}
	if t.child(parentID, name) != nil {
		return nil, fmt.Errorf("%w: a folder named %q is already here", ErrInvalid, name)
	}
	f := &Folder{ID: model.NewID("f_"), ParentID: parentID, Name: name}
	now := ts(time.Now())
	if _, err := tx.Exec(s.q(`INSERT INTO folders (id, user_id, parent_id, name, created_at, updated_at) VALUES (?,?,?,?,?,?)`), f.ID, userID, parent, name, now, now); err != nil {
		return nil, err
	}
	t[f.ID] = f
	t.fill(f)
	return f, nil
}

// Folders returns the user's folders in tree order (each folder followed by its sub-folders, names A–Z).
func (s *Store) Folders(userID string) ([]Folder, error) {
	t, err := s.loadFolders(s.db, userID)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(s.q(`SELECT folder_id, count(*) FROM notebooks WHERE user_id = ? AND deleted_at IS NULL AND folder_id IS NOT NULL GROUP BY folder_id`), userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		for f := t[id]; f != nil; f = t[f.ParentID] {
			f.Count += n
		}
	}
	out := make([]Folder, 0, len(t))
	for _, f := range t {
		out = append(out, *f)
	}
	key := func(f Folder) string { return strings.ReplaceAll(strings.ToLower(f.Path), "/", "\x00") }
	sort.Slice(out, func(i, j int) bool { return key(out[i]) < key(out[j]) })
	return out, rows.Err()
}

func (s *Store) CreateFolder(userID, parentID, name string) (Folder, error) {
	var f *Folder
	err := s.folderTx(userID, func(tx *sql.Tx, t folderTree) (err error) {
		f, err = s.addFolder(tx, t, userID, parentID, name)
		return err
	})
	if err != nil {
		return Folder{}, err
	}
	return *f, nil
}

// EnsureFolderPath finds the folder at path ("A/B", any case and spacing), creating what's missing.
// An empty path is the top level and returns a zero Folder.
func (s *Store) EnsureFolderPath(userID, path string) (Folder, error) {
	if strings.TrimSpace(path) == "" {
		return Folder{}, nil
	}
	parts := strings.Split(strings.Trim(strings.TrimSpace(path), "/"), "/")
	if len(parts) > MaxFolderDepth {
		return Folder{}, fmt.Errorf("%w: folders can be at most %d levels deep", ErrInvalid, MaxFolderDepth)
	}
	var cur *Folder
	err := s.folderTx(userID, func(tx *sql.Tx, t folderTree) error {
		parentID := ""
		for _, p := range parts {
			name, err := cleanFolderName(p)
			if err != nil {
				return err
			}
			if cur = t.child(parentID, name); cur == nil {
				if cur, err = s.addFolder(tx, t, userID, parentID, name); err != nil {
					return err
				}
			}
			parentID = cur.ID
		}
		return nil
	})
	if err != nil {
		return Folder{}, err
	}
	return *cur, nil
}

func (s *Store) RenameFolder(userID, id, name string) error {
	return s.folderTx(userID, func(tx *sql.Tx, t folderTree) error {
		f := t[id]
		if f == nil {
			return fmt.Errorf("%w: folder %s", ErrNotFound, id)
		}
		name, err := cleanFolderName(name)
		if err != nil {
			return err
		}
		if c := t.child(f.ParentID, name); c != nil && c.ID != id {
			return fmt.Errorf("%w: a folder named %q is already here", ErrInvalid, name)
		}
		_, err = tx.Exec(s.q(`UPDATE folders SET name = ?, updated_at = ? WHERE id = ?`), name, ts(time.Now()), id)
		return err
	})
}

// DeleteFolder removes the folder and moves everything in it up to its parent; no notebook is deleted.
// A sub-folder whose name clashes in the parent becomes "Name (2)".
func (s *Store) DeleteFolder(userID, id string) (Folder, error) {
	var gone Folder
	err := s.folderTx(userID, func(tx *sql.Tx, t folderTree) error {
		f := t[id]
		if f == nil {
			return fmt.Errorf("%w: folder %s", ErrNotFound, id)
		}
		gone = *f
		parent := sql.NullString{String: f.ParentID, Valid: f.ParentID != ""}
		if _, err := tx.Exec(s.q(`UPDATE notebooks SET folder_id = ? WHERE folder_id = ? AND user_id = ?`), parent, id, userID); err != nil {
			return err
		}
		delete(t, id)
		for _, c := range t {
			if c.ParentID != id {
				continue
			}
			name := c.Name
			for n := 2; t.child(f.ParentID, name) != nil; n++ {
				name = fmt.Sprintf("%s (%d)", c.Name, n)
			}
			if _, err := tx.Exec(s.q(`UPDATE folders SET parent_id = ?, name = ? WHERE id = ?`), parent, name, c.ID); err != nil {
				return err
			}
			c.ParentID, c.Name = f.ParentID, name
		}
		_, err := tx.Exec(s.q(`DELETE FROM folders WHERE id = ?`), id)
		return err
	})
	return gone, err
}

// MoveFolder puts folder id inside parentID ("" = top level), refusing cycles and trees deeper than MaxFolderDepth.
func (s *Store) MoveFolder(userID, id, parentID string) error {
	return s.folderTx(userID, func(tx *sql.Tx, t folderTree) error {
		f := t[id]
		if f == nil || (parentID != "" && t[parentID] == nil) {
			return fmt.Errorf("%w: folder", ErrNotFound)
		}
		depth := 0
		for p := t[parentID]; p != nil; p = t[p.ParentID] {
			if p.ID == id {
				return fmt.Errorf("%w: a folder can't go inside itself", ErrInvalid)
			}
			depth++
		}
		if depth+1+t.height(id) > MaxFolderDepth {
			return fmt.Errorf("%w: folders can be at most %d levels deep", ErrInvalid, MaxFolderDepth)
		}
		if c := t.child(parentID, f.Name); c != nil && c.ID != id {
			return fmt.Errorf("%w: a folder named %q is already there", ErrInvalid, f.Name)
		}
		_, err := tx.Exec(s.q(`UPDATE folders SET parent_id = ?, updated_at = ? WHERE id = ?`), sql.NullString{String: parentID, Valid: parentID != ""}, ts(time.Now()), id)
		return err
	})
}

func (s *Store) SetFolderColor(userID, id, color string) error {
	if color != "" && !slices.Contains(FolderColors, color) {
		return fmt.Errorf("%w: colour must be one of %s", ErrInvalid, strings.Join(FolderColors, ", "))
	}
	err := s.execOne(`UPDATE folders SET color = ?, updated_at = ? WHERE id = ? AND user_id = ?`, color, ts(time.Now()), id, userID)
	if errors.Is(err, ErrNotFound) {
		return fmt.Errorf("%w: folder %s", ErrNotFound, id)
	}
	return err
}

// SetFavorite stars or unstars a notebook. Like filing, it isn't an edit.
func (s *Store) SetFavorite(userID, notebookID string, on bool) error {
	at := sql.NullString{String: ts(time.Now()), Valid: on}
	return s.execOne(`UPDATE notebooks SET favorite_at = ? WHERE id = ? AND user_id = ? AND deleted_at IS NULL`, at, notebookID, userID)
}

// MoveNotebook files a notebook in folderID ("" = top level). Filing isn't an edit: version and
// updated_at stay as they are, and nothing is added to the history.
func (s *Store) MoveNotebook(userID, notebookID, folderID string) error {
	n, err := s.MoveNotebooks(userID, []string{notebookID}, folderID)
	if err == nil && n == 0 {
		err = ErrNotFound
	}
	return err
}

// MoveNotebooks files several notebooks at once and returns how many moved; IDs that aren't the
// user's live notebooks are skipped.
func (s *Store) MoveNotebooks(userID string, ids []string, folderID string) (int, error) {
	if len(ids) > MaxNotebooks {
		return 0, fmt.Errorf("%w: move at most %d notebooks at once", ErrInvalid, MaxNotebooks)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if folderID != "" {
		var n int
		if err := tx.QueryRow(s.q(`SELECT count(*) FROM folders WHERE id = ? AND user_id = ?`), folderID, userID).Scan(&n); err != nil {
			return 0, err
		}
		if n == 0 {
			return 0, fmt.Errorf("%w: folder %s", ErrNotFound, folderID)
		}
	}
	folder := sql.NullString{String: folderID, Valid: folderID != ""}
	moved := 0
	for _, id := range ids {
		res, err := tx.Exec(s.q(`UPDATE notebooks SET folder_id = ? WHERE id = ? AND user_id = ? AND deleted_at IS NULL`), folder, id, userID)
		if err != nil {
			return 0, err
		}
		n, _ := res.RowsAffected()
		moved += int(n)
	}
	return moved, tx.Commit()
}
