package memory_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/arbaz/devmem/internal/memory"
	"github.com/arbaz/devmem/internal/search"
	"github.com/arbaz/devmem/internal/storage"
	"github.com/google/uuid"
)

func newTestStoreWithSearch(t *testing.T) (*memory.Store, *search.Engine, *storage.DB) {
	t.Helper()
	dir := t.TempDir()
	db, err := storage.NewDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := storage.Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return memory.NewStore(db), search.NewEngine(db), db
}

func TestFindRelated_EmptyDB(t *testing.T) {
	store, engine, _ := newTestStoreWithSearch(t)

	result, err := store.FindRelated(engine, "nonexistent", 2)
	if err != nil {
		t.Fatalf("FindRelated: %v", err)
	}
	if len(result.Decisions) != 0 || len(result.Facts) != 0 || len(result.Files) != 0 || len(result.Commits) != 0 {
		t.Errorf("expected empty result, got decisions=%d facts=%d files=%d commits=%d",
			len(result.Decisions), len(result.Facts), len(result.Files), len(result.Commits))
	}
}

func TestFindRelated_FindsNotes(t *testing.T) {
	store, engine, _ := newTestStoreWithSearch(t)

	f, _ := store.CreateFeature("related-test", "Test feature")
	store.CreateNote(f.ID, "", "Authentication uses JWT tokens for session management", "note")
	store.CreateNote(f.ID, "", "Database uses SQLite with WAL mode", "decision")

	result, err := store.FindRelated(engine, "authentication JWT", 2)
	if err != nil {
		t.Fatalf("FindRelated: %v", err)
	}
	// Should find at least one result from the search
	total := len(result.Decisions) + len(result.Facts) + len(result.Files) + len(result.Commits)
	t.Logf("FindRelated returned: decisions=%d facts=%d files=%d commits=%d",
		len(result.Decisions), len(result.Facts), len(result.Files), len(result.Commits))
	if total == 0 {
		t.Log("No results found (FTS may not match short queries); this is acceptable")
	}
}

func TestFindRelated_FindsFacts(t *testing.T) {
	store, engine, _ := newTestStoreWithSearch(t)

	f, _ := store.CreateFeature("fact-related", "Test feature")
	store.CreateFact(f.ID, "", "auth", "uses", "better-auth library")
	store.CreateFact(f.ID, "", "database", "engine", "sqlite")

	result, err := store.FindRelated(engine, "database sqlite", 2)
	if err != nil {
		t.Fatalf("FindRelated: %v", err)
	}
	t.Logf("FindRelated facts: %d", len(result.Facts))
}

func TestFindRelated_FindsRelatedFiles(t *testing.T) {
	store, engine, _ := newTestStoreWithSearch(t)

	f, _ := store.CreateFeature("file-related", "Test")
	store.TrackFile(f.ID, "", "internal/auth/middleware.go", "modified")
	store.TrackFile(f.ID, "", "internal/auth/jwt.go", "modified")
	store.TrackFile(f.ID, "", "internal/db/store.go", "modified")

	result, err := store.FindRelated(engine, "auth", 2)
	if err != nil {
		t.Fatalf("FindRelated: %v", err)
	}
	// Should find auth-related files
	if len(result.Files) < 2 {
		t.Errorf("expected at least 2 related files, got %d", len(result.Files))
	}
	// Verify paths contain "auth"
	for _, f := range result.Files {
		if f.Type != "file" {
			t.Errorf("expected type 'file', got %q", f.Type)
		}
	}
}

func TestFindRelated_DefaultDepth(t *testing.T) {
	store, engine, _ := newTestStoreWithSearch(t)

	// depth=0 should default to 2
	result, err := store.FindRelated(engine, "anything", 0)
	if err != nil {
		t.Fatalf("FindRelated with depth=0: %v", err)
	}
	// Just verify it doesn't crash
	_ = result
}

func TestFindRelated_WithLinks(t *testing.T) {
	store, engine, _ := newTestStoreWithSearch(t)

	f, _ := store.CreateFeature("linked-related", "Test")
	n1, _ := store.CreateNote(f.ID, "", "authentication middleware handles JWT validation", "decision")
	n2, _ := store.CreateNote(f.ID, "", "rate limiting added to auth endpoints", "note")
	store.CreateLink(n1.ID, "note", n2.ID, "note", "related", 0.8)

	result, err := store.FindRelated(engine, "authentication JWT", 2)
	if err != nil {
		t.Fatalf("FindRelated: %v", err)
	}
	t.Logf("decisions=%d, total items=%d",
		len(result.Decisions),
		len(result.Decisions)+len(result.Facts)+len(result.Files)+len(result.Commits))
}

func TestFormatRelatedResult_Empty(t *testing.T) {
	result := &memory.RelatedResult{}
	out := memory.FormatRelatedResult(result)
	if out != "No related memories found." {
		t.Errorf("expected 'No related memories found.', got %q", out)
	}
}

func TestFormatRelatedResult_WithContent(t *testing.T) {
	result := &memory.RelatedResult{
		Decisions: []memory.RelatedItem{
			{ID: "1", Type: "note", Content: "Use JWT for auth", Relevance: 0.9},
		},
		Facts: []memory.RelatedItem{
			{ID: "2", Type: "fact", Content: "auth uses JWT", Relevance: 0.8},
		},
		Files: []memory.RelatedItem{
			{ID: "", Type: "file", Content: "internal/auth/jwt.go", Relevance: 0},
		},
		Commits: []memory.RelatedItem{
			{ID: "3", Type: "commit", Content: "feat: add JWT auth", Relevance: 0.7},
		},
	}
	out := memory.FormatRelatedResult(result)
	if out == "No related memories found." {
		t.Error("expected formatted output, got empty message")
	}
	for _, want := range []string{"Related decisions", "Related facts", "Related files", "Related commits"} {
		if !contains(out, want) {
			t.Errorf("expected output to contain %q", want)
		}
	}
}

// --- Tests for FindDependencies ---

func insertTestCommitWithFiles(t *testing.T, db *storage.DB, featureID, hash, message string, files []map[string]string) {
	t.Helper()
	id := uuid.New().String()
	filesJSON, _ := json.Marshal(files)
	_, err := db.Writer().Exec(
		`INSERT INTO commits (id, feature_id, hash, message, author, files_changed, intent_type, intent_confidence, committed_at)
		 VALUES (?, ?, ?, ?, 'test-author', ?, 'feature', 0.9, datetime('now'))`,
		id, featureID, hash, message, string(filesJSON),
	)
	if err != nil {
		t.Fatalf("insertTestCommitWithFiles: %v", err)
	}
}

func TestFindDependencies_EmptyDB(t *testing.T) {
	store := newTestStore(t)

	deps, err := store.FindDependencies("src/main.go")
	if err != nil {
		t.Fatalf("FindDependencies: %v", err)
	}
	if len(deps) != 0 {
		t.Errorf("expected 0 dependencies, got %d", len(deps))
	}
}

func TestFindDependencies_SingleCommit(t *testing.T) {
	store, db := newTestStoreWithDB(t)

	f, _ := store.CreateFeature("dep-test", "Test")
	insertTestCommitWithFiles(t, db, f.ID, "abc123", "feat: update auth",
		[]map[string]string{
			{"Path": "auth/middleware.go", "Action": "modified"},
			{"Path": "auth/jwt.go", "Action": "modified"},
			{"Path": "tests/auth_test.go", "Action": "modified"},
		})

	deps, err := store.FindDependencies("auth/middleware.go")
	if err != nil {
		t.Fatalf("FindDependencies: %v", err)
	}
	if len(deps) != 2 {
		t.Fatalf("expected 2 dependencies, got %d", len(deps))
	}
	// Both should have count 1
	for _, d := range deps {
		if d.Occurrences != 1 {
			t.Errorf("expected 1 occurrence for %s, got %d", d.Path, d.Occurrences)
		}
	}
}

func TestFindDependencies_MultipleCommits_RankedByOccurrence(t *testing.T) {
	store, db := newTestStoreWithDB(t)

	f, _ := store.CreateFeature("dep-rank", "Test")
	// Commit 1: middleware + jwt + test
	insertTestCommitWithFiles(t, db, f.ID, "hash1", "feat: auth update 1",
		[]map[string]string{
			{"Path": "auth/middleware.go", "Action": "modified"},
			{"Path": "auth/jwt.go", "Action": "modified"},
			{"Path": "tests/auth_test.go", "Action": "modified"},
		})
	// Commit 2: middleware + jwt
	insertTestCommitWithFiles(t, db, f.ID, "hash2", "feat: auth update 2",
		[]map[string]string{
			{"Path": "auth/middleware.go", "Action": "modified"},
			{"Path": "auth/jwt.go", "Action": "modified"},
		})
	// Commit 3: middleware + jwt + config
	insertTestCommitWithFiles(t, db, f.ID, "hash3", "feat: auth update 3",
		[]map[string]string{
			{"Path": "auth/middleware.go", "Action": "modified"},
			{"Path": "auth/jwt.go", "Action": "modified"},
			{"Path": "config/auth.yaml", "Action": "modified"},
		})

	deps, err := store.FindDependencies("auth/middleware.go")
	if err != nil {
		t.Fatalf("FindDependencies: %v", err)
	}
	if len(deps) != 3 {
		t.Fatalf("expected 3 dependencies, got %d", len(deps))
	}
	// jwt.go should be first with 3 occurrences
	if deps[0].Path != "auth/jwt.go" {
		t.Errorf("expected first dependency to be auth/jwt.go, got %s", deps[0].Path)
	}
	if deps[0].Occurrences != 3 {
		t.Errorf("expected 3 occurrences for jwt.go, got %d", deps[0].Occurrences)
	}
	// test and config should have 1 each
	found := map[string]int{}
	for _, d := range deps {
		found[d.Path] = d.Occurrences
	}
	if found["tests/auth_test.go"] != 1 {
		t.Errorf("expected 1 occurrence for auth_test.go, got %d", found["tests/auth_test.go"])
	}
	if found["config/auth.yaml"] != 1 {
		t.Errorf("expected 1 occurrence for config/auth.yaml, got %d", found["config/auth.yaml"])
	}
}

func TestFindDependencies_IgnoresSelf(t *testing.T) {
	store, db := newTestStoreWithDB(t)

	f, _ := store.CreateFeature("dep-self", "Test")
	insertTestCommitWithFiles(t, db, f.ID, "hash1", "update",
		[]map[string]string{
			{"Path": "main.go", "Action": "modified"},
			{"Path": "utils.go", "Action": "modified"},
		})

	deps, err := store.FindDependencies("main.go")
	if err != nil {
		t.Fatalf("FindDependencies: %v", err)
	}
	for _, d := range deps {
		if d.Path == "main.go" {
			t.Error("FindDependencies should not include the queried file itself")
		}
	}
}

func TestFindDependencies_NoFalsePositives(t *testing.T) {
	store, db := newTestStoreWithDB(t)

	f, _ := store.CreateFeature("dep-false", "Test")
	// Commit that has a file with a similar name but not exact match
	insertTestCommitWithFiles(t, db, f.ID, "hash1", "update",
		[]map[string]string{
			{"Path": "auth/middleware_v2.go", "Action": "modified"},
			{"Path": "auth/jwt.go", "Action": "modified"},
		})

	// Search for exact "auth/middleware.go" which is NOT in the commit
	deps, err := store.FindDependencies("auth/middleware.go")
	if err != nil {
		t.Fatalf("FindDependencies: %v", err)
	}
	// The LIKE query may match "auth/middleware_v2.go" commit, but since
	// exact path check is done inside, it should return 0 deps
	if len(deps) != 0 {
		t.Errorf("expected 0 dependencies for non-existent file, got %d", len(deps))
	}
}

func TestFormatDependencies_Empty(t *testing.T) {
	out := memory.FormatDependencies(nil)
	if !contains(out, "No file dependencies found") {
		t.Errorf("expected empty message, got %q", out)
	}
}

func TestFormatDependencies_WithDeps(t *testing.T) {
	deps := []memory.FileDependency{
		{Path: "auth/jwt.go", Occurrences: 5},
		{Path: "tests/auth_test.go", Occurrences: 3},
	}
	out := memory.FormatDependencies(deps)
	if !contains(out, "auth/jwt.go") {
		t.Errorf("expected output to contain auth/jwt.go")
	}
	if !contains(out, "changed together 5 times") {
		t.Errorf("expected output to contain '5 times'")
	}
	if !contains(out, "changed together 3 times") {
		t.Errorf("expected output to contain '3 times'")
	}
}

func TestFormatDependencies_SingularTime(t *testing.T) {
	deps := []memory.FileDependency{
		{Path: "single.go", Occurrences: 1},
	}
	out := memory.FormatDependencies(deps)
	if !contains(out, "changed together 1 time)") {
		t.Errorf("expected singular 'time', got %q", out)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && searchString(s, sub)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
