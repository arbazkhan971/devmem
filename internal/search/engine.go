package search

import (
	"database/sql"
	"fmt"
	"math"
	"strings"

	"github.com/arbaz/devmem/internal/storage"
)

// SearchResult represents a single result from the search engine.
type SearchResult struct {
	ID          string
	Type        string
	Content     string
	FeatureName string
	Relevance   float64
	CreatedAt   string
}

// Engine orchestrates the 3-layer search: FTS5 BM25 -> trigram -> (fuzzy placeholder).
type Engine struct {
	db *storage.DB
}

// NewEngine creates a new search engine backed by the given database.
func NewEngine(db *storage.DB) *Engine {
	return &Engine{db: db}
}

// Search executes a multi-layer search across memory types.
//
// query: the search text
// scope: "current_feature" to filter by featureID, or "all_features" for no filter
// types: which memory types to search (e.g. ["notes", "commits", "facts", "plans"]).
//
//	If empty, searches all types.
//
// featureID: required when scope is "current_feature"
// limit: max results to return (0 = default 20)
func (e *Engine) Search(query, scope string, types []string, featureID string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 20
	}
	if len(types) == 0 {
		types = []string{"notes", "commits", "facts", "plans"}
	}

	// Sanitize query for FTS5 MATCH: wrap individual tokens in double quotes
	// to avoid FTS5 syntax errors from special characters.
	ftsQuery := sanitizeFTSQuery(query)

	// Layer 1: FTS5 + BM25
	results, err := e.searchFTS(ftsQuery, scope, types, featureID, limit)
	if err != nil {
		return nil, fmt.Errorf("fts search: %w", err)
	}
	if len(results) > 0 {
		return results, nil
	}

	// Layer 2: Trigram substring
	results, err = e.searchTrigram(query, scope, types, featureID, limit)
	if err != nil {
		return nil, fmt.Errorf("trigram search: %w", err)
	}
	if len(results) > 0 {
		return results, nil
	}

	// Layer 3: Fuzzy (V1 placeholder — not yet implemented)
	return nil, nil
}

// sanitizeFTSQuery wraps each token in double quotes so that special characters
// (colons, hyphens, etc.) don't break FTS5 MATCH syntax.
func sanitizeFTSQuery(query string) string {
	tokens := strings.Fields(query)
	for i, t := range tokens {
		// Remove any existing quotes and re-wrap
		t = strings.ReplaceAll(t, "\"", "")
		if t != "" {
			tokens[i] = "\"" + t + "\""
		}
	}
	return strings.Join(tokens, " ")
}

// searchFTS runs FTS5 MATCH queries with BM25 ranking across requested types.
func (e *Engine) searchFTS(ftsQuery, scope string, types []string, featureID string, limit int) ([]SearchResult, error) {
	reader := e.db.Reader()
	var allResults []SearchResult

	for _, typ := range types {
		var results []SearchResult
		var err error
		switch typ {
		case "notes":
			results, err = e.searchNotesFTS(reader, ftsQuery, scope, featureID)
		case "commits":
			results, err = e.searchCommitsFTS(reader, ftsQuery, scope, featureID)
		case "facts":
			results, err = e.searchFactsFTS(reader, ftsQuery, scope, featureID)
		case "plans":
			results, err = e.searchPlansFTS(reader, ftsQuery, scope, featureID)
		}
		if err != nil {
			return nil, err
		}
		allResults = append(allResults, results...)
	}

	sortByRelevance(allResults)
	if len(allResults) > limit {
		allResults = allResults[:limit]
	}
	return allResults, nil
}

func (e *Engine) searchNotesFTS(reader *sql.DB, ftsQuery, scope, featureID string) ([]SearchResult, error) {
	q := `
SELECT n.id, n.content, n.type, n.created_at, COALESCE(f.name, '') as feature_name,
       bm25(notes_fts) as rank,
       (SELECT COUNT(*) FROM memory_links WHERE source_id = n.id AND source_type = 'note') as link_count
FROM notes_fts
JOIN notes n ON notes_fts.rowid = n.rowid
LEFT JOIN features f ON n.feature_id = f.id
WHERE notes_fts MATCH ?`

	args := []interface{}{ftsQuery}
	if scope == "current_feature" && featureID != "" {
		q += " AND n.feature_id = ?"
		args = append(args, featureID)
	}

	rows, err := reader.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("search notes fts: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		var noteType string
		var rank float64
		var linkCount int
		if err := rows.Scan(&r.ID, &r.Content, &noteType, &r.CreatedAt, &r.FeatureName, &rank, &linkCount); err != nil {
			return nil, fmt.Errorf("scan notes fts: %w", err)
		}
		r.Type = "note"
		r.Relevance = Score(math.Abs(rank), r.CreatedAt, noteType, linkCount)
		results = append(results, r)
	}
	return results, rows.Err()
}

func (e *Engine) searchCommitsFTS(reader *sql.DB, ftsQuery, scope, featureID string) ([]SearchResult, error) {
	q := `
SELECT c.id, c.message, c.intent_type, c.committed_at, COALESCE(f.name, '') as feature_name,
       bm25(commits_fts) as rank,
       (SELECT COUNT(*) FROM memory_links WHERE source_id = c.id AND source_type = 'commit') as link_count
FROM commits_fts
JOIN commits c ON commits_fts.rowid = c.rowid
LEFT JOIN features f ON c.feature_id = f.id
WHERE commits_fts MATCH ?`

	args := []interface{}{ftsQuery}
	if scope == "current_feature" && featureID != "" {
		q += " AND c.feature_id = ?"
		args = append(args, featureID)
	}

	rows, err := reader.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("search commits fts: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		var intentType string
		var rank float64
		var linkCount int
		if err := rows.Scan(&r.ID, &r.Content, &intentType, &r.CreatedAt, &r.FeatureName, &rank, &linkCount); err != nil {
			return nil, fmt.Errorf("scan commits fts: %w", err)
		}
		r.Type = "commit"
		r.Relevance = Score(math.Abs(rank), r.CreatedAt, intentType, linkCount)
		results = append(results, r)
	}
	return results, rows.Err()
}

func (e *Engine) searchFactsFTS(reader *sql.DB, ftsQuery, scope, featureID string) ([]SearchResult, error) {
	q := `
SELECT fa.id, fa.subject || ' ' || fa.predicate || ' ' || fa.object as content,
       'fact' as type, fa.valid_at, COALESCE(f.name, '') as feature_name,
       bm25(facts_fts) as rank,
       (SELECT COUNT(*) FROM memory_links WHERE source_id = fa.id AND source_type = 'fact') as link_count
FROM facts_fts
JOIN facts fa ON facts_fts.rowid = fa.rowid
LEFT JOIN features f ON fa.feature_id = f.id
WHERE facts_fts MATCH ?`

	args := []interface{}{ftsQuery}
	if scope == "current_feature" && featureID != "" {
		q += " AND fa.feature_id = ?"
		args = append(args, featureID)
	}

	rows, err := reader.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("search facts fts: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		var factType string
		var rank float64
		var linkCount int
		if err := rows.Scan(&r.ID, &r.Content, &factType, &r.CreatedAt, &r.FeatureName, &rank, &linkCount); err != nil {
			return nil, fmt.Errorf("scan facts fts: %w", err)
		}
		r.Type = "fact"
		r.Relevance = Score(math.Abs(rank), r.CreatedAt, factType, linkCount)
		results = append(results, r)
	}
	return results, rows.Err()
}

func (e *Engine) searchPlansFTS(reader *sql.DB, ftsQuery, scope, featureID string) ([]SearchResult, error) {
	q := `
SELECT p.id, p.title || ': ' || p.content as content,
       'plan' as type, p.created_at, COALESCE(f.name, '') as feature_name,
       bm25(plans_fts) as rank,
       (SELECT COUNT(*) FROM memory_links WHERE source_id = p.id AND source_type = 'plan') as link_count
FROM plans_fts
JOIN plans p ON plans_fts.rowid = p.rowid
LEFT JOIN features f ON p.feature_id = f.id
WHERE plans_fts MATCH ?`

	args := []interface{}{ftsQuery}
	if scope == "current_feature" && featureID != "" {
		q += " AND p.feature_id = ?"
		args = append(args, featureID)
	}

	rows, err := reader.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("search plans fts: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		var planType string
		var rank float64
		var linkCount int
		if err := rows.Scan(&r.ID, &r.Content, &planType, &r.CreatedAt, &r.FeatureName, &rank, &linkCount); err != nil {
			return nil, fmt.Errorf("scan plans fts: %w", err)
		}
		r.Type = "plan"
		r.Relevance = Score(math.Abs(rank), r.CreatedAt, planType, linkCount)
		results = append(results, r)
	}
	return results, rows.Err()
}

// searchTrigram runs trigram (substring) searches as Layer 2 fallback.
// Only notes and commits have trigram tables.
func (e *Engine) searchTrigram(query, scope string, types []string, featureID string, limit int) ([]SearchResult, error) {
	reader := e.db.Reader()
	var allResults []SearchResult

	for _, typ := range types {
		switch typ {
		case "notes":
			results, err := e.searchNotesTrigram(reader, query, scope, featureID)
			if err != nil {
				return nil, err
			}
			allResults = append(allResults, results...)
		case "commits":
			results, err := e.searchCommitsTrigram(reader, query, scope, featureID)
			if err != nil {
				return nil, err
			}
			allResults = append(allResults, results...)
		}
		// facts and plans don't have trigram tables
	}

	sortByRelevance(allResults)
	if len(allResults) > limit {
		allResults = allResults[:limit]
	}
	return allResults, nil
}

func (e *Engine) searchNotesTrigram(reader *sql.DB, query, scope, featureID string) ([]SearchResult, error) {
	q := `
SELECT n.id, n.content, n.type, n.created_at, COALESCE(f.name, '') as feature_name,
       (SELECT COUNT(*) FROM memory_links WHERE source_id = n.id AND source_type = 'note') as link_count
FROM notes_trigram
JOIN notes n ON notes_trigram.rowid = n.rowid
LEFT JOIN features f ON n.feature_id = f.id
WHERE notes_trigram MATCH ?`

	args := []interface{}{query}
	if scope == "current_feature" && featureID != "" {
		q += " AND n.feature_id = ?"
		args = append(args, featureID)
	}

	rows, err := reader.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("search notes trigram: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		var noteType string
		var linkCount int
		if err := rows.Scan(&r.ID, &r.Content, &noteType, &r.CreatedAt, &r.FeatureName, &linkCount); err != nil {
			return nil, fmt.Errorf("scan notes trigram: %w", err)
		}
		r.Type = "note"
		// Trigram matches get a base BM25-equivalent score of 1.0
		r.Relevance = Score(1.0, r.CreatedAt, noteType, linkCount)
		results = append(results, r)
	}
	return results, rows.Err()
}

func (e *Engine) searchCommitsTrigram(reader *sql.DB, query, scope, featureID string) ([]SearchResult, error) {
	q := `
SELECT c.id, c.message, c.intent_type, c.committed_at, COALESCE(f.name, '') as feature_name,
       (SELECT COUNT(*) FROM memory_links WHERE source_id = c.id AND source_type = 'commit') as link_count
FROM commits_trigram
JOIN commits c ON commits_trigram.rowid = c.rowid
LEFT JOIN features f ON c.feature_id = f.id
WHERE commits_trigram MATCH ?`

	args := []interface{}{query}
	if scope == "current_feature" && featureID != "" {
		q += " AND c.feature_id = ?"
		args = append(args, featureID)
	}

	rows, err := reader.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("search commits trigram: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		var intentType string
		var linkCount int
		if err := rows.Scan(&r.ID, &r.Content, &intentType, &r.CreatedAt, &r.FeatureName, &linkCount); err != nil {
			return nil, fmt.Errorf("scan commits trigram: %w", err)
		}
		r.Type = "commit"
		r.Relevance = Score(1.0, r.CreatedAt, intentType, linkCount)
		results = append(results, r)
	}
	return results, rows.Err()
}

// sortByRelevance sorts results in descending order of relevance.
func sortByRelevance(results []SearchResult) {
	for i := 1; i < len(results); i++ {
		for j := i; j > 0 && results[j].Relevance > results[j-1].Relevance; j-- {
			results[j], results[j-1] = results[j-1], results[j]
		}
	}
}
