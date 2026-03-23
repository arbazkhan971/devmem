package mcp

import (
	"context"
	"strings"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// These tests enforce token efficiency by setting maximum character limits
// on tool responses. If someone makes responses verbose again, these tests
// fail and act as guardrails against verbosity regression.

func TestResponseCompactness_Status(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	// Create a feature with some data
	_, err := srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name":        "test-compact",
		"description": "compact test feature",
	}))
	if err != nil {
		t.Fatalf("handleStartFeature error: %v", err)
	}

	res, err := srv.handleStatus(ctx, newReq("memorx_status", nil))
	if err != nil {
		t.Fatalf("handleStatus error: %v", err)
	}
	text := resultText(t, res)

	if len(text) > 500 {
		t.Errorf("status response too verbose: %d chars (max 500)\n%s", len(text), text)
	}
}

func TestResponseCompactness_Briefing(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	_, err := srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name":        "briefing-compact",
		"description": "testing briefing compactness",
	}))
	if err != nil {
		t.Fatalf("handleStartFeature error: %v", err)
	}

	_, err = srv.handleRemember(ctx, newReq("memorx_remember", map[string]interface{}{
		"content": "Implemented the login flow with OAuth2",
		"type":    "note",
	}))
	if err != nil {
		t.Fatalf("handleRemember error: %v", err)
	}

	res, err := srv.handleBriefing(ctx, newReq("memorx_briefing", nil))
	if err != nil {
		t.Fatalf("handleBriefing error: %v", err)
	}
	text := resultText(t, res)

	if len(text) > 300 {
		t.Errorf("briefing response too verbose: %d chars (max 300)\n%s", len(text), text)
	}
}

func TestResponseCompactness_ListFeatures(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	// Create a few features
	for _, name := range []string{"feat-alpha", "feat-beta", "feat-gamma"} {
		_, err := srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
			"name": name,
		}))
		if err != nil {
			t.Fatalf("handleStartFeature error for %s: %v", name, err)
		}
	}

	res, err := srv.handleListFeatures(ctx, newReq("memorx_list_features", map[string]interface{}{
		"status_filter": "all",
	}))
	if err != nil {
		t.Fatalf("handleListFeatures error: %v", err)
	}
	text := resultText(t, res)

	featureCount := 3
	maxPerFeature := 200
	maxTotal := featureCount * maxPerFeature
	if len(text) > maxTotal {
		t.Errorf("list_features response too verbose: %d chars (max %d for %d features, %d per feature)\n%s",
			len(text), maxTotal, featureCount, maxPerFeature, text)
	}
}

func TestResponseCompactness_GetContext_Compact(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	_, err := srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name":        "ctx-compact-test",
		"description": "testing compact context",
	}))
	if err != nil {
		t.Fatalf("handleStartFeature error: %v", err)
	}

	_, err = srv.handleRemember(ctx, newReq("memorx_remember", map[string]interface{}{
		"content": "Using REST API with JSON",
		"type":    "decision",
	}))
	if err != nil {
		t.Fatalf("handleRemember error: %v", err)
	}

	res, err := srv.handleGetContext(ctx, newReq("memorx_get_context", map[string]interface{}{
		"tier": "compact",
	}))
	if err != nil {
		t.Fatalf("handleGetContext compact error: %v", err)
	}
	text := resultText(t, res)

	if len(text) > 500 {
		t.Errorf("get_context compact response too verbose: %d chars (max 500)\n%s", len(text), text)
	}
}

func TestResponseCompactness_GetContext_Standard(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	_, err := srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name":        "ctx-standard-test",
		"description": "testing standard context",
	}))
	if err != nil {
		t.Fatalf("handleStartFeature error: %v", err)
	}

	// Add some data to make context richer
	for _, note := range []string{
		"Database uses PostgreSQL",
		"Auth via JWT tokens",
		"Rate limiting at 100 req/min",
	} {
		_, err = srv.handleRemember(ctx, newReq("memorx_remember", map[string]interface{}{
			"content": note,
			"type":    "decision",
		}))
		if err != nil {
			t.Fatalf("handleRemember error: %v", err)
		}
	}

	res, err := srv.handleGetContext(ctx, newReq("memorx_get_context", map[string]interface{}{
		"tier": "standard",
	}))
	if err != nil {
		t.Fatalf("handleGetContext standard error: %v", err)
	}
	text := resultText(t, res)

	if len(text) > 1500 {
		t.Errorf("get_context standard response too verbose: %d chars (max 1500)\n%s", len(text), text)
	}
}

func TestResponseCompactness_Search(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	_, err := srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name": "search-compact-test",
	}))
	if err != nil {
		t.Fatalf("handleStartFeature error: %v", err)
	}

	// Add several notes to search through
	notes := []string{
		"Authentication uses JWT tokens for session management",
		"The auth middleware validates tokens on every request",
		"Token refresh is handled by the auth service",
	}
	for _, note := range notes {
		_, err = srv.handleRemember(ctx, newReq("memorx_remember", map[string]interface{}{
			"content": note,
			"type":    "note",
		}))
		if err != nil {
			t.Fatalf("handleRemember error: %v", err)
		}
	}

	res, err := srv.handleSearch(ctx, newReq("memorx_search", map[string]interface{}{
		"query": "auth token",
	}))
	if err != nil {
		t.Fatalf("handleSearch error: %v", err)
	}
	text := resultText(t, res)

	// Count actual results (lines starting with '[')
	resultCount := 0
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "[") {
			resultCount++
		}
	}

	if resultCount == 0 {
		t.Fatalf("search returned no results, cannot validate compactness")
	}

	maxPerResult := 150
	maxTotal := resultCount*maxPerResult + 100 // +100 for header line
	if len(text) > maxTotal {
		t.Errorf("search response too verbose: %d chars (max %d for %d results, %d per result)\n%s",
			len(text), maxTotal, resultCount, maxPerResult, text)
	}
}

func TestResponseCompactness_Remember(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	_, err := srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name": "remember-compact-test",
	}))
	if err != nil {
		t.Fatalf("handleStartFeature error: %v", err)
	}

	res, err := srv.handleRemember(ctx, newReq("memorx_remember", map[string]interface{}{
		"content": "The database connection pool is set to 25 max connections",
		"type":    "decision",
	}))
	if err != nil {
		t.Fatalf("handleRemember error: %v", err)
	}
	text := resultText(t, res)

	if len(text) > 200 {
		t.Errorf("remember response too verbose: %d chars (max 200)\n%s", len(text), text)
	}
}

func TestResponseCompactness_Status_NoFeature(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	// No feature started — status should still be compact
	res, err := srv.handleStatus(ctx, newReq("memorx_status", nil))
	if err != nil {
		t.Fatalf("handleStatus error: %v", err)
	}
	text := resultText(t, res)

	if len(text) > 200 {
		t.Errorf("status (no feature) response too verbose: %d chars (max 200)\n%s", len(text), text)
	}
}

func TestResponseCompactness_Health(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	// Create a feature so there's something to check health on
	_, err := srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name": "health-compact-test",
	}))
	if err != nil {
		t.Fatalf("handleStartFeature error: %v", err)
	}

	_, err = srv.handleRemember(ctx, newReq("memorx_remember", map[string]interface{}{
		"content": "Health check test note",
		"type":    "note",
	}))
	if err != nil {
		t.Fatalf("handleRemember error: %v", err)
	}

	res, err := srv.handleHealth(ctx, newReq("memorx_health", nil))
	if err != nil {
		t.Fatalf("handleHealth error: %v", err)
	}
	text := resultText(t, res)

	if len(text) > 500 {
		t.Errorf("health response too verbose: %d chars (max 500)\n%s", len(text), text)
	}
}

func TestResponseCompactness_Analytics(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	// Create a feature with some data
	_, err := srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name":        "analytics-compact-test",
		"description": "testing analytics compactness",
	}))
	if err != nil {
		t.Fatalf("handleStartFeature error: %v", err)
	}

	_, err = srv.handleRemember(ctx, newReq("memorx_remember", map[string]interface{}{
		"content": "Analytics compactness note",
		"type":    "note",
	}))
	if err != nil {
		t.Fatalf("handleRemember error: %v", err)
	}

	// Feature-specific analytics
	res, err := srv.handleAnalytics(ctx, newReq("memorx_analytics", map[string]interface{}{
		"feature": "analytics-compact-test",
	}))
	if err != nil {
		t.Fatalf("handleAnalytics error: %v", err)
	}
	text := resultText(t, res)

	if len(text) > 800 {
		t.Errorf("analytics (feature) response too verbose: %d chars (max 800)\n%s", len(text), text)
	}
}

func TestResponseCompactness_PopulatedData(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()
	_, err := srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name": "populated-compact", "description": "populated compactness test",
	}))
	if err != nil {
		t.Fatalf("handleStartFeature: %v", err)
	}
	types := []string{"note", "decision", "blocker", "progress"}
	for i, tp := range types {
		_, err = srv.handleRemember(ctx, newReq("memorx_remember", map[string]interface{}{
			"content": "Item " + tp + " content for populated test",
			"type":    tp,
		}))
		if err != nil {
			t.Fatalf("handleRemember[%d]: %v", i, err)
		}
	}
	tests := []struct {
		name string
		fn   func() (*mcplib.CallToolResult, error)
		max  int
	}{
		{"status", func() (*mcplib.CallToolResult, error) { return srv.handleStatus(ctx, newReq("memorx_status", nil)) }, 600},
		{"briefing", func() (*mcplib.CallToolResult, error) { return srv.handleBriefing(ctx, newReq("memorx_briefing", nil)) }, 400},
		{"context_compact", func() (*mcplib.CallToolResult, error) {
			return srv.handleGetContext(ctx, newReq("memorx_get_context", map[string]interface{}{"tier": "compact"}))
		}, 600},
		{"health", func() (*mcplib.CallToolResult, error) { return srv.handleHealth(ctx, newReq("memorx_health", nil)) }, 600},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := tc.fn()
			if err != nil {
				t.Fatalf("handler error: %v", err)
			}
			text := resultText(t, res)
			if len(text) > tc.max {
				t.Errorf("%s too verbose with populated data: %d chars (max %d)", tc.name, len(text), tc.max)
			}
		})
	}
}

func TestResponseCompactness_EndSession(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	_, err := srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name": "endsession-compact-test",
	}))
	if err != nil {
		t.Fatalf("handleStartFeature error: %v", err)
	}

	res, err := srv.handleEndSession(ctx, newReq("memorx_end_session", map[string]interface{}{
		"summary": "Completed the initial setup and wrote unit tests",
	}))
	if err != nil {
		t.Fatalf("handleEndSession error: %v", err)
	}
	text := resultText(t, res)

	if len(text) > 300 {
		t.Errorf("end_session response too verbose: %d chars (max 300)\n%s", len(text), text)
	}
}
