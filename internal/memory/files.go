package memory

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// FileRecord represents a file touched during a session.
type FileRecord struct {
	ID, FeatureID, SessionID, Path, Action, FirstSeen string
}

const fileCols = `id, feature_id, COALESCE(session_id, ''), path, action, first_seen`

func scanFileRecord(sc interface{ Scan(...any) error }) (FileRecord, error) {
	var f FileRecord
	return f, sc.Scan(&f.ID, &f.FeatureID, &f.SessionID, &f.Path, &f.Action, &f.FirstSeen)
}

// TrackFile records a file as touched for a given feature and session.
// Uses INSERT OR IGNORE so duplicate (feature_id, session_id, path) is a no-op.
func (s *Store) TrackFile(featureID, sessionID, path, action string) error {
	if action == "" {
		action = "modified"
	}
	id := uuid.New().String()
	now := time.Now().UTC().Format(time.DateTime)
	_, err := s.db.Writer().Exec(
		`INSERT OR IGNORE INTO files_touched (id, feature_id, session_id, path, action, first_seen) VALUES (?, ?, ?, ?, ?, ?)`,
		id, featureID, nullIfEmpty(sessionID), path, action, now,
	)
	if err != nil {
		return fmt.Errorf("track file: %w", err)
	}
	return nil
}

// GetFilesTouched returns files touched for a feature, ordered by most recent first.
func (s *Store) GetFilesTouched(featureID string, limit int) ([]FileRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	return collectRows(s.db.Reader(),
		`SELECT `+fileCols+` FROM files_touched WHERE feature_id = ? ORDER BY first_seen DESC LIMIT ?`,
		[]any{featureID, limit},
		func(rows *sql.Rows) (FileRecord, error) { return scanFileRecord(rows) },
	)
}

// GetSessionFiles returns files touched in a specific session.
func (s *Store) GetSessionFiles(sessionID string) ([]FileRecord, error) {
	return collectRows(s.db.Reader(),
		`SELECT `+fileCols+` FROM files_touched WHERE session_id = ? ORDER BY first_seen DESC`,
		[]any{sessionID},
		func(rows *sql.Rows) (FileRecord, error) { return scanFileRecord(rows) },
	)
}
