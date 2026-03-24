package memory_test

import (
	"strings"
	"testing"

	"github.com/arbazkhan971/memorx/internal/memory"
	"github.com/arbazkhan971/memorx/internal/plans"
)

func TestGetFocusTime_EmptyDB(t *testing.T) {
	store := newTestStore(t)

	entries, err := store.GetFocusTime("", 7)
	if err != nil {
		t.Fatalf("GetFocusTime: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestGetFocusTime_WithSessions(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("focus-test", "Focus time test")
	sess, _ := store.CreateSession(f.ID, "test")
	store.EndSession(sess.ID)

	entries, err := store.GetFocusTime("", 7)
	if err != nil {
		t.Fatalf("GetFocusTime: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].FeatureName != "focus-test" {
		t.Errorf("expected feature name 'focus-test', got %q", entries[0].FeatureName)
	}
	if entries[0].SessionCount != 1 {
		t.Errorf("expected 1 session, got %d", entries[0].SessionCount)
	}
}

func TestGetFocusTime_FilterByFeature(t *testing.T) {
	store := newTestStore(t)

	f1, _ := store.CreateFeature("feat-a", "Feature A")
	f2, _ := store.CreateFeature("feat-b", "Feature B")
	s1, _ := store.CreateSession(f1.ID, "test")
	store.EndSession(s1.ID)
	s2, _ := store.CreateSession(f2.ID, "test")
	store.EndSession(s2.ID)

	entries, err := store.GetFocusTime("feat-a", 7)
	if err != nil {
		t.Fatalf("GetFocusTime: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry for feat-a, got %d", len(entries))
	}
	if entries[0].FeatureName != "feat-a" {
		t.Errorf("expected 'feat-a', got %q", entries[0].FeatureName)
	}
}

func TestGetVelocity_NoPlan(t *testing.T) {
	store := newTestStore(t)
	store.CreateFeature("no-plan", "No plan feature")

	entries, err := store.GetVelocity("")
	if err != nil {
		t.Fatalf("GetVelocity: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries (no plan), got %d", len(entries))
	}
}

func TestGetVelocity_WithPlan(t *testing.T) {
	store, db := newTestStoreWithDB(t)
	mgr := plans.NewManager(db)

	f, _ := store.CreateFeature("velocity-feat", "Velocity test")
	sess, _ := store.CreateSession(f.ID, "test")

	steps := []plans.StepInput{
		{Title: "Step 1"}, {Title: "Step 2"}, {Title: "Step 3"},
		{Title: "Step 4"}, {Title: "Step 5"},
	}
	plan, err := mgr.CreatePlan(f.ID, sess.ID, "Velocity Plan", "", "test", steps)
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}

	planSteps, _ := mgr.GetPlanSteps(plan.ID)
	mgr.UpdateStepStatus(planSteps[0].ID, "completed")
	mgr.UpdateStepStatus(planSteps[1].ID, "completed")

	entries, err := store.GetVelocity("")
	if err != nil {
		t.Fatalf("GetVelocity: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least 1 velocity entry")
	}

	entry := entries[0]
	if entry.FeatureName != "velocity-feat" {
		t.Errorf("expected 'velocity-feat', got %q", entry.FeatureName)
	}
	if entry.StepsCompleted != 2 {
		t.Errorf("expected 2 completed steps, got %d", entry.StepsCompleted)
	}
	if entry.TotalSteps != 5 {
		t.Errorf("expected 5 total steps, got %d", entry.TotalSteps)
	}
}

func TestGetInterruptions_EmptyDB(t *testing.T) {
	store := newTestStore(t)

	report, err := store.GetInterruptions(7)
	if err != nil {
		t.Fatalf("GetInterruptions: %v", err)
	}
	if report.SwitchCount != 0 {
		t.Errorf("expected 0 switches, got %d", report.SwitchCount)
	}
	if report.DaysAnalyzed != 7 {
		t.Errorf("expected 7 days analyzed, got %d", report.DaysAnalyzed)
	}
}

func TestGetInterruptions_WithSwitches(t *testing.T) {
	store := newTestStore(t)

	f1, _ := store.CreateFeature("feat-x", "Feature X")
	f2, _ := store.CreateFeature("feat-y", "Feature Y")

	s1, _ := store.CreateSession(f1.ID, "test")
	store.EndSession(s1.ID)
	s2, _ := store.CreateSession(f2.ID, "test")
	store.EndSession(s2.ID)
	s3, _ := store.CreateSession(f1.ID, "test")
	store.EndSession(s3.ID)

	report, err := store.GetInterruptions(7)
	if err != nil {
		t.Fatalf("GetInterruptions: %v", err)
	}
	if report.SwitchCount != 2 {
		t.Errorf("expected 2 switches (X->Y, Y->X), got %d", report.SwitchCount)
	}
}

func TestGetInterruptions_NoSwitches(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("single-feat", "Single feature")
	s1, _ := store.CreateSession(f.ID, "test")
	store.EndSession(s1.ID)
	s2, _ := store.CreateSession(f.ID, "test")
	store.EndSession(s2.ID)

	report, err := store.GetInterruptions(7)
	if err != nil {
		t.Fatalf("GetInterruptions: %v", err)
	}
	if report.SwitchCount != 0 {
		t.Errorf("expected 0 switches for single feature, got %d", report.SwitchCount)
	}
}

func TestGetWeeklyReport_EmptyDB(t *testing.T) {
	store := newTestStore(t)

	report, err := store.GetWeeklyReport(7)
	if err != nil {
		t.Fatalf("GetWeeklyReport: %v", err)
	}
	if report.DaysBack != 7 {
		t.Errorf("expected 7 days back, got %d", report.DaysBack)
	}
	if report.TotalCommits != 0 {
		t.Errorf("expected 0 commits, got %d", report.TotalCommits)
	}
	if report.SessionCount != 0 {
		t.Errorf("expected 0 sessions, got %d", report.SessionCount)
	}
}

func TestGetWeeklyReport_WithData(t *testing.T) {
	store, db := newTestStoreWithDB(t)

	f, _ := store.CreateFeature("weekly-feat", "Weekly report test")
	sess, _ := store.CreateSession(f.ID, "test")
	store.EndSession(sess.ID)

	store.CreateNote(f.ID, "", "Use PostgreSQL for persistence", "decision")
	store.CreateNote(f.ID, "", "Migration tool not working", "blocker")

	insertTestCommit(t, db, f.ID, "abc123", "feat: add auth", "feature")
	insertTestCommit(t, db, f.ID, "def456", "fix: login bug", "bugfix")

	report, err := store.GetWeeklyReport(7)
	if err != nil {
		t.Fatalf("GetWeeklyReport: %v", err)
	}
	if report.SessionCount != 1 {
		t.Errorf("expected 1 session, got %d", report.SessionCount)
	}
	if report.TotalCommits != 2 {
		t.Errorf("expected 2 commits, got %d", report.TotalCommits)
	}
	if report.DecisionsMade != 1 {
		t.Errorf("expected 1 decision, got %d", report.DecisionsMade)
	}
	if report.BlockersAdded != 1 {
		t.Errorf("expected 1 blocker, got %d", report.BlockersAdded)
	}
	if len(report.FeaturesTouched) != 1 {
		t.Errorf("expected 1 feature touched, got %d", len(report.FeaturesTouched))
	}
}

func TestFormatFocusTime(t *testing.T) {
	entries := []memory.FocusTimeEntry{
		{FeatureName: "auth-v2", TotalHours: 12.5, SessionCount: 8},
		{FeatureName: "billing", TotalHours: 3.2, SessionCount: 2},
	}
	result := memory.FormatFocusTime(entries, 7)
	if !strings.Contains(result, "auth-v2") {
		t.Error("expected auth-v2 in output")
	}
	if !strings.Contains(result, "12.5h") {
		t.Error("expected 12.5h in output")
	}
	if !strings.Contains(result, "8 session(s)") {
		t.Error("expected session count in output")
	}
}

func TestFormatFocusTime_Empty(t *testing.T) {
	result := memory.FormatFocusTime(nil, 7)
	if !strings.Contains(result, "No sessions recorded") {
		t.Error("expected empty message")
	}
}

func TestFormatVelocity(t *testing.T) {
	entries := []memory.VelocityEntry{
		{FeatureName: "auth-v2", StepsCompleted: 3, TotalSteps: 5, StepsPerDay: 1.2, EstDaysLeft: 2},
		{FeatureName: "billing", StepsCompleted: 0, TotalSteps: 3, Stalled: true, StalledDays: 5},
	}
	result := memory.FormatVelocity(entries)
	if !strings.Contains(result, "1.2 steps/day") {
		t.Error("expected steps/day in output")
	}
	if !strings.Contains(result, "stalled 5 day(s)") {
		t.Error("expected stalled message")
	}
}

func TestFormatInterruptions(t *testing.T) {
	report := &memory.InterruptionReport{
		SwitchCount:          7,
		DaysAnalyzed:         3,
		LongestUninterrupted: 2.5,
		LongestFeature:       "auth-v2",
	}
	result := memory.FormatInterruptions(report)
	if !strings.Contains(result, "7 switch(es)") {
		t.Error("expected switch count")
	}
	if !strings.Contains(result, "2.5h on auth-v2") {
		t.Error("expected longest uninterrupted stretch")
	}
}

func TestFormatWeeklyReport(t *testing.T) {
	report := &memory.WeeklyReportData{
		DaysBack:        7,
		FeaturesTouched: []string{"auth-v2", "billing"},
		CommitsByIntent: map[string]int{"feature": 5, "bugfix": 3},
		TotalCommits:    8,
		DecisionsMade:   2,
		SessionCount:    10,
		TotalHours:      15.5,
		TopDecisions:    []string{"Use JWT for auth"},
	}
	result := memory.FormatWeeklyReport(report)
	if !strings.Contains(result, "# Weekly Dev Summary") {
		t.Error("expected weekly summary header")
	}
	if !strings.Contains(result, "auth-v2") {
		t.Error("expected feature name")
	}
	if !strings.Contains(result, "15.5h") {
		t.Error("expected total hours")
	}
}
