package memory_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/arbazkhan971/memorx/internal/memory"
	"github.com/arbazkhan971/memorx/internal/storage"
)

func TestWriteSnapshot_EmptyDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	db, err := storage.NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := storage.Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	store := memory.NewStore(db)
	memDir := filepath.Join(dir, ".memory")
	if err := os.MkdirAll(memDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	err = store.WriteSnapshot(memDir, "test-project", "/tmp/test-project")
	if err != nil {
		t.Fatalf("WriteSnapshot: %v", err)
	}

	// Read and parse the output
	data, err := os.ReadFile(filepath.Join(memDir, "current.json"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var snap memory.Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if snap.Project != "test-project" {
		t.Errorf("expected project 'test-project', got %q", snap.Project)
	}
	if snap.ProjectPath != "/tmp/test-project" {
		t.Errorf("expected project_path '/tmp/test-project', got %q", snap.ProjectPath)
	}
	if snap.ActiveFeature != nil {
		t.Error("expected nil active_feature for empty DB")
	}
	if snap.ActivePlan != nil {
		t.Error("expected nil active_plan for empty DB")
	}
	if len(snap.Features) != 0 {
		t.Errorf("expected 0 features, got %d", len(snap.Features))
	}
}

func TestWriteSnapshot_WithData(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	db, err := storage.NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := storage.Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	store := memory.NewStore(db)

	// Create features
	_, err = store.StartFeature("auth-v2", "Authentication system v2")
	if err != nil {
		t.Fatalf("StartFeature auth-v2: %v", err)
	}

	_, err = store.StartFeature("billing-fix", "Fix billing bugs")
	if err != nil {
		t.Fatalf("StartFeature billing-fix: %v", err)
	}

	// Create a session for billing-fix
	feature, err := store.GetActiveFeature()
	if err != nil {
		t.Fatalf("GetActiveFeature: %v", err)
	}
	_, err = store.CreateSession(feature.ID, "claude-code")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	memDir := filepath.Join(dir, ".memory")
	if err := os.MkdirAll(memDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	err = store.WriteSnapshot(memDir, "myproject", "/home/user/myproject")
	if err != nil {
		t.Fatalf("WriteSnapshot: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(memDir, "current.json"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var snap memory.Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	// Verify project info
	if snap.Project != "myproject" {
		t.Errorf("expected project 'myproject', got %q", snap.Project)
	}

	// Verify features
	if len(snap.Features) != 2 {
		t.Fatalf("expected 2 features, got %d", len(snap.Features))
	}

	// Verify active feature is billing-fix (last started)
	if snap.ActiveFeature == nil {
		t.Fatal("expected non-nil active_feature")
	}
	if snap.ActiveFeature.Name != "billing-fix" {
		t.Errorf("expected active feature 'billing-fix', got %q", snap.ActiveFeature.Name)
	}
	if snap.ActiveFeature.Status != "active" {
		t.Errorf("expected active status, got %q", snap.ActiveFeature.Status)
	}

	// Verify billing-fix has 1 session
	if snap.ActiveFeature.Sessions != 1 {
		t.Errorf("expected 1 session for billing-fix, got %d", snap.ActiveFeature.Sessions)
	}

	// Verify auth-v2 is paused
	found := false
	for _, f := range snap.Features {
		if f.Name == "auth-v2" {
			found = true
			if f.Status != "paused" {
				t.Errorf("expected auth-v2 status 'paused', got %q", f.Status)
			}
		}
	}
	if !found {
		t.Error("expected to find auth-v2 in features list")
	}

	// Verify no active plan
	if snap.ActivePlan != nil {
		t.Error("expected nil active_plan when no plan exists")
	}

	// Verify JSON is valid and pretty-printed (has indentation)
	if data[0] != '{' {
		t.Error("expected JSON to start with '{'")
	}
	// Pretty-printed JSON will have newlines
	hasNewline := false
	for _, b := range data {
		if b == '\n' {
			hasNewline = true
			break
		}
	}
	if !hasNewline {
		t.Error("expected pretty-printed JSON with newlines")
	}
}

func TestWriteSnapshot_OverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	db, err := storage.NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := storage.Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	store := memory.NewStore(db)
	memDir := filepath.Join(dir, ".memory")
	if err := os.MkdirAll(memDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// First write with no features
	err = store.WriteSnapshot(memDir, "proj", "/tmp/proj")
	if err != nil {
		t.Fatalf("first WriteSnapshot: %v", err)
	}

	// Add a feature
	_, err = store.StartFeature("new-feat", "A feature")
	if err != nil {
		t.Fatalf("StartFeature: %v", err)
	}

	// Second write should overwrite
	err = store.WriteSnapshot(memDir, "proj", "/tmp/proj")
	if err != nil {
		t.Fatalf("second WriteSnapshot: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(memDir, "current.json"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var snap memory.Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if len(snap.Features) != 1 {
		t.Errorf("expected 1 feature after overwrite, got %d", len(snap.Features))
	}
	if snap.Features[0].Name != "new-feat" {
		t.Errorf("expected feature 'new-feat', got %q", snap.Features[0].Name)
	}
}

func TestWriteSnapshot_WithActivePlan(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	db, err := storage.NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := storage.Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	store := memory.NewStore(db)

	// Create an active feature
	feat, err := store.StartFeature("plan-feat", "Feature with a plan")
	if err != nil {
		t.Fatalf("StartFeature: %v", err)
	}

	// Insert a plan directly into DB for this feature
	planID := "plan-test-id"
	_, err = db.Writer().Exec(
		`INSERT INTO plans (id, feature_id, title, content, status) VALUES (?, ?, ?, ?, 'active')`,
		planID, feat.ID, "Build the API", "Steps to build",
	)
	if err != nil {
		t.Fatalf("insert plan: %v", err)
	}

	// Insert 3 plan steps, 1 completed
	for i, step := range []struct {
		title  string
		status string
	}{
		{"Design schema", "completed"},
		{"Write handlers", "pending"},
		{"Add tests", "pending"},
	} {
		stepID := fmt.Sprintf("step-%d", i)
		_, err = db.Writer().Exec(
			`INSERT INTO plan_steps (id, plan_id, step_number, title, status) VALUES (?, ?, ?, ?, ?)`,
			stepID, planID, i+1, step.title, step.status,
		)
		if err != nil {
			t.Fatalf("insert step %d: %v", i, err)
		}
	}

	memDir := filepath.Join(dir, ".memory")
	if err := os.MkdirAll(memDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	err = store.WriteSnapshot(memDir, "plan-project", "/tmp/plan-project")
	if err != nil {
		t.Fatalf("WriteSnapshot: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(memDir, "current.json"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var snap memory.Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	// Verify plan progress appears
	if snap.ActivePlan == nil {
		t.Fatal("expected non-nil active_plan")
	}
	if snap.ActivePlan.Title != "Build the API" {
		t.Errorf("expected plan title 'Build the API', got %q", snap.ActivePlan.Title)
	}
	if snap.ActivePlan.StepsTotal != 3 {
		t.Errorf("expected 3 total steps, got %d", snap.ActivePlan.StepsTotal)
	}
	if snap.ActivePlan.StepsDone != 1 {
		t.Errorf("expected 1 done step, got %d", snap.ActivePlan.StepsDone)
	}
	if snap.ActivePlan.Progress != "1/3" {
		t.Errorf("expected progress '1/3', got %q", snap.ActivePlan.Progress)
	}
	if snap.ActivePlan.CurrentStep != "Write handlers" {
		t.Errorf("expected current step 'Write handlers', got %q", snap.ActivePlan.CurrentStep)
	}
}

func TestWriteSnapshot_MultipleFeaturesVariousStatuses(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	db, err := storage.NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := storage.Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	store := memory.NewStore(db)

	// Create feature-a (will become paused when feature-b starts)
	_, err = store.StartFeature("feature-a", "First feature")
	if err != nil {
		t.Fatalf("StartFeature a: %v", err)
	}

	// Create feature-b (will become paused when feature-c starts)
	_, err = store.StartFeature("feature-b", "Second feature")
	if err != nil {
		t.Fatalf("StartFeature b: %v", err)
	}

	// Mark feature-a as done directly
	err = store.UpdateFeatureStatus("feature-a", "done")
	if err != nil {
		t.Fatalf("UpdateFeatureStatus: %v", err)
	}

	// Create feature-c (active)
	_, err = store.StartFeature("feature-c", "Third feature")
	if err != nil {
		t.Fatalf("StartFeature c: %v", err)
	}

	memDir := filepath.Join(dir, ".memory")
	if err := os.MkdirAll(memDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	err = store.WriteSnapshot(memDir, "multi-proj", "/tmp/multi-proj")
	if err != nil {
		t.Fatalf("WriteSnapshot: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(memDir, "current.json"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var snap memory.Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if len(snap.Features) != 3 {
		t.Fatalf("expected 3 features, got %d", len(snap.Features))
	}

	// Verify we have each status represented
	statuses := map[string]string{}
	for _, f := range snap.Features {
		statuses[f.Name] = f.Status
	}
	if statuses["feature-a"] != "done" {
		t.Errorf("expected feature-a status 'done', got %q", statuses["feature-a"])
	}
	if statuses["feature-b"] != "paused" {
		t.Errorf("expected feature-b status 'paused', got %q", statuses["feature-b"])
	}
	if statuses["feature-c"] != "active" {
		t.Errorf("expected feature-c status 'active', got %q", statuses["feature-c"])
	}

	// Active feature should be feature-c
	if snap.ActiveFeature == nil {
		t.Fatal("expected non-nil active_feature")
	}
	if snap.ActiveFeature.Name != "feature-c" {
		t.Errorf("expected active feature 'feature-c', got %q", snap.ActiveFeature.Name)
	}
}

func TestWriteSnapshot_PlanProgressIncluded(t *testing.T) {
	dir := t.TempDir()
	db, _ := storage.NewDB(filepath.Join(dir, "test.db"))
	t.Cleanup(func() { db.Close() })
	storage.Migrate(db)
	store := memory.NewStore(db)

	feat, _ := store.StartFeature("plan-snap", "Plan snapshot test")
	db.Writer().Exec(`INSERT INTO plans (id, feature_id, title, content, status) VALUES ('p1', ?, 'Snap Plan', 'c', 'active')`, feat.ID)
	db.Writer().Exec(`INSERT INTO plan_steps (id, plan_id, step_number, title, status) VALUES ('s1', 'p1', 1, 'Step1', 'completed')`)
	db.Writer().Exec(`INSERT INTO plan_steps (id, plan_id, step_number, title, status) VALUES ('s2', 'p1', 2, 'Step2', 'pending')`)

	memDir := filepath.Join(dir, ".mem")
	os.MkdirAll(memDir, 0755)
	if err := store.WriteSnapshot(memDir, "proj", "/p"); err != nil {
		t.Fatalf("WriteSnapshot: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(memDir, "current.json"))
	var snap memory.Snapshot
	json.Unmarshal(data, &snap)
	if snap.ActivePlan == nil {
		t.Fatal("expected non-nil active_plan")
	}
	if snap.ActivePlan.StepsDone != 1 || snap.ActivePlan.StepsTotal != 2 {
		t.Errorf("expected 1/2 steps, got %d/%d", snap.ActivePlan.StepsDone, snap.ActivePlan.StepsTotal)
	}
}

func TestWriteSnapshot_JSONIsValid(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	db, err := storage.NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := storage.Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	store := memory.NewStore(db)

	// Add some data so JSON is non-trivial
	_, err = store.StartFeature("json-test", "Testing JSON validity")
	if err != nil {
		t.Fatalf("StartFeature: %v", err)
	}

	memDir := filepath.Join(dir, ".memory")
	if err := os.MkdirAll(memDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	err = store.WriteSnapshot(memDir, "json-proj", "/tmp/json-proj")
	if err != nil {
		t.Fatalf("WriteSnapshot: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(memDir, "current.json"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	// Verify JSON can be unmarshalled into a generic map
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal into map failed: %v", err)
	}

	// Verify expected top-level keys exist
	expectedKeys := []string{"project", "project_path", "active_feature", "active_plan", "features", "consolidation"}
	for _, key := range expectedKeys {
		if _, ok := raw[key]; !ok {
			t.Errorf("expected top-level key %q in JSON", key)
		}
	}

	// Verify project value
	if raw["project"] != "json-proj" {
		t.Errorf("expected project 'json-proj', got %v", raw["project"])
	}
}
