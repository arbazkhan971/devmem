package memory_test

import (
	"path/filepath"
	"testing"

	"github.com/arbazkhan971/memorx/internal/memory"
	"github.com/arbazkhan971/memorx/internal/plans"
	"github.com/arbazkhan971/memorx/internal/storage"
	"github.com/google/uuid"
)

// newTestStoreWithDB creates a Store and returns the raw DB too (for inserting commits directly).
func newTestStoreWithDB(t *testing.T) (*memory.Store, *storage.DB) {
	t.Helper()
	dir := t.TempDir()
	db, err := storage.NewDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := storage.Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return memory.NewStore(db), db
}

// insertTestCommit inserts a commit directly into the DB for testing.
func insertTestCommit(t *testing.T, db *storage.DB, featureID, hash, message, intentType string) {
	t.Helper()
	id := uuid.New().String()
	_, err := db.Writer().Exec(
		`INSERT INTO commits (id, feature_id, hash, message, author, intent_type, intent_confidence, committed_at)
		 VALUES (?, ?, ?, ?, 'test-author', ?, 0.9, datetime('now'))`,
		id, featureID, hash, message, intentType,
	)
	if err != nil {
		t.Fatalf("insertTestCommit: %v", err)
	}
	// Also insert into FTS
	var rowID int64
	if err := db.Writer().QueryRow(`SELECT rowid FROM commits WHERE id = ?`, id).Scan(&rowID); err != nil {
		t.Fatalf("get commit rowid: %v", err)
	}
	if _, err := db.Writer().Exec(`INSERT INTO commits_fts(rowid, message) VALUES (?, ?)`, rowID, message); err != nil {
		t.Fatalf("insert commit FTS: %v", err)
	}
}

func TestGetFeatureAnalytics_Counts(t *testing.T) {
	store, db := newTestStoreWithDB(t)

	f, err := store.CreateFeature("analytics-feat", "Testing analytics")
	if err != nil {
		t.Fatalf("CreateFeature: %v", err)
	}
	sess, _ := store.CreateSession(f.ID, "test")

	// Create notes of different types
	store.CreateNote(f.ID, sess.ID, "note 1", "note")
	store.CreateNote(f.ID, sess.ID, "decision 1", "decision")
	store.CreateNote(f.ID, sess.ID, "decision 2", "decision")
	store.CreateNote(f.ID, sess.ID, "blocker 1", "blocker")

	// Create facts: 2 active, 1 invalidated
	store.CreateFact(f.ID, sess.ID, "db", "uses", "postgres")
	store.CreateFact(f.ID, sess.ID, "cache", "uses", "redis")
	// This contradicts the first fact, invalidating it
	store.CreateFact(f.ID, sess.ID, "db", "uses", "sqlite")

	// Insert commits directly
	insertTestCommit(t, db, f.ID, "abc1234", "feat: add auth", "feature")
	insertTestCommit(t, db, f.ID, "abc1235", "fix: login bug", "bugfix")

	a, err := store.GetFeatureAnalytics(f.ID)
	if err != nil {
		t.Fatalf("GetFeatureAnalytics: %v", err)
	}

	if a.Name != "analytics-feat" {
		t.Errorf("expected name 'analytics-feat', got %q", a.Name)
	}
	if a.SessionCount != 1 {
		t.Errorf("expected 1 session, got %d", a.SessionCount)
	}
	if a.CommitCount != 2 {
		t.Errorf("expected 2 commits, got %d", a.CommitCount)
	}
	if a.NoteCount != 4 {
		t.Errorf("expected 4 notes, got %d", a.NoteCount)
	}
	if a.DecisionCount != 2 {
		t.Errorf("expected 2 decisions, got %d", a.DecisionCount)
	}
	if a.BlockerCount != 1 {
		t.Errorf("expected 1 blocker, got %d", a.BlockerCount)
	}
	// 3 total facts: postgres (invalidated), redis (active), sqlite (active)
	if a.FactCount != 3 {
		t.Errorf("expected 3 total facts, got %d", a.FactCount)
	}
	if a.ActiveFactCount != 2 {
		t.Errorf("expected 2 active facts, got %d", a.ActiveFactCount)
	}
	if a.InvalidatedFactCount != 1 {
		t.Errorf("expected 1 invalidated fact, got %d", a.InvalidatedFactCount)
	}
	if a.DaysSinceCreated != 0 {
		t.Errorf("expected 0 days since created, got %d", a.DaysSinceCreated)
	}
	if a.PlanProgress != "no plan" {
		t.Errorf("expected 'no plan', got %q", a.PlanProgress)
	}
}

func TestGetFeatureAnalytics_IntentBreakdown(t *testing.T) {
	store, db := newTestStoreWithDB(t)

	f, _ := store.CreateFeature("intent-feat", "Test intent breakdown")

	insertTestCommit(t, db, f.ID, "h1", "feat: add login", "feature")
	insertTestCommit(t, db, f.ID, "h2", "feat: add signup", "feature")
	insertTestCommit(t, db, f.ID, "h3", "feat: add logout", "feature")
	insertTestCommit(t, db, f.ID, "h4", "fix: login crash", "bugfix")
	insertTestCommit(t, db, f.ID, "h5", "fix: signup validation", "bugfix")
	insertTestCommit(t, db, f.ID, "h6", "refactor: auth module", "refactor")

	a, err := store.GetFeatureAnalytics(f.ID)
	if err != nil {
		t.Fatalf("GetFeatureAnalytics: %v", err)
	}

	if a.IntentBreakdown["feature"] != 3 {
		t.Errorf("expected 3 feature commits, got %d", a.IntentBreakdown["feature"])
	}
	if a.IntentBreakdown["bugfix"] != 2 {
		t.Errorf("expected 2 bugfix commits, got %d", a.IntentBreakdown["bugfix"])
	}
	if a.IntentBreakdown["refactor"] != 1 {
		t.Errorf("expected 1 refactor commit, got %d", a.IntentBreakdown["refactor"])
	}
}

func TestGetFeatureAnalytics_PlanProgress(t *testing.T) {
	store, db := newTestStoreWithDB(t)
	mgr := plans.NewManager(db)

	f, _ := store.CreateFeature("plan-feat", "Test plan progress")
	sess, _ := store.CreateSession(f.ID, "test")

	// Create a plan with 7 steps
	steps := []plans.StepInput{
		{Title: "Step 1"}, {Title: "Step 2"}, {Title: "Step 3"},
		{Title: "Step 4"}, {Title: "Step 5"}, {Title: "Step 6"},
		{Title: "Step 7"},
	}
	plan, err := mgr.CreatePlan(f.ID, sess.ID, "Big Plan", "", "test", steps)
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}

	// Complete 4 of 7 steps
	planSteps, _ := mgr.GetPlanSteps(plan.ID)
	for i := 0; i < 4; i++ {
		mgr.UpdateStepStatus(planSteps[i].ID, "completed")
	}

	a, err := store.GetFeatureAnalytics(f.ID)
	if err != nil {
		t.Fatalf("GetFeatureAnalytics: %v", err)
	}

	expected := "4/7 (57%)"
	if a.PlanProgress != expected {
		t.Errorf("expected plan progress %q, got %q", expected, a.PlanProgress)
	}
}

func TestGetProjectAnalytics_MultipleFeatures(t *testing.T) {
	store, db := newTestStoreWithDB(t)

	// Create 3 features with different states
	f1, _ := store.CreateFeature("feat-active", "Active feature")
	f2, _ := store.CreateFeature("feat-paused", "Paused feature")
	f3, _ := store.CreateFeature("feat-done", "Done feature")

	store.UpdateFeatureStatus("feat-paused", "paused")
	store.UpdateFeatureStatus("feat-done", "done")

	// Create sessions: f1 gets 3 sessions, f2 gets 1, f3 gets 1
	store.CreateSession(f1.ID, "test")
	store.CreateSession(f1.ID, "test")
	store.CreateSession(f1.ID, "test")
	store.CreateSession(f2.ID, "test")
	store.CreateSession(f3.ID, "test")

	// Add notes
	store.CreateNote(f1.ID, "", "note in f1", "note")
	store.CreateNote(f2.ID, "", "note in f2", "note")

	// Add blockers: f2 gets 3 blockers, f1 gets 1
	store.CreateNote(f1.ID, "", "blocker in f1", "blocker")
	store.CreateNote(f2.ID, "", "blocker 1 in f2", "blocker")
	store.CreateNote(f2.ID, "", "blocker 2 in f2", "blocker")
	store.CreateNote(f2.ID, "", "blocker 3 in f2", "blocker")

	// Add facts
	store.CreateFact(f1.ID, "", "db", "uses", "sqlite")

	// Add commits
	insertTestCommit(t, db, f1.ID, "c1", "feat: something", "feature")
	insertTestCommit(t, db, f3.ID, "c2", "fix: something", "bugfix")

	a, err := store.GetProjectAnalytics()
	if err != nil {
		t.Fatalf("GetProjectAnalytics: %v", err)
	}

	if a.TotalFeatures != 3 {
		t.Errorf("expected 3 total features, got %d", a.TotalFeatures)
	}
	if a.ActiveFeatures != 1 {
		t.Errorf("expected 1 active feature, got %d", a.ActiveFeatures)
	}
	if a.PausedFeatures != 1 {
		t.Errorf("expected 1 paused feature, got %d", a.PausedFeatures)
	}
	if a.DoneFeatures != 1 {
		t.Errorf("expected 1 done feature, got %d", a.DoneFeatures)
	}
	if a.TotalSessions != 5 {
		t.Errorf("expected 5 total sessions, got %d", a.TotalSessions)
	}
	if a.TotalCommits != 2 {
		t.Errorf("expected 2 total commits, got %d", a.TotalCommits)
	}
	if a.TotalNotes != 6 {
		t.Errorf("expected 6 total notes, got %d", a.TotalNotes)
	}
	if a.TotalFacts != 1 {
		t.Errorf("expected 1 total fact, got %d", a.TotalFacts)
	}

	// Most active = feat-active (3 sessions)
	if a.MostActiveFeature != "feat-active" {
		t.Errorf("expected most active 'feat-active', got %q", a.MostActiveFeature)
	}

	// Most blocked = feat-paused (3 blockers)
	if a.MostBlockedFeature != "feat-paused" {
		t.Errorf("expected most blocked 'feat-paused', got %q", a.MostBlockedFeature)
	}
}

func TestGetProjectAnalytics_MostBlockedFeature(t *testing.T) {
	store, _ := newTestStoreWithDB(t)

	f1, _ := store.CreateFeature("feat-a", "Feature A")
	f2, _ := store.CreateFeature("feat-b", "Feature B")
	f3, _ := store.CreateFeature("feat-c", "Feature C")

	// f1 gets 1 blocker, f2 gets 0, f3 gets 5
	store.CreateNote(f1.ID, "", "blocker 1", "blocker")

	store.CreateNote(f3.ID, "", "blocker 1", "blocker")
	store.CreateNote(f3.ID, "", "blocker 2", "blocker")
	store.CreateNote(f3.ID, "", "blocker 3", "blocker")
	store.CreateNote(f3.ID, "", "blocker 4", "blocker")
	store.CreateNote(f3.ID, "", "blocker 5", "blocker")

	// f2 has no blockers to ensure it's not picked
	_ = f2

	a, err := store.GetProjectAnalytics()
	if err != nil {
		t.Fatalf("GetProjectAnalytics: %v", err)
	}

	if a.MostBlockedFeature != "feat-c" {
		t.Errorf("expected most blocked 'feat-c', got %q", a.MostBlockedFeature)
	}
}

func TestGetProjectAnalytics_EmptyDB(t *testing.T) {
	store := newTestStore(t)

	a, err := store.GetProjectAnalytics()
	if err != nil {
		t.Fatalf("GetProjectAnalytics: %v", err)
	}

	if a.TotalFeatures != 0 {
		t.Errorf("expected 0 features, got %d", a.TotalFeatures)
	}
	if a.MostActiveFeature != "" {
		t.Errorf("expected empty most active, got %q", a.MostActiveFeature)
	}
	if a.MostBlockedFeature != "" {
		t.Errorf("expected empty most blocked, got %q", a.MostBlockedFeature)
	}
}

func TestGetFeatureAnalytics_ZeroSessions(t *testing.T) {
	store := newTestStore(t)
	f, _ := store.CreateFeature("zero-sessions", "No sessions")
	a, err := store.GetFeatureAnalytics(f.ID)
	if err != nil {
		t.Fatalf("GetFeatureAnalytics: %v", err)
	}
	if a.SessionCount != 0 {
		t.Errorf("expected 0 sessions, got %d", a.SessionCount)
	}
	if a.AvgSessionDuration != "n/a" {
		t.Errorf("expected avg duration 'n/a', got %q", a.AvgSessionDuration)
	}
	if a.PlanProgress != "no plan" {
		t.Errorf("expected 'no plan', got %q", a.PlanProgress)
	}
}

func TestGetFeatureAnalytics_NotFound(t *testing.T) {
	store := newTestStore(t)

	_, err := store.GetFeatureAnalytics("nonexistent-id")
	if err == nil {
		t.Fatal("expected error for nonexistent feature")
	}
}
