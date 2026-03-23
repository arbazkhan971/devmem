package memory_test

import (
	"path/filepath"
	"testing"

	"github.com/arbazkhan971/memorx/internal/memory"
	"github.com/arbazkhan971/memorx/internal/storage"
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

func TestStartFeature_AutoPauseVerifiesActiveFeature(t *testing.T) {
	store := newTestStore(t)

	// Start three features in sequence.
	store.StartFeature("feat-1", "First")
	store.StartFeature("feat-2", "Second")
	store.StartFeature("feat-3", "Third")

	// Only feat-3 should be active; feat-1 and feat-2 should be paused.
	active, err := store.GetActiveFeature()
	if err != nil {
		t.Fatalf("GetActiveFeature: %v", err)
	}
	if active.Name != "feat-3" {
		t.Errorf("expected active feature 'feat-3', got %q", active.Name)
	}

	f1, _ := store.GetFeature("feat-1")
	if f1.Status != "paused" {
		t.Errorf("expected feat-1 status 'paused', got %q", f1.Status)
	}
	f2, _ := store.GetFeature("feat-2")
	if f2.Status != "paused" {
		t.Errorf("expected feat-2 status 'paused', got %q", f2.Status)
	}
}

func TestGetFeature_NonExistentReturnsError(t *testing.T) {
	store := newTestStore(t)

	// Create one feature to ensure DB is not empty.
	store.CreateFeature("existing-feat", "Exists")

	_, err := store.GetFeature("totally-missing")
	if err == nil {
		t.Fatal("expected error for non-existent feature, got nil")
	}
	if got := err.Error(); got == "" {
		t.Error("expected non-empty error message")
	}
}

func TestCreateFeature_DuplicateNameReturnsError(t *testing.T) {
	store := newTestStore(t)

	_, err := store.CreateFeature("dup-test", "Original")
	if err != nil {
		t.Fatalf("first CreateFeature: %v", err)
	}

	_, err = store.CreateFeature("dup-test", "Duplicate with different description")
	if err == nil {
		t.Fatal("expected error for duplicate feature name, got nil")
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

func TestCreateFeature_HasNonEmptyID(t *testing.T) {
	store := newTestStore(t)
	f, err := store.CreateFeature("id-test", "Testing ID generation")
	if err != nil {
		t.Fatalf("CreateFeature: %v", err)
	}
	if len(f.ID) < 10 {
		t.Errorf("expected UUID-like ID with len >= 10, got %q (len %d)", f.ID, len(f.ID))
	}
}

func TestListFeatures_EmptyDB(t *testing.T) {
	store := newTestStore(t)
	features, err := store.ListFeatures("all")
	if err != nil {
		t.Fatalf("ListFeatures: %v", err)
	}
	if len(features) != 0 {
		t.Errorf("expected 0 features on empty DB, got %d", len(features))
	}
}

func TestStartFeature_EmptyNameCreatesFeature(t *testing.T) {
	store := newTestStore(t)
	// Empty name is technically valid in the current implementation (no validation).
	// This test documents that behavior.
	f, err := store.StartFeature("", "empty name test")
	if err != nil {
		t.Fatalf("StartFeature empty name: %v", err)
	}
	if f.Name != "" {
		t.Errorf("expected empty name, got %q", f.Name)
	}
}

func TestStartFeature_MultipleSwitches(t *testing.T) {
	store := newTestStore(t)
	names := []string{"sw-1", "sw-2", "sw-3", "sw-4", "sw-5"}
	for _, n := range names {
		t.Run("create_"+n, func(t *testing.T) {
			_, err := store.StartFeature(n, "desc")
			if err != nil {
				t.Fatalf("StartFeature(%s): %v", n, err)
			}
		})
	}
	// Only last should be active
	active, err := store.GetActiveFeature()
	if err != nil {
		t.Fatalf("GetActiveFeature: %v", err)
	}
	if active.Name != "sw-5" {
		t.Errorf("expected sw-5 active, got %q", active.Name)
	}
	for _, n := range names[:4] {
		t.Run("paused_"+n, func(t *testing.T) {
			f, _ := store.GetFeature(n)
			if f.Status != "paused" {
				t.Errorf("expected %s paused, got %q", n, f.Status)
			}
		})
	}
}

func TestGetActiveFacts_EmptyFeature(t *testing.T) {
	store := newTestStore(t)
	f, _ := store.CreateFeature("empty-facts", "No facts")
	facts, err := store.GetActiveFacts(f.ID)
	if err != nil {
		t.Fatalf("GetActiveFacts: %v", err)
	}
	if len(facts) != 0 {
		t.Errorf("expected 0 facts, got %d", len(facts))
	}
}

func TestListSessions_EmptyFeature(t *testing.T) {
	store := newTestStore(t)
	f, _ := store.CreateFeature("empty-sess", "No sessions")
	sessions, err := store.ListSessions(f.ID, 10)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestFeatureStatus_ActiveToPaused(t *testing.T) {
	store := newTestStore(t)
	store.CreateFeature("st-a2p", "test")
	if err := store.UpdateFeatureStatus("st-a2p", "paused"); err != nil {
		t.Fatalf("active->paused: %v", err)
	}
	f, _ := store.GetFeature("st-a2p")
	if f.Status != "paused" { t.Errorf("got %q", f.Status) }
}

func TestFeatureStatus_PausedToActive(t *testing.T) {
	store := newTestStore(t)
	store.CreateFeature("st-p2a", "test")
	store.UpdateFeatureStatus("st-p2a", "paused")
	if err := store.UpdateFeatureStatus("st-p2a", "active"); err != nil {
		t.Fatalf("paused->active: %v", err)
	}
	f, _ := store.GetFeature("st-p2a")
	if f.Status != "active" { t.Errorf("got %q", f.Status) }
}

func TestFeatureStatus_ActiveToDone(t *testing.T) {
	store := newTestStore(t)
	store.CreateFeature("st-a2d", "test")
	if err := store.UpdateFeatureStatus("st-a2d", "done"); err != nil {
		t.Fatalf("active->done: %v", err)
	}
	f, _ := store.GetFeature("st-a2d")
	if f.Status != "done" { t.Errorf("got %q", f.Status) }
}

func TestFeatureStatus_PausedToDone(t *testing.T) {
	store := newTestStore(t)
	store.CreateFeature("st-p2d", "test")
	store.UpdateFeatureStatus("st-p2d", "paused")
	if err := store.UpdateFeatureStatus("st-p2d", "done"); err != nil {
		t.Fatalf("paused->done: %v", err)
	}
	f, _ := store.GetFeature("st-p2d")
	if f.Status != "done" { t.Errorf("got %q", f.Status) }
}

func TestFeatureStatus_DoneToActive(t *testing.T) {
	store := newTestStore(t)
	store.CreateFeature("st-d2a", "test")
	store.UpdateFeatureStatus("st-d2a", "done")
	if err := store.UpdateFeatureStatus("st-d2a", "active"); err != nil {
		t.Fatalf("done->active: %v", err)
	}
	f, _ := store.GetFeature("st-d2a")
	if f.Status != "active" { t.Errorf("got %q", f.Status) }
}

func TestFeatureStatus_DoneToPaused(t *testing.T) {
	store := newTestStore(t)
	store.CreateFeature("st-d2p", "test")
	store.UpdateFeatureStatus("st-d2p", "done")
	if err := store.UpdateFeatureStatus("st-d2p", "paused"); err != nil {
		t.Fatalf("done->paused: %v", err)
	}
	f, _ := store.GetFeature("st-d2p")
	if f.Status != "paused" { t.Errorf("got %q", f.Status) }
}

func TestFeatureStatusTransitions(t *testing.T) {
	for _, tc := range []struct {
		name     string
		from, to string
	}{
		{"active_to_paused", "active", "paused"},
		{"paused_to_active", "paused", "active"},
		{"active_to_done", "active", "done"},
		{"paused_to_done", "paused", "done"},
		{"done_to_active", "done", "active"},
		{"done_to_paused", "done", "paused"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newTestStore(t)
			store.CreateFeature("feat-trans", "Transition test")
			if tc.from != "active" {
				store.UpdateFeatureStatus("feat-trans", tc.from)
			}
			if err := store.UpdateFeatureStatus("feat-trans", tc.to); err != nil {
				t.Fatalf("transition %s->%s: %v", tc.from, tc.to, err)
			}
			f, _ := store.GetFeature("feat-trans")
			if f.Status != tc.to {
				t.Errorf("expected status %q, got %q", tc.to, f.Status)
			}
		})
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
