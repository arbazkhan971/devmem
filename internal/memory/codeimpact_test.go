package memory_test

import (
	"strings"
	"testing"

	"github.com/arbazkhan971/memorx/internal/memory"
)

func TestCodeImpact_EmptyDB(t *testing.T) {
	store := newTestStore(t)

	result, err := store.CodeImpact("nonexistent/file.go")
	if err != nil {
		t.Fatalf("CodeImpact: %v", err)
	}
	if len(result.Features) != 0 {
		t.Errorf("expected 0 features, got %d", len(result.Features))
	}
	if len(result.Notes) != 0 {
		t.Errorf("expected 0 notes, got %d", len(result.Notes))
	}
	if len(result.Dependencies) != 0 {
		t.Errorf("expected 0 dependencies, got %d", len(result.Dependencies))
	}
}

func TestCodeImpact_FindsFeatures(t *testing.T) {
	store := newTestStore(t)

	f1, _ := store.CreateFeature("ci-feat-1", "Feature 1")
	f2, _ := store.CreateFeature("ci-feat-2", "Feature 2")

	s1, _ := store.CreateSession(f1.ID, "test")
	s2, _ := store.CreateSession(f2.ID, "test")

	// Both features touch the same file
	store.TrackFile(f1.ID, s1.ID, "auth/middleware.go", "modified")
	store.TrackFile(f2.ID, s2.ID, "auth/middleware.go", "modified")

	result, err := store.CodeImpact("auth/middleware.go")
	if err != nil {
		t.Fatalf("CodeImpact: %v", err)
	}
	if len(result.Features) != 2 {
		t.Errorf("expected 2 features, got %d", len(result.Features))
	}
}

func TestCodeImpact_FindsNotes(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("ci-notes", "Notes test")

	// Create notes referencing the file
	store.CreateNote(f.ID, "", "Updated auth/middleware.go to add JWT validation", "decision")
	store.CreateNote(f.ID, "", "auth/middleware.go needs rate limiting", "note")
	store.CreateNote(f.ID, "", "Unrelated note about database", "note")

	result, err := store.CodeImpact("auth/middleware.go")
	if err != nil {
		t.Fatalf("CodeImpact: %v", err)
	}
	if len(result.Notes) < 2 {
		t.Errorf("expected at least 2 notes referencing the file, got %d", len(result.Notes))
	}
}

func TestCodeImpact_FindsDependencies(t *testing.T) {
	store, db := newTestStoreWithDB(t)

	f, _ := store.CreateFeature("ci-deps", "Deps test")

	// Create commits that show files changing together
	insertTestCommitWithFiles(t, db, f.ID, "hash1", "feat: update auth",
		[]map[string]string{
			{"Path": "auth/middleware.go", "Action": "modified"},
			{"Path": "auth/jwt.go", "Action": "modified"},
		})
	insertTestCommitWithFiles(t, db, f.ID, "hash2", "feat: auth fix",
		[]map[string]string{
			{"Path": "auth/middleware.go", "Action": "modified"},
			{"Path": "auth/jwt.go", "Action": "modified"},
			{"Path": "auth/config.go", "Action": "modified"},
		})

	result, err := store.CodeImpact("auth/middleware.go")
	if err != nil {
		t.Fatalf("CodeImpact: %v", err)
	}
	if len(result.Dependencies) < 1 {
		t.Errorf("expected at least 1 dependency, got %d", len(result.Dependencies))
	}
	// jwt.go should be the top dependency
	if len(result.Dependencies) > 0 && result.Dependencies[0].Path != "auth/jwt.go" {
		t.Errorf("expected top dependency to be auth/jwt.go, got %s", result.Dependencies[0].Path)
	}
}

func TestCodeImpact_FindsNotesByFilename(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("ci-filename", "Filename test")

	// Note references only the filename, not the full path
	store.CreateNote(f.ID, "", "The middleware.go file handles all request interceptors", "note")

	result, err := store.CodeImpact("internal/auth/middleware.go")
	if err != nil {
		t.Fatalf("CodeImpact: %v", err)
	}
	// Should find notes matching the filename
	if len(result.Notes) < 1 {
		t.Errorf("expected at least 1 note matching filename, got %d", len(result.Notes))
	}
}

func TestCodeImpact_CompleteAnalysis(t *testing.T) {
	store, db := newTestStoreWithDB(t)

	f, _ := store.CreateFeature("ci-complete", "Complete test")
	s, _ := store.CreateSession(f.ID, "test")

	// Track files
	store.TrackFile(f.ID, s.ID, "auth/middleware.go", "modified")

	// Create related notes
	store.CreateNote(f.ID, "", "Decided to use auth/middleware.go for request validation", "decision")
	store.CreateNote(f.ID, "", "auth/middleware.go performance is critical", "note")

	// Create commits showing co-changes
	insertTestCommitWithFiles(t, db, f.ID, "hash1", "update",
		[]map[string]string{
			{"Path": "auth/middleware.go", "Action": "modified"},
			{"Path": "auth/handler.go", "Action": "modified"},
		})

	result, err := store.CodeImpact("auth/middleware.go")
	if err != nil {
		t.Fatalf("CodeImpact: %v", err)
	}

	if result.FilePath != "auth/middleware.go" {
		t.Errorf("expected file path 'auth/middleware.go', got %q", result.FilePath)
	}
	if len(result.Features) < 1 {
		t.Errorf("expected at least 1 feature, got %d", len(result.Features))
	}
	if len(result.Notes) < 2 {
		t.Errorf("expected at least 2 notes, got %d", len(result.Notes))
	}
	if len(result.Dependencies) < 1 {
		t.Errorf("expected at least 1 dependency, got %d", len(result.Dependencies))
	}
}

func TestFormatCodeImpact_Nil(t *testing.T) {
	out := memory.FormatCodeImpact(nil)
	if out != "No code impact result." {
		t.Errorf("expected nil message, got %q", out)
	}
}

func TestFormatCodeImpact_WithContent(t *testing.T) {
	result := &memory.CodeImpactResult{
		FilePath: "auth/middleware.go",
		Features: []memory.CodeImpactFeature{
			{Name: "auth-system", Status: "active"},
			{Name: "rate-limiting", Status: "paused"},
			{Name: "api-v2", Status: "done"},
		},
		Notes: []memory.Note{
			{ID: "1", Content: "Decided to add JWT validation in middleware", Type: "decision"},
			{ID: "2", Content: "Middleware handles rate limiting", Type: "note"},
			{ID: "3", Content: "Performance optimization for middleware", Type: "note"},
			{ID: "4", Content: "Auth flow goes through middleware first", Type: "decision"},
			{ID: "5", Content: "Middleware logging added", Type: "progress"},
		},
		Dependencies: []memory.FileDependency{
			{Path: "auth/jwt.go", Occurrences: 5},
			{Path: "auth/config.go", Occurrences: 2},
		},
	}

	out := memory.FormatCodeImpact(result)
	if !strings.Contains(out, "auth/middleware.go") {
		t.Errorf("expected file path in output")
	}
	if !strings.Contains(out, "touched by 3 features") {
		t.Errorf("expected feature count in output, got: %s", out)
	}
	if !strings.Contains(out, "5 notes reference it") {
		t.Errorf("expected note count in output, got: %s", out)
	}
	if !strings.Contains(out, "2 dependencies") {
		t.Errorf("expected dependency count in output, got: %s", out)
	}
	if !strings.Contains(out, "auth-system") {
		t.Errorf("expected feature name in output")
	}
	if !strings.Contains(out, "2 decisions") {
		t.Errorf("expected decision count in output, got: %s", out)
	}
}

func TestFormatCodeImpact_Empty(t *testing.T) {
	result := &memory.CodeImpactResult{
		FilePath: "unknown.go",
	}
	out := memory.FormatCodeImpact(result)
	if !strings.Contains(out, "unknown.go") {
		t.Errorf("expected file path in output")
	}
	if !strings.Contains(out, "touched by 0 features") {
		t.Errorf("expected 0 features, got: %s", out)
	}
}
