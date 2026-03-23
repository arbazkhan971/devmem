package memory_test

import (
	"testing"

	"github.com/arbazkhan971/memorx/internal/memory"
)

func TestPinMemory_Note(t *testing.T) {
	store := newTestStore(t)
	f, _ := store.CreateFeature("pin-note", "Pin test")
	note, err := store.CreateNote(f.ID, "", "Important decision", "decision")
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}

	if err := store.PinMemory(note.ID, "note"); err != nil {
		t.Fatalf("PinMemory: %v", err)
	}

	// Verify it shows up as pinned in ListMemories
	pinned := true
	items, err := store.ListMemories(f.ID, memory_filter(t, "notes", &pinned, 20))
	if err != nil {
		t.Fatalf("ListMemories: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 pinned note, got %d", len(items))
	}
	if !items[0].Pinned {
		t.Error("expected item to be pinned")
	}
	if items[0].ID != note.ID {
		t.Errorf("expected ID %q, got %q", note.ID, items[0].ID)
	}
}

func TestPinMemory_Fact(t *testing.T) {
	store := newTestStore(t)
	f, _ := store.CreateFeature("pin-fact", "Pin fact test")
	fact, err := store.CreateFact(f.ID, "", "db", "uses", "sqlite")
	if err != nil {
		t.Fatalf("CreateFact: %v", err)
	}

	if err := store.PinMemory(fact.ID, "fact"); err != nil {
		t.Fatalf("PinMemory: %v", err)
	}

	pinned := true
	items, err := store.ListMemories(f.ID, memory_filter(t, "facts", &pinned, 20))
	if err != nil {
		t.Fatalf("ListMemories: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 pinned fact, got %d", len(items))
	}
	if !items[0].Pinned {
		t.Error("expected item to be pinned")
	}
}

func TestUnpinMemory(t *testing.T) {
	store := newTestStore(t)
	f, _ := store.CreateFeature("unpin-test", "Unpin test")
	note, _ := store.CreateNote(f.ID, "", "Pinned note", "note")

	// Pin it
	if err := store.PinMemory(note.ID, "note"); err != nil {
		t.Fatalf("PinMemory: %v", err)
	}

	// Verify pinned
	pinned := true
	items, _ := store.ListMemories(f.ID, memory_filter(t, "", &pinned, 20))
	if len(items) != 1 {
		t.Fatalf("expected 1 pinned item, got %d", len(items))
	}

	// Unpin it
	if err := store.UnpinMemory(note.ID, "note"); err != nil {
		t.Fatalf("UnpinMemory: %v", err)
	}

	// Verify unpinned
	items, _ = store.ListMemories(f.ID, memory_filter(t, "", &pinned, 20))
	if len(items) != 0 {
		t.Fatalf("expected 0 pinned items after unpin, got %d", len(items))
	}
}

func TestListMemories_MergedNotesAndFacts(t *testing.T) {
	store := newTestStore(t)
	f, _ := store.CreateFeature("list-merge", "Merge test")

	store.CreateNote(f.ID, "", "A note", "note")
	store.CreateNote(f.ID, "", "A decision", "decision")
	store.CreateFact(f.ID, "", "db", "uses", "sqlite")
	store.CreateFact(f.ID, "", "api", "framework", "gin")

	items, err := store.ListMemories(f.ID, memory_filter(t, "", nil, 20))
	if err != nil {
		t.Fatalf("ListMemories: %v", err)
	}
	if len(items) != 4 {
		t.Fatalf("expected 4 merged items, got %d", len(items))
	}

	// Check that we have both types
	noteCount, factCount := 0, 0
	for _, item := range items {
		switch item.Type {
		case "note":
			noteCount++
		case "fact":
			factCount++
		}
	}
	if noteCount != 2 {
		t.Errorf("expected 2 notes, got %d", noteCount)
	}
	if factCount != 2 {
		t.Errorf("expected 2 facts, got %d", factCount)
	}
}

func TestListMemories_FilterNotes(t *testing.T) {
	store := newTestStore(t)
	f, _ := store.CreateFeature("list-notes-only", "Filter notes")

	store.CreateNote(f.ID, "", "A note", "note")
	store.CreateFact(f.ID, "", "db", "uses", "sqlite")

	items, err := store.ListMemories(f.ID, memory_filter(t, "notes", nil, 20))
	if err != nil {
		t.Fatalf("ListMemories: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 note, got %d", len(items))
	}
	if items[0].Type != "note" {
		t.Errorf("expected type 'note', got %q", items[0].Type)
	}
}

func TestListMemories_FilterFacts(t *testing.T) {
	store := newTestStore(t)
	f, _ := store.CreateFeature("list-facts-only", "Filter facts")

	store.CreateNote(f.ID, "", "A note", "note")
	store.CreateFact(f.ID, "", "db", "uses", "sqlite")

	items, err := store.ListMemories(f.ID, memory_filter(t, "facts", nil, 20))
	if err != nil {
		t.Fatalf("ListMemories: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 fact, got %d", len(items))
	}
	if items[0].Type != "fact" {
		t.Errorf("expected type 'fact', got %q", items[0].Type)
	}
}

func TestListMemories_PinnedFilter(t *testing.T) {
	store := newTestStore(t)
	f, _ := store.CreateFeature("list-pinned", "Pinned filter")

	n1, _ := store.CreateNote(f.ID, "", "Pinned note", "note")
	store.CreateNote(f.ID, "", "Unpinned note", "note")
	store.PinMemory(n1.ID, "note")

	pinned := true
	items, err := store.ListMemories(f.ID, memory_filter(t, "", &pinned, 20))
	if err != nil {
		t.Fatalf("ListMemories: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 pinned item, got %d", len(items))
	}
	if items[0].ID != n1.ID {
		t.Errorf("expected pinned note ID %q, got %q", n1.ID, items[0].ID)
	}
}

func TestListMemories_Limit(t *testing.T) {
	store := newTestStore(t)
	f, _ := store.CreateFeature("list-limit", "Limit test")

	for i := 0; i < 10; i++ {
		store.CreateNote(f.ID, "", "Note content", "note")
	}

	items, err := store.ListMemories(f.ID, memory_filter(t, "", nil, 3))
	if err != nil {
		t.Fatalf("ListMemories: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items (limited), got %d", len(items))
	}
}

func TestGetContext_IncludesPinnedAtCompactTier(t *testing.T) {
	store := newTestStore(t)
	f, _ := store.CreateFeature("pinned-compact", "Pinned in compact tier")

	// Create notes and facts
	n1, _ := store.CreateNote(f.ID, "", "Important pinned note", "decision")
	store.CreateNote(f.ID, "", "Regular note", "note")
	fact, _ := store.CreateFact(f.ID, "", "db", "uses", "sqlite")

	// Pin the note and fact
	store.PinMemory(n1.ID, "note")
	store.PinMemory(fact.ID, "fact")

	// Get compact context (normally has 0 notes, 0 facts)
	ctx, err := store.GetContext(f.ID, "compact", nil)
	if err != nil {
		t.Fatalf("GetContext compact: %v", err)
	}

	// Compact should still have 0 regular notes and 0 regular facts
	if len(ctx.RecentNotes) != 0 {
		t.Errorf("compact tier should have 0 recent notes, got %d", len(ctx.RecentNotes))
	}
	if len(ctx.ActiveFacts) != 0 {
		t.Errorf("compact tier should have 0 active facts, got %d", len(ctx.ActiveFacts))
	}

	// But pinned memories should always be present
	if len(ctx.PinnedMemories) != 2 {
		t.Fatalf("expected 2 pinned memories in compact tier, got %d", len(ctx.PinnedMemories))
	}

	// Verify types
	types := map[string]int{}
	for _, m := range ctx.PinnedMemories {
		types[m.Type]++
		if !m.Pinned {
			t.Errorf("expected pinned=true for memory %s", m.ID)
		}
	}
	if types["note"] != 1 {
		t.Errorf("expected 1 pinned note, got %d", types["note"])
	}
	if types["fact"] != 1 {
		t.Errorf("expected 1 pinned fact, got %d", types["fact"])
	}
}

func TestPinMemory_NotFound(t *testing.T) {
	store := newTestStore(t)
	err := store.PinMemory("nonexistent-id", "note")
	if err == nil {
		t.Fatal("expected error for nonexistent note")
	}
}

func TestPinMemory_InvalidType(t *testing.T) {
	store := newTestStore(t)
	err := store.PinMemory("some-id", "invalid")
	if err == nil {
		t.Fatal("expected error for invalid memory type")
	}
}

func TestListMemories_ExcludesInvalidatedFacts(t *testing.T) {
	store := newTestStore(t)
	f, _ := store.CreateFeature("invalidated-facts", "Invalidated test")

	fact, _ := store.CreateFact(f.ID, "", "db", "uses", "postgres")
	store.InvalidateFact(fact.ID)

	// Create a new fact (contradicting)
	store.CreateFact(f.ID, "", "db", "uses", "sqlite")

	items, err := store.ListMemories(f.ID, memory_filter(t, "facts", nil, 20))
	if err != nil {
		t.Fatalf("ListMemories: %v", err)
	}
	// Should only see the active fact, not the invalidated one
	if len(items) != 1 {
		t.Fatalf("expected 1 active fact, got %d", len(items))
	}
}

// memory_filter is a test helper that builds a MemoryFilter.
func memory_filter(t *testing.T, typ string, pinned *bool, limit int) memory.MemoryFilter {
	t.Helper()
	return memory.MemoryFilter{Type: typ, Pinned: pinned, Limit: limit}
}
