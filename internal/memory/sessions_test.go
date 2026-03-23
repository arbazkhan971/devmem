package memory_test

import (
	"testing"
)

func TestCreateSession(t *testing.T) {
	store := newTestStore(t)

	f, err := store.CreateFeature("feat-a", "A")
	if err != nil {
		t.Fatalf("CreateFeature: %v", err)
	}

	sess, err := store.CreateSession(f.ID, "claude-code")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	if sess.ID == "" {
		t.Error("expected non-empty ID")
	}
	if sess.FeatureID != f.ID {
		t.Errorf("expected feature ID %q, got %q", f.ID, sess.FeatureID)
	}
	if sess.Tool != "claude-code" {
		t.Errorf("expected tool 'claude-code', got %q", sess.Tool)
	}
	if sess.StartedAt == "" {
		t.Error("expected non-empty StartedAt")
	}
	if sess.EndedAt != "" {
		t.Errorf("expected empty EndedAt, got %q", sess.EndedAt)
	}
}

func TestEndSession(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("feat-a", "A")
	sess, _ := store.CreateSession(f.ID, "claude-code")

	if err := store.EndSession(sess.ID); err != nil {
		t.Fatalf("EndSession: %v", err)
	}

	// Verify it's ended by listing sessions
	sessions, err := store.ListSessions(f.ID, 10)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].EndedAt == "" {
		t.Error("expected non-empty EndedAt after ending session")
	}
}

func TestEndSession_SetsEndedAtCorrectly(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("feat-end", "End session test")
	sess, _ := store.CreateSession(f.ID, "test-tool")

	// Verify EndedAt is empty before ending
	if sess.EndedAt != "" {
		t.Fatalf("expected empty EndedAt before ending, got %q", sess.EndedAt)
	}

	// End the session
	if err := store.EndSession(sess.ID); err != nil {
		t.Fatalf("EndSession: %v", err)
	}

	// List sessions and verify ended_at is set and non-empty
	sessions, err := store.ListSessions(f.ID, 10)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	ended := sessions[0]
	if ended.EndedAt == "" {
		t.Error("expected non-empty EndedAt after EndSession")
	}

	// The session should no longer appear as current
	_, err = store.GetCurrentSession()
	if err == nil {
		t.Error("expected error from GetCurrentSession after ending the only session")
	}

	// Verify ended_at is a valid datetime (should be parseable)
	if ended.EndedAt != "" {
		// Just check it looks like a date (not empty, contains digits)
		if len(ended.EndedAt) < 10 {
			t.Errorf("ended_at looks invalid: %q", ended.EndedAt)
		}
	}
}

func TestEndSession_NotFound(t *testing.T) {
	store := newTestStore(t)

	err := store.EndSession("nonexistent-id")
	if err == nil {
		t.Fatal("expected error for nonexistent session")
	}
}

func TestGetCurrentSession(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("feat-a", "A")
	sess, _ := store.CreateSession(f.ID, "claude-code")

	current, err := store.GetCurrentSession()
	if err != nil {
		t.Fatalf("GetCurrentSession: %v", err)
	}
	if current.ID != sess.ID {
		t.Errorf("expected session ID %q, got %q", sess.ID, current.ID)
	}
}

func TestGetCurrentSession_NoActive(t *testing.T) {
	store := newTestStore(t)

	_, err := store.GetCurrentSession()
	if err == nil {
		t.Fatal("expected error when no active session")
	}
}

func TestGetCurrentSession_AfterEnd(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("feat-a", "A")
	sess, _ := store.CreateSession(f.ID, "claude-code")
	store.EndSession(sess.ID)

	_, err := store.GetCurrentSession()
	if err == nil {
		t.Fatal("expected error when all sessions ended")
	}
}

func TestListSessions(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("feat-a", "A")
	store.CreateSession(f.ID, "tool-1")
	store.CreateSession(f.ID, "tool-2")
	store.CreateSession(f.ID, "tool-3")

	sessions, err := store.ListSessions(f.ID, 10)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 3 {
		t.Fatalf("expected 3 sessions, got %d", len(sessions))
	}
}

func TestListSessions_Limit(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("feat-a", "A")
	store.CreateSession(f.ID, "tool-1")
	store.CreateSession(f.ID, "tool-2")
	store.CreateSession(f.ID, "tool-3")

	sessions, err := store.ListSessions(f.ID, 2)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions (limited), got %d", len(sessions))
	}
}

func TestListSessions_FilterByFeature(t *testing.T) {
	store := newTestStore(t)

	fa, _ := store.CreateFeature("feat-a", "A")
	fb, _ := store.CreateFeature("feat-b", "B")
	store.CreateSession(fa.ID, "tool-1")
	store.CreateSession(fb.ID, "tool-2")

	sessions, err := store.ListSessions(fa.ID, 10)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session for feat-a, got %d", len(sessions))
	}
	if sessions[0].FeatureID != fa.ID {
		t.Errorf("expected feature ID %q, got %q", fa.ID, sessions[0].FeatureID)
	}
}
