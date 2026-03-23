package memory

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ContextSnapshot represents a saved conversation context snapshot.
type ContextSnapshot struct {
	ID           string
	FeatureID    string
	SessionID    string
	Content      string
	SnapshotType string
	CreatedAt    string
}

// SnapshotMatch represents a search result from context snapshots.
type SnapshotMatch struct {
	ID           string
	FeatureID    string
	Content      string
	SnapshotType string
	CreatedAt    string
	Snippet      string
}

// SaveSnapshot stores a context snapshot for the given feature.
func (s *Store) SaveSnapshot(featureID, sessionID, content, snapshotType string) error {
	if snapshotType == "" {
		snapshotType = "pre_compaction"
	}
	id := uuid.New().String()
	now := time.Now().UTC().Format(time.DateTime)
	w := s.db.Writer()

	if _, err := w.Exec(
		`INSERT INTO context_snapshots (id, feature_id, session_id, content, snapshot_type, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		id, featureID, nullIfEmpty(sessionID), content, snapshotType, now,
	); err != nil {
		return fmt.Errorf("save snapshot: %w", err)
	}

	// Get the rowid for the FTS table
	var rowID int64
	if err := w.QueryRow(`SELECT rowid FROM context_snapshots WHERE id = ?`, id).Scan(&rowID); err != nil {
		return fmt.Errorf("get snapshot rowid: %w", err)
	}
	if _, err := w.Exec(
		`INSERT INTO context_snapshots_fts(rowid, content, snapshot_type) VALUES (?, ?, ?)`,
		rowID, content, snapshotType,
	); err != nil {
		return fmt.Errorf("sync snapshot to fts: %w", err)
	}
	return nil
}

// RecoverContext searches context snapshots using FTS5 for the query term.
// Returns matching snippets with surrounding context, scoped to the given feature.
func (s *Store) RecoverContext(featureID, query string, limit int) ([]SnapshotMatch, error) {
	if limit <= 0 {
		limit = 3
	}
	r := s.db.Reader()
	rows, err := r.Query(
		`SELECT cs.id, cs.feature_id, cs.content, cs.snapshot_type, cs.created_at,
			snippet(context_snapshots_fts, 0, '>>>', '<<<', '...', 40) AS snippet
		FROM context_snapshots_fts fts
		JOIN context_snapshots cs ON cs.rowid = fts.rowid
		WHERE context_snapshots_fts MATCH ?
		AND cs.feature_id = ?
		ORDER BY rank
		LIMIT ?`,
		query, featureID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("recover context: %w", err)
	}
	defer rows.Close()

	var results []SnapshotMatch
	for rows.Next() {
		var m SnapshotMatch
		if err := rows.Scan(&m.ID, &m.FeatureID, &m.Content, &m.SnapshotType, &m.CreatedAt, &m.Snippet); err != nil {
			return nil, fmt.Errorf("scan snapshot match: %w", err)
		}
		results = append(results, m)
	}
	return results, rows.Err()
}

// ListSnapshots lists snapshots for a feature, newest first.
func (s *Store) ListSnapshots(featureID string, limit int) ([]ContextSnapshot, error) {
	if limit <= 0 {
		limit = 10
	}
	return collectRows(s.db.Reader(),
		`SELECT id, feature_id, COALESCE(session_id, ''), content, snapshot_type, created_at
		FROM context_snapshots WHERE feature_id = ? ORDER BY created_at DESC LIMIT ?`,
		[]any{featureID, limit},
		func(rows *sql.Rows) (ContextSnapshot, error) {
			var cs ContextSnapshot
			return cs, rows.Scan(&cs.ID, &cs.FeatureID, &cs.SessionID, &cs.Content, &cs.SnapshotType, &cs.CreatedAt)
		},
	)
}
