package memory_test

import (
	"testing"
	"time"
)

func TestCreateNote(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("feat-a", "A")
	sess, _ := store.CreateSession(f.ID, "claude-code")

	note, err := store.CreateNote(f.ID, sess.ID, "Decided to use SQLite for storage", "decision")
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}

	if note.ID == "" {
		t.Error("expected non-empty ID")
	}
	if note.Content != "Decided to use SQLite for storage" {
		t.Errorf("expected content match, got %q", note.Content)
	}
	if note.Type != "decision" {
		t.Errorf("expected type 'decision', got %q", note.Type)
	}
	if note.FeatureID != f.ID {
		t.Errorf("expected feature ID %q, got %q", f.ID, note.FeatureID)
	}
	if note.SessionID != sess.ID {
		t.Errorf("expected session ID %q, got %q", sess.ID, note.SessionID)
	}
}

func TestCreateNote_DefaultType(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("feat-a", "A")

	note, err := store.CreateNote(f.ID, "", "A general note", "")
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}
	if note.Type != "note" {
		t.Errorf("expected default type 'note', got %q", note.Type)
	}
}

func TestListNotes_All(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("feat-a", "A")
	store.CreateNote(f.ID, "", "Note 1", "note")
	store.CreateNote(f.ID, "", "Note 2", "decision")
	store.CreateNote(f.ID, "", "Note 3", "blocker")

	notes, err := store.ListNotes(f.ID, "", 10)
	if err != nil {
		t.Fatalf("ListNotes: %v", err)
	}
	if len(notes) != 3 {
		t.Fatalf("expected 3 notes, got %d", len(notes))
	}
}

func TestListNotes_FilterByType(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("feat-a", "A")
	store.CreateNote(f.ID, "", "Note 1", "note")
	store.CreateNote(f.ID, "", "Decision 1", "decision")
	store.CreateNote(f.ID, "", "Decision 2", "decision")

	decisions, err := store.ListNotes(f.ID, "decision", 10)
	if err != nil {
		t.Fatalf("ListNotes: %v", err)
	}
	if len(decisions) != 2 {
		t.Fatalf("expected 2 decisions, got %d", len(decisions))
	}
	for _, n := range decisions {
		if n.Type != "decision" {
			t.Errorf("expected type 'decision', got %q", n.Type)
		}
	}
}

func TestListNotes_Limit(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("feat-a", "A")
	store.CreateNote(f.ID, "", "Note 1", "note")
	store.CreateNote(f.ID, "", "Note 2", "note")
	store.CreateNote(f.ID, "", "Note 3", "note")

	notes, err := store.ListNotes(f.ID, "", 2)
	if err != nil {
		t.Fatalf("ListNotes: %v", err)
	}
	if len(notes) != 2 {
		t.Fatalf("expected 2 notes (limited), got %d", len(notes))
	}
}

func TestListNotes_FilterByFeature(t *testing.T) {
	store := newTestStore(t)

	fa, _ := store.CreateFeature("feat-a", "A")
	fb, _ := store.CreateFeature("feat-b", "B")
	store.CreateNote(fa.ID, "", "Note for A", "note")
	store.CreateNote(fb.ID, "", "Note for B", "note")

	notes, err := store.ListNotes(fa.ID, "", 10)
	if err != nil {
		t.Fatalf("ListNotes: %v", err)
	}
	if len(notes) != 1 {
		t.Fatalf("expected 1 note for feat-a, got %d", len(notes))
	}
	if notes[0].Content != "Note for A" {
		t.Errorf("expected 'Note for A', got %q", notes[0].Content)
	}
}

func TestGetNote(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("feat-a", "A")
	created, _ := store.CreateNote(f.ID, "", "A note", "note")

	got, err := store.GetNote(created.ID)
	if err != nil {
		t.Fatalf("GetNote: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("ID mismatch: %q vs %q", got.ID, created.ID)
	}
	if got.Content != "A note" {
		t.Errorf("Content mismatch: %q", got.Content)
	}
}

func TestGetNote_NotFound(t *testing.T) {
	store := newTestStore(t)

	_, err := store.GetNote("nonexistent-id")
	if err == nil {
		t.Fatal("expected error for nonexistent note")
	}
}

func TestCreateNote_VeryLongContent(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("feat-long", "Long Content Feature")

	// Create content > 10000 characters
	longContent := ""
	for i := 0; i < 200; i++ {
		longContent += "This is a repeated sentence to build very long content. "
	}
	if len(longContent) < 10000 {
		t.Fatalf("expected content > 10000 chars, got %d", len(longContent))
	}

	note, err := store.CreateNote(f.ID, "", longContent, "note")
	if err != nil {
		t.Fatalf("CreateNote with long content: %v", err)
	}
	if note.Content != longContent {
		t.Error("note content does not match the long input")
	}

	// Verify we can retrieve it
	got, err := store.GetNote(note.ID)
	if err != nil {
		t.Fatalf("GetNote: %v", err)
	}
	if got.Content != longContent {
		t.Error("retrieved note content does not match long input")
	}
}

func TestListNotes_LimitZeroUsesDefault(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("feat-lz", "Limit Zero")
	for i := 0; i < 5; i++ {
		store.CreateNote(f.ID, "", "Note content", "note")
	}

	// limit=0 should use default (50), so all 5 notes are returned
	notes, err := store.ListNotes(f.ID, "", 0)
	if err != nil {
		t.Fatalf("ListNotes limit=0: %v", err)
	}
	if len(notes) != 5 {
		t.Errorf("expected 5 notes with limit=0 (default), got %d", len(notes))
	}
}

func TestListNotes_LimitOne(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("feat-l1", "Limit One")
	store.CreateNote(f.ID, "", "First note", "note")
	store.CreateNote(f.ID, "", "Second note", "note")
	store.CreateNote(f.ID, "", "Third note", "note")

	notes, err := store.ListNotes(f.ID, "", 1)
	if err != nil {
		t.Fatalf("ListNotes limit=1: %v", err)
	}
	if len(notes) != 1 {
		t.Fatalf("expected 1 note with limit=1, got %d", len(notes))
	}
}

func TestCreateNote_EachNoteType(t *testing.T) {
	store := newTestStore(t)
	f, _ := store.CreateFeature("feat-types", "Note types test")

	types := []string{"progress", "decision", "blocker", "next_step", "note"}
	for _, noteType := range types {
		t.Run(noteType, func(t *testing.T) {
			note, err := store.CreateNote(f.ID, "", "Content for "+noteType, noteType)
			if err != nil {
				t.Fatalf("CreateNote(%s): %v", noteType, err)
			}
			if note.Type != noteType {
				t.Errorf("expected type %q, got %q", noteType, note.Type)
			}
		})
	}
}

func TestListNotes_FilterBlocker(t *testing.T) {
	store := newTestStore(t)
	f, _ := store.CreateFeature("filter-blocker", "Filter")
	store.CreateNote(f.ID, "", "b1", "blocker")
	store.CreateNote(f.ID, "", "n1", "note")
	notes, err := store.ListNotes(f.ID, "blocker", 10)
	if err != nil {
		t.Fatalf("ListNotes blocker: %v", err)
	}
	if len(notes) != 1 || notes[0].Type != "blocker" {
		t.Errorf("expected 1 blocker, got %d", len(notes))
	}
}

func TestListNotes_FilterNextStep(t *testing.T) {
	store := newTestStore(t)
	f, _ := store.CreateFeature("filter-ns", "Filter")
	store.CreateNote(f.ID, "", "ns1", "next_step")
	store.CreateNote(f.ID, "", "ns2", "next_step")
	store.CreateNote(f.ID, "", "p1", "progress")
	notes, err := store.ListNotes(f.ID, "next_step", 10)
	if err != nil {
		t.Fatalf("ListNotes next_step: %v", err)
	}
	if len(notes) != 2 {
		t.Errorf("expected 2 next_step, got %d", len(notes))
	}
}

func TestListNotes_FilterProgress(t *testing.T) {
	store := newTestStore(t)
	f, _ := store.CreateFeature("filter-prog", "Filter")
	store.CreateNote(f.ID, "", "p1", "progress")
	store.CreateNote(f.ID, "", "d1", "decision")
	notes, err := store.ListNotes(f.ID, "progress", 10)
	if err != nil {
		t.Fatalf("ListNotes progress: %v", err)
	}
	if len(notes) != 1 || notes[0].Type != "progress" {
		t.Errorf("expected 1 progress, got %d", len(notes))
	}
}

func TestCreateNote_AllTypes(t *testing.T) {
	store := newTestStore(t)
	f, _ := store.CreateFeature("all-types", "All types")
	types := []struct{ name, want string }{
		{"progress", "progress"}, {"decision", "decision"}, {"blocker", "blocker"},
		{"next_step", "next_step"}, {"note", "note"},
	}
	for _, tc := range types {
		t.Run(tc.name, func(t *testing.T) {
			n, err := store.CreateNote(f.ID, "", "content", tc.name)
			if err != nil {
				t.Fatalf("CreateNote(%s): %v", tc.name, err)
			}
			if n.Type != tc.want {
				t.Errorf("got type %q, want %q", n.Type, tc.want)
			}
		})
	}
}

func TestNoteType_ProgressCreateRetrieve(t *testing.T) {
	store := newTestStore(t)
	f, _ := store.CreateFeature("nt-prog", "test")
	n, _ := store.CreateNote(f.ID, "", "progress content", "progress")
	got, err := store.GetNote(n.ID)
	if err != nil { t.Fatalf("GetNote: %v", err) }
	if got.Type != "progress" { t.Errorf("got %q", got.Type) }
}

func TestNoteType_DecisionCreateRetrieve(t *testing.T) {
	store := newTestStore(t)
	f, _ := store.CreateFeature("nt-dec", "test")
	n, _ := store.CreateNote(f.ID, "", "decision content", "decision")
	got, err := store.GetNote(n.ID)
	if err != nil { t.Fatalf("GetNote: %v", err) }
	if got.Type != "decision" { t.Errorf("got %q", got.Type) }
}

func TestNoteType_BlockerCreateRetrieve(t *testing.T) {
	store := newTestStore(t)
	f, _ := store.CreateFeature("nt-blk", "test")
	n, _ := store.CreateNote(f.ID, "", "blocker content", "blocker")
	got, err := store.GetNote(n.ID)
	if err != nil { t.Fatalf("GetNote: %v", err) }
	if got.Type != "blocker" { t.Errorf("got %q", got.Type) }
}

func TestNoteType_NextStepCreateRetrieve(t *testing.T) {
	store := newTestStore(t)
	f, _ := store.CreateFeature("nt-ns", "test")
	n, _ := store.CreateNote(f.ID, "", "next_step content", "next_step")
	got, err := store.GetNote(n.ID)
	if err != nil { t.Fatalf("GetNote: %v", err) }
	if got.Type != "next_step" { t.Errorf("got %q", got.Type) }
}

func TestNoteType_NoteCreateRetrieve(t *testing.T) {
	store := newTestStore(t)
	f, _ := store.CreateFeature("nt-note", "test")
	n, _ := store.CreateNote(f.ID, "", "note content", "note")
	got, err := store.GetNote(n.ID)
	if err != nil { t.Fatalf("GetNote: %v", err) }
	if got.Type != "note" { t.Errorf("got %q", got.Type) }
}

func TestNoteTypes_CreateAndRetrieve(t *testing.T) {
	store := newTestStore(t)
	f, _ := store.CreateFeature("feat-types-cr", "Note types create+retrieve")
	for _, tc := range []struct{ name, noteType string }{
		{"progress_type", "progress"}, {"decision_type", "decision"},
		{"blocker_type", "blocker"}, {"next_step_type", "next_step"}, {"note_type", "note"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			created, err := store.CreateNote(f.ID, "", "Content for "+tc.noteType, tc.noteType)
			if err != nil { t.Fatalf("CreateNote(%s): %v", tc.noteType, err) }
			got, err := store.GetNote(created.ID)
			if err != nil { t.Fatalf("GetNote: %v", err) }
			if got.Type != tc.noteType { t.Errorf("got %q, want %q", got.Type, tc.noteType) }
		})
	}
}

func TestListNotes_OrderNewestFirst(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("feat-order", "Ordering")

	// Create notes with 1-second gaps so created_at definitely differs.
	// created_at uses second-precision formatting.
	store.CreateNote(f.ID, "", "First created", "note")
	time.Sleep(1100 * time.Millisecond)
	store.CreateNote(f.ID, "", "Second created", "note")
	time.Sleep(1100 * time.Millisecond)
	store.CreateNote(f.ID, "", "Third created", "note")

	notes, err := store.ListNotes(f.ID, "", 10)
	if err != nil {
		t.Fatalf("ListNotes: %v", err)
	}
	if len(notes) != 3 {
		t.Fatalf("expected 3 notes, got %d", len(notes))
	}
	// ListNotes orders by created_at DESC, so newest should come first
	if notes[0].Content != "Third created" {
		t.Errorf("expected newest note first ('Third created'), got %q", notes[0].Content)
	}
	if notes[2].Content != "First created" {
		t.Errorf("expected oldest note last ('First created'), got %q", notes[2].Content)
	}
}
