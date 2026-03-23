package memory

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Fact struct {
	ID, FeatureID, SessionID    string
	Subject, Predicate, Object  string
	ValidAt, InvalidAt, RecordedAt string
	Confidence                  float64
}

const factColumns = `id, feature_id, COALESCE(session_id, ''), subject, predicate, object,
        valid_at, COALESCE(invalid_at, ''), recorded_at, confidence`

func scanFact(row interface{ Scan(dest ...any) error }) (Fact, error) {
	var f Fact
	err := row.Scan(&f.ID, &f.FeatureID, &f.SessionID,
		&f.Subject, &f.Predicate, &f.Object,
		&f.ValidAt, &f.InvalidAt, &f.RecordedAt, &f.Confidence)
	return f, err
}

func collectFacts(rows *sql.Rows) ([]Fact, error) {
	defer rows.Close()
	var out []Fact
	for rows.Next() {
		f, err := scanFact(rows)
		if err != nil {
			return nil, fmt.Errorf("scan fact: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s *Store) CreateFact(featureID, sessionID, subject, predicate, object string) (*Fact, error) {
	now := time.Now().UTC().Format(time.DateTime)
	w := s.db.Writer()

	var existingID, existingObject string
	err := w.QueryRow(
		`SELECT id, object FROM facts WHERE feature_id = ? AND subject = ? AND predicate = ? AND invalid_at IS NULL`,
		featureID, subject, predicate,
	).Scan(&existingID, &existingObject)

	if err == nil {
		if existingObject == object {
			f, err := scanFact(s.db.Reader().QueryRow(`SELECT `+factColumns+` FROM facts WHERE id = ?`, existingID))
			if err != nil {
				return nil, fmt.Errorf("read existing fact: %w", err)
			}
			return &f, nil
		}
		if _, err := w.Exec(`UPDATE facts SET invalid_at = ? WHERE id = ?`, now, existingID); err != nil {
			return nil, fmt.Errorf("invalidate old fact: %w", err)
		}
	}

	id := uuid.New().String()
	if _, err = w.Exec(
		`INSERT INTO facts (id, feature_id, session_id, subject, predicate, object, valid_at, recorded_at, confidence) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1.0)`,
		id, featureID, nullIfEmpty(sessionID), subject, predicate, object, now, now,
	); err != nil {
		return nil, fmt.Errorf("create fact: %w", err)
	}

	var rowID int64
	if err = w.QueryRow(`SELECT rowid FROM facts WHERE id = ?`, id).Scan(&rowID); err != nil {
		return nil, fmt.Errorf("get fact rowid: %w", err)
	}
	if _, err = w.Exec(`INSERT INTO facts_fts(rowid, subject, predicate, object) VALUES (?, ?, ?, ?)`, rowID, subject, predicate, object); err != nil {
		return nil, fmt.Errorf("sync fact to fts: %w", err)
	}

	return &Fact{
		ID: id, FeatureID: featureID, SessionID: sessionID,
		Subject: subject, Predicate: predicate, Object: object,
		ValidAt: now, RecordedAt: now, Confidence: 1.0,
	}, nil
}

func (s *Store) GetActiveFacts(featureID string) ([]Fact, error) {
	return s.queryFacts(featureID, nil)
}

func (s *Store) QueryFactsAsOf(featureID string, asOf time.Time) ([]Fact, error) {
	return s.queryFacts(featureID, &asOf)
}

func (s *Store) queryFacts(featureID string, asOf *time.Time) ([]Fact, error) {
	var q string
	var args []any
	if asOf != nil {
		ts := asOf.UTC().Format(time.DateTime)
		q = `SELECT ` + factColumns + ` FROM facts WHERE feature_id = ? AND valid_at <= ? AND (invalid_at IS NULL OR invalid_at > ?) ORDER BY valid_at DESC`
		args = []any{featureID, ts, ts}
	} else {
		q = `SELECT ` + factColumns + ` FROM facts WHERE feature_id = ? AND invalid_at IS NULL ORDER BY valid_at DESC`
		args = []any{featureID}
	}
	rows, err := s.db.Reader().Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("query facts: %w", err)
	}
	return collectFacts(rows)
}

func (s *Store) InvalidateFact(factID string) error {
	now := time.Now().UTC().Format(time.DateTime)
	result, err := s.db.Writer().Exec(`UPDATE facts SET invalid_at = ? WHERE id = ?`, now, factID)
	if err != nil {
		return fmt.Errorf("invalidate fact: %w", err)
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return fmt.Errorf("fact %q not found", factID)
	}
	return nil
}

func nullIfEmpty(v string) sql.NullString {
	if v == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: v, Valid: true}
}
