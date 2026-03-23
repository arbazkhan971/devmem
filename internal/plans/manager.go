package plans

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/arbaz/devmem/internal/storage"
	"github.com/google/uuid"
)

// StepInput represents a step to be created with a new plan.
type StepInput struct {
	Title       string
	Description string
}

// Plan represents a development plan for a feature.
type Plan struct {
	ID         string
	FeatureID  string
	SessionID  string
	Title      string
	Content    string
	Status     string
	SourceTool string
	ValidAt    string
	InvalidAt  string
	CreatedAt  string
	UpdatedAt  string
}

// PlanStep represents a single step within a plan.
type PlanStep struct {
	ID             string
	PlanID         string
	Title          string
	Description    string
	Status         string
	CompletedAt    string
	LinkedCommits  string
	StepNumber     int
}

// Manager provides plan CRUD operations with bi-temporal versioning.
type Manager struct {
	db *storage.DB
}

// NewManager creates a new Manager backed by the given DB.
func NewManager(db *storage.DB) *Manager {
	return &Manager{db: db}
}

// CreatePlan creates a new plan with steps. If an active plan exists for the
// feature, it is superseded (invalid_at set to now, status set to superseded).
// Completed steps from the old plan are copied to the new plan.
func (m *Manager) CreatePlan(featureID, sessionID, title, content, sourceTool string, steps []StepInput) (*Plan, error) {
	now := time.Now().UTC().Format(time.DateTime)
	planID := uuid.New().String()

	tx, err := m.db.Writer().Begin()
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Check for existing active plan and supersede it
	var oldPlanID string
	err = tx.QueryRow(
		`SELECT id FROM plans WHERE feature_id = ? AND invalid_at IS NULL AND status = 'active'`,
		featureID,
	).Scan(&oldPlanID)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("check existing plan: %w", err)
	}

	var completedSteps []PlanStep
	if oldPlanID != "" {
		// Supersede old plan
		_, err = tx.Exec(
			`UPDATE plans SET invalid_at = ?, status = 'superseded', updated_at = ? WHERE id = ?`,
			now, now, oldPlanID,
		)
		if err != nil {
			return nil, fmt.Errorf("supersede old plan: %w", err)
		}

		// Gather completed steps from old plan
		rows, err := tx.Query(
			`SELECT id, plan_id, title, description, status, COALESCE(completed_at, ''), COALESCE(linked_commits, '[]'), step_number
			 FROM plan_steps WHERE plan_id = ? AND status = 'completed' ORDER BY step_number`,
			oldPlanID,
		)
		if err != nil {
			return nil, fmt.Errorf("query old completed steps: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var s PlanStep
			if err := rows.Scan(&s.ID, &s.PlanID, &s.Title, &s.Description, &s.Status, &s.CompletedAt, &s.LinkedCommits, &s.StepNumber); err != nil {
				return nil, fmt.Errorf("scan old step: %w", err)
			}
			completedSteps = append(completedSteps, s)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("iterate old steps: %w", err)
		}
	}

	// Insert new plan
	_, err = tx.Exec(
		`INSERT INTO plans (id, feature_id, session_id, title, content, status, source_tool, valid_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, 'active', ?, ?, ?, ?)`,
		planID, featureID, sessionID, title, content, sourceTool, now, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert plan: %w", err)
	}

	// Sync to plans_fts
	var rowid int64
	err = tx.QueryRow(`SELECT rowid FROM plans WHERE id = ?`, planID).Scan(&rowid)
	if err != nil {
		return nil, fmt.Errorf("get plan rowid: %w", err)
	}
	_, err = tx.Exec(
		`INSERT INTO plans_fts(rowid, title, content) VALUES (?, ?, ?)`,
		rowid, title, content,
	)
	if err != nil {
		return nil, fmt.Errorf("sync plans_fts: %w", err)
	}

	// Copy completed steps from old plan
	stepNum := 1
	for _, cs := range completedSteps {
		stepID := uuid.New().String()
		_, err = tx.Exec(
			`INSERT INTO plan_steps (id, plan_id, step_number, title, description, status, completed_at, linked_commits)
			 VALUES (?, ?, ?, ?, ?, 'completed', ?, ?)`,
			stepID, planID, stepNum, cs.Title, cs.Description, cs.CompletedAt, cs.LinkedCommits,
		)
		if err != nil {
			return nil, fmt.Errorf("copy completed step: %w", err)
		}
		stepNum++
	}

	// Insert new steps
	for _, s := range steps {
		stepID := uuid.New().String()
		_, err = tx.Exec(
			`INSERT INTO plan_steps (id, plan_id, step_number, title, description, status, linked_commits)
			 VALUES (?, ?, ?, ?, ?, 'pending', '[]')`,
			stepID, planID, stepNum, s.Title, s.Description,
		)
		if err != nil {
			return nil, fmt.Errorf("insert step: %w", err)
		}
		stepNum++
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	return &Plan{
		ID:         planID,
		FeatureID:  featureID,
		SessionID:  sessionID,
		Title:      title,
		Content:    content,
		Status:     "active",
		SourceTool: sourceTool,
		ValidAt:    now,
		InvalidAt:  "",
		CreatedAt:  now,
		UpdatedAt:  now,
	}, nil
}

// GetActivePlan returns the current active plan for a feature.
func (m *Manager) GetActivePlan(featureID string) (*Plan, error) {
	p := &Plan{}
	err := m.db.Reader().QueryRow(
		`SELECT id, feature_id, COALESCE(session_id, ''), title, content, status, COALESCE(source_tool, 'unknown'),
		        COALESCE(valid_at, ''), COALESCE(invalid_at, ''), created_at, updated_at
		 FROM plans WHERE feature_id = ? AND invalid_at IS NULL AND status = 'active'`,
		featureID,
	).Scan(&p.ID, &p.FeatureID, &p.SessionID, &p.Title, &p.Content, &p.Status, &p.SourceTool,
		&p.ValidAt, &p.InvalidAt, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("no active plan for feature %q", featureID)
	}
	if err != nil {
		return nil, fmt.Errorf("get active plan: %w", err)
	}
	return p, nil
}

// ListPlans returns all plans for a feature, including superseded ones.
func (m *Manager) ListPlans(featureID string) ([]Plan, error) {
	rows, err := m.db.Reader().Query(
		`SELECT id, feature_id, COALESCE(session_id, ''), title, content, status, COALESCE(source_tool, 'unknown'),
		        COALESCE(valid_at, ''), COALESCE(invalid_at, ''), created_at, updated_at
		 FROM plans WHERE feature_id = ? ORDER BY created_at DESC`,
		featureID,
	)
	if err != nil {
		return nil, fmt.Errorf("list plans: %w", err)
	}
	defer rows.Close()

	var plans []Plan
	for rows.Next() {
		var p Plan
		if err := rows.Scan(&p.ID, &p.FeatureID, &p.SessionID, &p.Title, &p.Content, &p.Status, &p.SourceTool,
			&p.ValidAt, &p.InvalidAt, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan plan: %w", err)
		}
		plans = append(plans, p)
	}
	return plans, rows.Err()
}

// UpdateStepStatus updates a step's status and sets completed_at if completed.
func (m *Manager) UpdateStepStatus(stepID, status string) error {
	now := time.Now().UTC().Format(time.DateTime)

	var completedAt *string
	if status == "completed" {
		completedAt = &now
	}

	result, err := m.db.Writer().Exec(
		`UPDATE plan_steps SET status = ?, completed_at = ? WHERE id = ?`,
		status, completedAt, stepID,
	)
	if err != nil {
		return fmt.Errorf("update step status: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("step %q not found", stepID)
	}
	return nil
}

// GetPlanSteps returns all steps for a plan, ordered by step number.
func (m *Manager) GetPlanSteps(planID string) ([]PlanStep, error) {
	rows, err := m.db.Reader().Query(
		`SELECT id, plan_id, title, COALESCE(description, ''), status, COALESCE(completed_at, ''), COALESCE(linked_commits, '[]'), step_number
		 FROM plan_steps WHERE plan_id = ? ORDER BY step_number`,
		planID,
	)
	if err != nil {
		return nil, fmt.Errorf("get plan steps: %w", err)
	}
	defer rows.Close()

	var steps []PlanStep
	for rows.Next() {
		var s PlanStep
		if err := rows.Scan(&s.ID, &s.PlanID, &s.Title, &s.Description, &s.Status, &s.CompletedAt, &s.LinkedCommits, &s.StepNumber); err != nil {
			return nil, fmt.Errorf("scan step: %w", err)
		}
		steps = append(steps, s)
	}
	return steps, rows.Err()
}

// LinkCommitToStep appends a commit hash to the step's linked_commits JSON array.
func (m *Manager) LinkCommitToStep(stepID, commitHash string) error {
	// Read current linked_commits
	var linkedJSON string
	err := m.db.Reader().QueryRow(
		`SELECT COALESCE(linked_commits, '[]') FROM plan_steps WHERE id = ?`, stepID,
	).Scan(&linkedJSON)
	if err == sql.ErrNoRows {
		return fmt.Errorf("step %q not found", stepID)
	}
	if err != nil {
		return fmt.Errorf("read linked_commits: %w", err)
	}

	var commits []string
	if err := json.Unmarshal([]byte(linkedJSON), &commits); err != nil {
		commits = []string{}
	}

	// Append new commit hash
	commits = append(commits, commitHash)
	updated, err := json.Marshal(commits)
	if err != nil {
		return fmt.Errorf("marshal linked_commits: %w", err)
	}

	_, err = m.db.Writer().Exec(
		`UPDATE plan_steps SET linked_commits = ? WHERE id = ?`,
		string(updated), stepID,
	)
	if err != nil {
		return fmt.Errorf("update linked_commits: %w", err)
	}
	return nil
}
