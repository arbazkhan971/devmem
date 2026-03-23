package memory_test

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/arbazkhan971/memorx/internal/storage"
)

// newTestStoreWithDB is defined in analytics_test.go

func writer(db *storage.DB) *sql.DB {
	return db.Writer()
}

func TestGetMemoryHealth_CleanDB(t *testing.T) {
	store := newTestStore(t)

	f, err := store.CreateFeature("health-clean", "Clean feature")
	if err != nil {
		t.Fatalf("CreateFeature: %v", err)
	}

	h, err := store.GetMemoryHealth(f.ID)
	if err != nil {
		t.Fatalf("GetMemoryHealth: %v", err)
	}

	if h.Score != 100 {
		t.Errorf("expected score 100 for clean DB, got %f", h.Score)
	}
	if h.TotalMemories != 0 {
		t.Errorf("expected 0 total memories, got %d", h.TotalMemories)
	}
	if h.ConflictCount != 0 {
		t.Errorf("expected 0 conflicts, got %d", h.ConflictCount)
	}
	if h.StaleFactCount != 0 {
		t.Errorf("expected 0 stale facts, got %d", h.StaleFactCount)
	}
	if h.OrphanNoteCount != 0 {
		t.Errorf("expected 0 orphan notes, got %d", h.OrphanNoteCount)
	}
	if len(h.Suggestions) != 0 {
		t.Errorf("expected 0 suggestions, got %v", h.Suggestions)
	}
}

func TestGetMemoryHealth_WithConflicts(t *testing.T) {
	store, db := newTestStoreWithDB(t)

	f, _ := store.CreateFeature("health-conflict", "Conflict test")

	// Insert two active facts with same subject+predicate directly (bypassing CreateFact's auto-resolution).
	w := writer(db)
	w.Exec(`INSERT INTO facts (id, feature_id, subject, predicate, object, valid_at, recorded_at, confidence) VALUES ('f1', ?, 'db', 'uses', 'Postgres', datetime('now'), datetime('now'), 1.0)`, f.ID)
	w.Exec(`INSERT INTO facts (id, feature_id, subject, predicate, object, valid_at, recorded_at, confidence) VALUES ('f2', ?, 'db', 'uses', 'MySQL', datetime('now'), datetime('now'), 1.0)`, f.ID)

	h, err := store.GetMemoryHealth(f.ID)
	if err != nil {
		t.Fatalf("GetMemoryHealth: %v", err)
	}

	if h.ConflictCount != 1 {
		t.Errorf("expected 1 conflict group, got %d", h.ConflictCount)
	}
	// 1 conflict = -10 points
	if h.Score != 90 {
		t.Errorf("expected score 90 (100 - 10 for conflict), got %f", h.Score)
	}
}

func TestGetMemoryHealth_ScoreDecreasesWithOrphanNotes(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("health-score", "Score test")

	// Add orphan notes (no links)
	for i := 0; i < 15; i++ {
		store.CreateNote(f.ID, "", fmt.Sprintf("orphan note %d", i), "note")
	}

	h, err := store.GetMemoryHealth(f.ID)
	if err != nil {
		t.Fatalf("GetMemoryHealth: %v", err)
	}

	if h.OrphanNoteCount != 15 {
		t.Errorf("expected 15 orphan notes, got %d", h.OrphanNoteCount)
	}
	// min(15, 10) * 2 = 20 points deducted
	expectedScore := 80.0
	if h.Score != expectedScore {
		t.Errorf("expected score %f, got %f", expectedScore, h.Score)
	}
}

func TestGetMemoryHealth_SuggestionsAppear(t *testing.T) {
	store, db := newTestStoreWithDB(t)

	f, _ := store.CreateFeature("health-suggest", "Suggestion test")

	// Insert conflicting facts directly
	w := writer(db)
	w.Exec(`INSERT INTO facts (id, feature_id, subject, predicate, object, valid_at, recorded_at, confidence) VALUES ('s1', ?, 'lang', 'is', 'Go', datetime('now'), datetime('now'), 1.0)`, f.ID)
	w.Exec(`INSERT INTO facts (id, feature_id, subject, predicate, object, valid_at, recorded_at, confidence) VALUES ('s2', ?, 'lang', 'is', 'Rust', datetime('now'), datetime('now'), 1.0)`, f.ID)

	// Add many orphan notes
	for i := 0; i < 12; i++ {
		store.CreateNote(f.ID, "", fmt.Sprintf("orphan note for suggestion test %d", i), "note")
	}

	h, err := store.GetMemoryHealth(f.ID)
	if err != nil {
		t.Fatalf("GetMemoryHealth: %v", err)
	}

	if len(h.Suggestions) < 2 {
		t.Errorf("expected at least 2 suggestions, got %d: %v", len(h.Suggestions), h.Suggestions)
	}

	// Check specific suggestion text
	foundConflict := false
	foundOrphan := false
	for _, s := range h.Suggestions {
		if strings.Contains(s, "contradicting") && strings.Contains(s, "consolidation") {
			foundConflict = true
		}
		if strings.Contains(s, "notes") && strings.Contains(s, "no connections") {
			foundOrphan = true
		}
	}
	if !foundConflict {
		t.Errorf("expected conflict suggestion, got %v", h.Suggestions)
	}
	if !foundOrphan {
		t.Errorf("expected orphan note suggestion, got %v", h.Suggestions)
	}
}

func TestGetMemoryHealth_AllFeatures(t *testing.T) {
	store := newTestStore(t)

	f1, _ := store.CreateFeature("health-all-a", "Feature A")
	f2, _ := store.CreateFeature("health-all-b", "Feature B")

	store.CreateNote(f1.ID, "", "note in A", "note")
	store.CreateNote(f2.ID, "", "note in B", "note")

	// Empty featureID means all features
	h, err := store.GetMemoryHealth("")
	if err != nil {
		t.Fatalf("GetMemoryHealth all: %v", err)
	}

	if h.TotalMemories < 2 {
		t.Errorf("expected at least 2 total memories across all features, got %d", h.TotalMemories)
	}
}

func TestForgetStaleFacts_RemovesOldInvalidated(t *testing.T) {
	store, db := newTestStoreWithDB(t)

	f, _ := store.CreateFeature("forget-stale", "Forget test")

	// Insert an invalidated fact older than 30 days
	w := writer(db)
	w.Exec(`INSERT INTO facts (id, feature_id, subject, predicate, object, valid_at, invalid_at, recorded_at, confidence)
		VALUES ('old1', ?, 'db', 'was', 'Mongo', datetime('now', '-60 days'), datetime('now', '-35 days'), datetime('now', '-60 days'), 1.0)`, f.ID)

	// Insert an active fact (should not be deleted)
	store.CreateFact(f.ID, "", "db", "uses", "PostgreSQL")

	deleted, err := store.ForgetStaleFacts(f.ID)
	if err != nil {
		t.Fatalf("ForgetStaleFacts: %v", err)
	}
	if deleted != 1 {
		t.Errorf("expected 1 stale fact deleted, got %d", deleted)
	}

	// Active fact should still be there
	facts, err := store.GetActiveFacts(f.ID)
	if err != nil {
		t.Fatalf("GetActiveFacts: %v", err)
	}
	if len(facts) != 1 {
		t.Errorf("expected 1 active fact preserved, got %d", len(facts))
	}
	if facts[0].Object != "PostgreSQL" {
		t.Errorf("expected preserved fact to be PostgreSQL, got %q", facts[0].Object)
	}
}

func TestForgetStaleFacts_PreservesActiveFacts(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("forget-preserve", "Preserve test")

	// Create active facts only
	store.CreateFact(f.ID, "", "api", "uses", "REST")
	store.CreateFact(f.ID, "", "auth", "method", "JWT")

	deleted, err := store.ForgetStaleFacts(f.ID)
	if err != nil {
		t.Fatalf("ForgetStaleFacts: %v", err)
	}
	if deleted != 0 {
		t.Errorf("expected 0 stale facts deleted (all active), got %d", deleted)
	}

	facts, err := store.GetActiveFacts(f.ID)
	if err != nil {
		t.Fatalf("GetActiveFacts: %v", err)
	}
	if len(facts) != 2 {
		t.Errorf("expected 2 active facts preserved, got %d", len(facts))
	}
}

func TestForgetByID_Note(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("forget-id", "ID test")
	note, _ := store.CreateNote(f.ID, "", "a note to forget", "note")

	typ, err := store.ForgetByID(note.ID)
	if err != nil {
		t.Fatalf("ForgetByID: %v", err)
	}
	if typ != "note" {
		t.Errorf("expected type 'note', got %q", typ)
	}

	// Note should be gone
	_, err = store.GetNote(note.ID)
	if err == nil {
		t.Error("expected error getting deleted note")
	}
}

func TestForgetByID_Fact(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("forget-fact-id", "Fact ID test")
	fact, _ := store.CreateFact(f.ID, "", "test", "is", "deletable")

	typ, err := store.ForgetByID(fact.ID)
	if err != nil {
		t.Fatalf("ForgetByID: %v", err)
	}
	if typ != "fact" {
		t.Errorf("expected type 'fact', got %q", typ)
	}

	// Fact should be gone
	facts, err := store.GetActiveFacts(f.ID)
	if err != nil {
		t.Fatalf("GetActiveFacts: %v", err)
	}
	if len(facts) != 0 {
		t.Errorf("expected 0 active facts after forget, got %d", len(facts))
	}
}

func TestForgetByID_NotFound(t *testing.T) {
	store := newTestStore(t)

	_, err := store.ForgetByID("nonexistent-id")
	if err == nil {
		t.Error("expected error for nonexistent ID")
	}
}

func TestForgetByID_InvalidUUID(t *testing.T) {
	store := newTestStore(t)
	_, err := store.ForgetByID("not-a-valid-uuid-at-all")
	if err == nil {
		t.Fatal("expected error for invalid ID")
	}
}

func TestGetMemoryHealth_ScoreIs100ForCleanFeature(t *testing.T) {
	store := newTestStore(t)
	f, _ := store.CreateFeature("score-100", "Perfect health")
	h, err := store.GetMemoryHealth(f.ID)
	if err != nil {
		t.Fatalf("GetMemoryHealth: %v", err)
	}
	if h.Score != 100 {
		t.Errorf("expected score 100, got %f", h.Score)
	}
}

func TestGetMemoryHealth_ScoreCappedAtZero(t *testing.T) {
	store, db := newTestStoreWithDB(t)

	f, _ := store.CreateFeature("health-cap", "Cap test")

	// Insert many conflict groups to drive score below 0
	w := writer(db)
	for i := 0; i < 15; i++ {
		id1 := fmt.Sprintf("cap-a-%d", i)
		id2 := fmt.Sprintf("cap-b-%d", i)
		subj := fmt.Sprintf("subject-%d", i)
		w.Exec(`INSERT INTO facts (id, feature_id, subject, predicate, object, valid_at, recorded_at, confidence) VALUES (?, ?, ?, 'is', 'A', datetime('now'), datetime('now'), 1.0)`, id1, f.ID, subj)
		w.Exec(`INSERT INTO facts (id, feature_id, subject, predicate, object, valid_at, recorded_at, confidence) VALUES (?, ?, ?, 'is', 'B', datetime('now'), datetime('now'), 1.0)`, id2, f.ID, subj)
	}

	h, err := store.GetMemoryHealth(f.ID)
	if err != nil {
		t.Fatalf("GetMemoryHealth: %v", err)
	}

	if h.Score < 0 {
		t.Errorf("score should be capped at 0, got %f", h.Score)
	}
	if h.Score != 0 {
		t.Errorf("expected score 0 with 15 conflicts (150 point penalty > 100), got %f", h.Score)
	}
}
