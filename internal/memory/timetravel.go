package memory

import (
	"database/sql"
	"fmt"
	"time"
)

// TimeTravelResult holds the complete state of memory at a specific point in time.
type TimeTravelResult struct {
	AsOf        string
	ActiveFacts []Fact
	NotesAtTime []Note
	PlanAtTime  *TimeTravelPlanSnapshot
	Feature     *Feature
}

// TimeTravelPlanSnapshot captures the state of a plan at a point in time.
type TimeTravelPlanSnapshot struct {
	Title string
	Steps []TimeTravelStepSnapshot
}

// TimeTravelStepSnapshot captures a step's state at a point in time.
type TimeTravelStepSnapshot struct {
	Title  string
	Status string
}

// TimeTravel queries the complete state of memory for a feature at a given point in time.
func (s *Store) TimeTravel(featureID string, asOf time.Time) (*TimeTravelResult, error) {
	ts := asOf.UTC().Format(time.DateTime)
	r := s.db.Reader()

	result := &TimeTravelResult{AsOf: ts}

	// Load feature
	feature, err := scanFeature(r.QueryRow("SELECT "+featureCols+" FROM features WHERE id = ?", featureID))
	if err != nil {
		return nil, fmt.Errorf("feature not found: %w", err)
	}
	result.Feature = feature

	// Query facts active at asOf: valid_at <= asOf AND (invalid_at IS NULL OR invalid_at > asOf)
	result.ActiveFacts, err = s.QueryFactsAsOf(featureID, asOf)
	if err != nil {
		return nil, fmt.Errorf("query facts as of %s: %w", ts, err)
	}

	// Query notes created at or before asOf
	result.NotesAtTime, err = collectRows(r,
		`SELECT `+noteCols+` FROM notes WHERE feature_id = ? AND created_at <= ? ORDER BY created_at DESC LIMIT 20`,
		[]any{featureID, ts},
		func(rows *sql.Rows) (Note, error) { return scanNote(rows) },
	)
	if err != nil {
		return nil, fmt.Errorf("query notes as of %s: %w", ts, err)
	}

	// Query the most recent plan that existed at asOf
	var planID, planTitle string
	err = r.QueryRow(
		`SELECT id, title FROM plans WHERE feature_id = ? AND created_at <= ? ORDER BY created_at DESC LIMIT 1`,
		featureID, ts,
	).Scan(&planID, &planTitle)
	if err == nil {
		snap := &TimeTravelPlanSnapshot{Title: planTitle}

		// Get steps for that plan, evaluating status as of the timestamp
		stepRows, err := r.Query(
			`SELECT title, CASE WHEN completed_at IS NOT NULL AND completed_at <= ? THEN 'completed' ELSE status END as effective_status FROM plan_steps WHERE plan_id = ? AND rowid IN (SELECT rowid FROM plan_steps WHERE plan_id = ?) ORDER BY step_number`,
			ts, planID, planID,
		)
		if err == nil {
			defer stepRows.Close()
			for stepRows.Next() {
				var st TimeTravelStepSnapshot
				if stepRows.Scan(&st.Title, &st.Status) == nil {
					snap.Steps = append(snap.Steps, st)
				}
			}
		}

		result.PlanAtTime = snap
	}

	return result, nil
}

// FormatTimeTravel produces a compact summary string for a TimeTravelResult.
func FormatTimeTravel(r *TimeTravelResult) string {
	if r == nil {
		return "No time travel result."
	}

	planSummary := "no plan"
	if r.PlanAtTime != nil {
		completed := 0
		for _, st := range r.PlanAtTime.Steps {
			if st.Status == "completed" {
				completed++
			}
		}
		planSummary = fmt.Sprintf("plan '%s' %d/%d steps", r.PlanAtTime.Title, completed, len(r.PlanAtTime.Steps))
	}

	featureName := "unknown"
	if r.Feature != nil {
		featureName = r.Feature.Name
	}

	return fmt.Sprintf("As of %s [%s]: %d facts, %d notes, %s",
		r.AsOf, featureName, len(r.ActiveFacts), len(r.NotesAtTime), planSummary)
}
