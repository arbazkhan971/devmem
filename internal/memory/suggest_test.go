package memory_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/arbazkhan971/memorx/internal/memory"
	"github.com/arbazkhan971/memorx/internal/plans"
	"github.com/google/uuid"
)

func TestGetSuggestions_EmptyDB(t *testing.T) {
	store := newTestStore(t)
	suggestions, err := store.GetSuggestions()
	if err != nil {
		t.Fatalf("GetSuggestions: %v", err)
	}
	if len(suggestions) != 0 {
		t.Errorf("expected 0 suggestions on empty DB, got %d: %v", len(suggestions), suggestions)
	}
}

func TestGetSuggestions_StaleBlockers(t *testing.T) {
	store, db := newTestStoreWithDB(t)
	f, _ := store.CreateFeature("blocker-test", "Test stale blockers")
	sess, _ := store.CreateSession(f.ID, "test")

	// Insert a blocker that's 5 days old
	w := db.Writer()
	w.Exec(`INSERT INTO notes (id, feature_id, session_id, content, type, created_at, updated_at)
		VALUES (?, ?, ?, 'auth is broken', 'blocker', datetime('now', '-5 days'), datetime('now', '-5 days'))`,
		uuid.New().String(), f.ID, sess.ID)

	suggestions, err := store.GetSuggestions()
	if err != nil {
		t.Fatalf("GetSuggestions: %v", err)
	}

	found := false
	for _, s := range suggestions {
		if s.Category == "blocker" && strings.Contains(s.Message, "blocker-test") {
			found = true
			if !strings.Contains(s.Message, "5") {
				t.Errorf("expected message to mention 5+ days, got: %s", s.Message)
			}
		}
	}
	if !found {
		t.Errorf("expected stale blocker suggestion, got: %v", suggestions)
	}
}

func TestGetSuggestions_InactiveFeatures(t *testing.T) {
	store, db := newTestStoreWithDB(t)

	// Insert an active feature that hasn't been touched in 20 days
	w := db.Writer()
	fID := uuid.New().String()
	w.Exec(`INSERT INTO features (id, name, description, status, created_at, last_active)
		VALUES (?, 'stale-feature', 'Old feature', 'active', datetime('now', '-30 days'), datetime('now', '-20 days'))`, fID)

	suggestions, err := store.GetSuggestions()
	if err != nil {
		t.Fatalf("GetSuggestions: %v", err)
	}

	found := false
	for _, s := range suggestions {
		if s.Category == "inactive" && strings.Contains(s.Message, "stale-feature") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected inactive feature suggestion, got: %v", suggestions)
	}
}

func TestGetSuggestions_NearCompletePlan(t *testing.T) {
	store, db := newTestStoreWithDB(t)
	f, _ := store.CreateFeature("plan-test", "Test near-complete plan")
	sess, _ := store.CreateSession(f.ID, "test")

	pm := plans.NewManager(db)
	p, err := pm.CreatePlan(f.ID, sess.ID, "Almost done plan", "content", "test",
		[]plans.StepInput{
			{Title: "Step 1"}, {Title: "Step 2"}, {Title: "Step 3"},
			{Title: "Step 4"}, {Title: "Step 5"},
		})
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}

	// Complete 4 out of 5 steps (80%)
	steps, _ := pm.GetPlanSteps(p.ID)
	for i := 0; i < 4; i++ {
		pm.UpdateStepStatus(steps[i].ID, "completed")
	}

	suggestions, err := store.GetSuggestions()
	if err != nil {
		t.Fatalf("GetSuggestions: %v", err)
	}

	found := false
	for _, s := range suggestions {
		if s.Category == "plan" && strings.Contains(s.Message, "4/5") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected near-complete plan suggestion, got: %v", suggestions)
	}
}

func TestGetSuggestions_LowHealth(t *testing.T) {
	store, db := newTestStoreWithDB(t)
	f, _ := store.CreateFeature("health-test", "Test low health")

	// Insert many conflicts to bring health below 70
	w := db.Writer()
	for i := 0; i < 5; i++ {
		subj := fmt.Sprintf("item-%d", i)
		w.Exec(`INSERT INTO facts (id, feature_id, subject, predicate, object, valid_at, recorded_at, confidence)
			VALUES (?, ?, ?, 'is', 'A', datetime('now'), datetime('now'), 1.0)`,
			uuid.New().String(), f.ID, subj)
		w.Exec(`INSERT INTO facts (id, feature_id, subject, predicate, object, valid_at, recorded_at, confidence)
			VALUES (?, ?, ?, 'is', 'B', datetime('now'), datetime('now'), 1.0)`,
			uuid.New().String(), f.ID, subj)
	}

	suggestions, err := store.GetSuggestions()
	if err != nil {
		t.Fatalf("GetSuggestions: %v", err)
	}

	found := false
	for _, s := range suggestions {
		if s.Category == "health" && strings.Contains(s.Message, "memorx_forget") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected low health suggestion, got: %v", suggestions)
	}
}

func TestGetSuggestions_MissingSummary(t *testing.T) {
	store := newTestStore(t)
	f, _ := store.CreateFeature("summary-test", "Test missing summary")
	sess, _ := store.CreateSession(f.ID, "test")
	// End session without summary
	store.EndSession(sess.ID)

	suggestions, err := store.GetSuggestions()
	if err != nil {
		t.Fatalf("GetSuggestions: %v", err)
	}

	found := false
	for _, s := range suggestions {
		if s.Category == "session" && strings.Contains(s.Message, "no summary") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected missing summary suggestion, got: %v", suggestions)
	}
}

func TestGetSuggestions_ConflictingFacts(t *testing.T) {
	store, db := newTestStoreWithDB(t)
	f, _ := store.CreateFeature("conflict-test", "Test conflicts")

	// Insert conflicting facts directly (bypassing auto-resolution)
	w := db.Writer()
	w.Exec(`INSERT INTO facts (id, feature_id, subject, predicate, object, valid_at, recorded_at, confidence)
		VALUES (?, ?, 'db', 'uses', 'Postgres', datetime('now'), datetime('now'), 1.0)`,
		uuid.New().String(), f.ID)
	w.Exec(`INSERT INTO facts (id, feature_id, subject, predicate, object, valid_at, recorded_at, confidence)
		VALUES (?, ?, 'db', 'uses', 'MySQL', datetime('now'), datetime('now'), 1.0)`,
		uuid.New().String(), f.ID)

	suggestions, err := store.GetSuggestions()
	if err != nil {
		t.Fatalf("GetSuggestions: %v", err)
	}

	found := false
	for _, s := range suggestions {
		if s.Category == "conflict" && strings.Contains(s.Message, "contradicting") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected conflict suggestion, got: %v", suggestions)
	}
}

func TestFormatSuggestions_Empty(t *testing.T) {
	result := memory.FormatSuggestions(nil)
	if !strings.Contains(result, "No suggestions") {
		t.Errorf("expected 'No suggestions' for empty list, got: %s", result)
	}
}

func TestFormatSuggestions_WithItems(t *testing.T) {
	items := []memory.Suggestion{
		{Category: "blocker", Message: "3 blockers on auth"},
		{Category: "plan", Message: "plan 4/5 done"},
	}
	result := memory.FormatSuggestions(items)
	if !strings.Contains(result, "3 blockers on auth") {
		t.Errorf("expected first suggestion in output, got: %s", result)
	}
	if !strings.Contains(result, "plan 4/5 done") {
		t.Errorf("expected second suggestion in output, got: %s", result)
	}
}
