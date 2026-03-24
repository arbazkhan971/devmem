package mcp

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arbazkhan971/memorx/internal/storage"
	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
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

func TestAllToolsExist(t *testing.T) {
	srv := server.NewMCPServer("memorx", "1.0.0")
	dir := t.TempDir()
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	dbPath := filepath.Join(dir, ".memory", "memory.db")
	db, err := storage.NewDB(dbPath)
	if err != nil { t.Fatalf("NewDB: %v", err) }
	t.Cleanup(func() { db.Close() })
	if err := storage.Migrate(db); err != nil { t.Fatalf("Migrate: %v", err) }
	memorx := NewServer(db, dir)
	memorx.registerTools(srv)
	toolMap := srv.ListTools()
	for _, tc := range []struct{ name string }{
		{"memorx_briefing"}, {"memorx_status"}, {"memorx_list_features"},
		{"memorx_start_feature"}, {"memorx_switch_feature"}, {"memorx_get_context"},
		{"memorx_sync"}, {"memorx_remember"}, {"memorx_search"}, {"memorx_history"},
		{"memorx_save_plan"}, {"memorx_import_session"}, {"memorx_end_session"},
		{"memorx_export"}, {"memorx_health"}, {"memorx_forget"},
		{"memorx_analytics"}, {"memorx_generate_rules"},
		{"memorx_snapshot"}, {"memorx_recover"},
		{"memorx_related"}, {"memorx_dependencies"},
		{"memorx_diff"},
		{"memorx_onboard"}, {"memorx_changelog"}, {"memorx_share"},
		{"memorx_review_context"}, {"memorx_review_risk"}, {"memorx_review_checklist"},
		{"memorx_focus_time"}, {"memorx_velocity"}, {"memorx_interruptions"}, {"memorx_weekly_report"},
		// Predictive Intelligence
		{"memorx_predict_blocker"}, {"memorx_risk_score"}, {"memorx_burndown"}, {"memorx_compare"}, {"memorx_summarize_period"},
		// Self-Healing Memory
		{"memorx_deduplicate"}, {"memorx_integrity_check"}, {"memorx_auto_link_code"},
		// Workflow Integration
		{"memorx_standup"}, {"memorx_branch_context"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, ok := toolMap[tc.name]; !ok {
				t.Errorf("tool %q not found in registered tools", tc.name)
			}
		})
	}
}

func TestHandleStatus(t *testing.T) {
	srv, dir := setupTestServer(t)
	ctx := context.Background()

	res, err := srv.handleStatus(ctx, newReq("memorx_status", nil))
	if err != nil {
		t.Fatalf("handleStatus error: %v", err)
	}

	text := resultText(t, res)
	projectName := filepath.Base(dir)
	if !strings.Contains(text, projectName) {
		t.Errorf("status should contain project name %q, got:\n%s", projectName, text)
	}
	if !strings.Contains(text, "# memorx status") {
		t.Errorf("status should contain markdown header, got:\n%s", text)
	}
	if !strings.Contains(text, "Active feature:") {
		t.Errorf("status should mention active feature section, got:\n%s", text)
	}
}

func TestHandleStartFeature(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	res, err := srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
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
	// Start feature response includes compact context after the separator
	if !strings.Contains(text, "---") {
		t.Errorf("start feature result should contain context separator, got:\n%s", text)
	}
}

func TestHandleRemember(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	// Start a feature first (required for remember).
	_, err := srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name": "remember-test",
	}))
	if err != nil {
		t.Fatalf("handleStartFeature error: %v", err)
	}

	res, err := srv.handleRemember(ctx, newReq("memorx_remember", map[string]interface{}{
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
	_, err := srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name": "search-test",
	}))
	if err != nil {
		t.Fatalf("handleStartFeature error: %v", err)
	}

	res, err := srv.handleSearch(ctx, newReq("memorx_search", map[string]interface{}{
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
	_, err := srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name": "search-results-test",
	}))
	if err != nil {
		t.Fatalf("handleStartFeature error: %v", err)
	}
	_, err = srv.handleRemember(ctx, newReq("memorx_remember", map[string]interface{}{
		"content": "The authentication system uses JWT tokens for session management",
	}))
	if err != nil {
		t.Fatalf("handleRemember error: %v", err)
	}

	res, err := srv.handleSearch(ctx, newReq("memorx_search", map[string]interface{}{
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

	res, err := srv.handleImportSession(ctx, newReq("memorx_import_session", map[string]interface{}{
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
	_, err := srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name":        "export-test",
		"description": "testing export",
	}))
	if err != nil {
		t.Fatalf("handleStartFeature error: %v", err)
	}
	_, err = srv.handleRemember(ctx, newReq("memorx_remember", map[string]interface{}{
		"content": "export note content",
		"type":    "note",
	}))
	if err != nil {
		t.Fatalf("handleRemember error: %v", err)
	}

	res, err := srv.handleExport(ctx, newReq("memorx_export", map[string]interface{}{
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
	res, err := srv.handleListFeatures(ctx, newReq("memorx_list_features", nil))
	if err != nil {
		t.Fatalf("handleListFeatures error: %v", err)
	}
	text := resultText(t, res)
	if !strings.Contains(text, "No features found") {
		t.Errorf("should say no features when empty, got:\n%s", text)
	}

	// Create some features.
	_, _ = srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name": "feature-alpha",
	}))
	_, _ = srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name": "feature-beta",
	}))

	res, err = srv.handleListFeatures(ctx, newReq("memorx_list_features", map[string]interface{}{
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
	_, err := srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name": "plan-test",
	}))
	if err != nil {
		t.Fatalf("handleStartFeature error: %v", err)
	}

	res, err := srv.handleSavePlan(ctx, newReq("memorx_save_plan", map[string]interface{}{
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

	res, err := srv.handleStartFeature(ctx, newReq("memorx_start_feature", nil))
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

	res, err := srv.handleRemember(ctx, newReq("memorx_remember", map[string]interface{}{
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
	res, err := srv.handleSync(ctx, newReq("memorx_sync", nil))
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
	_, err := srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name":        "context-compact-test",
		"description": "Testing compact tier",
	}))
	if err != nil {
		t.Fatalf("handleStartFeature error: %v", err)
	}

	// Add some data
	_, err = srv.handleRemember(ctx, newReq("memorx_remember", map[string]interface{}{
		"content": "The API uses REST with JSON payloads",
		"type":    "decision",
	}))
	if err != nil {
		t.Fatalf("handleRemember error: %v", err)
	}

	// Get compact context
	res, err := srv.handleGetContext(ctx, newReq("memorx_get_context", map[string]interface{}{
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
	_, err := srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name":        "context-standard-test",
		"description": "Testing standard tier",
	}))
	if err != nil {
		t.Fatalf("handleStartFeature error: %v", err)
	}

	// Add a decision note
	_, err = srv.handleRemember(ctx, newReq("memorx_remember", map[string]interface{}{
		"content": "Using PostgreSQL for the database",
		"type":    "decision",
	}))
	if err != nil {
		t.Fatalf("handleRemember error: %v", err)
	}

	// Get standard context
	res, err := srv.handleGetContext(ctx, newReq("memorx_get_context", map[string]interface{}{
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
	_, err := srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name":        "context-detailed-test",
		"description": "Testing detailed tier",
	}))
	if err != nil {
		t.Fatalf("handleStartFeature error: %v", err)
	}

	// Get detailed context
	res, err := srv.handleGetContext(ctx, newReq("memorx_get_context", map[string]interface{}{
		"tier": "detailed",
	}))
	if err != nil {
		t.Fatalf("handleGetContext detailed error: %v", err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "context-detailed-test") {
		t.Errorf("detailed context should contain feature name, got:\n%s", text)
	}
	// Detailed tier includes session info
	if !strings.Contains(text, "Sessions:") {
		t.Errorf("detailed context should include session info, got:\n%s", text)
	}
}

func TestHandleGetContext_NoActiveFeature(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	// Get context without an active feature
	res, err := srv.handleGetContext(ctx, newReq("memorx_get_context", map[string]interface{}{
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

func TestHandleExport_JSONFormat(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	// Create a feature with some data.
	_, err := srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name":        "export-json-test",
		"description": "testing JSON export",
	}))
	if err != nil {
		t.Fatalf("handleStartFeature error: %v", err)
	}
	_, err = srv.handleRemember(ctx, newReq("memorx_remember", map[string]interface{}{
		"content": "json export note content",
		"type":    "decision",
	}))
	if err != nil {
		t.Fatalf("handleRemember error: %v", err)
	}

	res, err := srv.handleExport(ctx, newReq("memorx_export", map[string]interface{}{
		"feature_name": "export-json-test",
		"format":       "json",
	}))
	if err != nil {
		t.Fatalf("handleExport error: %v", err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, `"feature": "export-json-test"`) {
		t.Errorf("JSON export should contain feature name, got:\n%s", text)
	}
	if !strings.Contains(text, `"status": "active"`) {
		t.Errorf("JSON export should contain status, got:\n%s", text)
	}
	if !strings.Contains(text, `"description": "testing JSON export"`) {
		t.Errorf("JSON export should contain description, got:\n%s", text)
	}
	// Should be valid JSON-ish with curly braces
	if !strings.HasPrefix(strings.TrimSpace(text), "{") || !strings.HasSuffix(strings.TrimSpace(text), "}") {
		t.Errorf("JSON export should be wrapped in curly braces, got:\n%s", text)
	}
}

func TestHandleRemember_PlanAutoPromotion(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	// Start a feature first.
	_, err := srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name": "plan-auto-promote-test",
	}))
	if err != nil {
		t.Fatalf("handleStartFeature error: %v", err)
	}

	// Content with 3+ numbered steps and a plan keyword triggers auto-promotion.
	planContent := `Implementation plan for the feature:
1. Set up database schema
2. Create API endpoints
3. Write integration tests
4. Deploy to staging`

	res, err := srv.handleRemember(ctx, newReq("memorx_remember", map[string]interface{}{
		"content": planContent,
		"type":    "note",
	}))
	if err != nil {
		t.Fatalf("handleRemember error: %v", err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "Remembered") {
		t.Errorf("remember result should contain 'Remembered', got:\n%s", text)
	}
	if !strings.Contains(text, "Auto-promoted to plan") {
		t.Errorf("plan-like content should trigger auto-promotion, got:\n%s", text)
	}
	if !strings.Contains(text, "4 steps") {
		t.Errorf("auto-promoted plan should have 4 steps, got:\n%s", text)
	}
}

func TestHandleStartFeature_ResumeExisting(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	// Create a feature.
	res1, err := srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name":        "resume-test",
		"description": "first feature",
	}))
	if err != nil {
		t.Fatalf("handleStartFeature error: %v", err)
	}
	text1 := resultText(t, res1)
	if !strings.Contains(text1, "created") {
		t.Errorf("first start should say 'created', got:\n%s", text1)
	}

	// Start a second feature (pausing the first).
	_, err = srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name": "other-feature",
	}))
	if err != nil {
		t.Fatalf("handleStartFeature other error: %v", err)
	}

	// Resume the first feature — should say "resumed", not "created".
	res2, err := srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name": "resume-test",
	}))
	if err != nil {
		t.Fatalf("handleStartFeature resume error: %v", err)
	}
	text2 := resultText(t, res2)
	if !strings.Contains(text2, "resumed") {
		t.Errorf("second start of existing feature should say 'resumed', got:\n%s", text2)
	}
	if !strings.Contains(text2, "resume-test") {
		t.Errorf("resumed feature should contain feature name, got:\n%s", text2)
	}
}

func TestHandleSearch_AllFeaturesScope(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	// Create feature A and remember something.
	_, err := srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name": "search-all-feat-a",
	}))
	if err != nil {
		t.Fatalf("handleStartFeature A error: %v", err)
	}
	_, err = srv.handleRemember(ctx, newReq("memorx_remember", map[string]interface{}{
		"content": "The payment gateway uses Stripe for all transactions",
	}))
	if err != nil {
		t.Fatalf("handleRemember error: %v", err)
	}

	// Create feature B (which pauses A).
	_, err = srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name": "search-all-feat-b",
	}))
	if err != nil {
		t.Fatalf("handleStartFeature B error: %v", err)
	}

	// Search across all features should find the note from feature A.
	res, err := srv.handleSearch(ctx, newReq("memorx_search", map[string]interface{}{
		"query": "payment Stripe",
		"scope": "all_features",
	}))
	if err != nil {
		t.Fatalf("handleSearch error: %v", err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "Search results") {
		t.Errorf("all_features search should find results from other features, got:\n%s", text)
	}
}

func TestHandleStatus_WithFeaturesInAllStates(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	// Create features in different states
	_, _ = srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name":        "feat-done",
		"description": "will be done",
	}))
	_, _ = srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name":        "feat-paused",
		"description": "will be paused",
	}))
	_, _ = srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name":        "feat-active",
		"description": "the active one",
	}))

	res, err := srv.handleStatus(ctx, newReq("memorx_status", nil))
	if err != nil {
		t.Fatalf("handleStatus error: %v", err)
	}
	text := resultText(t, res)
	if !strings.Contains(text, "feat-active") {
		t.Errorf("status should contain active feature name, got:\n%s", text)
	}
}

func TestHandleRemember_EachNoteType(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	_, err := srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name": "note-type-test",
	}))
	if err != nil {
		t.Fatalf("handleStartFeature: %v", err)
	}

	noteTypes := []string{"progress", "decision", "blocker", "next_step", "note"}
	for _, nt := range noteTypes {
		t.Run(nt, func(t *testing.T) {
			res, err := srv.handleRemember(ctx, newReq("memorx_remember", map[string]interface{}{
				"content": "Test content for " + nt,
				"type":    nt,
			}))
			if err != nil {
				t.Fatalf("handleRemember(%s) error: %v", nt, err)
			}
			text := resultText(t, res)
			if !strings.Contains(text, "Remembered") {
				t.Errorf("expected 'Remembered' in result for type %s, got:\n%s", nt, text)
			}
		})
	}
}

func TestHandleImportSession_EmptyFeatureName(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	res, err := srv.handleImportSession(ctx, newReq("memorx_import_session", map[string]interface{}{
		"feature_name": "",
		"description":  "should fail",
	}))
	if err != nil {
		t.Fatalf("handleImportSession error: %v", err)
	}

	// Empty feature_name should return an error result.
	if res.IsError {
		// Good — it's an explicit MCP error.
		return
	}
	text := resultText(t, res)
	if !strings.Contains(text, "required") {
		t.Errorf("empty feature_name should return error about required param, got:\n%s", text)
	}
}

func TestHandleSwitchFeature_CreatesNewSession(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	// Create two features
	_, err := srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name": "switch-feat-a",
	}))
	if err != nil {
		t.Fatalf("handleStartFeature a error: %v", err)
	}

	_, err = srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name": "switch-feat-b",
	}))
	if err != nil {
		t.Fatalf("handleStartFeature b error: %v", err)
	}

	// Switch back to feat-a
	res, err := srv.handleSwitchFeature(ctx, newReq("memorx_switch_feature", map[string]interface{}{
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
	statusRes, err := srv.handleStatus(ctx, newReq("memorx_status", nil))
	if err != nil {
		t.Fatalf("handleStatus error: %v", err)
	}
	statusText := resultText(t, statusRes)
	if !strings.Contains(statusText, "switch-feat-a") {
		t.Errorf("status after switch should show switch-feat-a as active, got:\n%s", statusText)
	}
}

func TestHandleEndSession(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	// Start a feature first.
	_, err := srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name": "end-session-test",
	}))
	if err != nil {
		t.Fatalf("handleStartFeature error: %v", err)
	}

	// End session with a summary.
	res, err := srv.handleEndSession(ctx, newReq("memorx_end_session", map[string]interface{}{
		"summary": "Implemented user auth flow and wrote integration tests",
	}))
	if err != nil {
		t.Fatalf("handleEndSession error: %v", err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "Session ended") {
		t.Errorf("end session result should contain 'Session ended', got:\n%s", text)
	}
	if !strings.Contains(text, "Summary saved") {
		t.Errorf("end session result should contain 'Summary saved', got:\n%s", text)
	}
	if !strings.Contains(text, "Progress note created") {
		t.Errorf("end session result should contain 'Progress note created', got:\n%s", text)
	}
	if !strings.Contains(text, "next session will see this summary") {
		t.Errorf("end session result should mention next session, got:\n%s", text)
	}
}

func TestHandleEndSession_NoActiveSession(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	// Don't start a feature — no session exists.
	res, err := srv.handleEndSession(ctx, newReq("memorx_end_session", map[string]interface{}{
		"summary": "nothing happened",
	}))
	if err != nil {
		t.Fatalf("handleEndSession error: %v", err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "No active session") {
		t.Errorf("end session without active session should error, got:\n%s", text)
	}
}

func TestHandleEndSession_MissingSummary(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	_, _ = srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name": "end-session-missing-summary",
	}))

	res, err := srv.handleEndSession(ctx, newReq("memorx_end_session", map[string]interface{}{}))
	if err != nil {
		t.Fatalf("handleEndSession error: %v", err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "required") {
		t.Errorf("end session without summary should error about required param, got:\n%s", text)
	}
}

func TestHandleForget_StaleFacts(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()
	_, _ = srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{"name": "forget-stale"}))

	res, err := srv.handleForget(ctx, newReq("memorx_forget", map[string]interface{}{"what": "stale_facts"}))
	if err != nil {
		t.Fatalf("handleForget: %v", err)
	}
	text := resultText(t, res)
	if !strings.Contains(text, "stale facts") {
		t.Errorf("expected stale facts message, got: %s", text)
	}
}

func TestHandleForget_StaleNotes(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()
	_, _ = srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{"name": "forget-notes"}))

	res, err := srv.handleForget(ctx, newReq("memorx_forget", map[string]interface{}{"what": "stale_notes"}))
	if err != nil {
		t.Fatalf("handleForget: %v", err)
	}
	text := resultText(t, res)
	if !strings.Contains(text, "stale notes") {
		t.Errorf("expected stale notes message, got: %s", text)
	}
}

func TestHandleForget_CompletedFeatures(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	res, err := srv.handleForget(ctx, newReq("memorx_forget", map[string]interface{}{"what": "completed_features"}))
	if err != nil {
		t.Fatalf("handleForget: %v", err)
	}
	text := resultText(t, res)
	if !strings.Contains(text, "completed features") {
		t.Errorf("expected completed features message, got: %s", text)
	}
}

func TestHandleGenerateRules_DryRun(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	res, err := srv.handleGenerateRules(ctx, newReq("memorx_generate_rules", map[string]interface{}{"dry_run": true}))
	if err != nil {
		t.Fatalf("handleGenerateRules: %v", err)
	}
	text := resultText(t, res)
	if !strings.Contains(text, "Preview") {
		t.Errorf("dry_run should contain 'Preview', got: %s", text)
	}
	if !strings.Contains(text, "AGENTS.md") {
		t.Errorf("dry_run should contain AGENTS.md content, got: %s", text)
	}
}

func TestHandleAnalytics_NoFeatures(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	res, err := srv.handleAnalytics(ctx, newReq("memorx_analytics", map[string]interface{}{}))
	if err != nil {
		t.Fatalf("handleAnalytics: %v", err)
	}
	text := resultText(t, res)
	// Without specifying a feature, it should show project analytics
	if !strings.Contains(text, "Project") || !strings.Contains(text, "0") {
		t.Errorf("expected project analytics with 0 counts, got: %s", text)
	}
}

func TestHandleEndSession_SummaryAppearsInContext(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	// Start a feature and end the session with a summary.
	_, err := srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name": "summary-in-context-test",
	}))
	if err != nil {
		t.Fatalf("handleStartFeature error: %v", err)
	}

	_, err = srv.handleEndSession(ctx, newReq("memorx_end_session", map[string]interface{}{
		"summary": "Built the database schema and migration system",
	}))
	if err != nil {
		t.Fatalf("handleEndSession error: %v", err)
	}

	// Start a new session (re-start the same feature).
	_, err = srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name": "summary-in-context-test",
	}))
	if err != nil {
		t.Fatalf("handleStartFeature resume error: %v", err)
	}

	// Get context — should include the previous session's summary.
	res, err := srv.handleGetContext(ctx, newReq("memorx_get_context", map[string]interface{}{
		"tier": "compact",
	}))
	if err != nil {
		t.Fatalf("handleGetContext error: %v", err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "LastSession:") {
		t.Errorf("context should contain 'LastSession:' section, got:\n%s", text)
	}
	if !strings.Contains(text, "database schema and migration") {
		t.Errorf("context should contain the session summary text, got:\n%s", text)
	}
}

func TestHandlerErrors_RequiresActiveFeature(t *testing.T) {
	handlers := []struct {
		name string
		req  map[string]interface{}
		fn   func(*DevMemServer, context.Context, map[string]interface{}) (*mcplib.CallToolResult, error)
	}{
		{"sync", nil, func(s *DevMemServer, ctx context.Context, _ map[string]interface{}) (*mcplib.CallToolResult, error) {
			return s.handleSync(ctx, newReq("memorx_sync", nil))
		}},
		{"remember", map[string]interface{}{"content": "x"}, func(s *DevMemServer, ctx context.Context, args map[string]interface{}) (*mcplib.CallToolResult, error) {
			return s.handleRemember(ctx, newReq("memorx_remember", args))
		}},
		{"search", map[string]interface{}{"query": "test"}, func(s *DevMemServer, ctx context.Context, args map[string]interface{}) (*mcplib.CallToolResult, error) {
			return s.handleSearch(ctx, newReq("memorx_search", args))
		}},
		{"save_plan", map[string]interface{}{"title": "t", "steps": []interface{}{map[string]interface{}{"title": "s1"}}}, func(s *DevMemServer, ctx context.Context, args map[string]interface{}) (*mcplib.CallToolResult, error) {
			return s.handleSavePlan(ctx, newReq("memorx_save_plan", args))
		}},
	}
	for _, tc := range handlers {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := setupTestServer(t)
			res, err := tc.fn(srv, context.Background(), tc.req)
			if err != nil {
				t.Fatalf("handler error: %v", err)
			}
			text := resultText(t, res)
			if !strings.Contains(strings.ToLower(text), "no active feature") && !strings.Contains(strings.ToLower(text), "feature") {
				t.Errorf("expected error about no active feature, got: %s", text)
			}
		})
	}
}

func TestHandleDiff_WithNewData(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	// Start a feature.
	_, err := srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name":        "diff-data-test",
		"description": "testing diff with data",
	}))
	if err != nil {
		t.Fatalf("handleStartFeature error: %v", err)
	}

	// Add some notes and facts via remember + import_session.
	_, err = srv.handleRemember(ctx, newReq("memorx_remember", map[string]interface{}{
		"content": "decided to use GraphQL",
		"type":    "decision",
	}))
	if err != nil {
		t.Fatalf("handleRemember error: %v", err)
	}
	_, err = srv.handleRemember(ctx, newReq("memorx_remember", map[string]interface{}{
		"content": "set up project structure",
		"type":    "progress",
	}))
	if err != nil {
		t.Fatalf("handleRemember error: %v", err)
	}

	// Call diff with a past time.
	res, err := srv.handleDiff(ctx, newReq("memorx_diff", map[string]interface{}{
		"since": "2020-01-01",
	}))
	if err != nil {
		t.Fatalf("handleDiff error: %v", err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "Since last session") {
		t.Errorf("diff should start with 'Since last session', got:\n%s", text)
	}
	if !strings.Contains(text, "+0 facts") && !strings.Contains(text, "facts") {
		t.Errorf("diff should mention facts, got:\n%s", text)
	}
	if !strings.Contains(text, "notes") {
		t.Errorf("diff should mention notes, got:\n%s", text)
	}
	if !strings.Contains(text, "commits") {
		t.Errorf("diff should mention commits, got:\n%s", text)
	}
	if !strings.Contains(text, "plan:") {
		t.Errorf("diff should mention plan, got:\n%s", text)
	}
	if !strings.Contains(text, "links") {
		t.Errorf("diff should mention links, got:\n%s", text)
	}
	if !strings.Contains(text, "files") {
		t.Errorf("diff should mention files, got:\n%s", text)
	}
}

func TestHandleDiff_NoChanges(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	// Start a feature.
	_, err := srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name": "diff-nochange-test",
	}))
	if err != nil {
		t.Fatalf("handleStartFeature error: %v", err)
	}

	// Diff from the future — nothing should show.
	res, err := srv.handleDiff(ctx, newReq("memorx_diff", map[string]interface{}{
		"since": "2099-01-01",
	}))
	if err != nil {
		t.Fatalf("handleDiff error: %v", err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "+0 facts") {
		t.Errorf("diff with no changes should show +0 facts, got:\n%s", text)
	}
	if !strings.Contains(text, "+0 notes") {
		t.Errorf("diff with no changes should show +0 notes, got:\n%s", text)
	}
	if !strings.Contains(text, "+0 commits") {
		t.Errorf("diff with no changes should show +0 commits, got:\n%s", text)
	}
}

func TestHandleDiff_DefaultSince(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	// Start a feature (this creates a session).
	_, err := srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name": "diff-default-test",
	}))
	if err != nil {
		t.Fatalf("handleStartFeature error: %v", err)
	}

	// Call diff with no since parameter — should default to last session or creation time.
	res, err := srv.handleDiff(ctx, newReq("memorx_diff", nil))
	if err != nil {
		t.Fatalf("handleDiff error: %v", err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "Since last session") {
		t.Errorf("diff with default since should show 'Since last session', got:\n%s", text)
	}
}

func TestHandleDiff_NoActiveFeature(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	res, err := srv.handleDiff(ctx, newReq("memorx_diff", nil))
	if err != nil {
		t.Fatalf("handleDiff error: %v", err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "No active feature") {
		t.Errorf("diff without active feature should error, got:\n%s", text)
	}
}
