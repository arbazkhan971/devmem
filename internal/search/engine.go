package search

import (
	"database/sql"
	"fmt"
	"math"
	"strings"
	"time"

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

// Engine orchestrates the 3-layer search: FTS5 BM25 -> trigram.
type Engine struct {
	db *storage.DB
}

// NewEngine creates a new search engine backed by the given database.
func NewEngine(db *storage.DB) *Engine {
	return &Engine{db: db}
}

// LinkedMemory represents a memory item discovered via link traversal.
type LinkedMemory struct {
	ID           string
	Type         string
	Relationship string
	Strength     float64
	Depth        int
}

// ftsTable describes how to search a single FTS5 (or trigram) virtual table.
type ftsTable struct {
	typeName    string // search type key: "notes", "commits", "facts", "plans"
	resultType  string // result Type field: "note", "commit", "fact", "plan"
	ftsName     string // virtual table name: "notes_fts", "commits_fts", etc.
	sourceTable string // source table: "notes", "commits", "facts", "plans"
	alias       string // SQL alias for the source table
	contentExpr string // SQL expression for the content column (uses alias)
	typeExpr    string // SQL expression for the type/subtype column (uses alias)
	timeCol     string // column name for timestamp (uses alias)
	featureCol  string // column for feature_id filter (uses alias)
	sourceType  string // source_type value for memory_links count
	trigramName string // trigram virtual table name, empty if none
}

var ftsTables = []ftsTable{
	{
		typeName: "notes", resultType: "note",
		ftsName: "notes_fts", sourceTable: "notes", alias: "n",
		contentExpr: "n.content", typeExpr: "n.type",
		timeCol: "n.created_at", featureCol: "n.feature_id", sourceType: "note",
		trigramName: "notes_trigram",
	},
	{
		typeName: "commits", resultType: "commit",
		ftsName: "commits_fts", sourceTable: "commits", alias: "c",
		contentExpr: "c.message", typeExpr: "c.intent_type",
		timeCol: "c.committed_at", featureCol: "c.feature_id", sourceType: "commit",
		trigramName: "commits_trigram",
	},
	{
		typeName: "facts", resultType: "fact",
		ftsName: "facts_fts", sourceTable: "facts", alias: "fa",
		contentExpr: "fa.subject || ' ' || fa.predicate || ' ' || fa.object", typeExpr: "'fact'",
		timeCol: "fa.valid_at", featureCol: "fa.feature_id", sourceType: "fact",
	},
	{
		typeName: "plans", resultType: "plan",
		ftsName: "plans_fts", sourceTable: "plans", alias: "p",
		contentExpr: "p.title || ': ' || p.content", typeExpr: "'plan'",
		timeCol: "p.created_at", featureCol: "p.feature_id", sourceType: "plan",
	},
}

// ftsTableMap provides O(1) lookup by typeName.
var ftsTableMap = func() map[string]*ftsTable {
	m := make(map[string]*ftsTable, len(ftsTables))
	for i := range ftsTables {
		m[ftsTables[i].typeName] = &ftsTables[i]
	}
	return m
}()

// typeWeights maps note/commit types to relevance multipliers.
var typeWeights = map[string]float64{
	"decision":  2.0,
	"blocker":   1.5,
	"progress":  1.0,
	"feature":   1.2,
	"note":      0.5,
	"next_step": 1.0,
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
	results, err := e.searchLayer(ftsQuery, scope, types, featureID, limit, false)
	if err != nil {
		return nil, fmt.Errorf("fts search: %w", err)
	}
	if len(results) > 0 {
		return results, nil
	}

	// Layer 2: Trigram substring
	results, err = e.searchLayer(query, scope, types, featureID, limit, true)
	if err != nil {
		return nil, fmt.Errorf("trigram search: %w", err)
	}
	return results, nil
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

// searchLayer runs either FTS5 or trigram queries across requested types.
func (e *Engine) searchLayer(matchQuery, scope string, types []string, featureID string, limit int, trigram bool) ([]SearchResult, error) {
	reader := e.db.Reader()
	var allResults []SearchResult

	for _, typ := range types {
		tbl, ok := ftsTableMap[typ]
		if !ok {
			continue
		}
		results, err := e.searchTable(reader, tbl, matchQuery, scope, featureID, trigram)
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

// searchTable executes a single FTS5 or trigram query for one table definition.
func (e *Engine) searchTable(reader *sql.DB, tbl *ftsTable, matchQuery, scope, featureID string, trigram bool) ([]SearchResult, error) {
	vtable := tbl.ftsName
	if trigram {
		if tbl.trigramName == "" {
			return nil, nil // this type has no trigram table
		}
		vtable = tbl.trigramName
	}

	// Build SELECT columns: id, content, subtype, timestamp, feature_name, [rank,] link_count
	rankCol := ""
	if !trigram {
		rankCol = fmt.Sprintf(",\n       bm25(%s) as rank", vtable)
	}

	q := fmt.Sprintf(`
SELECT %s.id, %s as content, %s as subtype, %s, COALESCE(f.name, '') as feature_name%s,
       (SELECT COUNT(*) FROM memory_links WHERE source_id = %s.id AND source_type = '%s') as link_count
FROM %s
JOIN %s %s ON %s.rowid = %s.rowid
LEFT JOIN features f ON %s.feature_id = f.id
WHERE %s MATCH ?`,
		tbl.alias, tbl.contentExpr, tbl.typeExpr, tbl.timeCol, rankCol,
		tbl.alias, tbl.sourceType,
		vtable,
		tbl.sourceTable, tbl.alias, vtable, tbl.alias,
		tbl.alias,
		vtable,
	)

	args := []interface{}{matchQuery}
	if scope == "current_feature" && featureID != "" {
		q += fmt.Sprintf(" AND %s = ?", tbl.featureCol)
		args = append(args, featureID)
	}

	rows, err := reader.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("search %s %s: %w", tbl.typeName, vtable, err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		var subtype string
		var linkCount int
		var rank float64

		if trigram {
			if err := rows.Scan(&r.ID, &r.Content, &subtype, &r.CreatedAt, &r.FeatureName, &linkCount); err != nil {
				return nil, fmt.Errorf("scan %s %s: %w", tbl.typeName, vtable, err)
			}
			rank = 1.0
		} else {
			if err := rows.Scan(&r.ID, &r.Content, &subtype, &r.CreatedAt, &r.FeatureName, &rank, &linkCount); err != nil {
				return nil, fmt.Errorf("scan %s %s: %w", tbl.typeName, vtable, err)
			}
			rank = math.Abs(rank)
		}

		r.Type = tbl.resultType
		r.Relevance = Score(rank, r.CreatedAt, subtype, linkCount)
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

// TraverseLinks follows memory_links from a starting memory, using a recursive
// CTE to discover connected items up to maxDepth hops away.
func (e *Engine) TraverseLinks(memoryID, memoryType string, maxDepth int) ([]LinkedMemory, error) {
	if maxDepth < 1 {
		maxDepth = 1
	}

	const query = `
WITH RECURSIVE connected AS (
    SELECT target_id, target_type, relationship, strength, 1 as depth
    FROM memory_links
    WHERE source_id = ? AND source_type = ?
    UNION ALL
    SELECT ml.target_id, ml.target_type, ml.relationship, ml.strength, c.depth + 1
    FROM memory_links ml
    JOIN connected c ON ml.source_id = c.target_id AND ml.source_type = c.target_type
    WHERE c.depth < ?
)
SELECT DISTINCT target_id, target_type, relationship, strength, depth
FROM connected
ORDER BY depth, strength DESC
`

	rows, err := e.db.Reader().Query(query, memoryID, memoryType, maxDepth)
	if err != nil {
		return nil, fmt.Errorf("traverse links: %w", err)
	}
	defer rows.Close()

	var results []LinkedMemory
	for rows.Next() {
		var lm LinkedMemory
		if err := rows.Scan(&lm.ID, &lm.Type, &lm.Relationship, &lm.Strength, &lm.Depth); err != nil {
			return nil, fmt.Errorf("scan linked memory: %w", err)
		}
		results = append(results, lm)
	}
	return results, rows.Err()
}

// Score computes a composite relevance score for a search result.
//
// Formula: score = bm25Score * temporalDecay * typeWeight * linkBoost
//
// bm25Score should be the absolute (positive) BM25 value.
// createdAt is an RFC3339/datetime string.
// noteType is the type column value (e.g. "decision", "blocker", "progress", "note").
// linkCount is the number of memory_links referencing this item.
func Score(bm25Score float64, createdAt string, noteType string, linkCount int) float64 {
	decay := temporalDecay(createdAt)
	weight := typeWeight(noteType)
	boost := linkBoost(linkCount)
	return bm25Score * decay * weight * boost
}

// temporalDecay returns an exponential decay factor with a 14-day half-life.
// Items created now return ~1.0; items 14 days old return ~0.5.
func temporalDecay(createdAt string) float64 {
	t, err := time.Parse("2006-01-02 15:04:05", createdAt)
	if err != nil {
		// Try RFC3339 as fallback
		t, err = time.Parse(time.RFC3339, createdAt)
		if err != nil {
			return 1.0
		}
	}
	days := time.Since(t).Hours() / 24.0
	if days < 0 {
		days = 0
	}
	return math.Exp(-0.693 * days / 14.0)
}

// typeWeight returns a relevance multiplier based on the memory type.
func typeWeight(noteType string) float64 {
	if w, ok := typeWeights[noteType]; ok {
		return w
	}
	return 1.0
}

// linkBoost returns a boost factor based on how connected an item is.
func linkBoost(linkCount int) float64 {
	return 1.0 + float64(linkCount)*0.1
}
