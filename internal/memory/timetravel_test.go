package memory_test

import (
	"strings"
	"testing"
	"time"

	"github.com/arbazkhan971/memorx/internal/memory"
)

func TestTimeTravel_Basic(t *testing.T) {
	store := newTestStore(t)

	f, err := store.CreateFeature("tt-feat", "Time travel test")
	if err != nil {
		t.Fatalf("CreateFeature: %v", err)
	}

	sess, err := store.CreateSession(f.ID, "test-tool")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// Create some facts and notes
	store.CreateFact(f.ID, sess.ID, "auth", "uses", "JWT")
	store.CreateNote(f.ID, sess.ID, "Set up authentication", "note")
	store.CreateNote(f.ID, sess.ID, "Decided to use JWT tokens", "decision")

	// Time travel to now+1s should see all data
	result, err := store.TimeTravel(f.ID, time.Now().Add(time.Second))
	if err != nil {
		t.Fatalf("TimeTravel: %v", err)
	}

	if result.Feature == nil {
		t.Fatal("expected non-nil Feature")
	}
	if result.Feature.Name != "tt-feat" {
		t.Errorf("expected feature name 'tt-feat', got %q", result.Feature.Name)
	}
	if len(result.ActiveFacts) != 1 {
		t.Errorf("expected 1 active fact, got %d", len(result.ActiveFacts))
	}
	if len(result.NotesAtTime) != 2 {
		t.Errorf("expected 2 notes, got %d", len(result.NotesAtTime))
	}
}

func TestTimeTravel_PastBeforeCreation(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("tt-past", "Past test")
	sess, _ := store.CreateSession(f.ID, "test-tool")

	store.CreateFact(f.ID, sess.ID, "db", "uses", "postgres")
	store.CreateNote(f.ID, sess.ID, "Started development", "note")

	// Time travel to 1 hour ago should see nothing
	result, err := store.TimeTravel(f.ID, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("TimeTravel: %v", err)
	}

	if len(result.ActiveFacts) != 0 {
		t.Errorf("expected 0 facts in the past, got %d", len(result.ActiveFacts))
	}
	if len(result.NotesAtTime) != 0 {
		t.Errorf("expected 0 notes in the past, got %d", len(result.NotesAtTime))
	}
}

func TestTimeTravel_FeatureNotFound(t *testing.T) {
	store := newTestStore(t)

	_, err := store.TimeTravel("nonexistent-id", time.Now())
	if err == nil {
		t.Fatal("expected error for nonexistent feature")
	}
}

func TestTimeTravel_WithContradiction(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("tt-contra", "Contradiction test")

	// Create initial fact
	store.CreateFact(f.ID, "", "database", "uses", "PostgreSQL")

	// Sleep to ensure distinct timestamps
	time.Sleep(1100 * time.Millisecond)
	beforeChange := time.Now()
	time.Sleep(1100 * time.Millisecond)

	// Contradict the fact
	store.CreateFact(f.ID, "", "database", "uses", "SQLite")

	// Time travel before contradiction should see PostgreSQL
	before, err := store.TimeTravel(f.ID, beforeChange)
	if err != nil {
		t.Fatalf("TimeTravel before: %v", err)
	}
	if len(before.ActiveFacts) != 1 {
		t.Fatalf("expected 1 fact before contradiction, got %d", len(before.ActiveFacts))
	}
	if before.ActiveFacts[0].Object != "PostgreSQL" {
		t.Errorf("expected 'PostgreSQL', got %q", before.ActiveFacts[0].Object)
	}

	// Time travel after contradiction should see SQLite
	after, err := store.TimeTravel(f.ID, time.Now().Add(time.Second))
	if err != nil {
		t.Fatalf("TimeTravel after: %v", err)
	}
	if len(after.ActiveFacts) != 1 {
		t.Fatalf("expected 1 fact after contradiction, got %d", len(after.ActiveFacts))
	}
	if after.ActiveFacts[0].Object != "SQLite" {
		t.Errorf("expected 'SQLite', got %q", after.ActiveFacts[0].Object)
	}
}

func TestTimeTravel_NilPlanWhenNoPlan(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("tt-noplan", "No plan test")
	store.CreateNote(f.ID, "", "Just a note", "note")

	result, err := store.TimeTravel(f.ID, time.Now().Add(time.Second))
	if err != nil {
		t.Fatalf("TimeTravel: %v", err)
	}

	if result.PlanAtTime != nil {
		t.Error("expected nil PlanAtTime when no plan exists")
	}
}

func TestFormatTimeTravel(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("tt-format", "Format test")
	store.CreateFact(f.ID, "", "auth", "uses", "JWT")
	store.CreateFact(f.ID, "", "db", "uses", "postgres")
	store.CreateNote(f.ID, "", "Note 1", "note")

	result, err := store.TimeTravel(f.ID, time.Now().Add(time.Second))
	if err != nil {
		t.Fatalf("TimeTravel: %v", err)
	}

	formatted := memory.FormatTimeTravel(result)
	if !strings.Contains(formatted, "2 facts") {
		t.Errorf("expected '2 facts' in output, got: %s", formatted)
	}
	if !strings.Contains(formatted, "1 notes") {
		t.Errorf("expected '1 notes' in output, got: %s", formatted)
	}
	if !strings.Contains(formatted, "tt-format") {
		t.Errorf("expected feature name in output, got: %s", formatted)
	}
}

func TestFormatTimeTravel_Nil(t *testing.T) {
	formatted := memory.FormatTimeTravel(nil)
	if formatted != "No time travel result." {
		t.Errorf("expected nil message, got: %s", formatted)
	}
}
