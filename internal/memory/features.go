package memory

import (
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/arbaz/devmem/internal/git"
	"github.com/arbaz/devmem/internal/storage"
	"github.com/google/uuid"
)

type Feature struct {
	ID, Name, Description, Status, Branch, CreatedAt, LastActive string
}

const featureCols = `id, name, description, status, COALESCE(branch, ''), created_at, last_active`

func scanFeature(row interface{ Scan(...any) error }) (*Feature, error) {
	f := &Feature{}
	err := row.Scan(&f.ID, &f.Name, &f.Description, &f.Status, &f.Branch, &f.CreatedAt, &f.LastActive)
	return f, err
}

type Store struct{ db *storage.DB }

func NewStore(db *storage.DB) *Store { return &Store{db: db} }

func countRows(r *sql.DB, query string, args ...interface{}) int {
	var c int
	r.QueryRow(query, args...).Scan(&c)
	return c
}

func detectBranch() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	root, err := git.FindGitRoot(cwd)
	if err != nil {
		return ""
	}
	branch, err := git.CurrentBranch(root)
	if err != nil {
		return ""
	}
	return branch
}

func (s *Store) insertFeature(name, description, now string) (*Feature, error) {
	id, branch := uuid.New().String(), detectBranch()
	if _, err := s.db.Writer().Exec(
		`INSERT INTO features (id, name, description, status, branch, created_at, last_active) VALUES (?, ?, ?, 'active', ?, ?, ?)`,
		id, name, description, branch, now, now,
	); err != nil {
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
	rows, err := s.db.Reader().Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("list features: %w", err)
	}
	defer rows.Close()
	var out []Feature
	for rows.Next() {
		f, err := scanFeature(rows)
		if err != nil {
			return nil, fmt.Errorf("scan feature: %w", err)
		}
		out = append(out, *f)
	}
	return out, rows.Err()
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
