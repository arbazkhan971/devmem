package search_test

import (
	"fmt"
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/arbazkhan971/memorx/internal/search"
	"github.com/arbazkhan971/memorx/internal/storage"
)

// setupTestDB creates a temp database, migrates it, and inserts seed data.
func setupTestDB(t *testing.T) *storage.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := storage.NewDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	if err := storage.Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return db
}

// insertFeature inserts a feature row and returns the feature ID.
func insertFeature(t *testing.T, db *storage.DB, id, name string) {
	t.Helper()
	_, err := db.Writer().Exec(
		"INSERT INTO features (id, name, status) VALUES (?, ?, 'active')",
		id, name,
	)
	if err != nil {
		t.Fatalf("insert feature %s: %v", id, err)
	}
}

// insertNote inserts a note and syncs it into the FTS5 and trigram tables.
func insertNote(t *testing.T, db *storage.DB, id, featureID, content, noteType, createdAt string) {
	t.Helper()
	_, err := db.Writer().Exec(
		"INSERT INTO notes (id, feature_id, content, type, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		id, featureID, content, noteType, createdAt, createdAt,
	)
	if err != nil {
		t.Fatalf("insert note %s: %v", id, err)
	}
	// Sync into FTS5 content-sync table
	_, err = db.Writer().Exec(
		"INSERT INTO notes_fts (rowid, content, type) SELECT rowid, content, type FROM notes WHERE id = ?", id,
	)
	if err != nil {
		t.Fatalf("insert notes_fts for %s: %v", id, err)
	}
	// Sync into trigram table
	_, err = db.Writer().Exec(
		"INSERT INTO notes_trigram (rowid, content) SELECT rowid, content FROM notes WHERE id = ?", id,
	)
	if err != nil {
		t.Fatalf("insert notes_trigram for %s: %v", id, err)
	}
}

// insertCommit inserts a commit and syncs it into FTS5 and trigram tables.
func insertCommit(t *testing.T, db *storage.DB, id, featureID, hash, message, intentType, committedAt string) {
	t.Helper()
	_, err := db.Writer().Exec(
		"INSERT INTO commits (id, feature_id, hash, message, author, intent_type, committed_at) VALUES (?, ?, ?, ?, 'test', ?, ?)",
		id, featureID, hash, message, intentType, committedAt,
	)
	if err != nil {
		t.Fatalf("insert commit %s: %v", id, err)
	}
	_, err = db.Writer().Exec(
		"INSERT INTO commits_fts (rowid, message) SELECT rowid, message FROM commits WHERE id = ?", id,
	)
	if err != nil {
		t.Fatalf("insert commits_fts for %s: %v", id, err)
	}
	_, err = db.Writer().Exec(
		"INSERT INTO commits_trigram (rowid, message) SELECT rowid, message FROM commits WHERE id = ?", id,
	)
	if err != nil {
		t.Fatalf("insert commits_trigram for %s: %v", id, err)
	}
}

// insertFact inserts a fact and syncs it into the FTS5 table.
func insertFact(t *testing.T, db *storage.DB, id, featureID, subject, predicate, object, validAt string) {
	t.Helper()
	_, err := db.Writer().Exec(
		"INSERT INTO facts (id, feature_id, subject, predicate, object, valid_at, recorded_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		id, featureID, subject, predicate, object, validAt, validAt,
	)
	if err != nil {
		t.Fatalf("insert fact %s: %v", id, err)
	}
	_, err = db.Writer().Exec(
		"INSERT INTO facts_fts (rowid, subject, predicate, object) SELECT rowid, subject, predicate, object FROM facts WHERE id = ?", id,
	)
	if err != nil {
		t.Fatalf("insert facts_fts for %s: %v", id, err)
	}
}

// insertPlan inserts a plan and syncs it into the FTS5 table.
func insertPlan(t *testing.T, db *storage.DB, id, featureID, title, content, createdAt string) {
	t.Helper()
	_, err := db.Writer().Exec(
		"INSERT INTO plans (id, feature_id, title, content, status, created_at, updated_at, valid_at) VALUES (?, ?, ?, ?, 'active', ?, ?, ?)",
		id, featureID, title, content, createdAt, createdAt, createdAt,
	)
	if err != nil {
		t.Fatalf("insert plan %s: %v", id, err)
	}
	_, err = db.Writer().Exec(
		"INSERT INTO plans_fts (rowid, title, content) SELECT rowid, title, content FROM plans WHERE id = ?", id,
	)
	if err != nil {
		t.Fatalf("insert plans_fts for %s: %v", id, err)
	}
}

// insertLink inserts a memory link.
func insertLink(t *testing.T, db *storage.DB, id, srcID, srcType, tgtID, tgtType, relationship string, strength float64) {
	t.Helper()
	_, err := db.Writer().Exec(
		"INSERT INTO memory_links (id, source_id, source_type, target_id, target_type, relationship, strength) VALUES (?, ?, ?, ?, ?, ?, ?)",
		id, srcID, srcType, tgtID, tgtType, relationship, strength,
	)
	if err != nil {
		t.Fatalf("insert link %s: %v", id, err)
	}
}

func now() string {
	return time.Now().UTC().Format("2006-01-02 15:04:05")
}

func daysAgo(days int) string {
	return time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour).Format("2006-01-02 15:04:05")
}

// ---------- FTS5 search tests ----------

func TestFTS5SearchFindsExactWordMatches(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	insertFeature(t, db, "f1", "auth-feature")
	insertNote(t, db, "n1", "f1", "Implemented authentication middleware for JWT tokens", "progress", now())
	insertNote(t, db, "n2", "f1", "Database migration completed successfully", "progress", now())

	engine := search.NewEngine(db)
	results, err := engine.Search("authentication", "all_features", []string{"notes"}, "", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one result for 'authentication'")
	}
	if results[0].ID != "n1" {
		t.Errorf("expected first result ID=n1, got %s", results[0].ID)
	}
}

func TestFTS5SearchAcrossMultipleTypes(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	insertFeature(t, db, "f1", "search-feature")
	insertNote(t, db, "n1", "f1", "Refactored the search engine module", "progress", now())
	insertCommit(t, db, "c1", "f1", "abc123", "Refactored search engine for better performance", "refactor", now())
	insertFact(t, db, "fact1", "f1", "search engine", "uses", "FTS5 for full text search", now())
	insertPlan(t, db, "p1", "f1", "Search Engine Improvement", "Refactor the search engine to use BM25 scoring", now())

	engine := search.NewEngine(db)
	results, err := engine.Search("search engine", "all_features", nil, "", 20)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) < 3 {
		t.Fatalf("expected at least 3 results across types, got %d", len(results))
	}

	// Verify we get different types
	typeSet := make(map[string]bool)
	for _, r := range results {
		typeSet[r.Type] = true
	}
	for _, typ := range []string{"note", "commit", "fact", "plan"} {
		if !typeSet[typ] {
			t.Errorf("expected type %s in results", typ)
		}
	}
}

func TestSearchResultsSortedByRelevance(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	insertFeature(t, db, "f1", "test-feature")
	// A decision (weight 2.0) should rank higher than a plain note (weight 0.5)
	insertNote(t, db, "n1", "f1", "We decided to use PostgreSQL for the database", "decision", now())
	insertNote(t, db, "n2", "f1", "Note about database schema being complex", "note", now())

	engine := search.NewEngine(db)
	results, err := engine.Search("database", "all_features", []string{"notes"}, "", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("expected at least 2 results, got %d", len(results))
	}
	// Results should be sorted by relevance descending
	for i := 1; i < len(results); i++ {
		if results[i].Relevance > results[i-1].Relevance {
			t.Errorf("results not sorted: result[%d].Relevance=%f > result[%d].Relevance=%f",
				i, results[i].Relevance, i-1, results[i-1].Relevance)
		}
	}
}

// ---------- Scope filtering tests ----------

func TestScopeFilteringCurrentFeature(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	insertFeature(t, db, "f1", "feature-one")
	insertFeature(t, db, "f2", "feature-two")
	insertNote(t, db, "n1", "f1", "Implemented caching layer for requests", "progress", now())
	insertNote(t, db, "n2", "f2", "Implemented caching layer for responses", "progress", now())

	engine := search.NewEngine(db)

	// Scope to feature f1
	results, err := engine.Search("caching", "current_feature", []string{"notes"}, "f1", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result for current_feature f1, got %d", len(results))
	}
	if results[0].ID != "n1" {
		t.Errorf("expected result ID=n1, got %s", results[0].ID)
	}
}

func TestScopeFilteringAllFeatures(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	insertFeature(t, db, "f1", "feature-one")
	insertFeature(t, db, "f2", "feature-two")
	insertNote(t, db, "n1", "f1", "Implemented caching layer for requests", "progress", now())
	insertNote(t, db, "n2", "f2", "Implemented caching layer for responses", "progress", now())

	engine := search.NewEngine(db)

	results, err := engine.Search("caching", "all_features", []string{"notes"}, "", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results for all_features, got %d", len(results))
	}
}

// ---------- Type filtering tests ----------

func TestTypeFiltering(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	insertFeature(t, db, "f1", "test-feature")
	insertNote(t, db, "n1", "f1", "Refactored the API endpoint handler", "progress", now())
	insertCommit(t, db, "c1", "f1", "def456", "Refactored API endpoint handler", "refactor", now())

	engine := search.NewEngine(db)

	// Only search notes
	results, err := engine.Search("API endpoint", "all_features", []string{"notes"}, "", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	for _, r := range results {
		if r.Type != "note" {
			t.Errorf("expected only note type, got %s", r.Type)
		}
	}

	// Only search commits
	results, err = engine.Search("API endpoint", "all_features", []string{"commits"}, "", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	for _, r := range results {
		if r.Type != "commit" {
			t.Errorf("expected only commit type, got %s", r.Type)
		}
	}
}

// ---------- Scoring tests ----------

func TestScoringRecentItemsScoreHigher(t *testing.T) {
	recentTime := now()
	oldTime := daysAgo(30)

	recentScore := search.Score(1.0, recentTime, "note", 0)
	oldScore := search.Score(1.0, oldTime, "note", 0)

	if recentScore <= oldScore {
		t.Errorf("recent score (%f) should be higher than old score (%f)", recentScore, oldScore)
	}
}

func TestScoringDecisionsScoreHigherThanNotes(t *testing.T) {
	ts := now()
	decisionScore := search.Score(1.0, ts, "decision", 0)
	noteScore := search.Score(1.0, ts, "note", 0)

	if decisionScore <= noteScore {
		t.Errorf("decision score (%f) should be higher than note score (%f)", decisionScore, noteScore)
	}
}

func TestScoringBlockersScoreHigherThanProgress(t *testing.T) {
	ts := now()
	blockerScore := search.Score(1.0, ts, "blocker", 0)
	progressScore := search.Score(1.0, ts, "progress", 0)

	if blockerScore <= progressScore {
		t.Errorf("blocker score (%f) should be higher than progress score (%f)", blockerScore, progressScore)
	}
}

func TestScoringLinkedItemsGetBoost(t *testing.T) {
	ts := now()
	noLinks := search.Score(1.0, ts, "note", 0)
	withLinks := search.Score(1.0, ts, "note", 5)

	if withLinks <= noLinks {
		t.Errorf("linked score (%f) should be higher than unlinked score (%f)", withLinks, noLinks)
	}

	// linkBoost = 1.0 + 5*0.1 = 1.5
	expectedRatio := 1.5
	actualRatio := withLinks / noLinks
	if math.Abs(actualRatio-expectedRatio) > 0.01 {
		t.Errorf("expected link boost ratio ~%.2f, got %.4f", expectedRatio, actualRatio)
	}
}

func TestScoringTemporalDecayHalfLife(t *testing.T) {
	ts14DaysAgo := daysAgo(14)
	score := search.Score(1.0, ts14DaysAgo, "progress", 0)
	// At 14 days, temporal decay should be ~0.5
	// score = 1.0 * ~0.5 * 1.0 (progress weight) * 1.0 (no links) = ~0.5
	if math.Abs(score-0.5) > 0.05 {
		t.Errorf("expected score ~0.5 at 14-day half-life, got %f", score)
	}
}

// ---------- Trigram fallback tests ----------

func TestTrigramFallbackFindsSubstring(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	insertFeature(t, db, "f1", "test-feature")
	// Insert a note with unique content that won't match FTS stemming but has a substring
	insertNote(t, db, "n1", "f1", "Configuration value xyz789abc in the settings", "progress", now())

	engine := search.NewEngine(db)
	// "xyz789abc" won't match FTS word stemming but will match trigram substring
	results, err := engine.Search("xyz789abc", "all_features", []string{"notes"}, "", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected trigram fallback to find substring match")
	}
	if results[0].ID != "n1" {
		t.Errorf("expected result ID=n1, got %s", results[0].ID)
	}
}

// ---------- Graph traversal tests ----------

func TestTraverseLinksBasic(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	insertFeature(t, db, "f1", "test-feature")
	insertNote(t, db, "n1", "f1", "Root note", "progress", now())
	insertNote(t, db, "n2", "f1", "Linked note", "progress", now())
	insertNote(t, db, "n3", "f1", "Second hop note", "progress", now())

	// n1 -> n2 (depth 1)
	insertLink(t, db, "l1", "n1", "note", "n2", "note", "related", 0.8)
	// n2 -> n3 (depth 2)
	insertLink(t, db, "l2", "n2", "note", "n3", "note", "extends", 0.6)

	engine := search.NewEngine(db)

	// Traverse from n1 with maxDepth=2
	linked, err := engine.TraverseLinks("n1", "note", 2)
	if err != nil {
		t.Fatalf("TraverseLinks: %v", err)
	}
	if len(linked) != 2 {
		t.Fatalf("expected 2 linked memories, got %d", len(linked))
	}

	// First should be depth 1 (n2)
	if linked[0].ID != "n2" {
		t.Errorf("expected first linked ID=n2, got %s", linked[0].ID)
	}
	if linked[0].Depth != 1 {
		t.Errorf("expected depth 1, got %d", linked[0].Depth)
	}
	if linked[0].Relationship != "related" {
		t.Errorf("expected relationship 'related', got %s", linked[0].Relationship)
	}

	// Second should be depth 2 (n3)
	if linked[1].ID != "n3" {
		t.Errorf("expected second linked ID=n3, got %s", linked[1].ID)
	}
	if linked[1].Depth != 2 {
		t.Errorf("expected depth 2, got %d", linked[1].Depth)
	}
}

func TestTraverseLinksDepthLimit(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	insertFeature(t, db, "f1", "test-feature")
	insertNote(t, db, "n1", "f1", "Root", "progress", now())
	insertNote(t, db, "n2", "f1", "Hop 1", "progress", now())
	insertNote(t, db, "n3", "f1", "Hop 2", "progress", now())

	insertLink(t, db, "l1", "n1", "note", "n2", "note", "related", 0.8)
	insertLink(t, db, "l2", "n2", "note", "n3", "note", "extends", 0.6)

	engine := search.NewEngine(db)

	// maxDepth=1 should only find n2
	linked, err := engine.TraverseLinks("n1", "note", 1)
	if err != nil {
		t.Fatalf("TraverseLinks: %v", err)
	}
	if len(linked) != 1 {
		t.Fatalf("expected 1 linked memory at depth 1, got %d", len(linked))
	}
	if linked[0].ID != "n2" {
		t.Errorf("expected linked ID=n2, got %s", linked[0].ID)
	}
}

func TestTraverseLinksCrossType(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	insertFeature(t, db, "f1", "test-feature")
	insertNote(t, db, "n1", "f1", "Decision note", "decision", now())
	insertCommit(t, db, "c1", "f1", "aaa111", "Implemented the decision", "feature", now())

	// note -> commit
	insertLink(t, db, "l1", "n1", "note", "c1", "commit", "implements", 0.9)

	engine := search.NewEngine(db)
	linked, err := engine.TraverseLinks("n1", "note", 1)
	if err != nil {
		t.Fatalf("TraverseLinks: %v", err)
	}
	if len(linked) != 1 {
		t.Fatalf("expected 1 linked memory, got %d", len(linked))
	}
	if linked[0].Type != "commit" {
		t.Errorf("expected type 'commit', got %s", linked[0].Type)
	}
	if linked[0].Relationship != "implements" {
		t.Errorf("expected relationship 'implements', got %s", linked[0].Relationship)
	}
}

// ---------- Empty result tests ----------

func TestSearchNoMatches(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	insertFeature(t, db, "f1", "test-feature")
	insertNote(t, db, "n1", "f1", "Something about databases", "progress", now())

	engine := search.NewEngine(db)
	results, err := engine.Search("xyznonexistentterm999", "all_features", nil, "", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil results for no matches, got %d results", len(results))
	}
}

func TestTraverseLinksNoLinks(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	insertFeature(t, db, "f1", "test-feature")
	insertNote(t, db, "n1", "f1", "Isolated note", "progress", now())

	engine := search.NewEngine(db)
	linked, err := engine.TraverseLinks("n1", "note", 3)
	if err != nil {
		t.Fatalf("TraverseLinks: %v", err)
	}
	if len(linked) != 0 {
		t.Errorf("expected 0 linked memories, got %d", len(linked))
	}
}

// ---------- Feature name in results ----------

func TestSearchReturnsFeatureName(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	insertFeature(t, db, "f1", "authentication-service")
	insertNote(t, db, "n1", "f1", "Implemented token validation logic", "progress", now())

	engine := search.NewEngine(db)
	results, err := engine.Search("token validation", "all_features", []string{"notes"}, "", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}
	if results[0].FeatureName != "authentication-service" {
		t.Errorf("expected FeatureName='authentication-service', got '%s'", results[0].FeatureName)
	}
}

// ---------- Scoring formula edge cases ----------

func TestScoringFormulaCombined(t *testing.T) {
	ts := now()
	// decision (2.0) with 3 links: score = bm25 * ~1.0 * 2.0 * 1.3
	score := search.Score(2.5, ts, "decision", 3)
	expected := 2.5 * 1.0 * 2.0 * 1.3 // ~6.5, temporal decay ~1.0 for "now"
	if math.Abs(score-expected) > 0.1 {
		t.Errorf("expected combined score ~%.2f, got %.4f", expected, score)
	}
}

func TestScoringUnknownType(t *testing.T) {
	ts := now()
	// Unknown type gets weight 1.0
	score := search.Score(1.0, ts, "unknown_type", 0)
	if math.Abs(score-1.0) > 0.05 {
		t.Errorf("expected score ~1.0 for unknown type, got %f", score)
	}
}

// ---------- Plans search test ----------

func TestFTS5SearchPlans(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	insertFeature(t, db, "f1", "planning-feature")
	insertPlan(t, db, "p1", "f1", "Implement User Authentication", "Build JWT-based auth system with refresh tokens", now())

	engine := search.NewEngine(db)
	results, err := engine.Search("authentication", "all_features", []string{"plans"}, "", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one plan result")
	}
	if results[0].Type != "plan" {
		t.Errorf("expected type 'plan', got '%s'", results[0].Type)
	}
}

// ---------- Facts search test ----------

func TestFTS5SearchFacts(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	insertFeature(t, db, "f1", "fact-feature")
	insertFact(t, db, "fact1", "f1", "database", "uses", "PostgreSQL version 15", now())

	engine := search.NewEngine(db)
	results, err := engine.Search("PostgreSQL", "all_features", []string{"facts"}, "", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one fact result")
	}
	if results[0].Type != "fact" {
		t.Errorf("expected type 'fact', got '%s'", results[0].Type)
	}
}

// ---------- Limit test ----------

func TestSearchRespectsLimit(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	insertFeature(t, db, "f1", "test-feature")
	for i := 0; i < 10; i++ {
		insertNote(t, db, fmt.Sprintf("n%d", i), "f1",
			fmt.Sprintf("Performance optimization technique number %d for the system", i),
			"progress", now())
	}

	engine := search.NewEngine(db)
	results, err := engine.Search("optimization", "all_features", []string{"notes"}, "", 3)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) > 3 {
		t.Errorf("expected at most 3 results, got %d", len(results))
	}
}

// ---------- Empty query test ----------

func TestSearchEmptyQueryReturnsNil(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	insertFeature(t, db, "f1", "test-feature")
	insertNote(t, db, "n1", "f1", "Some content about databases", "progress", now())

	engine := search.NewEngine(db)
	// An empty query string cannot match anything in FTS5 or trigram.
	results, err := engine.Search("", "all_features", []string{"notes"}, "", 10)
	// The engine may return an error or nil results; either is acceptable.
	if err == nil && len(results) > 0 {
		t.Errorf("expected empty or nil results for empty query, got %d results", len(results))
	}
}

// ---------- Search across multiple features with scope=all_features ----------

func TestSearchAcrossMultipleFeaturesAllScope(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	insertFeature(t, db, "f1", "feature-alpha")
	insertFeature(t, db, "f2", "feature-beta")
	insertFeature(t, db, "f3", "feature-gamma")

	insertNote(t, db, "n1", "f1", "Logging infrastructure for microservices", "progress", now())
	insertNote(t, db, "n2", "f2", "Logging framework setup for analytics", "progress", now())
	insertNote(t, db, "n3", "f3", "Logging pipeline configuration completed", "progress", now())

	engine := search.NewEngine(db)
	results, err := engine.Search("logging", "all_features", []string{"notes"}, "", 20)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results across 3 features, got %d", len(results))
	}

	// Verify results come from different features
	featureNames := make(map[string]bool)
	for _, r := range results {
		featureNames[r.FeatureName] = true
	}
	if len(featureNames) != 3 {
		t.Errorf("expected results from 3 different features, got %d", len(featureNames))
	}
}

// ---------- Scoring with zero links vs many links ----------

func TestScoringZeroLinksVsManyLinks(t *testing.T) {
	ts := now()
	zeroLinks := search.Score(1.0, ts, "progress", 0)
	manyLinks := search.Score(1.0, ts, "progress", 20)

	if manyLinks <= zeroLinks {
		t.Errorf("many links score (%f) should be higher than zero links score (%f)", manyLinks, zeroLinks)
	}

	// linkBoost(0) = 1.0, linkBoost(20) = 1.0 + 20*0.1 = 3.0
	expectedRatio := 3.0
	actualRatio := manyLinks / zeroLinks
	if math.Abs(actualRatio-expectedRatio) > 0.01 {
		t.Errorf("expected link boost ratio ~%.2f, got %.4f", expectedRatio, actualRatio)
	}
}

// ---------- Search with special characters in query ----------

func TestScoreWithNegativeBM25(t *testing.T) {
	// BM25 values from SQLite are negative; Score expects positive.
	// But if someone passes a negative value, the function should still work.
	ts := now()
	score := search.Score(-2.0, ts, "note", 0)
	// Negative bm25 * positive factors = negative score
	if score >= 0 {
		t.Errorf("negative BM25 should produce negative score, got %f", score)
	}
}

func TestScoreWithVeryOldDate(t *testing.T) {
	// 365 days ago - very high decay
	oldDate := daysAgo(365)
	score := search.Score(1.0, oldDate, "note", 0)
	// At 365 days, decay should be very small
	if score > 0.01 {
		t.Errorf("expected very small score for 365-day-old item, got %f", score)
	}
}

func TestScoreWithInvalidDate(t *testing.T) {
	// Invalid date should default to decay=1.0
	score := search.Score(1.0, "not-a-date", "note", 0)
	// typeWeight("note") = 0.5, linkBoost(0) = 1.0, decay = 1.0
	expected := 1.0 * 1.0 * 0.5 * 1.0
	if math.Abs(score-expected) > 0.05 {
		t.Errorf("expected score ~%.2f for invalid date, got %f", expected, score)
	}
}

func TestSearchWithLimitOne(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	insertFeature(t, db, "f1", "test-feature")
	for i := 0; i < 5; i++ {
		insertNote(t, db, fmt.Sprintf("n%d", i), "f1",
			fmt.Sprintf("Database optimization technique %d", i), "progress", now())
	}

	engine := search.NewEngine(db)
	results, err := engine.Search("database", "all_features", []string{"notes"}, "", 1)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected exactly 1 result with limit=1, got %d", len(results))
	}
}

func TestSearch_EmptyTypesSearchesAll(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	insertFeature(t, db, "f1", "all-types-feat")
	insertNote(t, db, "n1", "f1", "searching all types note content", "progress", now())
	insertCommit(t, db, "c1", "f1", "h111", "searching all types commit", "feature", now())

	engine := search.NewEngine(db)
	results, err := engine.Search("searching", "all_features", nil, "", 20)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	types := map[string]bool{}
	for _, r := range results {
		types[r.Type] = true
	}
	if !types["note"] || !types["commit"] {
		t.Errorf("empty types should search all, got types: %v", types)
	}
}

func TestSearch_SingleTypeFilter(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	insertFeature(t, db, "f1", "single-type-feat")
	insertNote(t, db, "n1", "f1", "filtering single type content", "progress", now())
	insertCommit(t, db, "c1", "f1", "h222", "filtering single type commit", "feature", now())

	engine := search.NewEngine(db)
	results, err := engine.Search("filtering", "all_features", []string{"commits"}, "", 20)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	for _, r := range results {
		if r.Type != "commit" {
			t.Errorf("expected only commit type, got %s", r.Type)
		}
	}
}

func TestTraverseLinks_MaxDepthZeroReturnsMinDepthOne(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	insertFeature(t, db, "f1", "depth-zero")
	insertNote(t, db, "n1", "f1", "root", "note", now())
	insertNote(t, db, "n2", "f1", "linked", "note", now())
	insertLink(t, db, "l1", "n1", "note", "n2", "note", "related", 0.8)

	engine := search.NewEngine(db)
	// maxDepth=0 is clamped to 1 in the implementation
	linked, err := engine.TraverseLinks("n1", "note", 0)
	if err != nil {
		t.Fatalf("TraverseLinks: %v", err)
	}
	// Should find n2 at depth 1 (maxDepth clamped to 1)
	if len(linked) != 1 {
		t.Errorf("expected 1 linked memory (clamped to depth 1), got %d", len(linked))
	}
}

func TestScore_AllZeroValues(t *testing.T) {
	score := search.Score(0.0, now(), "note", 0)
	if score != 0.0 {
		t.Errorf("expected 0.0 for zero BM25, got %f", score)
	}
}

func TestScore_TypeWeights(t *testing.T) {
	ts := now()
	types := []struct {
		name   string
		weight float64
	}{
		{"decision", 2.0}, {"blocker", 1.5}, {"feature", 1.2},
		{"progress", 1.0}, {"next_step", 1.0}, {"note", 0.5},
	}
	for _, tc := range types {
		t.Run(tc.name, func(t *testing.T) {
			score := search.Score(1.0, ts, tc.name, 0)
			expected := tc.weight // bm25=1.0 * decay~1.0 * weight * linkBoost=1.0
			if math.Abs(score-expected) > 0.05 {
				t.Errorf("Score(1.0, now, %q, 0) = %f, want ~%f", tc.name, score, expected)
			}
		})
	}
}

func TestSearch_QueryPatterns(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	insertFeature(t, db, "f1", "qp-feat")
	insertNote(t, db, "n1", "f1", "Using PostgreSQL for relational data storage", "decision", now())
	insertNote(t, db, "n2", "f1", "Redis caching layer for session tokens", "progress", now())
	insertNote(t, db, "n3", "f1", "GraphQL API endpoint for user queries", "note", now())
	engine := search.NewEngine(db)
	patterns := []struct {
		query   string
		wantMin int
	}{
		{"PostgreSQL", 1}, {"caching session", 1}, {"API", 1},
	}
	for _, tc := range patterns {
		t.Run(tc.query, func(t *testing.T) {
			results, err := engine.Search(tc.query, "all_features", []string{"notes"}, "", 10)
			if err != nil {
				t.Fatalf("Search(%q): %v", tc.query, err)
			}
			if len(results) < tc.wantMin {
				t.Errorf("Search(%q) got %d results, want >= %d", tc.query, len(results), tc.wantMin)
			}
		})
	}
}

func TestSearchType_NotesOnly(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	insertFeature(t, db, "f1", "search-feat")
	insertNote(t, db, "n1", "f1", "Authentication module completed", "decision", now())
	insertCommit(t, db, "c1", "f1", "abc", "Authentication commit", "feature", now())
	engine := search.NewEngine(db)
	results, err := engine.Search("authentication", "all_features", []string{"notes"}, "", 10)
	if err != nil { t.Fatalf("Search: %v", err) }
	for _, r := range results {
		if r.Type != "note" { t.Errorf("expected note, got %s", r.Type) }
	}
}

func TestSearchType_CommitsOnly(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	insertFeature(t, db, "f1", "search-feat")
	insertNote(t, db, "n1", "f1", "Authentication module completed", "decision", now())
	insertCommit(t, db, "c1", "f1", "abc", "Authentication commit", "feature", now())
	engine := search.NewEngine(db)
	results, err := engine.Search("authentication", "all_features", []string{"commits"}, "", 10)
	if err != nil { t.Fatalf("Search: %v", err) }
	for _, r := range results {
		if r.Type != "commit" { t.Errorf("expected commit, got %s", r.Type) }
	}
}

func TestSearchType_FactsOnly(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	insertFeature(t, db, "f1", "search-feat")
	insertFact(t, db, "fa1", "f1", "auth", "uses", "JWT authentication", now())
	engine := search.NewEngine(db)
	results, err := engine.Search("authentication", "all_features", []string{"facts"}, "", 10)
	if err != nil { t.Fatalf("Search: %v", err) }
	for _, r := range results {
		if r.Type != "fact" { t.Errorf("expected fact, got %s", r.Type) }
	}
}

func TestSearchType_PlansOnly(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	insertFeature(t, db, "f1", "search-feat")
	insertPlan(t, db, "p1", "f1", "Authentication plan", "Plan for authentication system", now())
	engine := search.NewEngine(db)
	results, err := engine.Search("authentication", "all_features", []string{"plans"}, "", 10)
	if err != nil { t.Fatalf("Search: %v", err) }
	for _, r := range results {
		if r.Type != "plan" { t.Errorf("expected plan, got %s", r.Type) }
	}
}

func TestSearchTypes(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	insertFeature(t, db, "f1", "search-types-feat")
	ts := now()
	insertNote(t, db, "n1", "f1", "Authentication module completed", "decision", ts)
	insertCommit(t, db, "c1", "f1", "abc123", "Authentication commit message", "feature", ts)
	insertFact(t, db, "fa1", "f1", "auth", "uses", "JWT authentication", ts)
	insertPlan(t, db, "p1", "f1", "Authentication plan", "Plan for authentication system", ts)
	engine := search.NewEngine(db)
	for _, tc := range []struct{ name string; types []string; wantType string }{
		{"notes_only", []string{"notes"}, "note"},
		{"commits_only", []string{"commits"}, "commit"},
		{"facts_only", []string{"facts"}, "fact"},
		{"plans_only", []string{"plans"}, "plan"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			results, err := engine.Search("authentication", "all_features", tc.types, "", 10)
			if err != nil { t.Fatalf("Search(%v): %v", tc.types, err) }
			if len(results) == 0 { t.Fatalf("expected results, got none") }
			for _, r := range results {
				if r.Type != tc.wantType { t.Errorf("got %s, want %s", r.Type, tc.wantType) }
			}
		})
	}
}

func TestSearchSpecialCharactersInQuery(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	insertFeature(t, db, "f1", "test-feature")
	insertNote(t, db, "n1", "f1", "Fixed the authentication (OAuth2) flow in production", "progress", now())
	insertNote(t, db, "n2", "f1", `Updated the config: "max_retries" = 5`, "progress", now())

	engine := search.NewEngine(db)

	// Parentheses in query should not cause FTS5 syntax errors
	results, err := engine.Search("authentication (OAuth2)", "all_features", []string{"notes"}, "", 10)
	if err != nil {
		t.Fatalf("Search with parentheses: %v", err)
	}
	if len(results) == 0 {
		t.Error("expected at least one result for query with parentheses")
	}

	// Quotes in query should not cause errors
	results, err = engine.Search(`"max_retries"`, "all_features", []string{"notes"}, "", 10)
	if err != nil {
		t.Fatalf("Search with quotes: %v", err)
	}
	// Should either find results or return empty without error
	_ = results

	// Colons in query should not cause FTS5 syntax errors
	results, err = engine.Search("config: max_retries", "all_features", []string{"notes"}, "", 10)
	if err != nil {
		t.Fatalf("Search with colon: %v", err)
	}
	_ = results
}

// ---------- History / SearchWithOpts tests ----------

func TestHistorySearchFindsNotesFromDaysAgo(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	insertFeature(t, db, "f1", "history-feature")
	insertNote(t, db, "n1", "f1", "Set up authentication service for user login", "progress", daysAgo(7))
	insertNote(t, db, "n2", "f1", "Updated authentication to support OAuth", "progress", now())

	engine := search.NewEngine(db)
	since := time.Now().UTC().AddDate(0, 0, -30)
	opts := &search.SearchOpts{Since: &since}
	results, err := engine.SearchWithOpts("authentication", "all_features", []string{"notes"}, "", 20, opts)
	if err != nil {
		t.Fatalf("SearchWithOpts: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("expected at least 2 results (including 7-day-old note), got %d", len(results))
	}
	// Both notes should be found — the one from 7 days ago and the recent one
	ids := map[string]bool{}
	for _, r := range results {
		ids[r.ID] = true
	}
	if !ids["n1"] {
		t.Error("expected to find note n1 from 7 days ago")
	}
	if !ids["n2"] {
		t.Error("expected to find note n2 from today")
	}
}

func TestHistorySearchRespectsDaysBackFilter(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	insertFeature(t, db, "f1", "filter-feature")
	insertNote(t, db, "n1", "f1", "Old deployment configuration notes", "progress", daysAgo(60))
	insertNote(t, db, "n2", "f1", "Recent deployment configuration update", "progress", daysAgo(5))

	engine := search.NewEngine(db)

	// Search with 10 days back — should only find the recent note
	since10 := time.Now().UTC().AddDate(0, 0, -10)
	opts10 := &search.SearchOpts{Since: &since10}
	results, err := engine.SearchWithOpts("deployment", "all_features", []string{"notes"}, "", 20, opts10)
	if err != nil {
		t.Fatalf("SearchWithOpts (10 days): %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result within 10 days, got %d", len(results))
	}
	if results[0].ID != "n2" {
		t.Errorf("expected result ID=n2, got %s", results[0].ID)
	}

	// Search with 90 days back — should find both notes
	since90 := time.Now().UTC().AddDate(0, 0, -90)
	opts90 := &search.SearchOpts{Since: &since90}
	results, err = engine.SearchWithOpts("deployment", "all_features", []string{"notes"}, "", 20, opts90)
	if err != nil {
		t.Fatalf("SearchWithOpts (90 days): %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results within 90 days, got %d", len(results))
	}
}

func TestHistorySearchReturnsResultsChronologically(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	insertFeature(t, db, "f1", "chrono-feature")
	insertNote(t, db, "n1", "f1", "First migration script completed", "progress", daysAgo(20))
	insertNote(t, db, "n2", "f1", "Second migration script added", "progress", daysAgo(10))
	insertNote(t, db, "n3", "f1", "Third migration script deployed", "progress", daysAgo(1))

	engine := search.NewEngine(db)
	since := time.Now().UTC().AddDate(0, 0, -30)
	opts := &search.SearchOpts{Since: &since}
	results, err := engine.SearchWithOpts("migration", "all_features", []string{"notes"}, "", 20, opts)
	if err != nil {
		t.Fatalf("SearchWithOpts: %v", err)
	}
	if len(results) < 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Sort chronologically and verify order
	search.SortByTimeAsc(results)
	for i := 1; i < len(results); i++ {
		if results[i].CreatedAt < results[i-1].CreatedAt {
			t.Errorf("results not in chronological order: result[%d].CreatedAt=%s < result[%d].CreatedAt=%s",
				i, results[i].CreatedAt, i-1, results[i-1].CreatedAt)
		}
	}
	// First result should be the oldest
	if results[0].ID != "n1" {
		t.Errorf("expected oldest result first (n1), got %s", results[0].ID)
	}
	if results[len(results)-1].ID != "n3" {
		t.Errorf("expected newest result last (n3), got %s", results[len(results)-1].ID)
	}
}

func TestHistorySearchAcrossMultipleFeatures(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	insertFeature(t, db, "f1", "api-feature")
	insertFeature(t, db, "f2", "frontend-feature")
	insertNote(t, db, "n1", "f1", "Implemented API caching with Redis", "progress", daysAgo(14))
	insertNote(t, db, "n2", "f2", "Added frontend caching for static assets", "progress", daysAgo(7))
	insertCommit(t, db, "c1", "f1", "abc123", "Add caching layer to API endpoints", "feature", daysAgo(13))

	engine := search.NewEngine(db)
	since := time.Now().UTC().AddDate(0, 0, -30)
	opts := &search.SearchOpts{Since: &since}
	results, err := engine.SearchWithOpts("caching", "all_features", nil, "", 20, opts)
	if err != nil {
		t.Fatalf("SearchWithOpts: %v", err)
	}
	if len(results) < 3 {
		t.Fatalf("expected at least 3 results across features, got %d", len(results))
	}

	// Verify results come from multiple features
	features := map[string]bool{}
	for _, r := range results {
		if r.FeatureName != "" {
			features[r.FeatureName] = true
		}
	}
	if !features["api-feature"] {
		t.Error("expected results from api-feature")
	}
	if !features["frontend-feature"] {
		t.Error("expected results from frontend-feature")
	}

	// Verify results span multiple types
	types := map[string]bool{}
	for _, r := range results {
		types[r.Type] = true
	}
	if !types["note"] {
		t.Error("expected note type in results")
	}
	if !types["commit"] {
		t.Error("expected commit type in results")
	}
}
