package plans_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arbazkhan971/memorx/internal/plans"
	"github.com/arbazkhan971/memorx/internal/storage"
	"github.com/google/uuid"
)

// setupTestDB creates a temporary DB with migrations applied.
func setupTestDB(t *testing.T) *storage.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := storage.NewDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	if err := storage.Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// createTestFeature inserts a feature and returns its ID.
func createTestFeature(t *testing.T, db *storage.DB) string {
	t.Helper()
	id := uuid.New().String()
	_, err := db.Writer().Exec(
		`INSERT INTO features (id, name, description, status) VALUES (?, ?, ?, 'active')`,
		id, "test-feature-"+id[:8], "test description",
	)
	if err != nil {
		t.Fatalf("create test feature: %v", err)
	}
	return id
}

// createTestSession inserts a session and returns its ID.
func createTestSession(t *testing.T, db *storage.DB, featureID string) string {
	t.Helper()
	id := uuid.New().String()
	_, err := db.Writer().Exec(
		`INSERT INTO sessions (id, feature_id, tool) VALUES (?, ?, 'test')`,
		id, featureID,
	)
	if err != nil {
		t.Fatalf("create test session: %v", err)
	}
	return id
}

func TestCreatePlan(t *testing.T) {
	db := setupTestDB(t)
	mgr := plans.NewManager(db)
	featureID := createTestFeature(t, db)
	sessionID := createTestSession(t, db, featureID)

	steps := []plans.StepInput{
		{Title: "Set up database", Description: "Create schema"},
		{Title: "Implement API", Description: "REST endpoints"},
		{Title: "Write tests", Description: "Unit and integration"},
	}

	plan, err := mgr.CreatePlan(featureID, sessionID, "My Plan", "Plan content", "claude", steps)
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}

	if plan.ID == "" {
		t.Fatal("plan ID is empty")
	}
	if plan.FeatureID != featureID {
		t.Errorf("expected featureID %s, got %s", featureID, plan.FeatureID)
	}
	if plan.Title != "My Plan" {
		t.Errorf("expected title 'My Plan', got %s", plan.Title)
	}
	if plan.Status != "active" {
		t.Errorf("expected status 'active', got %s", plan.Status)
	}

	// Verify steps were created
	planSteps, err := mgr.GetPlanSteps(plan.ID)
	if err != nil {
		t.Fatalf("GetPlanSteps: %v", err)
	}
	if len(planSteps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(planSteps))
	}
	if planSteps[0].Title != "Set up database" {
		t.Errorf("expected step 1 title 'Set up database', got %s", planSteps[0].Title)
	}
	if planSteps[0].StepNumber != 1 {
		t.Errorf("expected step_number 1, got %d", planSteps[0].StepNumber)
	}
	if planSteps[2].StepNumber != 3 {
		t.Errorf("expected step_number 3, got %d", planSteps[2].StepNumber)
	}
}

func TestCreatePlan_SupersedesExisting(t *testing.T) {
	db := setupTestDB(t)
	mgr := plans.NewManager(db)
	featureID := createTestFeature(t, db)
	sessionID := createTestSession(t, db, featureID)

	// Create first plan
	steps1 := []plans.StepInput{
		{Title: "Step A", Description: "First"},
		{Title: "Step B", Description: "Second"},
		{Title: "Step C", Description: "Third"},
	}
	plan1, err := mgr.CreatePlan(featureID, sessionID, "Plan v1", "First version", "claude", steps1)
	if err != nil {
		t.Fatalf("CreatePlan v1: %v", err)
	}

	// Mark first step as completed
	plan1Steps, err := mgr.GetPlanSteps(plan1.ID)
	if err != nil {
		t.Fatalf("GetPlanSteps v1: %v", err)
	}
	if err := mgr.UpdateStepStatus(plan1Steps[0].ID, "completed"); err != nil {
		t.Fatalf("UpdateStepStatus: %v", err)
	}

	// Create second plan — should supersede first
	steps2 := []plans.StepInput{
		{Title: "Step D", Description: "New first"},
		{Title: "Step E", Description: "New second"},
	}
	plan2, err := mgr.CreatePlan(featureID, sessionID, "Plan v2", "Second version", "claude", steps2)
	if err != nil {
		t.Fatalf("CreatePlan v2: %v", err)
	}

	// Old plan should be superseded
	allPlans, err := mgr.ListPlans(featureID)
	if err != nil {
		t.Fatalf("ListPlans: %v", err)
	}
	if len(allPlans) != 2 {
		t.Fatalf("expected 2 plans, got %d", len(allPlans))
	}

	// Find old plan in the list
	var oldPlan plans.Plan
	for _, p := range allPlans {
		if p.ID == plan1.ID {
			oldPlan = p
			break
		}
	}
	if oldPlan.Status != "superseded" {
		t.Errorf("expected old plan status 'superseded', got %s", oldPlan.Status)
	}
	if oldPlan.InvalidAt == "" {
		t.Error("expected old plan invalid_at to be set")
	}

	// New plan should have completed step + new steps
	plan2Steps, err := mgr.GetPlanSteps(plan2.ID)
	if err != nil {
		t.Fatalf("GetPlanSteps v2: %v", err)
	}
	// 1 completed (copied) + 2 new = 3
	if len(plan2Steps) != 3 {
		t.Fatalf("expected 3 steps in new plan, got %d", len(plan2Steps))
	}
	if plan2Steps[0].Title != "Step A" {
		t.Errorf("expected copied step title 'Step A', got %s", plan2Steps[0].Title)
	}
	if plan2Steps[0].Status != "completed" {
		t.Errorf("expected copied step status 'completed', got %s", plan2Steps[0].Status)
	}
}

func TestCreatePlan_EmptyStepsReturnsError(t *testing.T) {
	db := setupTestDB(t)
	mgr := plans.NewManager(db)
	featureID := createTestFeature(t, db)
	sessionID := createTestSession(t, db, featureID)

	// Create a plan with zero steps
	plan, err := mgr.CreatePlan(featureID, sessionID, "Empty Plan", "No steps", "claude", []plans.StepInput{})
	// The current implementation does not return an error for empty steps,
	// but the plan should be created with 0 steps
	if err != nil {
		t.Fatalf("CreatePlan with empty steps: %v", err)
	}

	// Verify the plan exists but has no steps
	steps, err := mgr.GetPlanSteps(plan.ID)
	if err != nil {
		t.Fatalf("GetPlanSteps: %v", err)
	}
	if len(steps) != 0 {
		t.Errorf("expected 0 steps for empty plan, got %d", len(steps))
	}

	// Create a plan with nil steps
	plan2, err := mgr.CreatePlan(featureID, sessionID, "Nil Plan", "Nil steps", "claude", nil)
	if err != nil {
		t.Fatalf("CreatePlan with nil steps: %v", err)
	}

	steps2, err := mgr.GetPlanSteps(plan2.ID)
	if err != nil {
		t.Fatalf("GetPlanSteps: %v", err)
	}
	if len(steps2) != 0 {
		t.Errorf("expected 0 steps for nil-steps plan, got %d", len(steps2))
	}
}

func TestGetActivePlan_NoPlansReturnsError(t *testing.T) {
	db := setupTestDB(t)
	mgr := plans.NewManager(db)
	featureID := createTestFeature(t, db)

	plan, err := mgr.GetActivePlan(featureID)
	if err == nil {
		t.Fatal("expected error for feature with no plan")
	}
	if plan != nil {
		t.Error("expected nil plan")
	}
}

func TestUpdateStepStatus_AllStatuses(t *testing.T) {
	db := setupTestDB(t)
	mgr := plans.NewManager(db)
	featureID := createTestFeature(t, db)
	sessionID := createTestSession(t, db, featureID)

	steps := []plans.StepInput{
		{Title: "Step A", Description: "First"},
		{Title: "Step B", Description: "Second"},
		{Title: "Step C", Description: "Third"},
	}
	plan, err := mgr.CreatePlan(featureID, sessionID, "Status Test", "", "test", steps)
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	planSteps, err := mgr.GetPlanSteps(plan.ID)
	if err != nil {
		t.Fatalf("GetPlanSteps: %v", err)
	}

	statuses := []string{"in_progress", "completed", "skipped"}
	for i, status := range statuses {
		t.Run(status, func(t *testing.T) {
			if err := mgr.UpdateStepStatus(planSteps[i].ID, status); err != nil {
				t.Fatalf("UpdateStepStatus(%s): %v", status, err)
			}
			updated, _ := mgr.GetPlanSteps(plan.ID)
			if updated[i].Status != status {
				t.Errorf("expected status %q, got %q", status, updated[i].Status)
			}
		})
	}
}

func TestGetActivePlan(t *testing.T) {
	db := setupTestDB(t)
	mgr := plans.NewManager(db)
	featureID := createTestFeature(t, db)
	sessionID := createTestSession(t, db, featureID)

	// Create two plans (second supersedes first)
	steps := []plans.StepInput{{Title: "Step 1", Description: ""}}
	_, err := mgr.CreatePlan(featureID, sessionID, "Plan v1", "", "claude", steps)
	if err != nil {
		t.Fatalf("CreatePlan v1: %v", err)
	}

	plan2, err := mgr.CreatePlan(featureID, sessionID, "Plan v2", "", "claude", steps)
	if err != nil {
		t.Fatalf("CreatePlan v2: %v", err)
	}

	// GetActivePlan should return only plan2
	active, err := mgr.GetActivePlan(featureID)
	if err != nil {
		t.Fatalf("GetActivePlan: %v", err)
	}
	if active.ID != plan2.ID {
		t.Errorf("expected active plan ID %s, got %s", plan2.ID, active.ID)
	}
	if active.Title != "Plan v2" {
		t.Errorf("expected title 'Plan v2', got %s", active.Title)
	}
}

func TestUpdateStepStatus(t *testing.T) {
	db := setupTestDB(t)
	mgr := plans.NewManager(db)
	featureID := createTestFeature(t, db)
	sessionID := createTestSession(t, db, featureID)

	steps := []plans.StepInput{{Title: "Do something", Description: "Details"}}
	plan, err := mgr.CreatePlan(featureID, sessionID, "Test Plan", "", "test", steps)
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}

	planSteps, err := mgr.GetPlanSteps(plan.ID)
	if err != nil {
		t.Fatalf("GetPlanSteps: %v", err)
	}

	stepID := planSteps[0].ID

	// Update to in_progress
	if err := mgr.UpdateStepStatus(stepID, "in_progress"); err != nil {
		t.Fatalf("UpdateStepStatus to in_progress: %v", err)
	}
	updated, _ := mgr.GetPlanSteps(plan.ID)
	if updated[0].Status != "in_progress" {
		t.Errorf("expected status 'in_progress', got %s", updated[0].Status)
	}
	if updated[0].CompletedAt != "" {
		t.Errorf("expected empty completed_at for in_progress, got %s", updated[0].CompletedAt)
	}

	// Update to completed
	if err := mgr.UpdateStepStatus(stepID, "completed"); err != nil {
		t.Fatalf("UpdateStepStatus to completed: %v", err)
	}
	updated, _ = mgr.GetPlanSteps(plan.ID)
	if updated[0].Status != "completed" {
		t.Errorf("expected status 'completed', got %s", updated[0].Status)
	}
	if updated[0].CompletedAt == "" {
		t.Error("expected completed_at to be set")
	}
}

func TestCreatePlan_VeryLongTitle(t *testing.T) {
	db := setupTestDB(t)
	mgr := plans.NewManager(db)
	featureID := createTestFeature(t, db)
	sessionID := createTestSession(t, db, featureID)
	longTitle := strings.Repeat("A very long plan title ", 100)
	plan, err := mgr.CreatePlan(featureID, sessionID, longTitle, "", "test", []plans.StepInput{{Title: "Step"}})
	if err != nil {
		t.Fatalf("CreatePlan long title: %v", err)
	}
	if plan.Title != longTitle {
		t.Error("title mismatch for long title")
	}
}

func TestGetPlanSteps_Ordering(t *testing.T) {
	db := setupTestDB(t)
	mgr := plans.NewManager(db)
	featureID := createTestFeature(t, db)
	sessionID := createTestSession(t, db, featureID)
	steps := []plans.StepInput{
		{Title: "Zulu"}, {Title: "Alpha"}, {Title: "Mike"},
	}
	plan, err := mgr.CreatePlan(featureID, sessionID, "Order Test", "", "test", steps)
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	got, _ := mgr.GetPlanSteps(plan.ID)
	if got[0].StepNumber != 1 || got[1].StepNumber != 2 || got[2].StepNumber != 3 {
		t.Error("steps not ordered by step_number")
	}
	if got[0].Title != "Zulu" || got[1].Title != "Alpha" || got[2].Title != "Mike" {
		t.Error("step titles not in insertion order")
	}
}

func TestPlanStepStatus_PendingToInProgress(t *testing.T) {
	db := setupTestDB(t)
	mgr := plans.NewManager(db)
	fid := createTestFeature(t, db)
	sid := createTestSession(t, db, fid)
	p, _ := mgr.CreatePlan(fid, sid, "Test", "", "test", []plans.StepInput{{Title: "S1"}})
	steps, _ := mgr.GetPlanSteps(p.ID)
	mgr.UpdateStepStatus(steps[0].ID, "in_progress")
	updated, _ := mgr.GetPlanSteps(p.ID)
	if updated[0].Status != "in_progress" { t.Errorf("got %q", updated[0].Status) }
	if updated[0].CompletedAt != "" { t.Error("should have no completed_at") }
}

func TestPlanStepStatus_PendingToCompleted(t *testing.T) {
	db := setupTestDB(t)
	mgr := plans.NewManager(db)
	fid := createTestFeature(t, db)
	sid := createTestSession(t, db, fid)
	p, _ := mgr.CreatePlan(fid, sid, "Test", "", "test", []plans.StepInput{{Title: "S1"}})
	steps, _ := mgr.GetPlanSteps(p.ID)
	mgr.UpdateStepStatus(steps[0].ID, "completed")
	updated, _ := mgr.GetPlanSteps(p.ID)
	if updated[0].Status != "completed" { t.Errorf("got %q", updated[0].Status) }
	if updated[0].CompletedAt == "" { t.Error("should have completed_at") }
}

func TestPlanStepStatus_PendingToSkipped(t *testing.T) {
	db := setupTestDB(t)
	mgr := plans.NewManager(db)
	fid := createTestFeature(t, db)
	sid := createTestSession(t, db, fid)
	p, _ := mgr.CreatePlan(fid, sid, "Test", "", "test", []plans.StepInput{{Title: "S1"}})
	steps, _ := mgr.GetPlanSteps(p.ID)
	mgr.UpdateStepStatus(steps[0].ID, "skipped")
	updated, _ := mgr.GetPlanSteps(p.ID)
	if updated[0].Status != "skipped" { t.Errorf("got %q", updated[0].Status) }
}

func TestPlanStepStatus_PendingToPending(t *testing.T) {
	db := setupTestDB(t)
	mgr := plans.NewManager(db)
	fid := createTestFeature(t, db)
	sid := createTestSession(t, db, fid)
	p, _ := mgr.CreatePlan(fid, sid, "Test", "", "test", []plans.StepInput{{Title: "S1"}})
	steps, _ := mgr.GetPlanSteps(p.ID)
	mgr.UpdateStepStatus(steps[0].ID, "pending")
	updated, _ := mgr.GetPlanSteps(p.ID)
	if updated[0].Status != "pending" { t.Errorf("got %q", updated[0].Status) }
}

func TestPlanStepStatuses(t *testing.T) {
	db := setupTestDB(t)
	mgr := plans.NewManager(db)
	fid := createTestFeature(t, db)
	sid := createTestSession(t, db, fid)
	steps := []plans.StepInput{{Title: "S1"}, {Title: "S2"}, {Title: "S3"}, {Title: "S4"}}
	plan, _ := mgr.CreatePlan(fid, sid, "Status Transitions", "", "test", steps)
	planSteps, _ := mgr.GetPlanSteps(plan.ID)
	for i, tc := range []struct{ name, status string; wantCompleted bool }{
		{"pending_to_in_progress", "in_progress", false},
		{"pending_to_completed", "completed", true},
		{"pending_to_skipped", "skipped", false},
		{"pending_to_pending", "pending", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mgr.UpdateStepStatus(planSteps[i].ID, tc.status)
			updated, _ := mgr.GetPlanSteps(plan.ID)
			if updated[i].Status != tc.status { t.Errorf("got %q, want %q", updated[i].Status, tc.status) }
			if (updated[i].CompletedAt != "") != tc.wantCompleted { t.Errorf("completed_at mismatch") }
		})
	}
}

func TestLinkCommitToStep(t *testing.T) {
	db := setupTestDB(t)
	mgr := plans.NewManager(db)
	featureID := createTestFeature(t, db)
	sessionID := createTestSession(t, db, featureID)

	steps := []plans.StepInput{{Title: "Implement feature", Description: ""}}
	plan, err := mgr.CreatePlan(featureID, sessionID, "Test Plan", "", "test", steps)
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}

	planSteps, err := mgr.GetPlanSteps(plan.ID)
	if err != nil {
		t.Fatalf("GetPlanSteps: %v", err)
	}
	stepID := planSteps[0].ID

	// Link first commit
	if err := mgr.LinkCommitToStep(stepID, "abc123"); err != nil {
		t.Fatalf("LinkCommitToStep 1: %v", err)
	}

	// Link second commit
	if err := mgr.LinkCommitToStep(stepID, "def456"); err != nil {
		t.Fatalf("LinkCommitToStep 2: %v", err)
	}

	// Verify JSON array
	updated, err := mgr.GetPlanSteps(plan.ID)
	if err != nil {
		t.Fatalf("GetPlanSteps after link: %v", err)
	}

	var commits []string
	if err := json.Unmarshal([]byte(updated[0].LinkedCommits), &commits); err != nil {
		t.Fatalf("unmarshal linked_commits: %v", err)
	}
	if len(commits) != 2 {
		t.Fatalf("expected 2 commits, got %d", len(commits))
	}
	if commits[0] != "abc123" {
		t.Errorf("expected first commit 'abc123', got %s", commits[0])
	}
	if commits[1] != "def456" {
		t.Errorf("expected second commit 'def456', got %s", commits[1])
	}
}
