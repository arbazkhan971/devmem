package mcp

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arbaz/devmem/internal/storage"
	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// setupTestServer creates a temp dir with a git repo, DB, and DevMemServer.
func setupTestServer(t *testing.T) (*DevMemServer, string) {
	t.Helper()
	dir := t.TempDir()

	// Init a git repo so ProjectName and branch detection work.
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}

	// Create an initial commit so HEAD exists.
	cmd = exec.Command("git", "commit", "--allow-empty", "-m", "init")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@test.com",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit failed: %v\n%s", err, out)
	}

	dbPath := filepath.Join(dir, ".memory", "memory.db")
	db, err := storage.NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB failed: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := storage.Migrate(db); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	srv := NewServer(db, dir)
	return srv, dir
}

// newReq builds a CallToolRequest with the given tool name and arguments.
func newReq(name string, args map[string]interface{}) mcplib.CallToolRequest {
	req := mcplib.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	return req
}

// resultText extracts the text from the first TextContent in a CallToolResult.
func resultText(t *testing.T, res *mcplib.CallToolResult) string {
	t.Helper()
	if len(res.Content) == 0 {
		t.Fatal("result has no content")
	}
	tc, ok := res.Content[0].(mcplib.TextContent)
	if !ok {
		t.Fatalf("first content is not TextContent, got %T", res.Content[0])
	}
	return tc.Text
}

func TestHandleStatus(t *testing.T) {
	srv, dir := setupTestServer(t)
	ctx := context.Background()

	res, err := srv.handleStatus(ctx, newReq("devmem_status", nil))
	if err != nil {
		t.Fatalf("handleStatus error: %v", err)
	}

	text := resultText(t, res)
	projectName := filepath.Base(dir)
	if !strings.Contains(text, projectName) {
		t.Errorf("status should contain project name %q, got:\n%s", projectName, text)
	}
	if !strings.Contains(text, "# devmem status") {
		t.Errorf("status should contain markdown header, got:\n%s", text)
	}
	if !strings.Contains(text, "Active feature:") {
		t.Errorf("status should mention active feature section, got:\n%s", text)
	}
}

func TestHandleStartFeature(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	res, err := srv.handleStartFeature(ctx, newReq("devmem_start_feature", map[string]interface{}{
		"name":        "test-feature",
		"description": "a test feature",
	}))
	if err != nil {
		t.Fatalf("handleStartFeature error: %v", err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "test-feature") {
		t.Errorf("start feature result should contain feature name, got:\n%s", text)
	}
	if !strings.Contains(text, "created") {
		t.Errorf("start feature result should say 'created' for new feature, got:\n%s", text)
	}
	if !strings.Contains(text, "Context:") {
		t.Errorf("start feature result should contain context section, got:\n%s", text)
	}
}

func TestHandleRemember(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	// Start a feature first (required for remember).
	_, err := srv.handleStartFeature(ctx, newReq("devmem_start_feature", map[string]interface{}{
		"name": "remember-test",
	}))
	if err != nil {
		t.Fatalf("handleStartFeature error: %v", err)
	}

	res, err := srv.handleRemember(ctx, newReq("devmem_remember", map[string]interface{}{
		"content": "Use dependency injection for the database layer",
		"type":    "decision",
	}))
	if err != nil {
		t.Fatalf("handleRemember error: %v", err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "Remembered") {
		t.Errorf("remember result should contain 'Remembered', got:\n%s", text)
	}
	if !strings.Contains(text, "decision") {
		t.Errorf("remember result should contain note type 'decision', got:\n%s", text)
	}
	if !strings.Contains(text, "Links created:") {
		t.Errorf("remember result should contain links count, got:\n%s", text)
	}
}

func TestHandleSearch_NoResults(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	// Start a feature so "current_feature" scope works.
	_, err := srv.handleStartFeature(ctx, newReq("devmem_start_feature", map[string]interface{}{
		"name": "search-test",
	}))
	if err != nil {
		t.Fatalf("handleStartFeature error: %v", err)
	}

	res, err := srv.handleSearch(ctx, newReq("devmem_search", map[string]interface{}{
		"query": "nonexistent-xyz-foobar",
	}))
	if err != nil {
		t.Fatalf("handleSearch error: %v", err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "No results") {
		t.Errorf("search for nonexistent term should say 'No results', got:\n%s", text)
	}
}

func TestHandleSearch_WithResults(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	// Start a feature and remember something.
	_, err := srv.handleStartFeature(ctx, newReq("devmem_start_feature", map[string]interface{}{
		"name": "search-results-test",
	}))
	if err != nil {
		t.Fatalf("handleStartFeature error: %v", err)
	}
	_, err = srv.handleRemember(ctx, newReq("devmem_remember", map[string]interface{}{
		"content": "The authentication system uses JWT tokens for session management",
	}))
	if err != nil {
		t.Fatalf("handleRemember error: %v", err)
	}

	res, err := srv.handleSearch(ctx, newReq("devmem_search", map[string]interface{}{
		"query": "authentication JWT",
	}))
	if err != nil {
		t.Fatalf("handleSearch error: %v", err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "Search results") {
		t.Errorf("search with matching content should return results, got:\n%s", text)
	}
}

func TestHandleImportSession(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	res, err := srv.handleImportSession(ctx, newReq("devmem_import_session", map[string]interface{}{
		"feature_name": "import-test",
		"description":  "testing import",
		"decisions":    []interface{}{"Use Go for the backend", "Use SQLite for storage"},
		"facts": []interface{}{
			map[string]interface{}{"subject": "backend", "predicate": "uses", "object": "Go"},
		},
		"plan_title": "Build MVP",
		"plan_steps": []interface{}{
			map[string]interface{}{"title": "Set up project", "status": "completed"},
			map[string]interface{}{"title": "Add database", "status": "pending"},
		},
	}))
	if err != nil {
		t.Fatalf("handleImportSession error: %v", err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "Importing session into: import-test") {
		t.Errorf("import result should mention feature name, got:\n%s", text)
	}
	if !strings.Contains(text, "Decisions imported: 2") {
		t.Errorf("import result should report 2 decisions imported, got:\n%s", text)
	}
	if !strings.Contains(text, "Facts imported: 1") {
		t.Errorf("import result should report 1 fact imported, got:\n%s", text)
	}
	if !strings.Contains(text, "Plan imported: Build MVP") {
		t.Errorf("import result should report plan imported, got:\n%s", text)
	}
}

func TestHandleExport(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	// Create a feature with some data.
	_, err := srv.handleStartFeature(ctx, newReq("devmem_start_feature", map[string]interface{}{
		"name":        "export-test",
		"description": "testing export",
	}))
	if err != nil {
		t.Fatalf("handleStartFeature error: %v", err)
	}
	_, err = srv.handleRemember(ctx, newReq("devmem_remember", map[string]interface{}{
		"content": "export note content",
		"type":    "note",
	}))
	if err != nil {
		t.Fatalf("handleRemember error: %v", err)
	}

	res, err := srv.handleExport(ctx, newReq("devmem_export", map[string]interface{}{
		"feature_name": "export-test",
		"format":       "markdown",
	}))
	if err != nil {
		t.Fatalf("handleExport error: %v", err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "# Feature: export-test") {
		t.Errorf("export should contain feature header, got:\n%s", text)
	}
	if !strings.Contains(text, "**Status:** active") {
		t.Errorf("export should contain status, got:\n%s", text)
	}
}

func TestHandleListFeatures(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	// No features yet.
	res, err := srv.handleListFeatures(ctx, newReq("devmem_list_features", nil))
	if err != nil {
		t.Fatalf("handleListFeatures error: %v", err)
	}
	text := resultText(t, res)
	if !strings.Contains(text, "No features found") {
		t.Errorf("should say no features when empty, got:\n%s", text)
	}

	// Create some features.
	_, _ = srv.handleStartFeature(ctx, newReq("devmem_start_feature", map[string]interface{}{
		"name": "feature-alpha",
	}))
	_, _ = srv.handleStartFeature(ctx, newReq("devmem_start_feature", map[string]interface{}{
		"name": "feature-beta",
	}))

	res, err = srv.handleListFeatures(ctx, newReq("devmem_list_features", map[string]interface{}{
		"status_filter": "all",
	}))
	if err != nil {
		t.Fatalf("handleListFeatures error: %v", err)
	}
	text = resultText(t, res)
	if !strings.Contains(text, "feature-alpha") {
		t.Errorf("list should contain feature-alpha, got:\n%s", text)
	}
	if !strings.Contains(text, "feature-beta") {
		t.Errorf("list should contain feature-beta, got:\n%s", text)
	}
	if !strings.Contains(text, "# Features") {
		t.Errorf("list should have markdown header, got:\n%s", text)
	}
}

func TestHandleSavePlan(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	// Start a feature first.
	_, err := srv.handleStartFeature(ctx, newReq("devmem_start_feature", map[string]interface{}{
		"name": "plan-test",
	}))
	if err != nil {
		t.Fatalf("handleStartFeature error: %v", err)
	}

	res, err := srv.handleSavePlan(ctx, newReq("devmem_save_plan", map[string]interface{}{
		"title":   "Implementation Plan",
		"content": "Steps to build the feature",
		"steps": []interface{}{
			map[string]interface{}{"title": "Write tests", "description": "Unit and integration tests"},
			map[string]interface{}{"title": "Implement core logic"},
			map[string]interface{}{"title": "Add documentation"},
		},
	}))
	if err != nil {
		t.Fatalf("handleSavePlan error: %v", err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "Plan saved: Implementation Plan") {
		t.Errorf("save plan result should contain plan title, got:\n%s", text)
	}
	if !strings.Contains(text, "Steps: 3") {
		t.Errorf("save plan result should show 3 steps, got:\n%s", text)
	}
	if !strings.Contains(text, "Write tests") {
		t.Errorf("save plan result should list steps, got:\n%s", text)
	}
	if !strings.Contains(text, "Implement core logic") {
		t.Errorf("save plan result should list second step, got:\n%s", text)
	}
}

func TestHandleStartFeature_MissingName(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	res, err := srv.handleStartFeature(ctx, newReq("devmem_start_feature", nil))
	if err != nil {
		t.Fatalf("handleStartFeature error: %v", err)
	}
	if !res.IsError {
		text := resultText(t, res)
		if !strings.Contains(text, "required") {
			t.Errorf("missing name should return error about required param, got:\n%s", text)
		}
	}
}

func TestHandleRemember_NoActiveFeature(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	res, err := srv.handleRemember(ctx, newReq("devmem_remember", map[string]interface{}{
		"content": "some note",
	}))
	if err != nil {
		t.Fatalf("handleRemember error: %v", err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "No active feature") {
		t.Errorf("remember without active feature should return error, got:\n%s", text)
	}
}

func TestHandleSync_NoActiveFeature(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	// Call sync without starting a feature
	res, err := srv.handleSync(ctx, newReq("devmem_sync", nil))
	if err != nil {
		t.Fatalf("handleSync error: %v", err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "No active feature") {
		t.Errorf("sync without active feature should return error about no active feature, got:\n%s", text)
	}
}

func TestHandleGetContext_CompactTier(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	// Start a feature
	_, err := srv.handleStartFeature(ctx, newReq("devmem_start_feature", map[string]interface{}{
		"name":        "context-compact-test",
		"description": "Testing compact tier",
	}))
	if err != nil {
		t.Fatalf("handleStartFeature error: %v", err)
	}

	// Add some data
	_, err = srv.handleRemember(ctx, newReq("devmem_remember", map[string]interface{}{
		"content": "The API uses REST with JSON payloads",
		"type":    "decision",
	}))
	if err != nil {
		t.Fatalf("handleRemember error: %v", err)
	}

	// Get compact context
	res, err := srv.handleGetContext(ctx, newReq("devmem_get_context", map[string]interface{}{
		"tier": "compact",
	}))
	if err != nil {
		t.Fatalf("handleGetContext compact error: %v", err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "context-compact-test") {
		t.Errorf("compact context should contain feature name, got:\n%s", text)
	}
}

func TestHandleGetContext_StandardTier(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	// Start a feature
	_, err := srv.handleStartFeature(ctx, newReq("devmem_start_feature", map[string]interface{}{
		"name":        "context-standard-test",
		"description": "Testing standard tier",
	}))
	if err != nil {
		t.Fatalf("handleStartFeature error: %v", err)
	}

	// Add a decision note
	_, err = srv.handleRemember(ctx, newReq("devmem_remember", map[string]interface{}{
		"content": "Using PostgreSQL for the database",
		"type":    "decision",
	}))
	if err != nil {
		t.Fatalf("handleRemember error: %v", err)
	}

	// Get standard context
	res, err := srv.handleGetContext(ctx, newReq("devmem_get_context", map[string]interface{}{
		"tier": "standard",
	}))
	if err != nil {
		t.Fatalf("handleGetContext standard error: %v", err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "context-standard-test") {
		t.Errorf("standard context should contain feature name, got:\n%s", text)
	}
}

func TestHandleGetContext_DetailedTier(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	// Start a feature
	_, err := srv.handleStartFeature(ctx, newReq("devmem_start_feature", map[string]interface{}{
		"name":        "context-detailed-test",
		"description": "Testing detailed tier",
	}))
	if err != nil {
		t.Fatalf("handleStartFeature error: %v", err)
	}

	// Get detailed context
	res, err := srv.handleGetContext(ctx, newReq("devmem_get_context", map[string]interface{}{
		"tier": "detailed",
	}))
	if err != nil {
		t.Fatalf("handleGetContext detailed error: %v", err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "context-detailed-test") {
		t.Errorf("detailed context should contain feature name, got:\n%s", text)
	}
	// Detailed tier includes session history
	if !strings.Contains(text, "Session History") {
		t.Errorf("detailed context should include session history, got:\n%s", text)
	}
}

func TestHandleGetContext_NoActiveFeature(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	// Get context without an active feature
	res, err := srv.handleGetContext(ctx, newReq("devmem_get_context", map[string]interface{}{
		"tier": "standard",
	}))
	if err != nil {
		t.Fatalf("handleGetContext error: %v", err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "No active feature") {
		t.Errorf("get_context without active feature should error, got:\n%s", text)
	}
}

func TestHandleSwitchFeature_CreatesNewSession(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	// Create two features
	_, err := srv.handleStartFeature(ctx, newReq("devmem_start_feature", map[string]interface{}{
		"name": "switch-feat-a",
	}))
	if err != nil {
		t.Fatalf("handleStartFeature a error: %v", err)
	}

	_, err = srv.handleStartFeature(ctx, newReq("devmem_start_feature", map[string]interface{}{
		"name": "switch-feat-b",
	}))
	if err != nil {
		t.Fatalf("handleStartFeature b error: %v", err)
	}

	// Switch back to feat-a
	res, err := srv.handleSwitchFeature(ctx, newReq("devmem_switch_feature", map[string]interface{}{
		"name": "switch-feat-a",
	}))
	if err != nil {
		t.Fatalf("handleSwitchFeature error: %v", err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "Switched to feature: switch-feat-a") {
		t.Errorf("switch result should contain feature name, got:\n%s", text)
	}
	if !strings.Contains(text, "active") {
		t.Errorf("switch result should show active status, got:\n%s", text)
	}

	// Verify the status shows the correct feature context
	statusRes, err := srv.handleStatus(ctx, newReq("devmem_status", nil))
	if err != nil {
		t.Fatalf("handleStatus error: %v", err)
	}
	statusText := resultText(t, statusRes)
	if !strings.Contains(statusText, "switch-feat-a") {
		t.Errorf("status after switch should show switch-feat-a as active, got:\n%s", statusText)
	}
}
