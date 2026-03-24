package memory_test

import (
	"strings"
	"testing"

	"github.com/arbazkhan971/memorx/internal/memory"
)

func TestReplaySession_Basic(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("replay-feat", "Replay test")
	sess, _ := store.CreateSession(f.ID, "claude-code")

	// Create events during the session
	store.CreateNote(f.ID, sess.ID, "Started auth implementation", "note")
	store.CreateNote(f.ID, sess.ID, "Decided to use JWT", "decision")
	store.CreateFact(f.ID, sess.ID, "auth", "uses", "JWT")

	// End the session
	store.EndSession(sess.ID)

	// Replay the session
	replay, err := store.ReplaySession(sess.ID)
	if err != nil {
		t.Fatalf("ReplaySession: %v", err)
	}

	if replay.SessionID != sess.ID {
		t.Errorf("expected session ID %q, got %q", sess.ID, replay.SessionID)
	}
	if replay.Tool != "claude-code" {
		t.Errorf("expected tool 'claude-code', got %q", replay.Tool)
	}
	if replay.Feature != "replay-feat" {
		t.Errorf("expected feature 'replay-feat', got %q", replay.Feature)
	}
	if replay.StartedAt == "" {
		t.Error("expected non-empty StartedAt")
	}
	if replay.EndedAt == "" {
		t.Error("expected non-empty EndedAt")
	}

	// Should have 3 events: 2 notes + 1 fact
	if len(replay.Events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(replay.Events))
	}

	// Verify event types
	typeCount := map[string]int{}
	for _, e := range replay.Events {
		typeCount[e.Type]++
	}
	if typeCount["note"] != 2 {
		t.Errorf("expected 2 note events, got %d", typeCount["note"])
	}
	if typeCount["fact"] != 1 {
		t.Errorf("expected 1 fact event, got %d", typeCount["fact"])
	}
}

func TestReplaySession_DefaultLastCompleted(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("replay-default", "Default replay test")
	sess, _ := store.CreateSession(f.ID, "test-tool")
	store.CreateNote(f.ID, sess.ID, "A note in session", "note")
	store.EndSession(sess.ID)

	// Replay without specifying session ID (should default to last completed)
	replay, err := store.ReplaySession("")
	if err != nil {
		t.Fatalf("ReplaySession default: %v", err)
	}

	if replay.SessionID != sess.ID {
		t.Errorf("expected last completed session %q, got %q", sess.ID, replay.SessionID)
	}
}

func TestReplaySession_NotFound(t *testing.T) {
	store := newTestStore(t)

	_, err := store.ReplaySession("nonexistent-session-id")
	if err == nil {
		t.Fatal("expected error for nonexistent session")
	}
}

func TestReplaySession_NoCompletedSessions(t *testing.T) {
	store := newTestStore(t)

	_, err := store.ReplaySession("")
	if err == nil {
		t.Fatal("expected error when no completed sessions exist")
	}
}

func TestReplaySession_EmptySession(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("replay-empty", "Empty session")
	sess, _ := store.CreateSession(f.ID, "test-tool")
	store.EndSession(sess.ID)

	replay, err := store.ReplaySession(sess.ID)
	if err != nil {
		t.Fatalf("ReplaySession: %v", err)
	}

	if len(replay.Events) != 0 {
		t.Errorf("expected 0 events for empty session, got %d", len(replay.Events))
	}
}

func TestReplaySession_ChronologicalOrder(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("replay-chrono", "Chronological test")
	sess, _ := store.CreateSession(f.ID, "test-tool")

	// Create events in order
	store.CreateNote(f.ID, sess.ID, "First event", "note")
	store.CreateFact(f.ID, sess.ID, "test", "is", "working")
	store.CreateNote(f.ID, sess.ID, "Third event", "decision")

	store.EndSession(sess.ID)

	replay, err := store.ReplaySession(sess.ID)
	if err != nil {
		t.Fatalf("ReplaySession: %v", err)
	}

	if len(replay.Events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(replay.Events))
	}

	// Events should be in chronological order
	for i := 1; i < len(replay.Events); i++ {
		if replay.Events[i].Timestamp < replay.Events[i-1].Timestamp {
			t.Errorf("events not in chronological order: %q before %q",
				replay.Events[i-1].Timestamp, replay.Events[i].Timestamp)
		}
	}
}

func TestFormatReplay(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("replay-format", "Format test")
	sess, _ := store.CreateSession(f.ID, "claude-code")
	store.CreateNote(f.ID, sess.ID, "A note", "note")
	store.CreateFact(f.ID, sess.ID, "auth", "uses", "JWT")
	store.EndSession(sess.ID)

	replay, err := store.ReplaySession(sess.ID)
	if err != nil {
		t.Fatalf("ReplaySession: %v", err)
	}

	formatted := memory.FormatReplay(replay)
	if !strings.Contains(formatted, "Session Replay") {
		t.Errorf("expected 'Session Replay' in output, got: %s", formatted)
	}
	if !strings.Contains(formatted, "claude-code") {
		t.Errorf("expected tool name in output, got: %s", formatted)
	}
	if !strings.Contains(formatted, "replay-format") {
		t.Errorf("expected feature name in output, got: %s", formatted)
	}
	if !strings.Contains(formatted, "Events: 2") {
		t.Errorf("expected 'Events: 2' in output, got: %s", formatted)
	}
}

func TestFormatReplay_Nil(t *testing.T) {
	formatted := memory.FormatReplay(nil)
	if formatted != "No session replay data." {
		t.Errorf("expected nil message, got: %s", formatted)
	}
}

func TestFormatReplay_Empty(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("replay-fmt-empty", "Empty format test")
	sess, _ := store.CreateSession(f.ID, "test-tool")
	store.EndSession(sess.ID)

	replay, err := store.ReplaySession(sess.ID)
	if err != nil {
		t.Fatalf("ReplaySession: %v", err)
	}

	formatted := memory.FormatReplay(replay)
	if !strings.Contains(formatted, "No events recorded") {
		t.Errorf("expected 'No events recorded' in output, got: %s", formatted)
	}
}
