package memory

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// ReviewContext holds enriched context for a file in a code review.
type ReviewContext struct {
	File    string
	Notes   []Note
	Facts   []Fact
	Commits []CommitInfo
	Summary string
}

// ReviewRisk holds risk assessment for a file.
type ReviewRisk struct {
	File           string
	ChangeCount    int
	RecentChanges  int
	BlockerCount   int
	RecentRefactor bool
	RiskLevel      string
	Reasons        []string
}

// ChecklistItem represents an item in an auto-generated review checklist.
type ChecklistItem struct {
	Text   string
	Source string
}


func fileBaseName(path string) string {
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		return path[idx+1:]
	}
	return path
}

func truncReview(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// GetReviewContext enriches a list of file paths with related memory.
func (s *Store) GetReviewContext(files []string) ([]ReviewContext, error) {
	r := s.db.Reader()
	var results []ReviewContext
	for _, file := range files {
		rc := ReviewContext{File: file}
		base := fileBaseName(file)
		rc.Commits = scanRows(r,
			`SELECT hash, message, author, committed_at FROM commits WHERE message LIKE ? ORDER BY committed_at DESC LIMIT 10`,
			[]any{"%" + base + "%"},
			func(rows *sql.Rows) (CommitInfo, error) {
				var c CommitInfo
				return c, rows.Scan(&c.Hash, &c.Message, &c.Author, &c.CommittedAt)
			},
		)
		notes := scanRows(r,
			`SELECT `+noteCols+` FROM notes WHERE content LIKE ? OR content LIKE ? ORDER BY created_at DESC LIMIT 5`,
			[]any{"%" + file + "%", "%" + base + "%"},
			func(rows *sql.Rows) (Note, error) { return scanNote(rows) },
		)
		rc.Notes = notes
		facts := scanRows(r,
			`SELECT `+factColumns+` FROM facts WHERE invalid_at IS NULL AND (subject LIKE ? OR object LIKE ?) ORDER BY recorded_at DESC LIMIT 5`,
			[]any{"%" + base + "%", "%" + base + "%"},
			func(rows *sql.Rows) (Fact, error) { return scanFact(rows) },
		)
		rc.Facts = facts
		var parts []string
		for _, n := range notes {
			if n.Type == "decision" {
				parts = append(parts, fmt.Sprintf("Decision: %s", truncReview(n.Content, 100)))
			}
		}
		for _, f := range facts {
			parts = append(parts, fmt.Sprintf("Fact: %s %s %s", f.Subject, f.Predicate, f.Object))
		}
		if len(rc.Commits) > 0 {
			parts = append(parts, fmt.Sprintf("Last commit: %s", rc.Commits[0].Message))
		}
		if len(parts) > 0 {
			rc.Summary = strings.Join(parts, ". ")
		} else {
			rc.Summary = "No related memories found."
		}
		results = append(results, rc)
	}
	return results, nil
}

// GetReviewRisk assesses risk for each file based on memory.
func (s *Store) GetReviewRisk(files []string) ([]ReviewRisk, error) {
	r := s.db.Reader()
	thirtyDaysAgo := time.Now().UTC().AddDate(0, 0, -30).Format(time.DateTime)
	var out []ReviewRisk
	for _, file := range files {
		base := fileBaseName(file)
		risk := ReviewRisk{File: file, RiskLevel: "LOW"}
		r.QueryRow(`SELECT COUNT(*) FROM commits WHERE message LIKE ?`, "%"+base+"%").Scan(&risk.ChangeCount)
		r.QueryRow(`SELECT COUNT(*) FROM commits WHERE message LIKE ? AND committed_at >= ?`, "%"+base+"%", thirtyDaysAgo).Scan(&risk.RecentChanges)
		r.QueryRow(`SELECT COUNT(*) FROM notes WHERE type = 'blocker' AND content LIKE ?`, "%"+base+"%").Scan(&risk.BlockerCount)
		if risk.ChangeCount > 10 {
			risk.Reasons = append(risk.Reasons, "frequently changed file")
		}
		if risk.BlockerCount > 0 {
			risk.Reasons = append(risk.Reasons, fmt.Sprintf("%d blockers related", risk.BlockerCount))
		}
		if risk.RecentChanges > 5 || risk.BlockerCount > 0 {
			risk.RiskLevel = "MEDIUM"
		}
		if risk.RecentChanges > 10 && risk.BlockerCount > 0 {
			risk.RiskLevel = "HIGH"
		}
		out = append(out, risk)
	}
	return out, nil
}

// GenerateReviewChecklist generates review checklist items from memory.
func (s *Store) GenerateReviewChecklist(featureID string) ([]ChecklistItem, error) {
	r := s.db.Reader()
	var items []ChecklistItem

	decisions := scanRows(r,
		`SELECT `+noteCols+` FROM notes WHERE feature_id = ? AND type = 'decision' ORDER BY created_at DESC LIMIT 20`,
		[]any{featureID},
		func(rows *sql.Rows) (Note, error) { return scanNote(rows) },
	)
	for i, d := range decisions {
		items = append(items, ChecklistItem{
			Text:   fmt.Sprintf("Verify decision #%d: %s", i+1, truncReview(d.Content, 100)),
			Source: "decision",
		})
	}

	blockers := scanRows(r,
		`SELECT `+noteCols+` FROM notes WHERE feature_id = ? AND type = 'blocker' ORDER BY created_at DESC LIMIT 10`,
		[]any{featureID},
		func(rows *sql.Rows) (Note, error) { return scanNote(rows) },
	)
	for _, bl := range blockers {
		items = append(items, ChecklistItem{
			Text:   fmt.Sprintf("Check blocker: %s", truncReview(bl.Content, 100)),
			Source: "blocker",
		})
	}

	facts := scanRows(r,
		`SELECT `+factColumns+` FROM facts WHERE feature_id = ? AND invalid_at IS NULL ORDER BY recorded_at DESC LIMIT 10`,
		[]any{featureID},
		func(rows *sql.Rows) (Fact, error) { return scanFact(rows) },
	)
	for _, f := range facts {
		items = append(items, ChecklistItem{
			Text:   fmt.Sprintf("Verify: %s %s %s", f.Subject, f.Predicate, f.Object),
			Source: "fact",
		})
	}

	var planID string
	if r.QueryRow(`SELECT id FROM plans WHERE feature_id = ? AND status = 'active' AND invalid_at IS NULL ORDER BY created_at DESC LIMIT 1`, featureID).Scan(&planID) == nil {
		type step struct{ title, status string }
		pending := scanRows(r,
			`SELECT title, status FROM plan_steps WHERE plan_id = ? AND status != 'completed' ORDER BY step_number LIMIT 5`,
			[]any{planID},
			func(rows *sql.Rows) (step, error) {
				var s step
				return s, rows.Scan(&s.title, &s.status)
			},
		)
		for _, st := range pending {
			items = append(items, ChecklistItem{
				Text:   fmt.Sprintf("Plan step (%s): %s", st.status, st.title),
				Source: "plan",
			})
		}
	}

	return items, nil
}

// FormatReviewContext formats review context results as markdown.
func FormatReviewContext(contexts []ReviewContext) string {
	var b strings.Builder
	b.WriteString("# Code Review Context\n\n")
	for _, rc := range contexts {
		fmt.Fprintf(&b, "## %s\n\n", rc.File)
		fmt.Fprintf(&b, "%s\n\n", rc.Summary)
		if len(rc.Notes) > 0 {
			b.WriteString("**Related Notes:**\n")
			for _, n := range rc.Notes {
				fmt.Fprintf(&b, "- [%s] %s\n", n.Type, truncReview(n.Content, 120))
			}
			b.WriteString("\n")
		}
		if len(rc.Facts) > 0 {
			b.WriteString("**Related Facts:**\n")
			for _, f := range rc.Facts {
				fmt.Fprintf(&b, "- %s %s %s\n", f.Subject, f.Predicate, f.Object)
			}
			b.WriteString("\n")
		}
		if len(rc.Commits) > 0 {
			b.WriteString("**Related Commits:**\n")
			for _, c := range rc.Commits {
				hash := c.Hash
				if len(hash) > 7 {
					hash = hash[:7]
				}
				fmt.Fprintf(&b, "- `%s` %s\n", hash, c.Message)
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}

// FormatReviewRisk formats risk assessment results as markdown.
func FormatReviewRisk(risks []ReviewRisk) string {
	var b strings.Builder
	b.WriteString("# Risk Assessment\n\n")
	for _, rr := range risks {
		reasons := "no risk factors"
		if len(rr.Reasons) > 0 {
			reasons = strings.Join(rr.Reasons, ", ")
		}
		fmt.Fprintf(&b, "- **%s**: %s (%s)\n", rr.File, rr.RiskLevel, reasons)
	}
	return b.String()
}

// FormatReviewChecklist formats a review checklist as markdown.
func FormatReviewChecklist(items []ChecklistItem, featureName string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Review Checklist: %s\n\n", featureName)
	if len(items) == 0 {
		b.WriteString("No checklist items generated. Add decisions, facts, or blockers first.\n")
		return b.String()
	}
	for _, item := range items {
		fmt.Fprintf(&b, "- [ ] %s\n", item.Text)
	}
	return b.String()
}
