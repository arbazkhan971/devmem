package memory

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/arbazkhan971/memorx/internal/plans"
)

// StaleAlert represents a feature that has been inactive with unresolved blockers.
type StaleAlert struct {
	FeatureName  string
	DaysInactive int
	Blockers     []string
}

// DependencyAlert represents a warning about files used by other features.
type DependencyAlert struct {
	File            string
	RelatedFeatures []string
	RecentCommits   int
}

// PlanAlert represents the status of an active plan.
type PlanAlert struct {
	FeatureName       string
	PlanTitle         string
	StepsDone         int
	StepsTotal        int
	DaysSinceActivity int
	Stalled           bool
	NearlyComplete    bool
	Blockers          int
}

// ContradictionAlert represents a contradiction between new content and existing facts.
type ContradictionAlert struct {
	Content     string
	FactSubject string
	FactPred    string
	FactObject  string
	FactDate    string
}

// FindStaleAlerts finds features inactive for N+ days with unresolved blockers.
func (s *Store) FindStaleAlerts(daysThreshold int) ([]StaleAlert, error) {
	if daysThreshold <= 0 {
		daysThreshold = 14
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -daysThreshold).Format(time.DateTime)

	rows, err := s.db.Reader().Query(
		`SELECT f.id, f.name, f.last_active FROM features f
		 WHERE f.last_active < ? AND f.status IN ('active', 'paused')
		 ORDER BY f.last_active ASC`, cutoff)
	if err != nil {
		return nil, fmt.Errorf("query stale features: %w", err)
	}
	defer rows.Close()

	var alerts []StaleAlert
	for rows.Next() {
		var fID, name, lastActive string
		if err := rows.Scan(&fID, &name, &lastActive); err != nil {
			continue
		}
		blockerRows, err := s.db.Reader().Query(
			`SELECT content FROM notes WHERE feature_id = ? AND type = 'blocker' ORDER BY created_at DESC`, fID)
		if err != nil {
			continue
		}
		var blockers []string
		for blockerRows.Next() {
			var content string
			if blockerRows.Scan(&content) == nil {
				if len(content) > 120 {
					content = content[:120] + "..."
				}
				blockers = append(blockers, content)
			}
		}
		blockerRows.Close()

		t, err := time.Parse(time.DateTime, lastActive)
		daysInactive := 0
		if err == nil {
			daysInactive = int(math.Floor(time.Since(t).Hours() / 24))
		}
		alerts = append(alerts, StaleAlert{FeatureName: name, DaysInactive: daysInactive, Blockers: blockers})
	}
	return alerts, rows.Err()
}

// FormatStaleAlerts formats stale alerts into a human-readable string.
func FormatStaleAlerts(alerts []StaleAlert) string {
	if len(alerts) == 0 {
		return "No stale features found. All features are active or have no blockers."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Stale Feature Alerts (%d)\n\n", len(alerts))
	for _, a := range alerts {
		fmt.Fprintf(&b, "## %s (inactive %d days)\n", a.FeatureName, a.DaysInactive)
		if len(a.Blockers) > 0 {
			fmt.Fprintf(&b, "Unresolved blockers (%d):\n", len(a.Blockers))
			for _, bl := range a.Blockers {
				fmt.Fprintf(&b, "- %s\n", strings.ReplaceAll(bl, "\n", " "))
			}
		} else {
			b.WriteString("No blockers recorded.\n")
		}
		b.WriteString("\n")
	}
	return b.String()
}

// FindDependencyAlerts checks which features reference the given files.
func (s *Store) FindDependencyAlerts(files []string) ([]DependencyAlert, error) {
	r := s.db.Reader()
	var alerts []DependencyAlert
	for _, file := range files {
		alert := DependencyAlert{File: file}
		featureRows, err := r.Query(
			`SELECT DISTINCT f.name FROM files_touched ft
			 JOIN features f ON ft.feature_id = f.id
			 WHERE ft.path = ? ORDER BY f.last_active DESC`, file)
		if err == nil {
			for featureRows.Next() {
				var name string
				if featureRows.Scan(&name) == nil {
					alert.RelatedFeatures = append(alert.RelatedFeatures, name)
				}
			}
			featureRows.Close()
		}
		weekAgo := time.Now().UTC().AddDate(0, 0, -7).Format(time.DateTime)
		var count int
		if r.QueryRow(`SELECT COUNT(*) FROM commits WHERE files_changed LIKE ? AND committed_at > ?`,
			"%"+file+"%", weekAgo).Scan(&count) == nil {
			alert.RecentCommits = count
		}
		alerts = append(alerts, alert)
	}
	return alerts, nil
}

// FormatDependencyAlerts formats dependency alerts into a human-readable string.
func FormatDependencyAlerts(alerts []DependencyAlert) string {
	if len(alerts) == 0 {
		return "No dependency alerts for the given files."
	}
	var b strings.Builder
	b.WriteString("# Dependency Alerts\n\n")
	for _, a := range alerts {
		fmt.Fprintf(&b, "## %s\n", a.File)
		if len(a.RelatedFeatures) > 0 {
			fmt.Fprintf(&b, "Used by features: %s\n", strings.Join(a.RelatedFeatures, ", "))
		}
		if a.RecentCommits > 0 {
			fmt.Fprintf(&b, "Recent activity: %d commits in last week\n", a.RecentCommits)
		}
		if len(a.RelatedFeatures) == 0 && a.RecentCommits == 0 {
			b.WriteString("No other features reference this file.\n")
		}
		b.WriteString("\n")
	}
	return b.String()
}

// FindPlanAlerts analyzes all active plans for velocity and status.
func (s *Store) FindPlanAlerts(pm *plans.Manager) ([]PlanAlert, error) {
	r := s.db.Reader()
	features, err := s.ListFeatures("all")
	if err != nil {
		return nil, fmt.Errorf("list features: %w", err)
	}
	var alerts []PlanAlert
	for _, f := range features {
		if f.Status == "done" {
			continue
		}
		plan, err := pm.GetActivePlan(f.ID)
		if err != nil {
			continue
		}
		steps, err := pm.GetPlanSteps(plan.ID)
		if err != nil {
			continue
		}
		done := 0
		for _, st := range steps {
			if st.Status == "completed" {
				done++
			}
		}
		daysInactive := 0
		if t, err := time.Parse(time.DateTime, f.LastActive); err == nil {
			daysInactive = int(math.Floor(time.Since(t).Hours() / 24))
		}
		blockerCount := countRows(r, `SELECT COUNT(*) FROM notes WHERE feature_id = ? AND type = 'blocker'`, f.ID)
		alert := PlanAlert{
			FeatureName:       f.Name,
			PlanTitle:         plan.Title,
			StepsDone:         done,
			StepsTotal:        len(steps),
			DaysSinceActivity: daysInactive,
			Blockers:          blockerCount,
			Stalled:           daysInactive >= 5 && done < len(steps),
			NearlyComplete:    len(steps) > 0 && len(steps)-done <= 1,
		}
		alerts = append(alerts, alert)
	}
	return alerts, nil
}

// FormatPlanAlerts formats plan alerts into a human-readable string.
func FormatPlanAlerts(alerts []PlanAlert) string {
	if len(alerts) == 0 {
		return "No active plans to analyze."
	}
	var b strings.Builder
	b.WriteString("# Plan Alerts\n\n")
	for _, a := range alerts {
		status := ""
		if a.NearlyComplete {
			remaining := a.StepsTotal - a.StepsDone
			status = fmt.Sprintf(" -- %d/%d done, %d step left!", a.StepsDone, a.StepsTotal, remaining)
		}
		if a.Stalled {
			status += fmt.Sprintf(" -- stalled %d days", a.DaysSinceActivity)
			if a.Blockers > 0 {
				status += fmt.Sprintf(", %d blockers", a.Blockers)
			}
		}
		if !a.NearlyComplete && !a.Stalled {
			status = fmt.Sprintf(" -- %d/%d done", a.StepsDone, a.StepsTotal)
		}
		fmt.Fprintf(&b, "- **%s** (%s)%s\n", a.FeatureName, a.PlanTitle, status)
	}
	return b.String()
}

// FindContradictions checks if content contradicts any existing active facts.
func (s *Store) FindContradictions(content string) ([]ContradictionAlert, error) {
	rows, err := s.db.Reader().Query(
		`SELECT subject, predicate, object, valid_at FROM facts WHERE invalid_at IS NULL ORDER BY valid_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("query active facts: %w", err)
	}
	defer rows.Close()

	contentLower := strings.ToLower(content)
	var alerts []ContradictionAlert
	for rows.Next() {
		var subj, pred, obj, validAt string
		if err := rows.Scan(&subj, &pred, &obj, &validAt); err != nil {
			continue
		}
		subjLower := strings.ToLower(subj)
		if !strings.Contains(contentLower, subjLower) {
			continue
		}
		objLower := strings.ToLower(obj)
		predLower := strings.ToLower(pred)
		if strings.Contains(contentLower, subjLower) &&
			(strings.Contains(contentLower, predLower) || strings.Contains(contentLower, "use") || strings.Contains(contentLower, "uses")) &&
			!strings.Contains(contentLower, objLower) {
			alerts = append(alerts, ContradictionAlert{
				Content: content, FactSubject: subj, FactPred: pred, FactObject: obj, FactDate: validAt,
			})
		}
	}
	return alerts, rows.Err()
}

// FormatContradictions formats contradiction alerts into a human-readable string.
func FormatContradictions(alerts []ContradictionAlert) string {
	if len(alerts) == 0 {
		return "No contradictions detected."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Contradiction Alerts (%d)\n\n", len(alerts))
	for _, a := range alerts {
		date := a.FactDate
		if len(date) > 10 {
			date = date[:10]
		}
		content := strings.ReplaceAll(a.Content, "\n", " ")
		if len(content) > 80 {
			content = content[:80] + "..."
		}
		fmt.Fprintf(&b, "- Warning: you said %q but fact from %s says '%s %s %s'\n",
			content, date, a.FactSubject, a.FactPred, a.FactObject)
	}
	return b.String()
}
