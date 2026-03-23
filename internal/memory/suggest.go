package memory

import (
	"database/sql"
	"fmt"
	"math"
	"strings"
	"time"
)

// Suggestion is a single actionable suggestion based on memory patterns.
type Suggestion struct {
	Category string // e.g. "blocker", "inactive", "plan", "health", "session", "conflict"
	Message  string
}

// GetSuggestions analyzes memory patterns and returns actionable suggestions.
func (s *Store) GetSuggestions() ([]Suggestion, error) {
	r := s.db.Reader()
	var suggestions []Suggestion

	// 1. Unresolved blockers > 3 days old
	if ss, err := s.suggestStaleBlockers(r); err == nil {
		suggestions = append(suggestions, ss...)
	}

	// 2. Features not touched in 14+ days with pending work
	if ss, err := s.suggestInactiveFeatures(r); err == nil {
		suggestions = append(suggestions, ss...)
	}

	// 3. Plans at 80%+ completion
	if ss, err := s.suggestNearCompletePlans(r); err == nil {
		suggestions = append(suggestions, ss...)
	}

	// 4. Memory health < 70
	if ss, err := s.suggestHealthIssues(r); err == nil {
		suggestions = append(suggestions, ss...)
	}

	// 5. Last session had no summary
	if ss, err := s.suggestMissingSummary(r); err == nil {
		suggestions = append(suggestions, ss...)
	}

	// 6. Fact contradictions
	if ss, err := s.suggestConflicts(r); err == nil {
		suggestions = append(suggestions, ss...)
	}

	return suggestions, nil
}

func (s *Store) suggestStaleBlockers(r *sql.DB) ([]Suggestion, error) {
	rows, err := r.Query(`
		SELECT f.name, COUNT(*) as cnt, MIN(n.created_at) as oldest
		FROM notes n
		JOIN features f ON n.feature_id = f.id
		WHERE n.type = 'blocker'
		  AND n.created_at < datetime('now', '-3 days')
		  AND f.status != 'done'
		GROUP BY f.name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Suggestion
	now := time.Now().UTC()
	for rows.Next() {
		var name string
		var cnt int
		var oldest string
		if rows.Scan(&name, &cnt, &oldest) != nil {
			continue
		}
		days := 3
		if t, err := time.Parse(time.DateTime, oldest); err == nil {
			days = int(math.Floor(now.Sub(t).Hours() / 24))
		}
		out = append(out, Suggestion{
			Category: "blocker",
			Message:  fmt.Sprintf("%d blocker(s) on %s unresolved for %d+ days", cnt, name, days),
		})
	}
	return out, rows.Err()
}

func (s *Store) suggestInactiveFeatures(r *sql.DB) ([]Suggestion, error) {
	rows, err := r.Query(`
		SELECT f.name, f.last_active,
			(SELECT COUNT(*) FROM notes n WHERE n.feature_id = f.id AND n.type = 'blocker') as blockers
		FROM features f
		WHERE f.status = 'active'
		  AND f.last_active < datetime('now', '-14 days')
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Suggestion
	now := time.Now().UTC()
	for rows.Next() {
		var name, lastActive string
		var blockers int
		if rows.Scan(&name, &lastActive, &blockers) != nil {
			continue
		}
		days := 14
		if t, err := time.Parse(time.DateTime, lastActive); err == nil {
			days = int(math.Floor(now.Sub(t).Hours() / 24))
		}
		msg := fmt.Sprintf("%s inactive for %d days", name, days)
		if blockers > 0 {
			msg += fmt.Sprintf(", %d blocker(s) pending", blockers)
		}
		out = append(out, Suggestion{
			Category: "inactive",
			Message:  msg,
		})
	}
	return out, rows.Err()
}

func (s *Store) suggestNearCompletePlans(r *sql.DB) ([]Suggestion, error) {
	rows, err := r.Query(`
		SELECT f.name, p.title,
			(SELECT COUNT(*) FROM plan_steps ps WHERE ps.plan_id = p.id) as total,
			(SELECT COUNT(*) FROM plan_steps ps WHERE ps.plan_id = p.id AND ps.status = 'completed') as done
		FROM plans p
		JOIN features f ON p.feature_id = f.id
		WHERE p.status = 'active' AND p.invalid_at IS NULL
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Suggestion
	for rows.Next() {
		var name, title string
		var total, done int
		if rows.Scan(&name, &title, &total, &done) != nil {
			continue
		}
		if total == 0 {
			continue
		}
		pct := done * 100 / total
		remaining := total - done
		if pct >= 80 {
			out = append(out, Suggestion{
				Category: "plan",
				Message:  fmt.Sprintf("%s plan %d/%d done — %d step(s) remaining!", name, done, total, remaining),
			})
		}
	}
	return out, rows.Err()
}

func (s *Store) suggestHealthIssues(r *sql.DB) ([]Suggestion, error) {
	h, err := s.GetMemoryHealth("")
	if err != nil {
		return nil, err
	}
	if h.Score < 70 {
		return []Suggestion{{
			Category: "health",
			Message:  fmt.Sprintf("Memory health is %.0f. Run memorx_forget to clean up.", h.Score),
		}}, nil
	}
	return nil, nil
}

func (s *Store) suggestMissingSummary(r *sql.DB) ([]Suggestion, error) {
	var sessionID, endedAt string
	var summary sql.NullString
	err := r.QueryRow(`
		SELECT id, COALESCE(ended_at, ''), summary
		FROM sessions
		WHERE ended_at IS NOT NULL
		ORDER BY ended_at DESC LIMIT 1
	`).Scan(&sessionID, &endedAt, &summary)
	if err != nil {
		return nil, nil // no ended sessions
	}
	if !summary.Valid || strings.TrimSpace(summary.String) == "" {
		return []Suggestion{{
			Category: "session",
			Message:  "Last session had no summary. Context may be lost.",
		}}, nil
	}
	return nil, nil
}

func (s *Store) suggestConflicts(r *sql.DB) ([]Suggestion, error) {
	var count int
	err := r.QueryRow(`
		SELECT COUNT(*) FROM (
			SELECT subject, predicate
			FROM facts
			WHERE invalid_at IS NULL
			GROUP BY subject, predicate
			HAVING COUNT(*) > 1
		)
	`).Scan(&count)
	if err != nil {
		return nil, err
	}
	if count > 0 {
		return []Suggestion{{
			Category: "conflict",
			Message:  fmt.Sprintf("%d contradicting fact(s) found. Run consolidation.", count),
		}}, nil
	}
	return nil, nil
}

// FormatSuggestions renders a list of suggestions as a bullet list string.
func FormatSuggestions(suggestions []Suggestion) string {
	if len(suggestions) == 0 {
		return "No suggestions. Everything looks good!"
	}
	var b strings.Builder
	for _, s := range suggestions {
		fmt.Fprintf(&b, "- %s\n", s.Message)
	}
	return b.String()
}
