package memory

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Session struct {
	ID        string
	FeatureID string
	Tool      string
	StartedAt string
	EndedAt   string
}

const sessionCols = `id, feature_id, tool, started_at, COALESCE(ended_at, '')`

func scanSession(sc interface{ Scan(...any) error }) (Session, error) {
	var s Session
	err := sc.Scan(&s.ID, &s.FeatureID, &s.Tool, &s.StartedAt, &s.EndedAt)
	return s, err
}

func (s *Store) CreateSession(featureID, tool string) (*Session, error) {
	id := uuid.New().String()
	now := time.Now().UTC().Format(time.DateTime)
	_, err := s.db.Writer().Exec(
		`INSERT INTO sessions (id, feature_id, tool, started_at) VALUES (?, ?, ?, ?)`,
		id, featureID, tool, now,
	)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	return &Session{ID: id, FeatureID: featureID, Tool: tool, StartedAt: now}, nil
}

func (s *Store) EndSession(sessionID string) error {
	now := time.Now().UTC().Format(time.DateTime)
	res, err := s.db.Writer().Exec(`UPDATE sessions SET ended_at = ? WHERE id = ?`, now, sessionID)
	if err != nil {
		return fmt.Errorf("end session: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("session %q not found", sessionID)
	}
	return nil
}

func (s *Store) GetCurrentSession() (*Session, error) {
	row := s.db.Reader().QueryRow(
		`SELECT `+sessionCols+` FROM sessions WHERE ended_at IS NULL ORDER BY started_at DESC LIMIT 1`,
	)
	sess, err := scanSession(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("no active session")
	}
	if err != nil {
		return nil, fmt.Errorf("get current session: %w", err)
	}
	return &sess, nil
}

func (s *Store) ListSessions(featureID string, limit int) ([]Session, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Reader().Query(
		`SELECT `+sessionCols+` FROM sessions WHERE feature_id = ? ORDER BY started_at DESC LIMIT ?`,
		featureID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()
	var out []Session
	for rows.Next() {
		sess, err := scanSession(rows)
		if err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		out = append(out, sess)
	}
	return out, rows.Err()
}
