package memory

import (
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/arbazkhan971/memorx/internal/git"
	"github.com/arbazkhan971/memorx/internal/storage"
	"github.com/google/uuid"
)

type Feature struct{ ID, Name, Description, Status, Branch, CreatedAt, LastActive string }
type Session struct{ ID, FeatureID, Tool, StartedAt, EndedAt, Summary string }
type Store struct{ db *storage.DB }

func NewStore(db *storage.DB) *Store { return &Store{db: db} }

const featureCols = `id, name, description, status, COALESCE(branch, ''), created_at, last_active`
const sessionCols = `id, feature_id, tool, started_at, COALESCE(ended_at, ''), COALESCE(summary, '')`

func scanFeature(row interface{ Scan(...any) error }) (*Feature, error) {
	f := &Feature{}
	return f, row.Scan(&f.ID, &f.Name, &f.Description, &f.Status, &f.Branch, &f.CreatedAt, &f.LastActive)
}
func scanSession(sc interface{ Scan(...any) error }) (Session, error) {
	var s Session
	return s, sc.Scan(&s.ID, &s.FeatureID, &s.Tool, &s.StartedAt, &s.EndedAt, &s.Summary)
}
func countRows(r *sql.DB, query string, args ...interface{}) int {
	var c int
	r.QueryRow(query, args...).Scan(&c)
	return c
}
func detectBranch() string {
	if cwd, err := os.Getwd(); err == nil {
		if root, err := git.FindGitRoot(cwd); err == nil {
			if b, err := git.CurrentBranch(root); err == nil {
				return b
			}
		}
	}
	return ""
}

func (s *Store) insertFeature(name, description, now string) (*Feature, error) {
	id, branch := uuid.New().String(), detectBranch()
	if _, err := s.db.Writer().Exec(`INSERT INTO features (id, name, description, status, branch, created_at, last_active) VALUES (?, ?, ?, 'active', ?, ?, ?)`, id, name, description, branch, now, now); err != nil {
		return nil, fmt.Errorf("create feature: %w", err)
	}
	return &Feature{ID: id, Name: name, Description: description, Status: "active", Branch: branch, CreatedAt: now, LastActive: now}, nil
}
func (s *Store) CreateFeature(name, description string) (*Feature, error) {
	return s.insertFeature(name, description, time.Now().UTC().Format(time.DateTime))
}
func (s *Store) GetFeature(name string) (*Feature, error) {
	f, err := scanFeature(s.db.Reader().QueryRow(`SELECT `+featureCols+` FROM features WHERE name = ?`, name))
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("feature %q not found", name)
	}
	if err != nil {
		return nil, fmt.Errorf("get feature: %w", err)
	}
	return f, nil
}
func (s *Store) ListFeatures(statusFilter string) ([]Feature, error) {
	q, args := `SELECT `+featureCols+` FROM features`, []any(nil)
	if statusFilter != "" && statusFilter != "all" {
		q += ` WHERE status = ?`
		args = append(args, statusFilter)
	}
	q += ` ORDER BY last_active DESC`
	return collectRows(s.db.Reader(), q, args, func(rows *sql.Rows) (Feature, error) {
		f, err := scanFeature(rows)
		if err != nil {
			return Feature{}, err
		}
		return *f, nil
	})
}
func (s *Store) UpdateFeatureStatus(name, status string) error {
	now := time.Now().UTC().Format(time.DateTime)
	result, err := s.db.Writer().Exec(`UPDATE features SET status = ?, last_active = ? WHERE name = ?`, status, now, name)
	if err != nil {
		return fmt.Errorf("update feature status: %w", err)
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return fmt.Errorf("feature %q not found", name)
	}
	return nil
}
func (s *Store) GetActiveFeature() (*Feature, error) {
	f, err := scanFeature(s.db.Reader().QueryRow(`SELECT `+featureCols+` FROM features WHERE status = 'active' ORDER BY last_active DESC LIMIT 1`))
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("no active feature")
	}
	if err != nil {
		return nil, fmt.Errorf("get active feature: %w", err)
	}
	return f, nil
}
func (s *Store) StartFeature(name, description string) (*Feature, error) {
	now := time.Now().UTC().Format(time.DateTime)
	if _, err := s.db.Writer().Exec(`UPDATE features SET status = 'paused', last_active = ? WHERE status = 'active'`, now); err != nil {
		return nil, fmt.Errorf("pause active features: %w", err)
	}
	existing, err := scanFeature(s.db.Reader().QueryRow(`SELECT `+featureCols+` FROM features WHERE name = ?`, name))
	if err == sql.ErrNoRows {
		return s.insertFeature(name, description, now)
	}
	if err != nil {
		return nil, fmt.Errorf("check existing feature: %w", err)
	}
	if _, err = s.db.Writer().Exec(`UPDATE features SET status = 'active', last_active = ? WHERE id = ?`, now, existing.ID); err != nil {
		return nil, fmt.Errorf("resume feature: %w", err)
	}
	existing.Status = "active"
	existing.LastActive = now
	return existing, nil
}
func (s *Store) CreateSession(featureID, tool string) (*Session, error) {
	id, now := uuid.New().String(), time.Now().UTC().Format(time.DateTime)
	if _, err := s.db.Writer().Exec(`INSERT INTO sessions (id, feature_id, tool, started_at) VALUES (?, ?, ?, ?)`, id, featureID, tool, now); err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	return &Session{ID: id, FeatureID: featureID, Tool: tool, StartedAt: now}, nil
}
func (s *Store) endSession(sessionID, summary string, withSummary bool) error {
	now := time.Now().UTC().Format(time.DateTime)
	var res sql.Result
	var err error
	if withSummary {
		res, err = s.db.Writer().Exec(`UPDATE sessions SET ended_at = ?, summary = ? WHERE id = ?`, now, summary, sessionID)
	} else {
		res, err = s.db.Writer().Exec(`UPDATE sessions SET ended_at = ? WHERE id = ?`, now, sessionID)
	}
	if err != nil {
		return fmt.Errorf("end session: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("session %q not found", sessionID)
	}
	return nil
}
func (s *Store) EndSessionWithSummary(sessionID, summary string) error {
	return s.endSession(sessionID, summary, true)
}
func (s *Store) EndSession(sessionID string) error { return s.endSession(sessionID, "", false) }
func (s *Store) GetCurrentSession() (*Session, error) {
	sess, err := scanSession(s.db.Reader().QueryRow(`SELECT ` + sessionCols + ` FROM sessions WHERE ended_at IS NULL ORDER BY started_at DESC LIMIT 1`))
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
	return collectRows(s.db.Reader(), `SELECT `+sessionCols+` FROM sessions WHERE feature_id = ? ORDER BY started_at DESC LIMIT ?`, []any{featureID, limit}, func(rows *sql.Rows) (Session, error) { return scanSession(rows) })
}
