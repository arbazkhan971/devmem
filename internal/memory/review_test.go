package memory_test

import (
	"strings"
	"testing"

	"github.com/arbazkhan971/memorx/internal/memory"
	"github.com/arbazkhan971/memorx/internal/plans"
)

func TestGetReviewContext_EmptyDB(t *testing.T) {
	store := newTestStore(t)
	files := []string{"src/auth/middleware.ts", "src/db/connection.ts"}

	contexts, err := store.GetReviewContext(files)
	if err != nil {
		t.Fatalf("GetReviewContext: %v", err)
	}
	if len(contexts) != 2 {
		t.Fatalf("expected 2 contexts, got %d", len(contexts))
	}
	for _, rc := range contexts {
		if rc.Summary != "No related memories found." {
			t.Errorf("expected no related memories, got %q", rc.Summary)
		}
	}
}

func TestGetReviewContext_WithNotes(t *testing.T) {
	store := newTestStore(t)

	f, err := store.CreateFeature("review-test", "Testing review context")
	if err != nil {
		t.Fatalf("CreateFeature: %v", err)
	}
	sess, _ := store.CreateSession(f.ID, "test")
	store.CreateNote(f.ID, sess.ID, "Decision: use middleware.ts for auth token validation", "decision")
	store.CreateNote(f.ID, sess.ID, "The middleware.ts file needs rate limiting", "note")

	contexts, err := store.GetReviewContext([]string{"middleware.ts"})
	if err != nil {
		t.Fatalf("GetReviewContext: %v", err)
	}
	if len(contexts) != 1 {
		t.Fatalf("expected 1 context, got %d", len(contexts))
	}

	rc := contexts[0]
	if len(rc.Notes) == 0 {
		t.Error("expected notes mentioning middleware.ts")
	}
	if !strings.Contains(rc.Summary, "Decision") {
		t.Errorf("expected summary to mention decision, got %q", rc.Summary)
	}
}

func TestGetReviewContext_WithFacts(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("fact-review", "Testing facts in review")
	store.CreateFact(f.ID, "", "auth", "uses", "better-auth")

	contexts, err := store.GetReviewContext([]string{"auth"})
	if err != nil {
		t.Fatalf("GetReviewContext: %v", err)
	}
	if len(contexts) != 1 {
		t.Fatalf("expected 1 context, got %d", len(contexts))
	}
	rc := contexts[0]
	if len(rc.Facts) == 0 {
		t.Error("expected facts mentioning auth")
	}
	if !strings.Contains(rc.Summary, "Fact") {
		t.Errorf("expected summary to mention fact, got %q", rc.Summary)
	}
}

func TestGetReviewRisk_EmptyDB(t *testing.T) {
	store := newTestStore(t)

	risks, err := store.GetReviewRisk([]string{"src/auth.ts", "src/db.ts"})
	if err != nil {
		t.Fatalf("GetReviewRisk: %v", err)
	}
	if len(risks) != 2 {
		t.Fatalf("expected 2 risks, got %d", len(risks))
	}
	for _, rr := range risks {
		if rr.RiskLevel != "LOW" {
			t.Errorf("expected LOW risk for %s, got %s", rr.File, rr.RiskLevel)
		}
	}
}

func TestGetReviewRisk_WithBlockers(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("risk-test", "Risk testing")
	sess, _ := store.CreateSession(f.ID, "test")
	store.CreateNote(f.ID, sess.ID, "Blocker: auth.ts has a race condition", "blocker")
	store.CreateNote(f.ID, sess.ID, "Blocker: auth.ts token refresh fails", "blocker")

	risks, err := store.GetReviewRisk([]string{"auth.ts"})
	if err != nil {
		t.Fatalf("GetReviewRisk: %v", err)
	}
	if len(risks) != 1 {
		t.Fatalf("expected 1 risk, got %d", len(risks))
	}
	if risks[0].BlockerCount != 2 {
		t.Errorf("expected 2 blockers, got %d", risks[0].BlockerCount)
	}
	if risks[0].RiskLevel == "LOW" {
		t.Error("expected risk level above LOW with blockers")
	}
}

func TestGenerateReviewChecklist_Empty(t *testing.T) {
	store := newTestStore(t)
	f, _ := store.CreateFeature("checklist-empty", "Empty checklist")

	items, err := store.GenerateReviewChecklist(f.ID)
	if err != nil {
		t.Fatalf("GenerateReviewChecklist: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items, got %d", len(items))
	}
}

func TestGenerateReviewChecklist_WithDecisionsAndFacts(t *testing.T) {
	store := newTestStore(t)
	f, _ := store.CreateFeature("checklist-full", "Full checklist")
	sess, _ := store.CreateSession(f.ID, "test")

	store.CreateNote(f.ID, sess.ID, "Use JWT for authentication", "decision")
	store.CreateNote(f.ID, sess.ID, "Database migration needs rollback support", "blocker")
	store.CreateFact(f.ID, sess.ID, "auth", "uses", "JWT")

	items, err := store.GenerateReviewChecklist(f.ID)
	if err != nil {
		t.Fatalf("GenerateReviewChecklist: %v", err)
	}

	if len(items) < 3 {
		t.Fatalf("expected at least 3 items (1 decision + 1 blocker + 1 fact), got %d", len(items))
	}

	sources := map[string]int{}
	for _, item := range items {
		sources[item.Source]++
	}
	if sources["decision"] != 1 {
		t.Errorf("expected 1 decision item, got %d", sources["decision"])
	}
	if sources["blocker"] != 1 {
		t.Errorf("expected 1 blocker item, got %d", sources["blocker"])
	}
	if sources["fact"] != 1 {
		t.Errorf("expected 1 fact item, got %d", sources["fact"])
	}
}

func TestGenerateReviewChecklist_WithPlanSteps(t *testing.T) {
	store, db := newTestStoreWithDB(t)
	mgr := plans.NewManager(db)

	f, _ := store.CreateFeature("checklist-plan", "Plan checklist")
	sess, _ := store.CreateSession(f.ID, "test")

	steps := []plans.StepInput{
		{Title: "Setup auth"},
		{Title: "Add middleware"},
		{Title: "Write tests"},
	}
	plan, err := mgr.CreatePlan(f.ID, sess.ID, "Auth Plan", "", "test", steps)
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	// Complete first step
	planSteps, _ := mgr.GetPlanSteps(plan.ID)
	mgr.UpdateStepStatus(planSteps[0].ID, "completed")

	items, err := store.GenerateReviewChecklist(f.ID)
	if err != nil {
		t.Fatalf("GenerateReviewChecklist: %v", err)
	}

	planItems := 0
	for _, item := range items {
		if item.Source == "plan" {
			planItems++
		}
	}
	if planItems != 2 {
		t.Errorf("expected 2 pending plan step items, got %d", planItems)
	}
}

func TestFormatReviewContext(t *testing.T) {
	contexts := []memory.ReviewContext{
		{
			File:    "src/auth.ts",
			Summary: "Decision: use JWT. Last commit: add auth",
		},
	}
	result := memory.FormatReviewContext(contexts)
	if !strings.Contains(result, "# Code Review Context") {
		t.Error("expected markdown header")
	}
	if !strings.Contains(result, "src/auth.ts") {
		t.Error("expected file name in output")
	}
}

func TestFormatReviewRisk(t *testing.T) {
	risks := []memory.ReviewRisk{
		{File: "src/auth.ts", RiskLevel: "HIGH", Reasons: []string{"changed 6 times", "2 blockers"}},
		{File: "src/db.ts", RiskLevel: "LOW"},
	}
	result := memory.FormatReviewRisk(risks)
	if !strings.Contains(result, "HIGH") {
		t.Error("expected HIGH risk level in output")
	}
	if !strings.Contains(result, "LOW") {
		t.Error("expected LOW risk level in output")
	}
}

func TestFormatReviewChecklist(t *testing.T) {
	items := []memory.ChecklistItem{
		{Text: "Verify JWT config", Source: "decision"},
		{Text: "Check migration", Source: "blocker"},
	}
	result := memory.FormatReviewChecklist(items, "auth-v2")
	if !strings.Contains(result, "# Review Checklist: auth-v2") {
		t.Error("expected checklist header")
	}
	if !strings.Contains(result, "[ ]") {
		t.Error("expected unchecked items")
	}
}

func TestFormatReviewChecklist_Empty(t *testing.T) {
	result := memory.FormatReviewChecklist(nil, "empty-feature")
	if !strings.Contains(result, "No checklist items") {
		t.Error("expected empty message")
	}
}
