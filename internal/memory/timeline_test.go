package memory_test

import (
	"strings"
	"testing"

	"github.com/arbazkhan971/memorx/internal/memory"
	"github.com/google/uuid"
)

func TestGetTimeline_EmptyDB(t *testing.T) {
	store := newTestStore(t)
	events, err := store.GetTimeline(30, "")
	if err != nil {
		t.Fatalf("GetTimeline: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events on empty DB, got %d", len(events))
	}
}

func TestGetTimeline_SessionsAndNotes(t *testing.T) {
	store := newTestStore(t)
	f, _ := store.CreateFeature("timeline-test", "Test timeline")
	sess, _ := store.CreateSession(f.ID, "claude-code")
	store.CreateNote(f.ID, sess.ID, "chose opaque tokens for compliance", "decision")
	store.CreateNote(f.ID, sess.ID, "token refresh working", "progress")
	store.EndSessionWithSummary(sess.ID, "completed token refresh tests")

	events, err := store.GetTimeline(30, "")
	if err != nil {
		t.Fatalf("GetTimeline: %v", err)
	}

	if len(events) < 3 {
		t.Fatalf("expected at least 3 events (session started, 2 notes), got %d", len(events))
	}

	// Verify we have session and note events
	types := map[string]bool{}
	for _, e := range events {
		types[e.Type] = true
	}
	if !types["session"] {
		t.Error("expected session events in timeline")
	}
	if !types["decision"] {
		t.Error("expected decision events in timeline")
	}
	if !types["progress"] {
		t.Error("expected progress events in timeline")
	}
}

func TestGetTimeline_FilterByFeature(t *testing.T) {
	store := newTestStore(t)

	f1, _ := store.CreateFeature("feature-a", "Feature A")
	f2, _ := store.CreateFeature("feature-b", "Feature B")

	store.CreateNote(f1.ID, "", "note for A", "note")
	store.CreateNote(f2.ID, "", "note for B", "note")

	events, err := store.GetTimeline(30, "feature-a")
	if err != nil {
		t.Fatalf("GetTimeline: %v", err)
	}

	for _, e := range events {
		if e.Feature != "feature-a" {
			t.Errorf("expected all events for feature-a, got event for %q", e.Feature)
		}
	}
}

func TestGetTimeline_FeatureNotFound(t *testing.T) {
	store := newTestStore(t)
	_, err := store.GetTimeline(30, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent feature")
	}
}

func TestGetTimeline_WithCommits(t *testing.T) {
	store, db := newTestStoreWithDB(t)
	f, _ := store.CreateFeature("commit-timeline", "Test commits in timeline")

	// Insert a commit directly
	w := db.Writer()
	w.Exec(`INSERT INTO commits (id, feature_id, hash, message, author, intent_type, committed_at)
		VALUES (?, ?, ?, 'feat: add token rotation', 'test', 'feature', datetime('now'))`,
		uuid.New().String(), f.ID, "abc1234")

	events, err := store.GetTimeline(30, "commit-timeline")
	if err != nil {
		t.Fatalf("GetTimeline: %v", err)
	}

	foundCommit := false
	for _, e := range events {
		if e.Type == "commit" && strings.Contains(e.Content, "token rotation") {
			foundCommit = true
		}
	}
	if !foundCommit {
		t.Error("expected commit event in timeline")
	}
}

func TestGetTimeline_WithFacts(t *testing.T) {
	store := newTestStore(t)
	f, _ := store.CreateFeature("fact-timeline", "Test facts in timeline")
	store.CreateFact(f.ID, "", "auth", "uses", "JWT")

	events, err := store.GetTimeline(30, "fact-timeline")
	if err != nil {
		t.Fatalf("GetTimeline: %v", err)
	}

	foundFact := false
	for _, e := range events {
		if e.Type == "fact" && strings.Contains(e.Content, "JWT") {
			foundFact = true
		}
	}
	if !foundFact {
		t.Error("expected fact event in timeline")
	}
}

func TestGetTimeline_ChronologicalOrder(t *testing.T) {
	store := newTestStore(t)
	f, _ := store.CreateFeature("chrono-timeline", "Test chronological order")

	store.CreateNote(f.ID, "", "first note", "note")
	store.CreateNote(f.ID, "", "second note", "note")

	events, err := store.GetTimeline(30, "chrono-timeline")
	if err != nil {
		t.Fatalf("GetTimeline: %v", err)
	}

	// Verify chronological order (earliest first)
	for i := 1; i < len(events); i++ {
		if events[i].Timestamp < events[i-1].Timestamp {
			t.Errorf("events not in chronological order: %s < %s", events[i].Timestamp, events[i-1].Timestamp)
		}
	}
}

func TestGetTimeline_DefaultDays(t *testing.T) {
	store := newTestStore(t)
	// 0 or negative should default to 30
	events, err := store.GetTimeline(0, "")
	if err != nil {
		t.Fatalf("GetTimeline: %v", err)
	}
	// Just verify it doesn't error out
	_ = events
}

func TestFormatTimeline_Empty(t *testing.T) {
	result := memory.FormatTimeline(nil)
	if !strings.Contains(result, "No events") {
		t.Errorf("expected 'No events' for empty list, got: %s", result)
	}
}

func TestFormatTimeline_WithEvents(t *testing.T) {
	events := []memory.TimelineEvent{
		{Timestamp: "2025-03-23 14:00:00", Type: "session", Content: "claude-code started", Feature: "auth-v2"},
		{Timestamp: "2025-03-23 14:05:00", Type: "decision", Content: "chose opaque tokens", Feature: "auth-v2"},
	}
	result := memory.FormatTimeline(events)
	if !strings.Contains(result, "[session]") {
		t.Errorf("expected [session] tag, got: %s", result)
	}
	if !strings.Contains(result, "[decision]") {
		t.Errorf("expected [decision] tag, got: %s", result)
	}
	if !strings.Contains(result, "auth-v2") {
		t.Errorf("expected feature name, got: %s", result)
	}
}
