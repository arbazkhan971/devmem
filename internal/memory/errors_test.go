package memory_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/arbazkhan971/memorx/internal/memory"
	"github.com/arbazkhan971/memorx/internal/search"
	"github.com/arbazkhan971/memorx/internal/storage"
)

func setupErrorStore(t *testing.T) (*memory.Store, *search.Engine) {
	t.Helper()
	db, err := storage.NewDB(filepath.Join(t.TempDir(), "memory.db"))
	if err != nil { t.Fatalf("NewDB: %v", err) }
	t.Cleanup(func() { db.Close() })
	if err := storage.Migrate(db); err != nil { t.Fatalf("Migrate: %v", err) }
	return memory.NewStore(db), search.NewEngine(db)
}

func TestLogError(t *testing.T) {
	store, _ := setupErrorStore(t)
	f, _ := store.CreateFeature("test-feature", "testing")
	entry, err := store.LogError(f.ID, "", "null pointer", "main.go", "missing nil check", "added nil guard")
	if err != nil { t.Fatalf("LogError: %v", err) }
	if entry.ID == "" { t.Fatal("expected non-empty ID") }
	if !entry.Resolved { t.Error("expected resolved=true when resolution provided") }
}

func TestLogError_Unresolved(t *testing.T) {
	store, _ := setupErrorStore(t)
	f, _ := store.CreateFeature("test-feature", "")
	entry, _ := store.LogError(f.ID, "", "connection refused", "db.go", "port mismatch", "")
	if entry.Resolved { t.Error("expected resolved=false when no resolution") }
}

func TestSearchErrors(t *testing.T) {
	store, _ := setupErrorStore(t)
	f, _ := store.CreateFeature("search-test", "")
	store.LogError(f.ID, "", "database connection timeout", "db.go", "pool exhaustion", "increased pool size")
	results, err := store.SearchErrors("database connection")
	if err != nil { t.Fatalf("SearchErrors: %v", err) }
	if len(results) == 0 { t.Fatal("expected at least one result") }
}

func TestFormatErrorSearch(t *testing.T) {
	if r := memory.FormatErrorSearch(nil); !strings.Contains(r, "No matching") { t.Errorf("expected no-match, got %q", r) }
	errors := []memory.ErrorEntry{{ErrorMessage: "test error", Resolved: true, Resolution: "fixed it", CreatedAt: "2024-01-01 00:00:00"}}
	if r := memory.FormatErrorSearch(errors); !strings.Contains(r, "RESOLVED") { t.Error("expected RESOLVED") }
}

func TestRecordTestResult(t *testing.T) {
	store, _ := setupErrorStore(t)
	f, _ := store.CreateFeature("test-feature", "")
	result, err := store.RecordTestResult(f.ID, "", "TestLogin", true, "")
	if err != nil { t.Fatalf("RecordTestResult: %v", err) }
	if !result.Passed { t.Error("expected passed=true") }
}

func TestGetTestHistory(t *testing.T) {
	store, _ := setupErrorStore(t)
	f, _ := store.CreateFeature("test-feature", "")
	store.RecordTestResult(f.ID, "", "TestLogin", true, "")
	store.RecordTestResult(f.ID, "", "TestLogin", false, "timeout")
	store.RecordTestResult(f.ID, "", "TestLogin", true, "")
	history, err := store.GetTestHistory("TestLogin", 10)
	if err != nil { t.Fatalf("GetTestHistory: %v", err) }
	if len(history) != 3 { t.Fatalf("expected 3, got %d", len(history)) }
}

func TestFormatTestMemory(t *testing.T) {
	current := &memory.TestResult{TestName: "TestLogin", Passed: true}
	history := []memory.TestResult{
		{Passed: true, CreatedAt: "2024-01-03 00:00:00"},
		{Passed: false, CreatedAt: "2024-01-02 00:00:00"},
		{Passed: true, CreatedAt: "2024-01-01 00:00:00"},
	}
	r := memory.FormatTestMemory(current, history)
	if !strings.Contains(r, "PASS") { t.Error("expected PASS") }
	if !strings.Contains(r, "History") { t.Error("expected History") }
}

func TestGetDebugContext(t *testing.T) {
	store, engine := setupErrorStore(t)
	f, _ := store.CreateFeature("debug-test", "")
	store.LogError(f.ID, "", "null pointer in auth module", "auth.go", "", "")
	store.RecordTestResult(f.ID, "", "TestAuth", false, "auth module failure")
	dc, err := store.GetDebugContext(engine, "auth")
	if err != nil { t.Fatalf("GetDebugContext: %v", err) }
	if len(dc.Errors) == 0 { t.Error("expected errors") }
	if len(dc.TestResults) == 0 { t.Error("expected test results") }
}

func TestFormatDebugContext_Empty(t *testing.T) {
	dc := &memory.DebugContext{}
	if r := memory.FormatDebugContext(dc, "nonexistent"); !strings.Contains(r, "No debug context") { t.Errorf("expected no-context, got %q", r) }
}
