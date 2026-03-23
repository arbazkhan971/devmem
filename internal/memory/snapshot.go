package memory

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Snapshot struct {
	Project       string                `json:"project"`
	ProjectPath   string                `json:"project_path"`
	ActiveFeature *FeatureSnapshot      `json:"active_feature"`
	ActivePlan    *PlanSnapshot         `json:"active_plan"`
	Features      []FeatureSnapshot     `json:"features"`
	Consolidation ConsolidationSnapshot `json:"consolidation"`
}

type FeatureSnapshot struct {
	Name        string `json:"name"`
	Status      string `json:"status"`
	Branch      string `json:"branch"`
	Description string `json:"description"`
	LastActive  string `json:"last_active"`
	Sessions    int    `json:"sessions"`
	Commits     int    `json:"commits"`
}

type PlanSnapshot struct {
	Title       string `json:"title"`
	Progress    string `json:"progress"`
	CurrentStep string `json:"current_step"`
	StepsDone   int    `json:"steps_done"`
	StepsTotal  int    `json:"steps_total"`
}

type ConsolidationSnapshot struct {
	EntropyScore      float64 `json:"entropy_score"`
	UnsummarizedCount int     `json:"unsummarized_count"`
	LastRun           string  `json:"last_run"`
}

func loadPlanSnapshot(r *sql.DB, featureID string) *PlanSnapshot {
	var planID, title string
	if r.QueryRow(`SELECT id, title FROM plans WHERE feature_id = ? AND status = 'active' AND invalid_at IS NULL ORDER BY created_at DESC LIMIT 1`, featureID).Scan(&planID, &title) != nil {
		return nil
	}
	total := countRows(r, `SELECT COUNT(*) FROM plan_steps WHERE plan_id = ?`, planID)
	done := countRows(r, `SELECT COUNT(*) FROM plan_steps WHERE plan_id = ? AND status = 'completed'`, planID)
	var cur string
	r.QueryRow(`SELECT title FROM plan_steps WHERE plan_id = ? AND status != 'completed' ORDER BY step_number LIMIT 1`, planID).Scan(&cur)
	return &PlanSnapshot{Title: title, Progress: fmt.Sprintf("%d/%d", done, total), CurrentStep: cur, StepsDone: done, StepsTotal: total}
}

func loadConsolidationSnapshot(r *sql.DB) ConsolidationSnapshot {
	var cs ConsolidationSnapshot
	var lastRun sql.NullString
	if r.QueryRow(`SELECT entropy_score, unsummarized_count, COALESCE(last_run_at, '') FROM consolidation_state WHERE id = 1`).Scan(&cs.EntropyScore, &cs.UnsummarizedCount, &lastRun) == nil {
		cs.LastRun = lastRun.String
	}
	return cs
}

func (s *Store) WriteSnapshot(memDir, projectName, projectPath string) error {
	r := s.db.Reader()
	snap := Snapshot{Project: projectName, ProjectPath: projectPath}
	features, err := s.ListFeatures("all")
	if err != nil {
		return fmt.Errorf("snapshot list features: %w", err)
	}
	for _, f := range features {
		fs := FeatureSnapshot{
			Name: f.Name, Status: f.Status, Branch: f.Branch, Description: f.Description, LastActive: f.LastActive,
			Sessions: countRows(r, `SELECT COUNT(*) FROM sessions WHERE feature_id = ?`, f.ID),
			Commits:  countRows(r, `SELECT COUNT(*) FROM commits WHERE feature_id = ?`, f.ID),
		}
		snap.Features = append(snap.Features, fs)
		if f.Status == "active" {
			active := fs
			snap.ActiveFeature = &active
		}
	}
	if snap.ActiveFeature != nil {
		var fid string
		if r.QueryRow(`SELECT id FROM features WHERE name = ?`, snap.ActiveFeature.Name).Scan(&fid) == nil {
			snap.ActivePlan = loadPlanSnapshot(r, fid)
		}
	}
	snap.Consolidation = loadConsolidationSnapshot(r)
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fmt.Errorf("snapshot marshal: %w", err)
	}
	return os.WriteFile(filepath.Join(memDir, "current.json"), data, 0644)
}
