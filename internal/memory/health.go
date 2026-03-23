package memory

import (
	"fmt"
	"math"
)

type MemoryHealth struct {
	Score           float64
	TotalMemories   int
	ActiveFacts     int
	StaleFactCount  int
	ConflictCount   int
	OrphanNoteCount int
	StaleNoteCount  int
	SummaryCount    int
	Suggestions     []string
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

	noLinks := ` AND NOT EXISTS (SELECT 1 FROM memory_links ml WHERE ml.source_id = n.id AND ml.source_type = 'note')
		AND NOT EXISTS (SELECT 1 FROM memory_links ml WHERE ml.target_id = n.id AND ml.target_type = 'note')`
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
		h.Suggestions = append(h.Suggestions, fmt.Sprintf("%d facts haven't been referenced in 30+ days. Review with devmem_search.", h.StaleFactCount))
	}
	if h.OrphanNoteCount > 10 {
		h.Suggestions = append(h.Suggestions, fmt.Sprintf("%d notes have no connections. Consider consolidation.", h.OrphanNoteCount))
	}
	if h.SummaryCount == 0 && h.TotalMemories > 20 {
		h.Suggestions = append(h.Suggestions, "No summaries generated. Memory may be fragmented.")
	}
	return h, nil
}

func featureFilter(featureID string) (string, []any) {
	if featureID != "" {
		return " AND feature_id = ?", []any{featureID}
	}
	return "", nil
}

func (s *Store) ForgetStaleFacts(featureID string) (int, error) {
	q := `DELETE FROM facts WHERE invalid_at IS NOT NULL AND invalid_at < datetime('now', '-30 days')`
	ff, args := featureFilter(featureID)
	result, err := s.db.Writer().Exec(q+ff, args...)
	if err != nil {
		return 0, fmt.Errorf("forget stale facts: %w", err)
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

func (s *Store) ForgetStaleNotes(featureID string) (int, error) {
	ff, args := featureFilter(featureID)
	q := `DELETE FROM notes WHERE id IN (
		SELECT n.id FROM notes n
		WHERE n.created_at < datetime('now', '-60 days')` + ff + `
		AND NOT EXISTS (SELECT 1 FROM memory_links ml WHERE ml.source_id = n.id AND ml.source_type = 'note')
		AND NOT EXISTS (SELECT 1 FROM memory_links ml WHERE ml.target_id = n.id AND ml.target_type = 'note'))`
	result, err := s.db.Writer().Exec(q, args...)
	if err != nil {
		return 0, fmt.Errorf("forget stale notes: %w", err)
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

type deleteOp struct {
	query string
	args  func(fid string) []any
}

var featureDeleteOps = []deleteOp{
	{`DELETE FROM memory_links WHERE source_id IN (SELECT id FROM notes WHERE feature_id = ?) OR target_id IN (SELECT id FROM notes WHERE feature_id = ?)`, func(fid string) []any { return []any{fid, fid} }},
	{`DELETE FROM memory_links WHERE source_id IN (SELECT id FROM facts WHERE feature_id = ?) OR target_id IN (SELECT id FROM facts WHERE feature_id = ?)`, func(fid string) []any { return []any{fid, fid} }},
	{`DELETE FROM summaries WHERE scope = ?`, func(fid string) []any { return []any{"feature:" + fid} }},
	{`DELETE FROM plan_steps WHERE plan_id IN (SELECT id FROM plans WHERE feature_id = ?)`, func(fid string) []any { return []any{fid} }},
	{`DELETE FROM plans WHERE feature_id = ?`, func(fid string) []any { return []any{fid} }},
	{`DELETE FROM semantic_changes WHERE session_id IN (SELECT id FROM sessions WHERE feature_id = ?)`, func(fid string) []any { return []any{fid} }},
	{`DELETE FROM commits WHERE feature_id = ?`, func(fid string) []any { return []any{fid} }},
	{`DELETE FROM notes WHERE feature_id = ?`, func(fid string) []any { return []any{fid} }},
	{`DELETE FROM facts WHERE feature_id = ?`, func(fid string) []any { return []any{fid} }},
	{`DELETE FROM sessions WHERE feature_id = ?`, func(fid string) []any { return []any{fid} }},
	{`DELETE FROM features WHERE id = ?`, func(fid string) []any { return []any{fid} }},
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
		for _, op := range featureDeleteOps {
			if _, err := w.Exec(op.query, op.args(fid)...); err != nil {
				return i, fmt.Errorf("delete data for feature %s: %w", fid, err)
			}
		}
	}
	return len(ids), nil
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
