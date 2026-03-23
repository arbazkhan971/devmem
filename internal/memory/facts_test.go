package memory_test

import (
	"strings"
	"testing"
	"time"
)

func TestCreateFact(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("feat-a", "A")
	sess, _ := store.CreateSession(f.ID, "claude-code")

	fact, err := store.CreateFact(f.ID, sess.ID, "database", "uses", "PostgreSQL")
	if err != nil {
		t.Fatalf("CreateFact: %v", err)
	}

	if fact.ID == "" {
		t.Error("expected non-empty ID")
	}
	if fact.Subject != "database" {
		t.Errorf("expected subject 'database', got %q", fact.Subject)
	}
	if fact.Predicate != "uses" {
		t.Errorf("expected predicate 'uses', got %q", fact.Predicate)
	}
	if fact.Object != "PostgreSQL" {
		t.Errorf("expected object 'PostgreSQL', got %q", fact.Object)
	}
	if fact.Confidence != 1.0 {
		t.Errorf("expected confidence 1.0, got %f", fact.Confidence)
	}
	if fact.InvalidAt != "" {
		t.Errorf("expected empty InvalidAt, got %q", fact.InvalidAt)
	}
}

func TestCreateFact_WithoutSession(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("feat-a", "A")

	fact, err := store.CreateFact(f.ID, "", "database", "uses", "PostgreSQL")
	if err != nil {
		t.Fatalf("CreateFact without session: %v", err)
	}
	if fact.SessionID != "" {
		t.Errorf("expected empty session ID, got %q", fact.SessionID)
	}
}

func TestCreateFact_ContradictionResolution(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("feat-a", "A")

	// Create initial fact
	fact1, err := store.CreateFact(f.ID, "", "database", "uses", "PostgreSQL")
	if err != nil {
		t.Fatalf("CreateFact 1: %v", err)
	}

	// Create contradicting fact (same subject+predicate, different object)
	fact2, err := store.CreateFact(f.ID, "", "database", "uses", "SQLite")
	if err != nil {
		t.Fatalf("CreateFact 2: %v", err)
	}

	if fact2.Object != "SQLite" {
		t.Errorf("expected new fact object 'SQLite', got %q", fact2.Object)
	}

	// The old fact should be invalidated
	activeFacts, err := store.GetActiveFacts(f.ID)
	if err != nil {
		t.Fatalf("GetActiveFacts: %v", err)
	}

	if len(activeFacts) != 1 {
		t.Fatalf("expected 1 active fact after contradiction, got %d", len(activeFacts))
	}
	if activeFacts[0].ID != fact2.ID {
		t.Errorf("expected active fact to be the new one (ID %q), got %q", fact2.ID, activeFacts[0].ID)
	}

	_ = fact1 // suppress unused warning
}

func TestCreateFact_SameFactNoop(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("feat-a", "A")

	fact1, err := store.CreateFact(f.ID, "", "database", "uses", "PostgreSQL")
	if err != nil {
		t.Fatalf("CreateFact 1: %v", err)
	}

	// Create same fact again
	fact2, err := store.CreateFact(f.ID, "", "database", "uses", "PostgreSQL")
	if err != nil {
		t.Fatalf("CreateFact 2: %v", err)
	}

	// Should return existing fact
	if fact2.ID != fact1.ID {
		t.Errorf("expected same fact ID %q, got %q", fact1.ID, fact2.ID)
	}
}

func TestCreateFact_IdenticalReturnsExistingNoDuplicate(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("feat-dup", "Dup test")

	// Create the fact
	fact1, err := store.CreateFact(f.ID, "", "auth", "method", "JWT")
	if err != nil {
		t.Fatalf("CreateFact 1: %v", err)
	}

	// Create same fact (same subject+predicate+object) multiple times
	fact2, err := store.CreateFact(f.ID, "", "auth", "method", "JWT")
	if err != nil {
		t.Fatalf("CreateFact 2: %v", err)
	}
	fact3, err := store.CreateFact(f.ID, "", "auth", "method", "JWT")
	if err != nil {
		t.Fatalf("CreateFact 3: %v", err)
	}

	// All should return the same ID
	if fact2.ID != fact1.ID {
		t.Errorf("second call: expected ID %q, got %q", fact1.ID, fact2.ID)
	}
	if fact3.ID != fact1.ID {
		t.Errorf("third call: expected ID %q, got %q", fact1.ID, fact3.ID)
	}

	// Verify only 1 active fact exists for this subject+predicate
	activeFacts, err := store.GetActiveFacts(f.ID)
	if err != nil {
		t.Fatalf("GetActiveFacts: %v", err)
	}
	if len(activeFacts) != 1 {
		t.Errorf("expected exactly 1 active fact, got %d", len(activeFacts))
	}

	// Returned fact should have all fields intact
	if fact2.Subject != "auth" || fact2.Predicate != "method" || fact2.Object != "JWT" {
		t.Errorf("returned fact fields mismatch: %+v", fact2)
	}
	if fact2.Confidence != 1.0 {
		t.Errorf("expected confidence 1.0, got %f", fact2.Confidence)
	}
}

func TestGetActiveFacts_NonExistentFeature(t *testing.T) {
	store := newTestStore(t)

	facts, err := store.GetActiveFacts("nonexistent-feature-id")
	if err != nil {
		t.Fatalf("GetActiveFacts: %v", err)
	}
	if len(facts) != 0 {
		t.Errorf("expected 0 active facts for non-existent feature, got %d", len(facts))
	}
}

func TestGetActiveFacts(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("feat-a", "A")
	store.CreateFact(f.ID, "", "database", "uses", "PostgreSQL")
	store.CreateFact(f.ID, "", "api", "framework", "Gin")

	facts, err := store.GetActiveFacts(f.ID)
	if err != nil {
		t.Fatalf("GetActiveFacts: %v", err)
	}
	if len(facts) != 2 {
		t.Fatalf("expected 2 active facts, got %d", len(facts))
	}
}

func TestInvalidateFact(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("feat-a", "A")
	fact, _ := store.CreateFact(f.ID, "", "database", "uses", "PostgreSQL")

	if err := store.InvalidateFact(fact.ID); err != nil {
		t.Fatalf("InvalidateFact: %v", err)
	}

	facts, err := store.GetActiveFacts(f.ID)
	if err != nil {
		t.Fatalf("GetActiveFacts: %v", err)
	}
	if len(facts) != 0 {
		t.Fatalf("expected 0 active facts after invalidation, got %d", len(facts))
	}
}

func TestInvalidateFact_NotFound(t *testing.T) {
	store := newTestStore(t)

	err := store.InvalidateFact("nonexistent-id")
	if err == nil {
		t.Fatal("expected error for nonexistent fact")
	}
}

func TestQueryFactsAsOf(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("feat-a", "A")

	// Create a fact
	store.CreateFact(f.ID, "", "database", "uses", "PostgreSQL")

	// Query as of now (should find it)
	facts, err := store.QueryFactsAsOf(f.ID, time.Now().Add(time.Second))
	if err != nil {
		t.Fatalf("QueryFactsAsOf: %v", err)
	}
	if len(facts) != 1 {
		t.Fatalf("expected 1 fact as of now, got %d", len(facts))
	}

	// Query as of the past (should not find it)
	pastFacts, err := store.QueryFactsAsOf(f.ID, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("QueryFactsAsOf past: %v", err)
	}
	if len(pastFacts) != 0 {
		t.Fatalf("expected 0 facts in the past, got %d", len(pastFacts))
	}
}

func TestCreateFact_VeryLongStrings(t *testing.T) {
	store := newTestStore(t)
	f, _ := store.CreateFeature("long-fact", "Long strings")
	long := strings.Repeat("abcdefghij", 500) // 5000 chars
	fact, err := store.CreateFact(f.ID, "", long, long, long)
	if err != nil {
		t.Fatalf("CreateFact long: %v", err)
	}
	if fact.Subject != long {
		t.Error("subject mismatch for long string")
	}
}

func TestFactContradiction_SameSubjPredDiffObj(t *testing.T) {
	store := newTestStore(t)
	f, _ := store.CreateFeature("fc-1", "test")
	store.CreateFact(f.ID, "", "db", "uses", "Postgres")
	store.CreateFact(f.ID, "", "db", "uses", "SQLite")
	active, _ := store.GetActiveFacts(f.ID)
	if len(active) != 1 { t.Errorf("expected 1, got %d", len(active)) }
	if active[0].Object != "SQLite" { t.Errorf("expected SQLite, got %q", active[0].Object) }
}

func TestFactContradiction_SameSubjPredSameObj(t *testing.T) {
	store := newTestStore(t)
	f, _ := store.CreateFeature("fc-2", "test")
	f1, _ := store.CreateFact(f.ID, "", "db", "uses", "Postgres")
	f2, _ := store.CreateFact(f.ID, "", "db", "uses", "Postgres")
	if f1.ID != f2.ID { t.Errorf("expected same ID for identical fact") }
	active, _ := store.GetActiveFacts(f.ID)
	if len(active) != 1 { t.Errorf("expected 1, got %d", len(active)) }
}

func TestFactContradiction_DiffSubjSamePredObj(t *testing.T) {
	store := newTestStore(t)
	f, _ := store.CreateFeature("fc-3", "test")
	store.CreateFact(f.ID, "", "db", "uses", "Postgres")
	store.CreateFact(f.ID, "", "cache", "uses", "Postgres")
	active, _ := store.GetActiveFacts(f.ID)
	if len(active) != 2 { t.Errorf("expected 2, got %d", len(active)) }
}

func TestFactContradiction_SameSubjDiffPred(t *testing.T) {
	store := newTestStore(t)
	f, _ := store.CreateFeature("fc-4", "test")
	store.CreateFact(f.ID, "", "db", "uses", "Postgres")
	store.CreateFact(f.ID, "", "db", "version", "16")
	active, _ := store.GetActiveFacts(f.ID)
	if len(active) != 2 { t.Errorf("expected 2, got %d", len(active)) }
}

func TestFactContradiction_CompletelyDifferent(t *testing.T) {
	store := newTestStore(t)
	f, _ := store.CreateFeature("fc-5", "test")
	store.CreateFact(f.ID, "", "db", "uses", "Postgres")
	store.CreateFact(f.ID, "", "api", "framework", "Gin")
	active, _ := store.GetActiveFacts(f.ID)
	if len(active) != 2 { t.Errorf("expected 2, got %d", len(active)) }
}

func TestFactContradiction_Variants(t *testing.T) {
	for _, tc := range []struct {
		name                               string
		subj1, pred1, obj1                 string
		subj2, pred2, obj2                 string
		wantActive                         int
		wantSameID                         bool
	}{
		{"same_subj_pred_diff_obj", "db", "uses", "Postgres", "db", "uses", "SQLite", 1, false},
		{"same_subj_pred_same_obj", "db", "uses", "Postgres", "db", "uses", "Postgres", 1, true},
		{"diff_subj_same_pred_obj", "db", "uses", "Postgres", "cache", "uses", "Postgres", 2, false},
		{"same_subj_diff_pred", "db", "uses", "Postgres", "db", "version", "16", 2, false},
		{"completely_different", "db", "uses", "Postgres", "api", "framework", "Gin", 2, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newTestStore(t)
			f, _ := store.CreateFeature("feat-"+tc.name, "test")
			f1, _ := store.CreateFact(f.ID, "", tc.subj1, tc.pred1, tc.obj1)
			f2, _ := store.CreateFact(f.ID, "", tc.subj2, tc.pred2, tc.obj2)
			active, _ := store.GetActiveFacts(f.ID)
			if len(active) != tc.wantActive { t.Errorf("expected %d, got %d", tc.wantActive, len(active)) }
			if tc.wantSameID && f1.ID != f2.ID { t.Error("expected same ID") }
			if !tc.wantSameID && f1.ID == f2.ID { t.Error("expected different IDs") }
		})
	}
}

func TestQueryFactsAsOf_AfterContradiction(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("feat-a", "A")

	// Create initial fact
	store.CreateFact(f.ID, "", "database", "uses", "PostgreSQL")

	// Sleep to ensure the next timestamp is at least 1 second later
	// (SQLite datetime has second-level precision)
	time.Sleep(1100 * time.Millisecond)
	beforeChange := time.Now()
	time.Sleep(1100 * time.Millisecond)

	// Contradict it
	store.CreateFact(f.ID, "", "database", "uses", "SQLite")

	// Query as of after change should show SQLite
	afterFacts, err := store.QueryFactsAsOf(f.ID, time.Now().Add(time.Second))
	if err != nil {
		t.Fatalf("QueryFactsAsOf after: %v", err)
	}
	if len(afterFacts) != 1 {
		t.Fatalf("expected 1 fact after contradiction, got %d", len(afterFacts))
	}
	if afterFacts[0].Object != "SQLite" {
		t.Errorf("expected 'SQLite', got %q", afterFacts[0].Object)
	}

	// Query as of before change should show PostgreSQL
	beforeFacts, err := store.QueryFactsAsOf(f.ID, beforeChange)
	if err != nil {
		t.Fatalf("QueryFactsAsOf before: %v", err)
	}
	if len(beforeFacts) != 1 {
		t.Fatalf("expected 1 fact before contradiction, got %d", len(beforeFacts))
	}
	if beforeFacts[0].Object != "PostgreSQL" {
		t.Errorf("expected 'PostgreSQL', got %q", beforeFacts[0].Object)
	}
}
