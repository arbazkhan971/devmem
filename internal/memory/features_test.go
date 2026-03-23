package memory_test

import (
	"path/filepath"
	"testing"

	"github.com/arbaz/devmem/internal/memory"
	"github.com/arbaz/devmem/internal/storage"
)

// newTestStore creates a Store backed by a temp DB with migrations applied.
func newTestStore(t *testing.T) *memory.Store {
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

	return memory.NewStore(db)
}

func TestCreateFeature(t *testing.T) {
	store := newTestStore(t)

	f, err := store.CreateFeature("auth-system", "Add authentication")
	if err != nil {
		t.Fatalf("CreateFeature: %v", err)
	}

	if f.ID == "" {
		t.Error("expected non-empty ID")
	}
	if f.Name != "auth-system" {
		t.Errorf("expected name 'auth-system', got %q", f.Name)
	}
	if f.Description != "Add authentication" {
		t.Errorf("expected description 'Add authentication', got %q", f.Description)
	}
	if f.Status != "active" {
		t.Errorf("expected status 'active', got %q", f.Status)
	}
	if f.CreatedAt == "" {
		t.Error("expected non-empty CreatedAt")
	}
}

func TestCreateFeature_DuplicateName(t *testing.T) {
	store := newTestStore(t)

	_, err := store.CreateFeature("auth-system", "First")
	if err != nil {
		t.Fatalf("first CreateFeature: %v", err)
	}

	_, err = store.CreateFeature("auth-system", "Second")
	if err == nil {
		t.Fatal("expected error for duplicate name, got nil")
	}
}

func TestGetFeature(t *testing.T) {
	store := newTestStore(t)

	created, err := store.CreateFeature("auth-system", "Add authentication")
	if err != nil {
		t.Fatalf("CreateFeature: %v", err)
	}

	got, err := store.GetFeature("auth-system")
	if err != nil {
		t.Fatalf("GetFeature: %v", err)
	}

	if got.ID != created.ID {
		t.Errorf("ID mismatch: %q vs %q", got.ID, created.ID)
	}
	if got.Name != "auth-system" {
		t.Errorf("Name mismatch: %q", got.Name)
	}
}

func TestGetFeature_NotFound(t *testing.T) {
	store := newTestStore(t)

	_, err := store.GetFeature("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent feature")
	}
}

func TestListFeatures_All(t *testing.T) {
	store := newTestStore(t)

	store.CreateFeature("feat-a", "A")
	store.CreateFeature("feat-b", "B")

	features, err := store.ListFeatures("all")
	if err != nil {
		t.Fatalf("ListFeatures: %v", err)
	}
	if len(features) != 2 {
		t.Fatalf("expected 2 features, got %d", len(features))
	}
}

func TestListFeatures_ByStatus(t *testing.T) {
	store := newTestStore(t)

	store.CreateFeature("feat-a", "A")
	store.CreateFeature("feat-b", "B")
	store.UpdateFeatureStatus("feat-a", "paused")

	active, err := store.ListFeatures("active")
	if err != nil {
		t.Fatalf("ListFeatures active: %v", err)
	}
	if len(active) != 1 {
		t.Fatalf("expected 1 active feature, got %d", len(active))
	}
	if active[0].Name != "feat-b" {
		t.Errorf("expected feat-b, got %q", active[0].Name)
	}

	paused, err := store.ListFeatures("paused")
	if err != nil {
		t.Fatalf("ListFeatures paused: %v", err)
	}
	if len(paused) != 1 {
		t.Fatalf("expected 1 paused feature, got %d", len(paused))
	}
	if paused[0].Name != "feat-a" {
		t.Errorf("expected feat-a, got %q", paused[0].Name)
	}
}

func TestListFeatures_DoneFilter(t *testing.T) {
	store := newTestStore(t)

	store.CreateFeature("feat-x", "X")
	store.CreateFeature("feat-y", "Y")
	store.CreateFeature("feat-z", "Z")

	// Mark one as done
	store.UpdateFeatureStatus("feat-x", "done")

	done, err := store.ListFeatures("done")
	if err != nil {
		t.Fatalf("ListFeatures done: %v", err)
	}
	if len(done) != 1 {
		t.Fatalf("expected 1 done feature, got %d", len(done))
	}
	if done[0].Name != "feat-x" {
		t.Errorf("expected feat-x, got %q", done[0].Name)
	}
	if done[0].Status != "done" {
		t.Errorf("expected status 'done', got %q", done[0].Status)
	}
}

func TestListFeatures_EachStatusFilter(t *testing.T) {
	store := newTestStore(t)

	store.CreateFeature("feat-active1", "Active 1")
	store.CreateFeature("feat-active2", "Active 2")
	store.UpdateFeatureStatus("feat-active1", "paused")
	store.UpdateFeatureStatus("feat-active2", "done")

	// Create a third feature that will be active
	store.CreateFeature("feat-active3", "Active 3")

	// Test active filter
	active, err := store.ListFeatures("active")
	if err != nil {
		t.Fatalf("ListFeatures active: %v", err)
	}
	if len(active) != 1 {
		t.Fatalf("expected 1 active feature, got %d", len(active))
	}
	if active[0].Name != "feat-active3" {
		t.Errorf("expected feat-active3, got %q", active[0].Name)
	}

	// Test paused filter
	paused, err := store.ListFeatures("paused")
	if err != nil {
		t.Fatalf("ListFeatures paused: %v", err)
	}
	if len(paused) != 1 {
		t.Fatalf("expected 1 paused feature, got %d", len(paused))
	}
	if paused[0].Name != "feat-active1" {
		t.Errorf("expected feat-active1, got %q", paused[0].Name)
	}

	// Test done filter
	done, err := store.ListFeatures("done")
	if err != nil {
		t.Fatalf("ListFeatures done: %v", err)
	}
	if len(done) != 1 {
		t.Fatalf("expected 1 done feature, got %d", len(done))
	}
	if done[0].Name != "feat-active2" {
		t.Errorf("expected feat-active2, got %q", done[0].Name)
	}

	// Test all filter
	all, err := store.ListFeatures("all")
	if err != nil {
		t.Fatalf("ListFeatures all: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 total features, got %d", len(all))
	}
}

func TestUpdateFeatureStatus(t *testing.T) {
	store := newTestStore(t)

	store.CreateFeature("feat-a", "A")

	if err := store.UpdateFeatureStatus("feat-a", "done"); err != nil {
		t.Fatalf("UpdateFeatureStatus: %v", err)
	}

	f, err := store.GetFeature("feat-a")
	if err != nil {
		t.Fatalf("GetFeature: %v", err)
	}
	if f.Status != "done" {
		t.Errorf("expected status 'done', got %q", f.Status)
	}
}

func TestUpdateFeatureStatus_NotFound(t *testing.T) {
	store := newTestStore(t)

	err := store.UpdateFeatureStatus("nonexistent", "done")
	if err == nil {
		t.Fatal("expected error for nonexistent feature")
	}
}

func TestGetActiveFeature(t *testing.T) {
	store := newTestStore(t)

	store.CreateFeature("feat-a", "A")

	f, err := store.GetActiveFeature()
	if err != nil {
		t.Fatalf("GetActiveFeature: %v", err)
	}
	if f.Name != "feat-a" {
		t.Errorf("expected feat-a, got %q", f.Name)
	}
}

func TestGetActiveFeature_NoActive(t *testing.T) {
	store := newTestStore(t)

	_, err := store.GetActiveFeature()
	if err == nil {
		t.Fatal("expected error when no active feature")
	}
}

func TestStartFeature_New(t *testing.T) {
	store := newTestStore(t)

	f, err := store.StartFeature("new-feat", "A new feature")
	if err != nil {
		t.Fatalf("StartFeature: %v", err)
	}
	if f.Status != "active" {
		t.Errorf("expected status 'active', got %q", f.Status)
	}
	if f.Name != "new-feat" {
		t.Errorf("expected name 'new-feat', got %q", f.Name)
	}
}

func TestStartFeature_PausesCurrentActive(t *testing.T) {
	store := newTestStore(t)

	store.StartFeature("feat-a", "A")
	store.StartFeature("feat-b", "B")

	// feat-a should now be paused
	a, err := store.GetFeature("feat-a")
	if err != nil {
		t.Fatalf("GetFeature: %v", err)
	}
	if a.Status != "paused" {
		t.Errorf("expected feat-a status 'paused', got %q", a.Status)
	}

	// feat-b should be active
	b, err := store.GetFeature("feat-b")
	if err != nil {
		t.Fatalf("GetFeature: %v", err)
	}
	if b.Status != "active" {
		t.Errorf("expected feat-b status 'active', got %q", b.Status)
	}
}

func TestStartFeature_Resume(t *testing.T) {
	store := newTestStore(t)

	store.StartFeature("feat-a", "A")
	store.StartFeature("feat-b", "B")

	// Resume feat-a
	f, err := store.StartFeature("feat-a", "A")
	if err != nil {
		t.Fatalf("StartFeature resume: %v", err)
	}
	if f.Status != "active" {
		t.Errorf("expected status 'active', got %q", f.Status)
	}
	if f.Name != "feat-a" {
		t.Errorf("expected name 'feat-a', got %q", f.Name)
	}

	// feat-b should now be paused
	b, err := store.GetFeature("feat-b")
	if err != nil {
		t.Fatalf("GetFeature: %v", err)
	}
	if b.Status != "paused" {
		t.Errorf("expected feat-b status 'paused', got %q", b.Status)
	}
}
