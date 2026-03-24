package memory_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arbazkhan971/memorx/internal/memory"
	"github.com/arbazkhan971/memorx/internal/storage"
)

func setupMultiStore(t *testing.T) *memory.Store {
	t.Helper()
	db, err := storage.NewDB(filepath.Join(t.TempDir(), "memory.db"))
	if err != nil { t.Fatalf("NewDB: %v", err) }
	t.Cleanup(func() { db.Close() })
	if err := storage.Migrate(db); err != nil { t.Fatalf("Migrate: %v", err) }
	return memory.NewStore(db)
}

func TestGlobalSearch_NoDBs(t *testing.T) {
	results, _ := memory.GlobalSearch("test query")
	if len(results) != 0 { t.Logf("Found %d results", len(results)) }
}

func TestFormatGlobalSearch_Empty(t *testing.T) {
	if r := memory.FormatGlobalSearch(nil); !strings.Contains(r, "No results") { t.Errorf("expected no-results, got %q", r) }
}

func TestFormatGlobalSearch_WithResults(t *testing.T) {
	results := []memory.GlobalSearchResult{
		{ProjectName: "proj-a", Type: "note", Content: "test content", CreatedAt: "2024-01-01"},
		{ProjectName: "proj-b", Type: "commit", Content: "add auth", CreatedAt: "2024-01-02"},
	}
	r := memory.FormatGlobalSearch(results)
	if !strings.Contains(r, "proj-a") || !strings.Contains(r, "proj-b") { t.Error("expected project names") }
}

func TestDetectPatterns_NoDBs(t *testing.T) {
	patterns, _ := memory.DetectPatterns()
	if len(patterns) != 0 { t.Logf("Found %d patterns", len(patterns)) }
}

func TestFormatPatterns_Empty(t *testing.T) {
	if r := memory.FormatPatterns(nil); !strings.Contains(r, "No cross-project") { t.Errorf("got %q", r) }
}

func TestSaveAndApplyTemplate(t *testing.T) {
	store := setupMultiStore(t)
	f, _ := store.CreateFeature("template-test", "")
	store.CreateNote(f.ID, "", "Use DI", "decision")
	store.CreateFact(f.ID, "", "auth", "uses", "better-auth")
	name := "test-tpl-" + t.Name()
	if err := store.SaveTemplate(f.ID, name); err != nil { t.Fatalf("SaveTemplate: %v", err) }
	home, _ := os.UserHomeDir()
	path := filepath.Join(home, ".memorx", "templates", name+".json")
	t.Cleanup(func() { os.Remove(path) })
	data, err := os.ReadFile(path)
	if err != nil { t.Fatalf("read: %v", err) }
	var td memory.TemplateData
	json.Unmarshal(data, &td)
	if len(td.Decisions) == 0 { t.Error("expected decisions") }
	if len(td.Facts) == 0 { t.Error("expected facts") }
	f2, _ := store.CreateFeature("apply-test", "")
	applied, err := store.ApplyTemplate(f2.ID, "", name)
	if err != nil { t.Fatalf("ApplyTemplate: %v", err) }
	if len(applied.Decisions) == 0 { t.Error("expected decisions applied") }
}

func TestLinkProject(t *testing.T) {
	store := setupMultiStore(t)
	dir := t.TempDir()
	lp, err := store.LinkProject(dir, "related")
	if err != nil { t.Fatalf("LinkProject: %v", err) }
	if lp.Relationship != "related" { t.Errorf("got %q", lp.Relationship) }
	all, _ := store.ListLinkedProjects()
	if len(all) != 1 { t.Fatalf("expected 1, got %d", len(all)) }
}

func TestLinkProject_InvalidPath(t *testing.T) {
	store := setupMultiStore(t)
	if _, err := store.LinkProject("/nonexistent/path/xyz", ""); err == nil { t.Error("expected error") }
}

func TestLinkProject_DefaultRelationship(t *testing.T) {
	store := setupMultiStore(t)
	lp, _ := store.LinkProject(t.TempDir(), "")
	if lp.Relationship != "related" { t.Errorf("got %q", lp.Relationship) }
}
