package memory_test

import (
	"testing"
)

func TestSaveSnapshot_StoresContent(t *testing.T) {
	store := newTestStore(t)
	f, err := store.CreateFeature("snap-feat", "Snapshot feature")
	if err != nil {
		t.Fatalf("CreateFeature: %v", err)
	}

	err = store.SaveSnapshot(f.ID, "", "Working on auth flow. OAuth2 tokens expire after 1 hour. Refresh tokens stored in httpOnly cookies.", "pre_compaction")
	if err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	snapshots, err := store.ListSnapshots(f.ID, 10)
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(snapshots) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snapshots))
	}
	if snapshots[0].Content != "Working on auth flow. OAuth2 tokens expire after 1 hour. Refresh tokens stored in httpOnly cookies." {
		t.Errorf("unexpected content: %q", snapshots[0].Content)
	}
	if snapshots[0].SnapshotType != "pre_compaction" {
		t.Errorf("expected type 'pre_compaction', got %q", snapshots[0].SnapshotType)
	}
	if snapshots[0].FeatureID != f.ID {
		t.Errorf("expected feature ID %q, got %q", f.ID, snapshots[0].FeatureID)
	}
}

func TestRecoverContext_FindsMatchingSnapshots(t *testing.T) {
	store := newTestStore(t)
	f, err := store.CreateFeature("recover-feat", "Recovery feature")
	if err != nil {
		t.Fatalf("CreateFeature: %v", err)
	}

	// Save several snapshots with different content
	err = store.SaveSnapshot(f.ID, "", "The database schema uses PostgreSQL with JSONB columns for metadata. Connection pool is set to 25.", "pre_compaction")
	if err != nil {
		t.Fatalf("SaveSnapshot 1: %v", err)
	}
	err = store.SaveSnapshot(f.ID, "", "Authentication uses JWT tokens with RS256 signing. Token expiry is 15 minutes.", "checkpoint")
	if err != nil {
		t.Fatalf("SaveSnapshot 2: %v", err)
	}
	err = store.SaveSnapshot(f.ID, "", "Rate limiting configured at 100 requests per minute per user. Using sliding window algorithm.", "milestone")
	if err != nil {
		t.Fatalf("SaveSnapshot 3: %v", err)
	}

	// Search for database-related context
	matches, err := store.RecoverContext(f.ID, "database schema PostgreSQL", 3)
	if err != nil {
		t.Fatalf("RecoverContext: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("expected at least 1 match for 'database schema PostgreSQL'")
	}
	found := false
	for _, m := range matches {
		if m.SnapshotType == "pre_compaction" {
			found = true
		}
	}
	if !found {
		t.Error("expected to find the pre_compaction snapshot about PostgreSQL")
	}

	// Search for authentication-related context
	matches, err = store.RecoverContext(f.ID, "JWT token authentication", 3)
	if err != nil {
		t.Fatalf("RecoverContext: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("expected at least 1 match for 'JWT token authentication'")
	}
}

func TestRecoverContext_ReturnsEmptyForNoMatches(t *testing.T) {
	store := newTestStore(t)
	f, err := store.CreateFeature("no-match-feat", "No match feature")
	if err != nil {
		t.Fatalf("CreateFeature: %v", err)
	}

	err = store.SaveSnapshot(f.ID, "", "Working on the billing integration with Stripe API", "pre_compaction")
	if err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	matches, err := store.RecoverContext(f.ID, "kubernetes deployment", 3)
	if err != nil {
		t.Fatalf("RecoverContext: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("expected 0 matches for unrelated query, got %d", len(matches))
	}
}

func TestMultipleSnapshots_Searchable(t *testing.T) {
	store := newTestStore(t)
	f, err := store.CreateFeature("multi-snap", "Multiple snapshots")
	if err != nil {
		t.Fatalf("CreateFeature: %v", err)
	}

	snapshots := []struct {
		content      string
		snapshotType string
	}{
		{"Implemented user registration with email verification. Using SendGrid for email delivery.", "pre_compaction"},
		{"Added password reset flow. Reset tokens expire after 30 minutes. Rate limited to 3 attempts per hour.", "checkpoint"},
		{"Completed OAuth2 integration with Google and GitHub providers. Storing provider tokens encrypted.", "milestone"},
		{"Fixed bug where email verification link expired prematurely. Root cause was timezone mismatch.", "pre_compaction"},
		{"Session management refactored to use Redis. Session TTL is 24 hours with sliding expiration.", "checkpoint"},
	}

	for i, s := range snapshots {
		if err := store.SaveSnapshot(f.ID, "", s.content, s.snapshotType); err != nil {
			t.Fatalf("SaveSnapshot %d: %v", i, err)
		}
	}

	// Verify all snapshots are stored
	listed, err := store.ListSnapshots(f.ID, 10)
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(listed) != 5 {
		t.Fatalf("expected 5 snapshots, got %d", len(listed))
	}

	// Search for email-related content — should find registration and verification bug
	matches, err := store.RecoverContext(f.ID, "email verification", 5)
	if err != nil {
		t.Fatalf("RecoverContext: %v", err)
	}
	if len(matches) < 2 {
		t.Errorf("expected at least 2 matches for 'email verification', got %d", len(matches))
	}

	// Search for OAuth-related content
	matches, err = store.RecoverContext(f.ID, "OAuth2 provider", 5)
	if err != nil {
		t.Fatalf("RecoverContext: %v", err)
	}
	if len(matches) == 0 {
		t.Error("expected at least 1 match for 'OAuth2 provider'")
	}

	// Test limit is respected
	matches, err = store.RecoverContext(f.ID, "email OR session OR OAuth2", 2)
	if err != nil {
		t.Fatalf("RecoverContext with limit: %v", err)
	}
	if len(matches) > 2 {
		t.Errorf("expected at most 2 matches with limit=2, got %d", len(matches))
	}
}

func TestSaveSnapshot_DefaultType(t *testing.T) {
	store := newTestStore(t)
	f, err := store.CreateFeature("default-type", "Default type test")
	if err != nil {
		t.Fatalf("CreateFeature: %v", err)
	}

	err = store.SaveSnapshot(f.ID, "", "Some context to save", "")
	if err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	snapshots, err := store.ListSnapshots(f.ID, 10)
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(snapshots) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snapshots))
	}
	if snapshots[0].SnapshotType != "pre_compaction" {
		t.Errorf("expected default type 'pre_compaction', got %q", snapshots[0].SnapshotType)
	}
}

func TestRecoverContext_ScopedToFeature(t *testing.T) {
	store := newTestStore(t)
	fa, err := store.CreateFeature("feat-alpha", "Feature Alpha")
	if err != nil {
		t.Fatalf("CreateFeature alpha: %v", err)
	}
	fb, err := store.CreateFeature("feat-beta", "Feature Beta")
	if err != nil {
		t.Fatalf("CreateFeature beta: %v", err)
	}

	err = store.SaveSnapshot(fa.ID, "", "Alpha uses GraphQL API with Apollo Server", "pre_compaction")
	if err != nil {
		t.Fatalf("SaveSnapshot alpha: %v", err)
	}
	err = store.SaveSnapshot(fb.ID, "", "Beta uses REST API with Express", "pre_compaction")
	if err != nil {
		t.Fatalf("SaveSnapshot beta: %v", err)
	}

	// Search within feature alpha should not return beta's snapshots
	matches, err := store.RecoverContext(fa.ID, "API", 5)
	if err != nil {
		t.Fatalf("RecoverContext alpha: %v", err)
	}
	for _, m := range matches {
		if m.FeatureID != fa.ID {
			t.Errorf("expected all matches to be from feature alpha, got feature ID %q", m.FeatureID)
		}
	}
}
