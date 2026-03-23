package memory_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/arbaz/devmem/internal/memory"
	"github.com/arbaz/devmem/internal/plans"
	"github.com/arbaz/devmem/internal/storage"
)

func TestGetContext_Compact(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("feat-a", "Feature A")

	ctx, err := store.GetContext(f.ID, "compact", nil)
	if err != nil {
		t.Fatalf("GetContext compact: %v", err)
	}

	if ctx.Feature == nil {
		t.Fatal("expected non-nil feature")
	}
	if ctx.Feature.Name != "feat-a" {
		t.Errorf("expected feature name 'feat-a', got %q", ctx.Feature.Name)
	}
}

func TestGetContext_Standard(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("feat-a", "Feature A")
	store.CreateNote(f.ID, "", "Note 1", "note")
	store.CreateNote(f.ID, "", "Note 2", "decision")
	store.CreateFact(f.ID, "", "db", "uses", "sqlite")

	ctx, err := store.GetContext(f.ID, "standard", nil)
	if err != nil {
		t.Fatalf("GetContext standard: %v", err)
	}

	if ctx.Feature == nil {
		t.Fatal("expected non-nil feature")
	}
	if len(ctx.RecentNotes) != 2 {
		t.Errorf("expected 2 recent notes, got %d", len(ctx.RecentNotes))
	}
	if len(ctx.ActiveFacts) != 1 {
		t.Errorf("expected 1 active fact, got %d", len(ctx.ActiveFacts))
	}
}

func TestGetContext_StandardWithAsOf(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("feat-a", "Feature A")
	store.CreateFact(f.ID, "", "db", "uses", "sqlite")

	// Query with a future time
	future := time.Now().Add(time.Hour)
	ctx, err := store.GetContext(f.ID, "standard", &future)
	if err != nil {
		t.Fatalf("GetContext standard with asOf: %v", err)
	}
	if len(ctx.ActiveFacts) != 1 {
		t.Errorf("expected 1 active fact with future asOf, got %d", len(ctx.ActiveFacts))
	}

	// Query with a past time
	past := time.Now().Add(-time.Hour)
	ctx, err = store.GetContext(f.ID, "standard", &past)
	if err != nil {
		t.Fatalf("GetContext standard with past asOf: %v", err)
	}
	if len(ctx.ActiveFacts) != 0 {
		t.Errorf("expected 0 active facts with past asOf, got %d", len(ctx.ActiveFacts))
	}
}

func TestGetContext_StandardAsOfHistorical(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("feat-temporal", "Temporal test")

	// Create first fact
	store.CreateFact(f.ID, "", "db", "uses", "PostgreSQL")

	// Sleep so second fact has a different valid_at
	time.Sleep(1100 * time.Millisecond)
	beforeChange := time.Now()
	time.Sleep(1100 * time.Millisecond)

	// Contradict the fact
	store.CreateFact(f.ID, "", "db", "uses", "SQLite")

	// Query with as_of before the change — should see PostgreSQL
	ctx, err := store.GetContext(f.ID, "standard", &beforeChange)
	if err != nil {
		t.Fatalf("GetContext with historical asOf: %v", err)
	}
	if len(ctx.ActiveFacts) != 1 {
		t.Fatalf("expected 1 fact at historical time, got %d", len(ctx.ActiveFacts))
	}
	if ctx.ActiveFacts[0].Object != "PostgreSQL" {
		t.Errorf("expected 'PostgreSQL' at historical time, got %q", ctx.ActiveFacts[0].Object)
	}

	// Query with as_of after the change — should see SQLite
	afterChange := time.Now().Add(time.Second)
	ctx, err = store.GetContext(f.ID, "standard", &afterChange)
	if err != nil {
		t.Fatalf("GetContext with current asOf: %v", err)
	}
	if len(ctx.ActiveFacts) != 1 {
		t.Fatalf("expected 1 fact at current time, got %d", len(ctx.ActiveFacts))
	}
	if ctx.ActiveFacts[0].Object != "SQLite" {
		t.Errorf("expected 'SQLite' at current time, got %q", ctx.ActiveFacts[0].Object)
	}

	// Query without as_of — should use current active facts (SQLite)
	ctx, err = store.GetContext(f.ID, "standard", nil)
	if err != nil {
		t.Fatalf("GetContext without asOf: %v", err)
	}
	if len(ctx.ActiveFacts) != 1 {
		t.Fatalf("expected 1 active fact, got %d", len(ctx.ActiveFacts))
	}
	if ctx.ActiveFacts[0].Object != "SQLite" {
		t.Errorf("expected 'SQLite' without asOf, got %q", ctx.ActiveFacts[0].Object)
	}
}

func TestGetContext_Detailed(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("feat-a", "Feature A")
	sess, _ := store.CreateSession(f.ID, "claude-code")
	store.CreateNote(f.ID, sess.ID, "Note 1", "note")
	store.CreateFact(f.ID, sess.ID, "db", "uses", "sqlite")

	ctx, err := store.GetContext(f.ID, "detailed", nil)
	if err != nil {
		t.Fatalf("GetContext detailed: %v", err)
	}

	if ctx.Feature == nil {
		t.Fatal("expected non-nil feature")
	}
	if len(ctx.RecentNotes) != 1 {
		t.Errorf("expected 1 note, got %d", len(ctx.RecentNotes))
	}
	if len(ctx.ActiveFacts) != 1 {
		t.Errorf("expected 1 active fact, got %d", len(ctx.ActiveFacts))
	}
	if len(ctx.SessionHistory) != 1 {
		t.Errorf("expected 1 session, got %d", len(ctx.SessionHistory))
	}
}

func TestGetContext_InvalidTier(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("feat-a", "Feature A")

	_, err := store.GetContext(f.ID, "invalid", nil)
	if err == nil {
		t.Fatal("expected error for invalid tier")
	}
}

func TestGetContext_FeatureNotFound(t *testing.T) {
	store := newTestStore(t)

	_, err := store.GetContext("nonexistent-id", "compact", nil)
	if err == nil {
		t.Fatal("expected error for nonexistent feature")
	}
}

func TestGetContext_CompactReturnsMinimalData(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("feat-compact", "Compact Feature")
	// Create multiple notes, facts, and sessions — compact should NOT return them all.
	store.CreateNote(f.ID, "", "Note 1", "note")
	store.CreateNote(f.ID, "", "Note 2", "decision")
	store.CreateNote(f.ID, "", "Note 3", "blocker")
	store.CreateFact(f.ID, "", "db", "uses", "sqlite")
	store.CreateSession(f.ID, "claude-code")
	store.CreateSession(f.ID, "cursor")

	ctx, err := store.GetContext(f.ID, "compact", nil)
	if err != nil {
		t.Fatalf("GetContext compact: %v", err)
	}

	if ctx.Feature == nil {
		t.Fatal("expected non-nil feature in compact tier")
	}
	// Compact tier should have at most 1 commit (loadRecentCommits with limit=1)
	if len(ctx.RecentCommits) > 1 {
		t.Errorf("compact tier should have at most 1 commit, got %d", len(ctx.RecentCommits))
	}
	// Compact tier should NOT load notes, facts, sessions, or links
	if len(ctx.RecentNotes) != 0 {
		t.Errorf("compact tier should have 0 recent notes, got %d", len(ctx.RecentNotes))
	}
	if len(ctx.ActiveFacts) != 0 {
		t.Errorf("compact tier should have 0 active facts, got %d", len(ctx.ActiveFacts))
	}
	if len(ctx.SessionHistory) != 0 {
		t.Errorf("compact tier should have 0 sessions, got %d", len(ctx.SessionHistory))
	}
	if len(ctx.Links) != 0 {
		t.Errorf("compact tier should have 0 links, got %d", len(ctx.Links))
	}
}

func TestGetContext_DetailedReturnsSessionHistoryAndLinks(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("feat-detail", "Detailed Feature")
	s1, _ := store.CreateSession(f.ID, "claude-code")
	s2, _ := store.CreateSession(f.ID, "cursor")
	store.CreateNote(f.ID, s1.ID, "Note in session 1", "note")
	store.CreateNote(f.ID, s2.ID, "Note in session 2", "decision")
	store.CreateFact(f.ID, s1.ID, "api", "uses", "REST")

	ctx, err := store.GetContext(f.ID, "detailed", nil)
	if err != nil {
		t.Fatalf("GetContext detailed: %v", err)
	}

	if ctx.Feature == nil {
		t.Fatal("expected non-nil feature")
	}
	if len(ctx.SessionHistory) < 2 {
		t.Errorf("expected at least 2 sessions in detailed tier, got %d", len(ctx.SessionHistory))
	}
	if len(ctx.RecentNotes) < 2 {
		t.Errorf("expected at least 2 notes in detailed tier, got %d", len(ctx.RecentNotes))
	}
	if len(ctx.ActiveFacts) < 1 {
		t.Errorf("expected at least 1 active fact in detailed tier, got %d", len(ctx.ActiveFacts))
	}
}

func TestGetContext_NoCommitsReturnsEmptyCommitsList(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("feat-nocommits", "No Commits Feature")
	store.CreateNote(f.ID, "", "A note without commits", "note")

	// Test all three tiers — commits list should be empty (nil) in each
	for _, tier := range []string{"compact", "standard", "detailed"} {
		ctx, err := store.GetContext(f.ID, tier, nil)
		if err != nil {
			t.Fatalf("GetContext %s: %v", tier, err)
		}
		if len(ctx.RecentCommits) != 0 {
			t.Errorf("tier %s: expected 0 commits, got %d", tier, len(ctx.RecentCommits))
		}
	}
}

func TestGetContext_WithPlanReturnsPlanInfo(t *testing.T) {
	// We need both the Store and the raw DB to create a plan via the plans.Manager.
	dir := t.TempDir()
	db, err := storage.NewDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := storage.Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	store := memory.NewStore(db)
	mgr := plans.NewManager(db)

	f, _ := store.CreateFeature("feat-plan", "Feature With Plan")
	sess, _ := store.CreateSession(f.ID, "test-tool")

	// Create a plan with 3 steps via the plans Manager (session_id required by FK)
	stepInputs := []plans.StepInput{
		{Title: "Step 1", Description: "First step"},
		{Title: "Step 2", Description: "Second step"},
		{Title: "Step 3", Description: "Third step"},
	}
	plan, err := mgr.CreatePlan(f.ID, sess.ID, "Implement Auth", "Auth implementation plan", "test", stepInputs)
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}

	// Complete the first step
	planSteps, err := mgr.GetPlanSteps(plan.ID)
	if err != nil {
		t.Fatalf("GetPlanSteps: %v", err)
	}
	if len(planSteps) != 3 {
		t.Fatalf("expected 3 plan steps, got %d", len(planSteps))
	}
	if err := mgr.UpdateStepStatus(planSteps[0].ID, "completed"); err != nil {
		t.Fatalf("UpdateStepStatus: %v", err)
	}

	ctx, err := store.GetContext(f.ID, "compact", nil)
	if err != nil {
		t.Fatalf("GetContext: %v", err)
	}

	if ctx.Plan == nil {
		t.Fatal("expected non-nil Plan in context")
	}
	if ctx.Plan.Title != "Implement Auth" {
		t.Errorf("expected plan title 'Implement Auth', got %q", ctx.Plan.Title)
	}
	if ctx.Plan.Status != "active" {
		t.Errorf("expected plan status 'active', got %q", ctx.Plan.Status)
	}
	if ctx.Plan.TotalSteps != 3 {
		t.Errorf("expected 3 total steps, got %d", ctx.Plan.TotalSteps)
	}
	if ctx.Plan.CompletedStep != 1 {
		t.Errorf("expected 1 completed step, got %d", ctx.Plan.CompletedStep)
	}
}

func TestGetContext_IncludesLastSessionSummary(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("feat-sess-summary", "Session summary test")

	// Create a session, end it with a summary.
	sess1, _ := store.CreateSession(f.ID, "claude-code")
	summary := "Added user authentication with JWT tokens and wrote unit tests"
	if err := store.EndSessionWithSummary(sess1.ID, summary); err != nil {
		t.Fatalf("EndSessionWithSummary: %v", err)
	}

	// Create a new (current) session.
	store.CreateSession(f.ID, "claude-code")

	// GetContext should include the previous session's summary.
	ctx, err := store.GetContext(f.ID, "compact", nil)
	if err != nil {
		t.Fatalf("GetContext: %v", err)
	}

	if ctx.LastSessionSummary != summary {
		t.Errorf("expected LastSessionSummary %q, got %q", summary, ctx.LastSessionSummary)
	}
}

func TestGetContext_NoLastSessionSummaryWhenNoPrevious(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("feat-no-summary", "No previous session")
	store.CreateSession(f.ID, "claude-code")

	ctx, err := store.GetContext(f.ID, "compact", nil)
	if err != nil {
		t.Fatalf("GetContext: %v", err)
	}

	if ctx.LastSessionSummary != "" {
		t.Errorf("expected empty LastSessionSummary, got %q", ctx.LastSessionSummary)
	}
}

func TestContextTier_CompactMinimal(t *testing.T) {
	store := newTestStore(t)
	f, _ := store.CreateFeature("ct-compact", "test")
	store.CreateNote(f.ID, "", "A note", "note")
	store.CreateFact(f.ID, "", "lang", "is", "Go")
	store.CreateSession(f.ID, "tool")
	ctx, err := store.GetContext(f.ID, "compact", nil)
	if err != nil { t.Fatalf("GetContext: %v", err) }
	if len(ctx.RecentNotes) != 0 { t.Error("compact should have 0 notes") }
	if len(ctx.ActiveFacts) != 0 { t.Error("compact should have 0 facts") }
	if len(ctx.SessionHistory) != 0 { t.Error("compact should have 0 sessions") }
}

func TestContextTier_StandardNotesAndFacts(t *testing.T) {
	store := newTestStore(t)
	f, _ := store.CreateFeature("ct-standard", "test")
	store.CreateNote(f.ID, "", "A note", "note")
	store.CreateFact(f.ID, "", "lang", "is", "Go")
	store.CreateSession(f.ID, "tool")
	ctx, err := store.GetContext(f.ID, "standard", nil)
	if err != nil { t.Fatalf("GetContext: %v", err) }
	if len(ctx.RecentNotes) == 0 { t.Error("standard should have notes") }
	if len(ctx.ActiveFacts) == 0 { t.Error("standard should have facts") }
}

func TestContextTier_DetailedEverything(t *testing.T) {
	store := newTestStore(t)
	f, _ := store.CreateFeature("ct-detail", "test")
	store.CreateNote(f.ID, "", "A note", "note")
	store.CreateFact(f.ID, "", "lang", "is", "Go")
	store.CreateSession(f.ID, "tool")
	ctx, err := store.GetContext(f.ID, "detailed", nil)
	if err != nil { t.Fatalf("GetContext: %v", err) }
	if len(ctx.RecentNotes) == 0 { t.Error("detailed should have notes") }
	if len(ctx.ActiveFacts) == 0 { t.Error("detailed should have facts") }
	if len(ctx.SessionHistory) == 0 { t.Error("detailed should have sessions") }
}

func TestContextTiers(t *testing.T) {
	store := newTestStore(t)
	f, _ := store.CreateFeature("tier-test", "Tier test feature")
	store.CreateNote(f.ID, "", "A note", "note")
	store.CreateFact(f.ID, "", "lang", "is", "Go")
	store.CreateSession(f.ID, "tool")
	for _, tc := range []struct {
		name         string
		tier         string
		wantNotes    bool
		wantFacts    bool
		wantSessions bool
	}{
		{"compact_minimal", "compact", false, false, false},
		{"standard_notes_and_facts", "standard", true, true, false},
		{"detailed_everything", "detailed", true, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, err := store.GetContext(f.ID, tc.tier, nil)
			if err != nil { t.Fatalf("GetContext(%s): %v", tc.tier, err) }
			if (len(ctx.RecentNotes) > 0) != tc.wantNotes { t.Errorf("notes mismatch") }
			if (len(ctx.ActiveFacts) > 0) != tc.wantFacts { t.Errorf("facts mismatch") }
			if (len(ctx.SessionHistory) > 0) != tc.wantSessions { t.Errorf("sessions mismatch") }
		})
	}
}

func TestGetContext_TiersDifferInData(t *testing.T) {
	store := newTestStore(t)
	f, _ := store.CreateFeature("tier-diff", "Tier comparison")
	store.CreateNote(f.ID, "", "A note", "note")
	store.CreateFact(f.ID, "", "lang", "is", "Go")
	store.CreateSession(f.ID, "test")

	compact, _ := store.GetContext(f.ID, "compact", nil)
	standard, _ := store.GetContext(f.ID, "standard", nil)
	detailed, _ := store.GetContext(f.ID, "detailed", nil)

	if len(compact.RecentNotes) != 0 {
		t.Error("compact should have 0 notes")
	}
	if len(standard.RecentNotes) == 0 {
		t.Error("standard should have notes")
	}
	if len(detailed.SessionHistory) == 0 {
		t.Error("detailed should have sessions")
	}
	if len(compact.ActiveFacts) != 0 {
		t.Error("compact should have 0 facts")
	}
	if len(standard.ActiveFacts) == 0 {
		t.Error("standard should have facts")
	}
}
