package memory

import (
	"database/sql"
	"fmt"
	"math"
	"time"
)

type FeatureAnalytics struct {
	Name                                             string
	SessionCount, CommitCount, NoteCount             int
	DecisionCount, BlockerCount                      int
	FactCount, ActiveFactCount, InvalidatedFactCount int
	PlanProgress                                     string
	IntentBreakdown                                  map[string]int
	DaysSinceCreated, DaysSinceLastActive            int
	AvgSessionDuration                               string
}
type ProjectAnalytics struct {
	TotalFeatures, ActiveFeatures, PausedFeatures, DoneFeatures int
	TotalSessions, TotalCommits, TotalNotes, TotalFacts         int
	MostActiveFeature, MostBlockedFeature                       string
	RecentActivity                                              []string
}
type MemoryHealth struct {
	Score                                                     float64
	TotalMemories, ActiveFacts, StaleFactCount, ConflictCount int
	OrphanNoteCount, StaleNoteCount, SummaryCount             int
	Suggestions                                               []string
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
	if rows, err := r.Query(`SELECT COALESCE(intent_type, 'unknown'), COUNT(*) FROM commits WHERE feature_id = ? GROUP BY intent_type`, featureID); err == nil {
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
	if h, m := int(d.Hours()), int(d.Minutes())%60; h > 0 {
		return fmt.Sprintf("%dh%dm", h, m)
	}
	return fmt.Sprintf("%dm", int(d.Minutes()))
}

func (s *Store) loadRecentActivity(r *sql.DB) []string {
	rows, err := r.Query(`SELECT event_type, content, feature_name, created_at FROM (SELECT 'note' AS event_type, n.content, f.name AS feature_name, n.created_at FROM notes n JOIN features f ON n.feature_id = f.id UNION ALL SELECT 'commit', c.message, f.name, c.committed_at FROM commits c JOIN features f ON c.feature_id = f.id) events ORDER BY created_at DESC LIMIT 5`)
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

func featureFilter(featureID string) (string, []any) {
	if featureID != "" {
		return " AND feature_id = ?", []any{featureID}
	}
	return "", nil
}

func (s *Store) GetMemoryHealth(featureID string) (*MemoryHealth, error) {
	h := &MemoryHealth{}
	r := s.db.Reader()
	ff, args := featureFilter(featureID)
	var totalNotes int
	if err := r.QueryRow(`SELECT COUNT(*) FROM notes WHERE 1=1`+ff, args...).Scan(&totalNotes); err != nil {
		return nil, fmt.Errorf("count notes: %w", err)
	}
	if err := r.QueryRow(`SELECT COUNT(*) FROM facts WHERE invalid_at IS NULL`+ff, args...).Scan(&h.ActiveFacts); err != nil {
		return nil, fmt.Errorf("count active facts: %w", err)
	}
	h.TotalMemories = totalNotes + h.ActiveFacts
	if err := r.QueryRow(`SELECT COUNT(*) FROM facts WHERE invalid_at IS NOT NULL AND invalid_at < datetime('now', '-30 days')`+ff, args...).Scan(&h.StaleFactCount); err != nil {
		return nil, fmt.Errorf("count stale facts: %w", err)
	}
	if err := r.QueryRow(`SELECT COUNT(*) FROM (SELECT subject, predicate FROM facts WHERE invalid_at IS NULL`+ff+` GROUP BY subject, predicate HAVING COUNT(*) > 1)`, args...).Scan(&h.ConflictCount); err != nil {
		return nil, fmt.Errorf("count conflicts: %w", err)
	}
	noLinks := ` AND NOT EXISTS (SELECT 1 FROM memory_links ml WHERE ml.source_id = n.id AND ml.source_type = 'note') AND NOT EXISTS (SELECT 1 FROM memory_links ml WHERE ml.target_id = n.id AND ml.target_type = 'note')`
	if err := r.QueryRow(`SELECT COUNT(*) FROM notes n WHERE 1=1`+ff+noLinks, args...).Scan(&h.OrphanNoteCount); err != nil {
		return nil, fmt.Errorf("count orphan notes: %w", err)
	}
	if err := r.QueryRow(`SELECT COUNT(*) FROM notes n WHERE n.created_at < datetime('now', '-30 days')`+ff+noLinks, args...).Scan(&h.StaleNoteCount); err != nil {
		return nil, fmt.Errorf("count stale notes: %w", err)
	}
	summaryQ, summaryArgs := `SELECT COUNT(*) FROM summaries`, []any(nil)
	if featureID != "" {
		summaryQ += ` WHERE scope = ?`
		summaryArgs = append(summaryArgs, "feature:"+featureID)
	}
	if err := r.QueryRow(summaryQ, summaryArgs...).Scan(&h.SummaryCount); err != nil {
		return nil, fmt.Errorf("count summaries: %w", err)
	}
	score := 100.0 - float64(h.ConflictCount)*10 - math.Min(float64(h.StaleFactCount), 5)*5 - math.Min(float64(h.OrphanNoteCount), 10)*2
	if h.SummaryCount == 0 && h.TotalMemories > 20 {
		score -= 10
	}
	h.Score = math.Max(0, math.Min(100, score))
	if h.ConflictCount > 0 {
		h.Suggestions = append(h.Suggestions, fmt.Sprintf("You have %d contradicting facts. Run consolidation.", h.ConflictCount))
	}
	if h.StaleFactCount > 5 {
		h.Suggestions = append(h.Suggestions, fmt.Sprintf("%d facts haven't been referenced in 30+ days. Review with memorx_search.", h.StaleFactCount))
	}
	if h.OrphanNoteCount > 10 {
		h.Suggestions = append(h.Suggestions, fmt.Sprintf("%d notes have no connections. Consider consolidation.", h.OrphanNoteCount))
	}
	if h.SummaryCount == 0 && h.TotalMemories > 20 {
		h.Suggestions = append(h.Suggestions, "No summaries generated. Memory may be fragmented.")
	}
	return h, nil
}

func (s *Store) ForgetStaleFacts(featureID string) (int, error) {
	ff, args := featureFilter(featureID)
	result, err := s.db.Writer().Exec(`DELETE FROM facts WHERE invalid_at IS NOT NULL AND invalid_at < datetime('now', '-30 days')`+ff, args...)
	if err != nil {
		return 0, fmt.Errorf("forget stale facts: %w", err)
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

func (s *Store) ForgetStaleNotes(featureID string) (int, error) {
	ff, args := featureFilter(featureID)
	result, err := s.db.Writer().Exec(`DELETE FROM notes WHERE id IN (SELECT n.id FROM notes n WHERE n.created_at < datetime('now', '-60 days')`+ff+` AND NOT EXISTS (SELECT 1 FROM memory_links ml WHERE ml.source_id = n.id AND ml.source_type = 'note') AND NOT EXISTS (SELECT 1 FROM memory_links ml WHERE ml.target_id = n.id AND ml.target_type = 'note'))`, args...)
	if err != nil {
		return 0, fmt.Errorf("forget stale notes: %w", err)
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

func (s *Store) ForgetCompletedFeatures() (int, error) {
	rows, err := s.db.Reader().Query(`SELECT id FROM features WHERE status = 'done' AND last_active < datetime('now', '-90 days')`)
	if err != nil {
		return 0, fmt.Errorf("query completed features: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return 0, fmt.Errorf("scan feature id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate features: %w", err)
	}
	w := s.db.Writer()
	for i, fid := range ids {
		if err := s.deleteFeatureData(w, fid); err != nil {
			return i, err
		}
	}
	return len(ids), nil
}

func (s *Store) deleteFeatureData(w *sql.DB, fid string) error {
	noteIDs := `SELECT id FROM notes WHERE feature_id = ?`
	factIDs := `SELECT id FROM facts WHERE feature_id = ?`
	for _, q := range []struct{ query string; args []any }{
		{`DELETE FROM memory_links WHERE source_id IN (` + noteIDs + `) OR target_id IN (` + noteIDs + `)`, []any{fid, fid}},
		{`DELETE FROM memory_links WHERE source_id IN (` + factIDs + `) OR target_id IN (` + factIDs + `)`, []any{fid, fid}},
		{`DELETE FROM summaries WHERE scope = 'feature:' || ?`, []any{fid}},
		{`DELETE FROM plan_steps WHERE plan_id IN (SELECT id FROM plans WHERE feature_id = ?)`, []any{fid}},
		{`DELETE FROM plans WHERE feature_id = ?`, []any{fid}},
		{`DELETE FROM semantic_changes WHERE session_id IN (SELECT id FROM sessions WHERE feature_id = ?)`, []any{fid}},
		{`DELETE FROM commits WHERE feature_id = ?`, []any{fid}},
		{`DELETE FROM notes WHERE feature_id = ?`, []any{fid}},
		{`DELETE FROM facts WHERE feature_id = ?`, []any{fid}},
		{`DELETE FROM sessions WHERE feature_id = ?`, []any{fid}},
		{`DELETE FROM features WHERE id = ?`, []any{fid}},
	} {
		if _, err := w.Exec(q.query, q.args...); err != nil {
			return fmt.Errorf("delete data for feature %s: %w", fid, err)
		}
	}
	return nil
}

func (s *Store) ForgetByID(id string) (string, error) {
	w := s.db.Writer()
	for _, typ := range []string{"note", "fact"} {
		result, err := w.Exec(fmt.Sprintf(`DELETE FROM %ss WHERE id = ?`, typ), id)
		if err != nil {
			return "", fmt.Errorf("delete %s: %w", typ, err)
		}
		if n, _ := result.RowsAffected(); n > 0 {
			w.Exec(`DELETE FROM memory_links WHERE (source_id = ? AND source_type = ?) OR (target_id = ? AND target_type = ?)`, id, typ, id, typ) //nolint:errcheck
			return typ, nil
		}
	}
	return "", fmt.Errorf("no note or fact found with ID %q", id)
}
