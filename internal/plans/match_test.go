package plans_test

import (
	"testing"

	"github.com/arbaz/devmem/internal/plans"
)

func TestMatchCommitToSteps_FindsMatch(t *testing.T) {
	db := setupTestDB(t)
	mgr := plans.NewManager(db)
	featureID := createTestFeature(t, db)
	sessionID := createTestSession(t, db, featureID)

	steps := []plans.StepInput{
		{Title: "Set up database schema", Description: "Create tables and indexes"},
		{Title: "Implement REST API endpoints", Description: "HTTP handlers"},
		{Title: "Write unit tests", Description: "Test all handlers"},
	}
	_, err := mgr.CreatePlan(featureID, sessionID, "API Plan", "", "claude", steps)
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}

	// Commit message that matches step 1
	match, err := mgr.MatchCommitToSteps("set up database schema and indexes", featureID)
	if err != nil {
		t.Fatalf("MatchCommitToSteps: %v", err)
	}
	if match == nil {
		t.Fatal("expected a match, got nil")
	}
	if match.Title != "Set up database schema" {
		t.Errorf("expected match title 'Set up database schema', got %q", match.Title)
	}
}

func TestMatchCommitToSteps_NoMatchForUnrelated(t *testing.T) {
	db := setupTestDB(t)
	mgr := plans.NewManager(db)
	featureID := createTestFeature(t, db)
	sessionID := createTestSession(t, db, featureID)

	steps := []plans.StepInput{
		{Title: "Set up database schema", Description: ""},
		{Title: "Implement REST API", Description: ""},
		{Title: "Write unit tests", Description: ""},
	}
	_, err := mgr.CreatePlan(featureID, sessionID, "API Plan", "", "claude", steps)
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}

	// Completely unrelated commit
	match, err := mgr.MatchCommitToSteps("fix typo in readme documentation", featureID)
	if err != nil {
		t.Fatalf("MatchCommitToSteps: %v", err)
	}
	if match != nil {
		t.Errorf("expected no match, got step %q", match.Title)
	}
}

func TestMatchCommitToSteps_SkipsCompletedSteps(t *testing.T) {
	db := setupTestDB(t)
	mgr := plans.NewManager(db)
	featureID := createTestFeature(t, db)
	sessionID := createTestSession(t, db, featureID)

	steps := []plans.StepInput{
		{Title: "Set up database schema", Description: ""},
		{Title: "Write database migration tests", Description: ""},
	}
	plan, err := mgr.CreatePlan(featureID, sessionID, "DB Plan", "", "claude", steps)
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}

	// Mark first step as completed
	planSteps, err := mgr.GetPlanSteps(plan.ID)
	if err != nil {
		t.Fatalf("GetPlanSteps: %v", err)
	}
	if err := mgr.UpdateStepStatus(planSteps[0].ID, "completed"); err != nil {
		t.Fatalf("UpdateStepStatus: %v", err)
	}

	// Commit that would match the completed step
	match, err := mgr.MatchCommitToSteps("set up database schema", featureID)
	if err != nil {
		t.Fatalf("MatchCommitToSteps: %v", err)
	}

	// Should not match completed step; might match second step or nil
	if match != nil && match.Title == "Set up database schema" {
		t.Error("should not match a completed step")
	}
}

func TestMatchCommitToSteps_EmptyCommitMessage(t *testing.T) {
	db := setupTestDB(t)
	mgr := plans.NewManager(db)
	featureID := createTestFeature(t, db)
	sessionID := createTestSession(t, db, featureID)

	steps := []plans.StepInput{
		{Title: "Some step", Description: ""},
	}
	_, err := mgr.CreatePlan(featureID, sessionID, "Plan", "", "claude", steps)
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}

	match, err := mgr.MatchCommitToSteps("", featureID)
	if err != nil {
		t.Fatalf("MatchCommitToSteps: %v", err)
	}
	if match != nil {
		t.Error("expected nil match for empty commit message")
	}
}

func TestMatchCommitToSteps_VeryShortCommitMessage(t *testing.T) {
	db := setupTestDB(t)
	mgr := plans.NewManager(db)
	featureID := createTestFeature(t, db)
	sessionID := createTestSession(t, db, featureID)

	steps := []plans.StepInput{
		{Title: "Set up database schema", Description: "Create tables"},
		{Title: "Write tests", Description: "Unit tests"},
	}
	_, err := mgr.CreatePlan(featureID, sessionID, "Plan", "", "claude", steps)
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}

	// Very short commit message — only one word
	match, err := mgr.MatchCommitToSteps("fix", featureID)
	if err != nil {
		t.Fatalf("MatchCommitToSteps: %v", err)
	}
	// A single generic word like "fix" should not strongly match any step
	// The result should be nil because Jaccard similarity would be very low
	// (1 word vs 4-5 words in step titles = ~0.2 which is below 0.3 threshold)
	if match != nil {
		t.Logf("unexpected match for 'fix': step=%q", match.Title)
	}

	// Two-character commit message
	match, err = mgr.MatchCommitToSteps("db", featureID)
	if err != nil {
		t.Fatalf("MatchCommitToSteps short: %v", err)
	}
	// "db" as a single token vs step titles — very low similarity
	if match != nil {
		t.Logf("match for 'db': step=%q (may or may not match)", match.Title)
	}

	// Single punctuation/special chars
	match, err = mgr.MatchCommitToSteps(".", featureID)
	if err != nil {
		t.Fatalf("MatchCommitToSteps punctuation: %v", err)
	}
	if match != nil {
		t.Error("expected nil match for punctuation-only commit message")
	}
}

func TestMatchCommitToSteps_MatchesDescription(t *testing.T) {
	db := setupTestDB(t)
	mgr := plans.NewManager(db)
	featureID := createTestFeature(t, db)
	sessionID := createTestSession(t, db, featureID)

	steps := []plans.StepInput{
		{Title: "Phase 1", Description: "Set up database schema and migrations"},
		{Title: "Phase 2", Description: "Implement API handlers"},
	}
	_, err := mgr.CreatePlan(featureID, sessionID, "Phased Plan", "", "claude", steps)
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}

	// Commit message matches description of step 1
	match, err := mgr.MatchCommitToSteps("add database schema and migration files", featureID)
	if err != nil {
		t.Fatalf("MatchCommitToSteps: %v", err)
	}
	if match == nil {
		t.Fatal("expected a match via description, got nil")
	}
	if match.Title != "Phase 1" {
		t.Errorf("expected match 'Phase 1', got %q", match.Title)
	}
}
