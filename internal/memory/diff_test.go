package memory

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/arbazkhan971/memorx/internal/storage"
)

func setupDiffStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, ".memory", "memory.db")
	db, err := storage.NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := storage.Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return NewStore(db)
}

func TestGetDiff_WithNewData(t *testing.T) {
	store := setupDiffStore(t)

	// Create feature and session.
	feat, err := store.CreateFeature("diff-test", "test diff feature")
	if err != nil {
		t.Fatalf("CreateFeature: %v", err)
	}
	sess, err := store.CreateSession(feat.ID, "test")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// Mark a time before creating data.
	before := time.Now().UTC().Add(-1 * time.Second)

	// Create some facts.
	_, err = store.CreateFact(feat.ID, sess.ID, "auth", "uses", "jwt")
	if err != nil {
		t.Fatalf("CreateFact: %v", err)
	}
	_, err = store.CreateFact(feat.ID, sess.ID, "db", "type", "postgres")
	if err != nil {
		t.Fatalf("CreateFact: %v", err)
	}

	// Create some notes of different types.
	_, err = store.CreateNote(feat.ID, sess.ID, "decided to use REST", "decision")
	if err != nil {
		t.Fatalf("CreateNote decision: %v", err)
	}
	_, err = store.CreateNote(feat.ID, sess.ID, "set up project structure", "progress")
	if err != nil {
		t.Fatalf("CreateNote progress: %v", err)
	}
	_, err = store.CreateNote(feat.ID, sess.ID, "need to add tests", "progress")
	if err != nil {
		t.Fatalf("CreateNote progress2: %v", err)
	}

	// Track some files.
	if err := store.TrackFile(feat.ID, sess.ID, "main.go", "added"); err != nil {
		t.Fatalf("TrackFile: %v", err)
	}
	if err := store.TrackFile(feat.ID, sess.ID, "go.mod", "added"); err != nil {
		t.Fatalf("TrackFile: %v", err)
	}

	// Get the diff.
	diff, err := store.GetDiff(feat.ID, before)
	if err != nil {
		t.Fatalf("GetDiff: %v", err)
	}

	if len(diff.NewFacts) != 2 {
		t.Errorf("expected 2 new facts, got %d", len(diff.NewFacts))
	}
	if len(diff.InvalidatedFacts) != 0 {
		t.Errorf("expected 0 invalidated facts, got %d", len(diff.InvalidatedFacts))
	}
	if len(diff.NewNotes) != 3 {
		t.Errorf("expected 3 new notes, got %d", len(diff.NewNotes))
	}
	if len(diff.NewFiles) != 2 {
		t.Errorf("expected 2 new files, got %d", len(diff.NewFiles))
	}
	if diff.SessionsSince < 1 {
		t.Errorf("expected at least 1 session since, got %d", diff.SessionsSince)
	}
	if diff.PlanDelta != "no plan" {
		t.Errorf("expected 'no plan' for plan delta, got %q", diff.PlanDelta)
	}
}

func TestGetDiff_NoChanges(t *testing.T) {
	store := setupDiffStore(t)

	feat, err := store.CreateFeature("diff-empty", "no changes test")
	if err != nil {
		t.Fatalf("CreateFeature: %v", err)
	}

	// Get diff from the future — nothing should appear.
	future := time.Now().UTC().Add(1 * time.Hour)
	diff, err := store.GetDiff(feat.ID, future)
	if err != nil {
		t.Fatalf("GetDiff: %v", err)
	}

	if len(diff.NewFacts) != 0 {
		t.Errorf("expected 0 new facts, got %d", len(diff.NewFacts))
	}
	if len(diff.InvalidatedFacts) != 0 {
		t.Errorf("expected 0 invalidated facts, got %d", len(diff.InvalidatedFacts))
	}
	if len(diff.NewNotes) != 0 {
		t.Errorf("expected 0 new notes, got %d", len(diff.NewNotes))
	}
	if diff.NewCommits != 0 {
		t.Errorf("expected 0 new commits, got %d", diff.NewCommits)
	}
	if diff.NewLinks != 0 {
		t.Errorf("expected 0 new links, got %d", diff.NewLinks)
	}
	if len(diff.NewFiles) != 0 {
		t.Errorf("expected 0 new files, got %d", len(diff.NewFiles))
	}
	if diff.SessionsSince != 0 {
		t.Errorf("expected 0 sessions since, got %d", diff.SessionsSince)
	}
}

func TestGetDiff_WithInvalidatedFacts(t *testing.T) {
	store := setupDiffStore(t)

	feat, err := store.CreateFeature("diff-invalidated", "invalidation test")
	if err != nil {
		t.Fatalf("CreateFeature: %v", err)
	}
	sess, err := store.CreateSession(feat.ID, "test")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// Create a fact, then supersede it.
	f1, err := store.CreateFact(feat.ID, sess.ID, "db", "type", "mysql")
	if err != nil {
		t.Fatalf("CreateFact: %v", err)
	}

	before := time.Now().UTC().Add(-1 * time.Second)

	// Supersede: creating a fact with the same subject+predicate but different object invalidates old one.
	_, err = store.CreateFact(feat.ID, sess.ID, "db", "type", "postgres")
	if err != nil {
		t.Fatalf("CreateFact supersede: %v", err)
	}

	diff, err := store.GetDiff(feat.ID, before)
	if err != nil {
		t.Fatalf("GetDiff: %v", err)
	}

	if len(diff.NewFacts) != 1 {
		t.Errorf("expected 1 new fact (postgres), got %d", len(diff.NewFacts))
	}
	if len(diff.NewFacts) > 0 && diff.NewFacts[0].Object != "postgres" {
		t.Errorf("expected new fact object 'postgres', got %q", diff.NewFacts[0].Object)
	}

	if len(diff.InvalidatedFacts) != 1 {
		t.Errorf("expected 1 invalidated fact (mysql), got %d", len(diff.InvalidatedFacts))
	}
	if len(diff.InvalidatedFacts) > 0 && diff.InvalidatedFacts[0].ID != f1.ID {
		t.Errorf("expected invalidated fact to be the original mysql one")
	}
}

func TestGetDiff_AcrossSessions(t *testing.T) {
	store := setupDiffStore(t)

	feat, err := store.CreateFeature("diff-sessions", "across sessions test")
	if err != nil {
		t.Fatalf("CreateFeature: %v", err)
	}

	// First session: create some data and end it.
	sess1, err := store.CreateSession(feat.ID, "test")
	if err != nil {
		t.Fatalf("CreateSession 1: %v", err)
	}
	_, err = store.CreateNote(feat.ID, sess1.ID, "first session note", "progress")
	if err != nil {
		t.Fatalf("CreateNote sess1: %v", err)
	}
	if err := store.EndSession(sess1.ID); err != nil {
		t.Fatalf("EndSession 1: %v", err)
	}

	// Verify GetLastSessionEndTime returns a valid time.
	lastEnd, err := store.GetLastSessionEndTime(feat.ID)
	if err != nil {
		t.Fatalf("GetLastSessionEndTime: %v", err)
	}
	if lastEnd.IsZero() {
		t.Fatal("expected non-zero last session end time")
	}

	// Second session: create more data.
	sess2, err := store.CreateSession(feat.ID, "test")
	if err != nil {
		t.Fatalf("CreateSession 2: %v", err)
	}
	_, err = store.CreateNote(feat.ID, sess2.ID, "second session note", "decision")
	if err != nil {
		t.Fatalf("CreateNote sess2: %v", err)
	}
	_, err = store.CreateFact(feat.ID, sess2.ID, "api", "format", "rest")
	if err != nil {
		t.Fatalf("CreateFact sess2: %v", err)
	}

	// Diff since last session end. Due to second-precision timestamps in tests,
	// all data may share the same second. We verify that at least session 2
	// data appears and that GetDiff + GetLastSessionEndTime work together.
	diff, err := store.GetDiff(feat.ID, lastEnd)
	if err != nil {
		t.Fatalf("GetDiff: %v", err)
	}

	// Session 2 note must appear (created at >= lastEnd).
	foundSecondNote := false
	for _, n := range diff.NewNotes {
		if n.Content == "second session note" {
			foundSecondNote = true
		}
	}
	if !foundSecondNote {
		t.Errorf("expected to find 'second session note' in diff, notes: %v", diff.NewNotes)
	}
	if len(diff.NewFacts) < 1 {
		t.Errorf("expected at least 1 new fact from session 2, got %d", len(diff.NewFacts))
	}
	// Should see at least 1 session since last end.
	if diff.SessionsSince < 1 {
		t.Errorf("expected at least 1 session since last end, got %d", diff.SessionsSince)
	}
}

func TestGetLastSessionEndTime_NoSessions(t *testing.T) {
	store := setupDiffStore(t)

	feat, err := store.CreateFeature("no-sessions", "no sessions test")
	if err != nil {
		t.Fatalf("CreateFeature: %v", err)
	}

	lastEnd, err := store.GetLastSessionEndTime(feat.ID)
	if err != nil {
		t.Fatalf("GetLastSessionEndTime: %v", err)
	}
	if !lastEnd.IsZero() {
		t.Errorf("expected zero time for feature with no sessions, got %v", lastEnd)
	}
}
