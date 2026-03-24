package memory

import (
	"database/sql"
	"fmt"
	"strings"
)

// CodeImpactResult holds the impact analysis for a file path.
type CodeImpactResult struct {
	FilePath     string
	Features     []CodeImpactFeature
	Notes        []Note
	Dependencies []FileDependency
}

// CodeImpactFeature represents a feature that touched the file.
type CodeImpactFeature struct {
	Name   string
	Status string
}

// CodeImpact analyzes the impact of changing a file by finding all features,
// decisions/notes, and dependencies related to that file.
func (s *Store) CodeImpact(filePath string) (*CodeImpactResult, error) {
	r := s.db.Reader()
	result := &CodeImpactResult{FilePath: filePath}

	// 1. Find all features that touched this file
	featureRows, err := r.Query(
		`SELECT DISTINCT f.name, f.status
		 FROM files_touched ft
		 JOIN features f ON ft.feature_id = f.id
		 WHERE ft.path = ?
		 ORDER BY f.last_active DESC`,
		filePath,
	)
	if err != nil {
		return nil, fmt.Errorf("query features for file: %w", err)
	}
	defer featureRows.Close()
	for featureRows.Next() {
		var cf CodeImpactFeature
		if featureRows.Scan(&cf.Name, &cf.Status) == nil {
			result.Features = append(result.Features, cf)
		}
	}
	if err := featureRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate features: %w", err)
	}

	// 2. Find all notes/decisions that reference this file (search note content)
	pattern := "%" + filePath + "%"
	result.Notes, err = collectRows(r,
		`SELECT `+noteCols+` FROM notes WHERE content LIKE ? ORDER BY created_at DESC LIMIT 20`,
		[]any{pattern},
		func(rows *sql.Rows) (Note, error) { return scanNote(rows) },
	)
	if err != nil {
		return nil, fmt.Errorf("query notes for file: %w", err)
	}

	// Also search with just the filename (last component)
	fileName := filePath
	if idx := strings.LastIndex(filePath, "/"); idx >= 0 {
		fileName = filePath[idx+1:]
	}
	if fileName != filePath && fileName != "" {
		fnPattern := "%" + fileName + "%"
		moreNotes, err := collectRows(r,
			`SELECT `+noteCols+` FROM notes WHERE content LIKE ? AND content NOT LIKE ? ORDER BY created_at DESC LIMIT 10`,
			[]any{fnPattern, pattern},
			func(rows *sql.Rows) (Note, error) { return scanNote(rows) },
		)
		if err == nil {
			result.Notes = append(result.Notes, moreNotes...)
		}
	}

	// 3. Find all dependencies of this file
	result.Dependencies, err = s.FindDependencies(filePath)
	if err != nil {
		return nil, fmt.Errorf("find dependencies: %w", err)
	}

	return result, nil
}

// FormatCodeImpact formats a CodeImpactResult into a human-readable string.
func FormatCodeImpact(r *CodeImpactResult) string {
	if r == nil {
		return "No code impact result."
	}

	featureCount := len(r.Features)
	noteCount := len(r.Notes)
	depCount := len(r.Dependencies)

	var b strings.Builder
	fmt.Fprintf(&b, "%s: touched by %d features, %d notes reference it, %d dependencies\n\n",
		r.FilePath, featureCount, noteCount, depCount)

	if featureCount > 0 {
		b.WriteString("**Features:**\n")
		for _, f := range r.Features {
			fmt.Fprintf(&b, "- %s (%s)\n", f.Name, f.Status)
		}
		b.WriteByte('\n')
	}

	if noteCount > 0 {
		decisionCount := 0
		for _, n := range r.Notes {
			if n.Type == "decision" {
				decisionCount++
			}
		}
		fmt.Fprintf(&b, "**Notes/decisions (%d total, %d decisions):**\n", noteCount, decisionCount)
		limit := noteCount
		if limit > 10 {
			limit = 10
		}
		for _, n := range r.Notes[:limit] {
			c := n.Content
			if len(c) > 100 {
				c = c[:100] + "..."
			}
			c = strings.ReplaceAll(c, "\n", " ")
			fmt.Fprintf(&b, "- [%s] %s\n", n.Type, c)
		}
		if noteCount > 10 {
			fmt.Fprintf(&b, "... and %d more\n", noteCount-10)
		}
		b.WriteByte('\n')
	}

	if depCount > 0 {
		b.WriteString("**Dependencies (co-changed files):**\n")
		limit := depCount
		if limit > 10 {
			limit = 10
		}
		for _, d := range r.Dependencies[:limit] {
			times := "times"
			if d.Occurrences == 1 {
				times = "time"
			}
			fmt.Fprintf(&b, "- %s (changed together %d %s)\n", d.Path, d.Occurrences, times)
		}
		if depCount > 10 {
			fmt.Fprintf(&b, "... and %d more\n", depCount-10)
		}
	}

	return b.String()
}
