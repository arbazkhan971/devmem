package memory

import (
	"database/sql"
	"fmt"
	"math"
	"strings"
	"time"
)

// FocusTimeEntry represents time spent on a single feature.
type FocusTimeEntry struct {
	FeatureName  string
	TotalHours   float64
	SessionCount int
}

// VelocityEntry represents plan completion velocity for a feature.
type VelocityEntry struct {
	FeatureName    string
	StepsCompleted int
	TotalSteps     int
	DaysActive     int
	StepsPerDay    float64
	EstDaysLeft    float64
	Stalled        bool
	StalledDays    int
}

// InterruptionReport represents context switch analysis.
type InterruptionReport struct {
	SwitchCount          int
	DaysAnalyzed         int
	LongestUninterrupted float64 // hours
	LongestFeature       string
	Switches             []FeatureSwitch
}

// FeatureSwitch represents a single context switch between features.
type FeatureSwitch struct {
	From, To  string
	Timestamp string
}

// WeeklyReportData holds aggregated data for a weekly report.
type WeeklyReportData struct {
	DaysBack         int
	FeaturesTouched  []string
	CommitsByIntent  map[string]int
	TotalCommits     int
	DecisionsMade    int
	BlockersAdded    int
	SessionCount     int
	TotalHours       float64
	TopDecisions     []string
}

// GetFocusTime calculates time spent per feature over the given number of days.
func (s *Store) GetFocusTime(featureName string, days int) ([]FocusTimeEntry, error) {
	r := s.db.Reader()
	since := time.Now().UTC().AddDate(0, 0, -days).Format(time.DateTime)

	q := `SELECT f.name, s.started_at, COALESCE(s.ended_at, datetime('now')) as ended
	      FROM sessions s
	      JOIN features f ON s.feature_id = f.id
	      WHERE s.started_at >= ?`
	args := []any{since}

	if featureName != "" {
		q += ` AND f.name = ?`
		args = append(args, featureName)
	}
	q += ` ORDER BY f.name, s.started_at`

	rows, err := r.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("query focus time: %w", err)
	}
	defer rows.Close()

	featureMap := map[string]*FocusTimeEntry{}
	featureOrder := []string{}

	for rows.Next() {
		var name, startStr, endStr string
		if rows.Scan(&name, &startStr, &endStr) != nil {
			continue
		}
		start, e1 := time.Parse(time.DateTime, startStr)
		end, e2 := time.Parse(time.DateTime, endStr)
		if e1 != nil || e2 != nil {
			continue
		}
		dur := end.Sub(start).Hours()
		if dur < 0 {
			dur = 0
		}
		// Cap individual sessions at 8 hours to filter out unclosed sessions
		if dur > 8 {
			dur = 8
		}
		entry, ok := featureMap[name]
		if !ok {
			entry = &FocusTimeEntry{FeatureName: name}
			featureMap[name] = entry
			featureOrder = append(featureOrder, name)
		}
		entry.TotalHours += dur
		entry.SessionCount++
	}

	var results []FocusTimeEntry
	for _, name := range featureOrder {
		e := featureMap[name]
		e.TotalHours = math.Round(e.TotalHours*10) / 10
		results = append(results, *e)
	}
	return results, nil
}

// GetVelocity calculates plan completion velocity per feature.
func (s *Store) GetVelocity(featureName string) ([]VelocityEntry, error) {
	r := s.db.Reader()

	q := `SELECT f.id, f.name, f.created_at FROM features f WHERE f.status IN ('active', 'paused')`
	args := []any(nil)
	if featureName != "" {
		q += ` AND f.name = ?`
		args = append(args, featureName)
	}
	q += ` ORDER BY f.last_active DESC`

	type featureRow struct {
		id, name, createdAt string
	}
	features := scanRows(r, q, args, func(rows *sql.Rows) (featureRow, error) {
		var f featureRow
		return f, rows.Scan(&f.id, &f.name, &f.createdAt)
	})

	var results []VelocityEntry
	for _, feat := range features {
		var planID, planCreatedAt string
		if r.QueryRow(
			`SELECT id, created_at FROM plans WHERE feature_id = ? AND status = 'active' AND invalid_at IS NULL ORDER BY created_at DESC LIMIT 1`,
			feat.id,
		).Scan(&planID, &planCreatedAt) != nil {
			continue
		}

		totalSteps := countRows(r, `SELECT COUNT(*) FROM plan_steps WHERE plan_id = ?`, planID)
		completedSteps := countRows(r, `SELECT COUNT(*) FROM plan_steps WHERE plan_id = ? AND status = 'completed'`, planID)

		if totalSteps == 0 {
			continue
		}

		planStart, err := time.Parse(time.DateTime, planCreatedAt)
		if err != nil {
			continue
		}
		daysActive := int(math.Ceil(time.Since(planStart).Hours() / 24))
		if daysActive < 1 {
			daysActive = 1
		}

		stepsPerDay := float64(completedSteps) / float64(daysActive)
		stepsPerDay = math.Round(stepsPerDay*10) / 10

		remaining := totalSteps - completedSteps
		var estDaysLeft float64
		if stepsPerDay > 0 {
			estDaysLeft = math.Ceil(float64(remaining) / stepsPerDay)
		}

		stalled := false
		stalledDays := 0
		var lastCompletedAt sql.NullString
		r.QueryRow(
			`SELECT completed_at FROM plan_steps WHERE plan_id = ? AND status = 'completed' ORDER BY completed_at DESC LIMIT 1`,
			planID,
		).Scan(&lastCompletedAt)

		if completedSteps > 0 && lastCompletedAt.Valid {
			if lc, err := time.Parse(time.DateTime, lastCompletedAt.String); err == nil {
				stalledDays = int(math.Floor(time.Since(lc).Hours() / 24))
				stalled = stalledDays >= 3
			}
		} else if completedSteps == 0 && daysActive >= 3 {
			stalled = true
			stalledDays = daysActive
		}

		results = append(results, VelocityEntry{
			FeatureName:    feat.name,
			StepsCompleted: completedSteps,
			TotalSteps:     totalSteps,
			DaysActive:     daysActive,
			StepsPerDay:    stepsPerDay,
			EstDaysLeft:    estDaysLeft,
			Stalled:        stalled,
			StalledDays:    stalledDays,
		})
	}

	return results, nil
}

// GetInterruptions analyzes context switches between features.
func (s *Store) GetInterruptions(days int) (*InterruptionReport, error) {
	r := s.db.Reader()
	since := time.Now().UTC().AddDate(0, 0, -days).Format(time.DateTime)

	type sessionEntry struct {
		featureName, startedAt, endedAt string
	}
	sessions := scanRows(r,
		`SELECT f.name, s.started_at, COALESCE(s.ended_at, datetime('now'))
		 FROM sessions s
		 JOIN features f ON s.feature_id = f.id
		 WHERE s.started_at >= ?
		 ORDER BY s.started_at ASC`,
		[]any{since},
		func(rows *sql.Rows) (sessionEntry, error) {
			var se sessionEntry
			return se, rows.Scan(&se.featureName, &se.startedAt, &se.endedAt)
		},
	)

	report := &InterruptionReport{DaysAnalyzed: days}

	if len(sessions) == 0 {
		return report, nil
	}

	lastFeature := ""
	for _, sess := range sessions {
		if lastFeature != "" && sess.featureName != lastFeature {
			report.SwitchCount++
			report.Switches = append(report.Switches, FeatureSwitch{
				From:      lastFeature,
				To:        sess.featureName,
				Timestamp: sess.startedAt,
			})
		}
		lastFeature = sess.featureName
	}

	type stretch struct {
		feature string
		start   time.Time
		end     time.Time
	}
	var stretches []stretch
	var current *stretch

	for _, sess := range sessions {
		start, e1 := time.Parse(time.DateTime, sess.startedAt)
		end, e2 := time.Parse(time.DateTime, sess.endedAt)
		if e1 != nil || e2 != nil {
			continue
		}

		if current == nil || current.feature != sess.featureName {
			if current != nil {
				stretches = append(stretches, *current)
			}
			current = &stretch{feature: sess.featureName, start: start, end: end}
		} else {
			if end.After(current.end) {
				current.end = end
			}
		}
	}
	if current != nil {
		stretches = append(stretches, *current)
	}

	for _, st := range stretches {
		hours := st.end.Sub(st.start).Hours()
		if hours > 8 {
			hours = 8
		}
		if hours > report.LongestUninterrupted {
			report.LongestUninterrupted = hours
			report.LongestFeature = st.feature
		}
	}
	report.LongestUninterrupted = math.Round(report.LongestUninterrupted*10) / 10

	return report, nil
}

// GetWeeklyReport generates aggregated data for a weekly dev summary.
func (s *Store) GetWeeklyReport(days int) (*WeeklyReportData, error) {
	r := s.db.Reader()
	since := time.Now().UTC().AddDate(0, 0, -days).Format(time.DateTime)

	report := &WeeklyReportData{
		DaysBack:        days,
		CommitsByIntent: make(map[string]int),
	}

	featureNames := scanRows(r,
		`SELECT DISTINCT f.name FROM sessions s JOIN features f ON s.feature_id = f.id WHERE s.started_at >= ? ORDER BY f.name`,
		[]any{since},
		func(rows *sql.Rows) (string, error) {
			var name string
			return name, rows.Scan(&name)
		},
	)
	report.FeaturesTouched = featureNames

	type intentRow struct {
		intent string
		count  int
	}
	intents := scanRows(r,
		`SELECT COALESCE(intent_type, 'unknown'), COUNT(*) FROM commits WHERE committed_at >= ? GROUP BY intent_type ORDER BY COUNT(*) DESC`,
		[]any{since},
		func(rows *sql.Rows) (intentRow, error) {
			var ir intentRow
			return ir, rows.Scan(&ir.intent, &ir.count)
		},
	)
	for _, ir := range intents {
		report.CommitsByIntent[ir.intent] = ir.count
		report.TotalCommits += ir.count
	}

	report.DecisionsMade = countRows(r,
		`SELECT COUNT(*) FROM notes WHERE type = 'decision' AND created_at >= ?`, since)

	report.BlockersAdded = countRows(r,
		`SELECT COUNT(*) FROM notes WHERE type = 'blocker' AND created_at >= ?`, since)

	report.SessionCount = countRows(r,
		`SELECT COUNT(*) FROM sessions WHERE started_at >= ?`, since)

	type sessTime struct {
		startedAt, endedAt string
	}
	sessTimes := scanRows(r,
		`SELECT started_at, COALESCE(ended_at, datetime('now')) FROM sessions WHERE started_at >= ?`,
		[]any{since},
		func(rows *sql.Rows) (sessTime, error) {
			var st sessTime
			return st, rows.Scan(&st.startedAt, &st.endedAt)
		},
	)
	for _, st := range sessTimes {
		start, e1 := time.Parse(time.DateTime, st.startedAt)
		end, e2 := time.Parse(time.DateTime, st.endedAt)
		if e1 == nil && e2 == nil {
			h := end.Sub(start).Hours()
			if h > 8 {
				h = 8
			}
			if h > 0 {
				report.TotalHours += h
			}
		}
	}
	report.TotalHours = math.Round(report.TotalHours*10) / 10

	topDecisions := scanRows(r,
		`SELECT content FROM notes WHERE type = 'decision' AND created_at >= ? ORDER BY created_at DESC LIMIT 5`,
		[]any{since},
		func(rows *sql.Rows) (string, error) {
			var content string
			return content, rows.Scan(&content)
		},
	)
	report.TopDecisions = topDecisions

	return report, nil
}

// FormatFocusTime formats focus time entries as markdown.
func FormatFocusTime(entries []FocusTimeEntry, days int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Focus Time (last %d days)\n\n", days)
	if len(entries) == 0 {
		b.WriteString("No sessions recorded in this period.\n")
		return b.String()
	}
	for _, e := range entries {
		fmt.Fprintf(&b, "- **%s**: %.1fh across %d session(s)\n", e.FeatureName, e.TotalHours, e.SessionCount)
	}
	return b.String()
}

// FormatVelocity formats velocity entries as markdown.
func FormatVelocity(entries []VelocityEntry) string {
	var b strings.Builder
	b.WriteString("# Plan Velocity\n\n")
	if len(entries) == 0 {
		b.WriteString("No features with active plans found.\n")
		return b.String()
	}
	for _, e := range entries {
		status := fmt.Sprintf("%.1f steps/day", e.StepsPerDay)
		if e.Stalled {
			status = fmt.Sprintf("stalled %d day(s)", e.StalledDays)
		}
		progress := fmt.Sprintf("%d/%d steps", e.StepsCompleted, e.TotalSteps)
		extra := ""
		if !e.Stalled && e.EstDaysLeft > 0 && e.StepsCompleted < e.TotalSteps {
			extra = fmt.Sprintf(", done in ~%.0f day(s)", e.EstDaysLeft)
		}
		fmt.Fprintf(&b, "- **%s**: %s (%s%s)\n", e.FeatureName, status, progress, extra)
	}
	return b.String()
}

// FormatInterruptions formats interruption report as markdown.
func FormatInterruptions(report *InterruptionReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Context Switches (last %d days)\n\n", report.DaysAnalyzed)
	fmt.Fprintf(&b, "**%d switch(es)** in %d day(s)\n", report.SwitchCount, report.DaysAnalyzed)
	if report.LongestFeature != "" {
		fmt.Fprintf(&b, "**Longest uninterrupted:** %.1fh on %s\n", report.LongestUninterrupted, report.LongestFeature)
	}
	if len(report.Switches) > 0 {
		b.WriteString("\n**Switches:**\n")
		for _, sw := range report.Switches {
			fmt.Fprintf(&b, "- %s -> %s (%s)\n", sw.From, sw.To, sw.Timestamp)
		}
	}
	return b.String()
}

// FormatWeeklyReport formats the weekly report data as markdown.
func FormatWeeklyReport(report *WeeklyReportData) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Weekly Dev Summary (last %d days)\n\n", report.DaysBack)

	b.WriteString("## Overview\n\n")
	fmt.Fprintf(&b, "- **Sessions:** %d (%.1fh total)\n", report.SessionCount, report.TotalHours)
	fmt.Fprintf(&b, "- **Commits:** %d\n", report.TotalCommits)
	fmt.Fprintf(&b, "- **Decisions:** %d\n", report.DecisionsMade)
	if report.BlockersAdded > 0 {
		fmt.Fprintf(&b, "- **Blockers added:** %d\n", report.BlockersAdded)
	}

	if len(report.FeaturesTouched) > 0 {
		b.WriteString("\n## Features Touched\n\n")
		for _, f := range report.FeaturesTouched {
			fmt.Fprintf(&b, "- %s\n", f)
		}
	}

	if len(report.CommitsByIntent) > 0 {
		b.WriteString("\n## Commits by Type\n\n")
		for intent, count := range report.CommitsByIntent {
			fmt.Fprintf(&b, "- %s: %d\n", intent, count)
		}
	}

	if len(report.TopDecisions) > 0 {
		b.WriteString("\n## Key Decisions\n\n")
		for _, d := range report.TopDecisions {
			content := strings.ReplaceAll(d, "\n", " ")
			if len(content) > 120 {
				content = content[:120] + "..."
			}
			fmt.Fprintf(&b, "- %s\n", content)
		}
	}

	return b.String()
}
