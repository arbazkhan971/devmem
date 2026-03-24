package memory

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

// DuplicateGroup holds a group of near-duplicate notes.
type DuplicateGroup struct {
	NoteIDs    []string
	Previews   []string
	FeatureID  string
	Similarity float64
}

// DeduplicateResult holds the result of a deduplication run.
type DeduplicateResult struct {
	Groups      []DuplicateGroup
	TotalDups   int
	MergedCount int
	DryRun      bool
}

// IntegrityResult holds the result of an integrity check.
type IntegrityResult struct {
	BrokenLinks     int
	OrphanSessions  int
	OrphanNotes     int
	OrphanFacts     int
	TotalChecked    int
	FixedCount      int
	ScorePercent    int
	Details         []string
}

// AutoLinkCodeResult holds the result of auto-linking code files.
type AutoLinkCodeResult struct {
	LinkedNotes int
	LinkedFiles int
	NewLinks    int
}

// filePathPattern matches file paths like *.go, *.ts, *.py, etc.
var filePathPattern = regexp.MustCompile(`(?:^|[\s(,"'])([a-zA-Z0-9_./-]+\.(?:go|ts|tsx|js|jsx|py|rs|rb|java|cpp|c|h|css|scss|html|yaml|yml|toml|json|sql|sh|md|proto))(?:[\s),"']|$)`)

// Deduplicate finds and optionally merges duplicate/near-duplicate notes.
func (s *Store) Deduplicate(featureName string, dryRun bool) (*DeduplicateResult, error) {
	r := s.db.Reader()

	result := &DeduplicateResult{DryRun: dryRun}

	q := `SELECT n1.id, n1.content, n1.feature_id, n2.id, n2.content
	      FROM notes n1
	      JOIN notes n2 ON n1.feature_id = n2.feature_id AND n1.id < n2.id AND n1.type = n2.type`
	args := []any{}

	if featureName != "" {
		f, err := s.GetFeature(featureName)
		if err != nil {
			return nil, fmt.Errorf("feature %q not found", featureName)
		}
		q += ` WHERE n1.feature_id = ?`
		args = append(args, f.ID)
	}
	q += ` ORDER BY n1.feature_id, n1.created_at`

	rows, err := r.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("query duplicates: %w", err)
	}
	defer rows.Close()

	seen := map[string]bool{}
	for rows.Next() {
		var id1, content1, featureID, id2, content2 string
		if rows.Scan(&id1, &content1, &featureID, &id2, &content2) != nil {
			continue
		}

		overlap := wordOverlap(content1, content2)
		if overlap < 0.8 {
			continue
		}

		key := id1 + ":" + id2
		if seen[key] {
			continue
		}
		seen[key] = true

		result.Groups = append(result.Groups, DuplicateGroup{
			NoteIDs:    []string{id1, id2},
			Previews:   []string{truncateContent(content1, 80), truncateContent(content2, 80)},
			FeatureID:  featureID,
			Similarity: overlap,
		})
		result.TotalDups++
	}

	// If not dry run, delete the second note in each pair
	if !dryRun && len(result.Groups) > 0 {
		w := s.db.Writer()
		for _, g := range result.Groups {
			if len(g.NoteIDs) > 1 {
				if _, err := w.Exec(`DELETE FROM notes WHERE id = ?`, g.NoteIDs[1]); err == nil {
					result.MergedCount++
				}
			}
		}
	}

	return result, nil
}

// wordOverlap calculates the percentage of words shared between two strings.
func wordOverlap(a, b string) float64 {
	wordsA := strings.Fields(strings.ToLower(a))
	wordsB := strings.Fields(strings.ToLower(b))
	if len(wordsA) == 0 || len(wordsB) == 0 {
		return 0
	}

	setB := make(map[string]bool, len(wordsB))
	for _, w := range wordsB {
		setB[w] = true
	}

	overlap := 0
	for _, w := range wordsA {
		if setB[w] {
			overlap++
		}
	}

	// Use the smaller set as denominator
	denom := len(wordsA)
	if len(wordsB) < denom {
		denom = len(wordsB)
	}
	return float64(overlap) / float64(denom)
}

// truncateContent truncates and flattens a string for display.
func truncateContent(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// FormatDeduplicate formats deduplication results as markdown.
func FormatDeduplicate(d *DeduplicateResult) string {
	var b strings.Builder
	mode := "DRY RUN"
	if !d.DryRun {
		mode = "APPLIED"
	}
	fmt.Fprintf(&b, "# Deduplicate [%s]\n\n", mode)
	if d.TotalDups == 0 {
		b.WriteString("No duplicates found.\n")
		return b.String()
	}
	fmt.Fprintf(&b, "**%d duplicate group(s) found**", d.TotalDups)
	if !d.DryRun {
		fmt.Fprintf(&b, ", **%d merged**", d.MergedCount)
	}
	b.WriteString("\n\n")
	for i, g := range d.Groups {
		fmt.Fprintf(&b, "%d. (%.0f%% overlap) %s...%s -> %s...%s\n",
			i+1, g.Similarity*100, g.NoteIDs[0][:8], "", g.NoteIDs[1][:8], "")
		fmt.Fprintf(&b, "   A: %q\n   B: %q\n", g.Previews[0], g.Previews[1])
	}
	return b.String()
}

// IntegrityCheck verifies all memory links, facts, and references are valid.
func (s *Store) IntegrityCheck(fix bool) (*IntegrityResult, error) {
	r := s.db.Reader()
	result := &IntegrityResult{}

	// Check broken links (source or target doesn't exist)
	type linkRow struct {
		id, sourceID, sourceType, targetID, targetType string
	}
	links := scanRows(r,
		`SELECT id, source_id, source_type, target_id, target_type FROM memory_links`,
		nil,
		func(rows *sql.Rows) (linkRow, error) {
			var l linkRow
			return l, rows.Scan(&l.id, &l.sourceID, &l.sourceType, &l.targetID, &l.targetType)
		},
	)
	result.TotalChecked += len(links)

	tableForType := map[string]string{
		"note": "notes", "fact": "facts", "commit": "commits",
		"plan": "plans", "plan_step": "plan_steps", "semantic_change": "semantic_changes",
	}

	brokenLinkIDs := []string{}
	for _, l := range links {
		sourceTable := tableForType[l.sourceType]
		targetTable := tableForType[l.targetType]
		if sourceTable == "" || targetTable == "" {
			result.BrokenLinks++
			brokenLinkIDs = append(brokenLinkIDs, l.id)
			result.Details = append(result.Details, fmt.Sprintf("link %s: unknown type %s/%s", l.id[:8], l.sourceType, l.targetType))
			continue
		}
		var dummy string
		sourceExists := r.QueryRow(fmt.Sprintf(`SELECT id FROM %s WHERE id = ?`, sourceTable), l.sourceID).Scan(&dummy) == nil
		targetExists := r.QueryRow(fmt.Sprintf(`SELECT id FROM %s WHERE id = ?`, targetTable), l.targetID).Scan(&dummy) == nil
		if !sourceExists || !targetExists {
			result.BrokenLinks++
			brokenLinkIDs = append(brokenLinkIDs, l.id)
			if !sourceExists {
				result.Details = append(result.Details, fmt.Sprintf("broken link %s: source %s:%s missing", l.id[:8], l.sourceType, l.sourceID[:8]))
			}
			if !targetExists {
				result.Details = append(result.Details, fmt.Sprintf("broken link %s: target %s:%s missing", l.id[:8], l.targetType, l.targetID[:8]))
			}
		}
	}

	// Check orphan sessions (sessions referencing non-existent features)
	type sessRow struct {
		id, featureID string
	}
	sessions := scanRows(r,
		`SELECT s.id, s.feature_id FROM sessions s LEFT JOIN features f ON s.feature_id = f.id WHERE f.id IS NULL`,
		nil,
		func(rows *sql.Rows) (sessRow, error) {
			var sr sessRow
			return sr, rows.Scan(&sr.id, &sr.featureID)
		},
	)
	result.OrphanSessions = len(sessions)
	result.TotalChecked += countRows(r, `SELECT COUNT(*) FROM sessions`)

	// Check orphan notes (notes referencing non-existent features)
	result.OrphanNotes = countRows(r,
		`SELECT COUNT(*) FROM notes n LEFT JOIN features f ON n.feature_id = f.id WHERE f.id IS NULL`)
	result.TotalChecked += countRows(r, `SELECT COUNT(*) FROM notes`)

	// Check orphan facts
	result.OrphanFacts = countRows(r,
		`SELECT COUNT(*) FROM facts fa LEFT JOIN features f ON fa.feature_id = f.id WHERE f.id IS NULL`)
	result.TotalChecked += countRows(r, `SELECT COUNT(*) FROM facts`)

	// Calculate score
	totalIssues := result.BrokenLinks + result.OrphanSessions + result.OrphanNotes + result.OrphanFacts
	if result.TotalChecked > 0 {
		result.ScorePercent = 100 - (totalIssues * 100 / result.TotalChecked)
		if result.ScorePercent < 0 {
			result.ScorePercent = 0
		}
	} else {
		result.ScorePercent = 100
	}

	// Fix if requested
	if fix && totalIssues > 0 {
		w := s.db.Writer()
		for _, id := range brokenLinkIDs {
			if _, err := w.Exec(`DELETE FROM memory_links WHERE id = ?`, id); err == nil {
				result.FixedCount++
			}
		}
		for _, sess := range sessions {
			if _, err := w.Exec(`DELETE FROM sessions WHERE id = ?`, sess.id); err == nil {
				result.FixedCount++
			}
		}
	}

	return result, nil
}

// FormatIntegrityCheck formats integrity check results as markdown.
func FormatIntegrityCheck(r *IntegrityResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Integrity Check\n\n")
	fmt.Fprintf(&b, "**Integrity: %d%%** -- %d items checked\n\n", r.ScorePercent, r.TotalChecked)
	if r.BrokenLinks > 0 {
		fmt.Fprintf(&b, "- %d broken link(s)\n", r.BrokenLinks)
	}
	if r.OrphanSessions > 0 {
		fmt.Fprintf(&b, "- %d orphan session(s)\n", r.OrphanSessions)
	}
	if r.OrphanNotes > 0 {
		fmt.Fprintf(&b, "- %d orphan note(s)\n", r.OrphanNotes)
	}
	if r.OrphanFacts > 0 {
		fmt.Fprintf(&b, "- %d orphan fact(s)\n", r.OrphanFacts)
	}
	if r.FixedCount > 0 {
		fmt.Fprintf(&b, "\n**Fixed: %d issue(s)**\n", r.FixedCount)
	}
	if r.BrokenLinks == 0 && r.OrphanSessions == 0 && r.OrphanNotes == 0 && r.OrphanFacts == 0 {
		b.WriteString("All clear. No issues found.\n")
	}
	return b.String()
}

// AutoLinkCode scans notes for file path patterns and creates links to files_touched.
func (s *Store) AutoLinkCode(featureName string) (*AutoLinkCodeResult, error) {
	r := s.db.Reader()
	w := s.db.Writer()

	result := &AutoLinkCodeResult{}

	q := `SELECT n.id, n.content, n.feature_id FROM notes n`
	args := []any{}
	if featureName != "" {
		f, err := s.GetFeature(featureName)
		if err != nil {
			return nil, fmt.Errorf("feature %q not found", featureName)
		}
		q += ` WHERE n.feature_id = ?`
		args = append(args, f.ID)
	}

	type noteRow struct {
		id, content, featureID string
	}
	notes := scanRows(r, q, args, func(rows *sql.Rows) (noteRow, error) {
		var nr noteRow
		return nr, rows.Scan(&nr.id, &nr.content, &nr.featureID)
	})

	linkedFiles := map[string]bool{}
	linkedNotes := map[string]bool{}

	for _, note := range notes {
		matches := filePathPattern.FindAllStringSubmatch(note.content, -1)
		if len(matches) == 0 {
			continue
		}

		for _, match := range matches {
			if len(match) < 2 {
				continue
			}
			filePath := match[1]

			// Insert into files_touched if not already there
			_, err := w.Exec(
				`INSERT OR IGNORE INTO files_touched (id, feature_id, path, action, first_seen) VALUES (?, ?, ?, 'referenced', datetime('now'))`,
				uuid.New().String(), note.featureID, filePath,
			)
			if err != nil {
				continue
			}

			linkedFiles[filePath] = true
			if !linkedNotes[note.id] {
				linkedNotes[note.id] = true
			}
			result.NewLinks++
		}
	}

	result.LinkedNotes = len(linkedNotes)
	result.LinkedFiles = len(linkedFiles)

	return result, nil
}

// FormatAutoLinkCode formats auto-link code results as markdown.
func FormatAutoLinkCode(r *AutoLinkCodeResult) string {
	var b strings.Builder
	b.WriteString("# Auto-Link Code\n\n")
	if r.NewLinks == 0 {
		b.WriteString("No file references found in notes.\n")
		return b.String()
	}
	fmt.Fprintf(&b, "Auto-linked %d notes to %d code files (%d new links)\n", r.LinkedNotes, r.LinkedFiles, r.NewLinks)
	return b.String()
}
