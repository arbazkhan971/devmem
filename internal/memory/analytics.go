package memory

import (
	"database/sql"
	"fmt"
	"math"
	"time"
)

type FeatureAnalytics struct {
	Name                 string
	SessionCount         int
	CommitCount          int
	NoteCount            int
	DecisionCount        int
	BlockerCount         int
	FactCount            int
	ActiveFactCount      int
	InvalidatedFactCount int
	PlanProgress         string
	IntentBreakdown      map[string]int
	DaysSinceCreated     int
	DaysSinceLastActive  int
	AvgSessionDuration   string
}

type ProjectAnalytics struct {
	TotalFeatures      int
	ActiveFeatures     int
	PausedFeatures     int
	DoneFeatures       int
	TotalSessions      int
	TotalCommits       int
	TotalNotes         int
	TotalFacts         int
	MostActiveFeature  string
	MostBlockedFeature string
	RecentActivity     []string
}

func (s *Store) GetFeatureAnalytics(featureID string) (*FeatureAnalytics, error) {
	r := s.db.Reader()
	f, err := scanFeature(r.QueryRow("SELECT "+featureCols+" FROM features WHERE id = ?", featureID))
	if err != nil {
		return nil, fmt.Errorf("feature not found: %w", err)
	}

	a := &FeatureAnalytics{Name: f.Name, IntentBreakdown: make(map[string]int)}
	a.SessionCount = countRows(r, `SELECT COUNT(*) FROM sessions WHERE feature_id = ?`, featureID)
	a.CommitCount = countRows(r, `SELECT COUNT(*) FROM commits WHERE feature_id = ?`, featureID)
	a.NoteCount = countRows(r, `SELECT COUNT(*) FROM notes WHERE feature_id = ?`, featureID)
	a.DecisionCount = countRows(r, `SELECT COUNT(*) FROM notes WHERE feature_id = ? AND type = 'decision'`, featureID)
	a.BlockerCount = countRows(r, `SELECT COUNT(*) FROM notes WHERE feature_id = ? AND type = 'blocker'`, featureID)
	a.FactCount = countRows(r, `SELECT COUNT(*) FROM facts WHERE feature_id = ?`, featureID)
	a.ActiveFactCount = countRows(r, `SELECT COUNT(*) FROM facts WHERE feature_id = ? AND invalid_at IS NULL`, featureID)
	a.InvalidatedFactCount = a.FactCount - a.ActiveFactCount
	a.PlanProgress = s.loadPlanProgress(r, featureID)

	rows, err := r.Query(`SELECT COALESCE(intent_type, 'unknown'), COUNT(*) FROM commits WHERE feature_id = ? GROUP BY intent_type`, featureID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var k string
			var v int
			if rows.Scan(&k, &v) == nil {
				a.IntentBreakdown[k] = v
			}
		}
	}

	now := time.Now().UTC()
	if t, err := time.Parse(time.DateTime, f.CreatedAt); err == nil {
		a.DaysSinceCreated = int(math.Floor(now.Sub(t).Hours() / 24))
	}
	if t, err := time.Parse(time.DateTime, f.LastActive); err == nil {
		a.DaysSinceLastActive = int(math.Floor(now.Sub(t).Hours() / 24))
	}
	a.AvgSessionDuration = s.loadAvgSessionDuration(r, featureID)
	return a, nil
}

func (s *Store) GetProjectAnalytics() (*ProjectAnalytics, error) {
	r := s.db.Reader()
	a := &ProjectAnalytics{
		TotalFeatures:  countRows(r, `SELECT COUNT(*) FROM features`),
		ActiveFeatures: countRows(r, `SELECT COUNT(*) FROM features WHERE status = 'active'`),
		PausedFeatures: countRows(r, `SELECT COUNT(*) FROM features WHERE status = 'paused'`),
		DoneFeatures:   countRows(r, `SELECT COUNT(*) FROM features WHERE status = 'done'`),
		TotalSessions:  countRows(r, `SELECT COUNT(*) FROM sessions`),
		TotalCommits:   countRows(r, `SELECT COUNT(*) FROM commits`),
		TotalNotes:     countRows(r, `SELECT COUNT(*) FROM notes`),
		TotalFacts:     countRows(r, `SELECT COUNT(*) FROM facts`),
	}

	if id := scanNullString(r, `SELECT feature_id FROM sessions GROUP BY feature_id ORDER BY COUNT(*) DESC LIMIT 1`); id != "" {
		r.QueryRow(`SELECT name FROM features WHERE id = ?`, id).Scan(&a.MostActiveFeature)
	}
	if id := scanNullString(r, `SELECT feature_id FROM notes WHERE type = 'blocker' GROUP BY feature_id ORDER BY COUNT(*) DESC LIMIT 1`); id != "" {
		r.QueryRow(`SELECT name FROM features WHERE id = ?`, id).Scan(&a.MostBlockedFeature)
	}
	a.RecentActivity = s.loadRecentActivity(r)
	return a, nil
}

func scanNullString(r *sql.DB, query string) string {
	var ns sql.NullString
	r.QueryRow(query).Scan(&ns)
	if ns.Valid {
		return ns.String
	}
	return ""
}

func (s *Store) loadPlanProgress(r *sql.DB, featureID string) string {
	var planID string
	if r.QueryRow(`SELECT id FROM plans WHERE feature_id = ? AND status = 'active' AND invalid_at IS NULL ORDER BY created_at DESC LIMIT 1`, featureID).Scan(&planID) != nil {
		return "no plan"
	}
	total := countRows(r, `SELECT COUNT(*) FROM plan_steps WHERE plan_id = ?`, planID)
	done := countRows(r, `SELECT COUNT(*) FROM plan_steps WHERE plan_id = ? AND status = 'completed'`, planID)
	if total == 0 {
		return "0/0 (0%)"
	}
	return fmt.Sprintf("%d/%d (%d%%)", done, total, done*100/total)
}

func (s *Store) loadAvgSessionDuration(r *sql.DB, featureID string) string {
	rows, err := r.Query(`SELECT started_at, ended_at FROM sessions WHERE feature_id = ? AND ended_at IS NOT NULL`, featureID)
	if err != nil {
		return "n/a"
	}
	defer rows.Close()
	var total time.Duration
	var n int
	for rows.Next() {
		var startStr, endStr string
		if rows.Scan(&startStr, &endStr) != nil {
			continue
		}
		start, e1 := time.Parse(time.DateTime, startStr)
		end, e2 := time.Parse(time.DateTime, endStr)
		if e1 != nil || e2 != nil {
			continue
		}
		total += end.Sub(start)
		n++
	}
	if n == 0 {
		return "n/a"
	}
	return formatDuration(total / time.Duration(n))
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	h, m := int(d.Hours()), int(d.Minutes())%60
	if h > 0 {
		return fmt.Sprintf("%dh%dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

func (s *Store) loadRecentActivity(r *sql.DB) []string {
	rows, err := r.Query(`
		SELECT event_type, content, feature_name, created_at FROM (
			SELECT 'note' AS event_type, n.content, f.name AS feature_name, n.created_at
			FROM notes n JOIN features f ON n.feature_id = f.id
			UNION ALL
			SELECT 'commit', c.message, f.name, c.committed_at
			FROM commits c JOIN features f ON c.feature_id = f.id
		) events ORDER BY created_at DESC LIMIT 5`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var typ, content, feat, at string
		if rows.Scan(&typ, &content, &feat, &at) != nil {
			continue
		}
		if len(content) > 80 {
			content = content[:80] + "..."
		}
		out = append(out, fmt.Sprintf("[%s] %s — %s (%s)", typ, content, feat, at))
	}
	return out
}
