package memory

import (
	"database/sql"
	"fmt"
	"strings"
)

// WhatIfResult holds the impact analysis when considering undoing a decision.
type WhatIfResult struct {
	Decision       Note
	AffectedNotes  []Note
	AffectedFacts  []Fact
	TopicRelated   []Note
	TopicFacts     []Fact
}

// WhatIf explores the impact of undoing a decision.
// It finds the matching decision note, finds all notes/facts created after it,
// and finds all notes/facts referencing the same topic.
func (s *Store) WhatIf(decisionQuery string) (*WhatIfResult, error) {
	r := s.db.Reader()

	// 1. Find the decision note matching the query via FTS
	decision, err := s.findDecisionByQuery(r, decisionQuery)
	if err != nil {
		return nil, err
	}

	result := &WhatIfResult{Decision: *decision}

	// 2. Find all notes created AFTER this decision (same feature)
	result.AffectedNotes, err = collectRows(r,
		`SELECT `+noteCols+` FROM notes WHERE feature_id = ? AND created_at > ? ORDER BY created_at ASC`,
		[]any{decision.FeatureID, decision.CreatedAt},
		func(rows *sql.Rows) (Note, error) { return scanNote(rows) },
	)
	if err != nil {
		return nil, fmt.Errorf("query affected notes: %w", err)
	}

	// 3. Find all facts created AFTER this decision (same feature)
	result.AffectedFacts, err = collectRows(r,
		`SELECT `+factColumns+` FROM facts WHERE feature_id = ? AND recorded_at > ? ORDER BY recorded_at ASC`,
		[]any{decision.FeatureID, decision.CreatedAt},
		func(rows *sql.Rows) (Fact, error) { return scanFact(rows) },
	)
	if err != nil {
		return nil, fmt.Errorf("query affected facts: %w", err)
	}

	// 4. Extract topic keywords from the decision content and find related notes/facts
	topic := extractTopicKeywords(decision.Content)
	if topic != "" {
		pattern := "%" + topic + "%"

		result.TopicRelated, _ = collectRows(r,
			`SELECT `+noteCols+` FROM notes WHERE feature_id = ? AND id != ? AND content LIKE ? ORDER BY created_at DESC LIMIT 20`,
			[]any{decision.FeatureID, decision.ID, pattern},
			func(rows *sql.Rows) (Note, error) { return scanNote(rows) },
		)

		result.TopicFacts, _ = collectRows(r,
			`SELECT `+factColumns+` FROM facts WHERE feature_id = ? AND (subject LIKE ? OR object LIKE ?) ORDER BY recorded_at DESC LIMIT 20`,
			[]any{decision.FeatureID, pattern, pattern},
			func(rows *sql.Rows) (Fact, error) { return scanFact(rows) },
		)
	}

	return result, nil
}

// findDecisionByQuery searches for a decision note matching the query.
// It first tries FTS, then falls back to LIKE search.
func (s *Store) findDecisionByQuery(r *sql.DB, query string) (*Note, error) {
	// Try FTS match first for decision notes
	row := r.QueryRow(
		`SELECT `+noteCols+` FROM notes WHERE type = 'decision' AND rowid IN (
			SELECT rowid FROM notes_fts WHERE notes_fts MATCH ?
		) ORDER BY created_at DESC LIMIT 1`, query,
	)
	n, err := scanNote(row)
	if err == nil {
		return &n, nil
	}

	// Fallback: LIKE search on decision notes
	row = r.QueryRow(
		`SELECT `+noteCols+` FROM notes WHERE type = 'decision' AND content LIKE ? ORDER BY created_at DESC LIMIT 1`,
		"%"+query+"%",
	)
	n, err = scanNote(row)
	if err == nil {
		return &n, nil
	}

	// Last resort: search all note types
	row = r.QueryRow(
		`SELECT `+noteCols+` FROM notes WHERE content LIKE ? ORDER BY created_at DESC LIMIT 1`,
		"%"+query+"%",
	)
	n, err = scanNote(row)
	if err == nil {
		return &n, nil
	}

	return nil, fmt.Errorf("no decision found matching %q", query)
}

// extractTopicKeywords picks the most meaningful word from the content for topic matching.
func extractTopicKeywords(content string) string {
	// Split on common delimiters and filter out stop words
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "is": true, "are": true,
		"was": true, "were": true, "be": true, "been": true, "being": true,
		"have": true, "has": true, "had": true, "do": true, "does": true,
		"did": true, "will": true, "would": true, "could": true, "should": true,
		"may": true, "might": true, "must": true, "shall": true,
		"to": true, "of": true, "in": true, "for": true, "on": true,
		"with": true, "at": true, "by": true, "from": true, "as": true,
		"into": true, "through": true, "during": true, "before": true, "after": true,
		"and": true, "but": true, "or": true, "nor": true, "not": true,
		"that": true, "this": true, "these": true, "those": true,
		"it": true, "its": true, "we": true, "use": true, "using": true,
		"decided": true, "instead": true, "chose": true, "chosen": true,
	}

	words := strings.Fields(strings.ToLower(content))
	var best string
	for _, w := range words {
		// Clean punctuation
		w = strings.Trim(w, ".,;:!?\"'()[]{}+-")
		if len(w) < 3 || stopWords[w] {
			continue
		}
		if best == "" || len(w) > len(best) {
			best = w
		}
	}
	return best
}

// FormatWhatIf produces a human-readable string for a WhatIfResult.
func FormatWhatIf(r *WhatIfResult) string {
	if r == nil {
		return "No what-if result."
	}

	content := r.Decision.Content
	if len(content) > 80 {
		content = content[:80] + "..."
	}

	affectedCount := len(r.AffectedNotes) + len(r.AffectedFacts)
	topicCount := len(r.TopicRelated) + len(r.TopicFacts)

	var b strings.Builder
	fmt.Fprintf(&b, "If you undo decision \"%s\", these %d items may be affected:\n\n",
		strings.ReplaceAll(content, "\n", " "), affectedCount)

	if len(r.AffectedNotes) > 0 {
		fmt.Fprintf(&b, "**Notes created after this decision (%d):**\n", len(r.AffectedNotes))
		for _, n := range r.AffectedNotes {
			c := n.Content
			if len(c) > 100 {
				c = c[:100] + "..."
			}
			c = strings.ReplaceAll(c, "\n", " ")
			fmt.Fprintf(&b, "- [%s] %s\n", n.Type, c)
		}
		b.WriteByte('\n')
	}

	if len(r.AffectedFacts) > 0 {
		fmt.Fprintf(&b, "**Facts recorded after this decision (%d):**\n", len(r.AffectedFacts))
		for _, f := range r.AffectedFacts {
			fmt.Fprintf(&b, "- %s %s %s\n", f.Subject, f.Predicate, f.Object)
		}
		b.WriteByte('\n')
	}

	if topicCount > 0 {
		fmt.Fprintf(&b, "**Additionally, %d items reference the same topic:**\n", topicCount)
		for _, n := range r.TopicRelated {
			c := n.Content
			if len(c) > 100 {
				c = c[:100] + "..."
			}
			c = strings.ReplaceAll(c, "\n", " ")
			fmt.Fprintf(&b, "- [note] %s\n", c)
		}
		for _, f := range r.TopicFacts {
			fmt.Fprintf(&b, "- [fact] %s %s %s\n", f.Subject, f.Predicate, f.Object)
		}
	}

	return b.String()
}
