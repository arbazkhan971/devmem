package memory

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Note struct {
	ID        string
	FeatureID string
	SessionID string
	Content   string
	Type      string
	CreatedAt string
	UpdatedAt string
}

const noteCols = `id, feature_id, COALESCE(session_id, ''), content, type, created_at, updated_at`

func scanNote(sc interface{ Scan(...any) error }) (Note, error) {
	var n Note
	err := sc.Scan(&n.ID, &n.FeatureID, &n.SessionID, &n.Content, &n.Type, &n.CreatedAt, &n.UpdatedAt)
	return n, err
}

func (s *Store) CreateNote(featureID, sessionID, content, noteType string) (*Note, error) {
	if noteType == "" {
		noteType = "note"
	}
	id := uuid.New().String()
	now := time.Now().UTC().Format(time.DateTime)
	w := s.db.Writer()

	if _, err := w.Exec(
		`INSERT INTO notes (id, feature_id, session_id, content, type, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, featureID, nullIfEmpty(sessionID), content, noteType, now, now,
	); err != nil {
		return nil, fmt.Errorf("create note: %w", err)
	}

	var rowID int64
	if err := w.QueryRow(`SELECT rowid FROM notes WHERE id = ?`, id).Scan(&rowID); err != nil {
		return nil, fmt.Errorf("get note rowid: %w", err)
	}
	if _, err := w.Exec(`INSERT INTO notes_fts(rowid, content, type) VALUES (?, ?, ?)`, rowID, content, noteType); err != nil {
		return nil, fmt.Errorf("sync note to fts: %w", err)
	}
	if _, err := w.Exec(`INSERT INTO notes_trigram(rowid, content) VALUES (?, ?)`, rowID, content); err != nil {
		return nil, fmt.Errorf("sync note to trigram: %w", err)
	}

	return &Note{
		ID: id, FeatureID: featureID, SessionID: sessionID,
		Content: content, Type: noteType, CreatedAt: now, UpdatedAt: now,
	}, nil
}

func (s *Store) ListNotes(featureID, noteType string, limit int) ([]Note, error) {
	if limit <= 0 {
		limit = 50
	}
	q := `SELECT ` + noteCols + ` FROM notes WHERE feature_id = ?`
	args := []any{featureID}
	if noteType != "" {
		q += ` AND type = ?`
		args = append(args, noteType)
	}
	q += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Reader().Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("list notes: %w", err)
	}
	defer rows.Close()
	var out []Note
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			return nil, fmt.Errorf("scan note: %w", err)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) GetNote(noteID string) (*Note, error) {
	row := s.db.Reader().QueryRow(`SELECT `+noteCols+` FROM notes WHERE id = ?`, noteID)
	n, err := scanNote(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("note %q not found", noteID)
	}
	if err != nil {
		return nil, fmt.Errorf("get note: %w", err)
	}
	return &n, nil
}
