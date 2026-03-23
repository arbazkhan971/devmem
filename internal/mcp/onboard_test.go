package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandleOnboard_NoData(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	res, err := srv.handleOnboard(ctx, newReq("memorx_onboard", nil))
	if err != nil {
		t.Fatalf("handleOnboard error: %v", err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "# Project Onboarding:") {
		t.Errorf("expected onboarding header, got:\n%s", text)
	}
	if !strings.Contains(text, "## Key Decisions") {
		t.Errorf("expected Key Decisions section, got:\n%s", text)
	}
}

func TestHandleOnboard_WithFeature(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	_, err := srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name":        "onboard-test",
		"description": "Testing onboarding",
	}))
	if err != nil {
		t.Fatalf("handleStartFeature: %v", err)
	}

	_, err = srv.handleRemember(ctx, newReq("memorx_remember", map[string]interface{}{
		"content": "Use PostgreSQL for persistence",
		"type":    "decision",
	}))
	if err != nil {
		t.Fatalf("handleRemember: %v", err)
	}

	res, err := srv.handleOnboard(ctx, newReq("memorx_onboard", nil))
	if err != nil {
		t.Fatalf("handleOnboard: %v", err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "Use PostgreSQL for persistence") {
		t.Errorf("expected decision in onboarding doc, got:\n%s", text)
	}
	if !strings.Contains(text, "onboard-test") {
		t.Errorf("expected feature name in current work, got:\n%s", text)
	}
}

func TestHandleOnboard_Scoped(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	_, _ = srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name": "feat-a",
	}))
	srv.handleRemember(ctx, newReq("memorx_remember", map[string]interface{}{
		"content": "Decision for A",
		"type":    "decision",
	}))
	_, _ = srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name": "feat-b",
	}))
	srv.handleRemember(ctx, newReq("memorx_remember", map[string]interface{}{
		"content": "Decision for B",
		"type":    "decision",
	}))

	res, err := srv.handleOnboard(ctx, newReq("memorx_onboard", map[string]interface{}{
		"feature": "feat-a",
	}))
	if err != nil {
		t.Fatalf("handleOnboard scoped: %v", err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "Decision for A") {
		t.Errorf("expected scoped decision, got:\n%s", text)
	}
	if strings.Contains(text, "Decision for B") {
		t.Errorf("should not include other feature's decisions, got:\n%s", text)
	}
}

func TestHandleChangelog_Empty(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	res, err := srv.handleChangelog(ctx, newReq("memorx_changelog", nil))
	if err != nil {
		t.Fatalf("handleChangelog error: %v", err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "Changelog") {
		t.Errorf("expected Changelog header, got:\n%s", text)
	}
}

func TestHandleChangelog_WithDecisions(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	_, _ = srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name": "changelog-test",
	}))
	srv.handleRemember(ctx, newReq("memorx_remember", map[string]interface{}{
		"content": "Use Redis for caching",
		"type":    "decision",
	}))

	res, err := srv.handleChangelog(ctx, newReq("memorx_changelog", map[string]interface{}{
		"days": float64(30),
	}))
	if err != nil {
		t.Fatalf("handleChangelog: %v", err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "changelog-test") {
		t.Errorf("expected feature name in changelog, got:\n%s", text)
	}
}

func TestHandleChangelog_SlackFormat(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	_, _ = srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name": "slack-test",
	}))
	srv.handleRemember(ctx, newReq("memorx_remember", map[string]interface{}{
		"content": "A slack decision",
		"type":    "decision",
	}))

	res, err := srv.handleChangelog(ctx, newReq("memorx_changelog", map[string]interface{}{
		"format": "slack",
	}))
	if err != nil {
		t.Fatalf("handleChangelog slack: %v", err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "*Changelog") {
		t.Errorf("expected slack-formatted Changelog header, got:\n%s", text)
	}
}

func TestHandleShare_JSONFile(t *testing.T) {
	srv, dir := setupTestServer(t)
	ctx := context.Background()

	// Write a JSON export file
	exportData := map[string]interface{}{
		"feature":     "shared-feature",
		"description": "Imported from teammate",
		"decisions":   []string{"Use gRPC internally"},
		"facts": []map[string]string{
			{"subject": "api", "predicate": "protocol", "object": "gRPC"},
		},
	}
	data, _ := json.Marshal(exportData)
	exportPath := filepath.Join(dir, "export.json")
	if err := os.WriteFile(exportPath, data, 0644); err != nil {
		t.Fatalf("write export file: %v", err)
	}

	res, err := srv.handleShare(ctx, newReq("memorx_share", map[string]interface{}{
		"path": exportPath,
	}))
	if err != nil {
		t.Fatalf("handleShare error: %v", err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "Import complete") {
		t.Errorf("expected import complete message, got:\n%s", text)
	}
	if !strings.Contains(text, "Features: 1") {
		t.Errorf("expected 1 feature imported, got:\n%s", text)
	}
}

func TestHandleShare_MarkdownFile(t *testing.T) {
	srv, dir := setupTestServer(t)
	ctx := context.Background()

	md := `# Feature: md-import-test

**Description:** A markdown exported feature

## Decisions

- Use JWT for authentication
- Use PostgreSQL for persistence

## Blockers

- Waiting on API keys

## Facts (Current)

- auth **uses** JWT
`
	exportPath := filepath.Join(dir, "export.md")
	if err := os.WriteFile(exportPath, []byte(md), 0644); err != nil {
		t.Fatalf("write export file: %v", err)
	}

	res, err := srv.handleShare(ctx, newReq("memorx_share", map[string]interface{}{
		"path": exportPath,
	}))
	if err != nil {
		t.Fatalf("handleShare error: %v", err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "Import complete") {
		t.Errorf("expected import complete message, got:\n%s", text)
	}
}

func TestHandleShare_MissingFile(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	res, err := srv.handleShare(ctx, newReq("memorx_share", map[string]interface{}{
		"path": "/nonexistent/file.json",
	}))
	if err != nil {
		t.Fatalf("handleShare error: %v", err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "Failed to read file") {
		t.Errorf("expected file read error, got:\n%s", text)
	}
}

func TestHandleShare_MissingPath(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	res, err := srv.handleShare(ctx, newReq("memorx_share", nil))
	if err != nil {
		t.Fatalf("handleShare error: %v", err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "required") {
		t.Errorf("expected parameter required error, got:\n%s", text)
	}
}

func TestParseMarkdownExport(t *testing.T) {
	md := `# Feature: test-parse

## Decisions

- Use REST API
- Use PostgreSQL

## Blockers

- Need API credentials
`
	data := parseMarkdownExport(md)
	if data == nil {
		t.Fatal("expected non-nil data from parseMarkdownExport")
	}
	if data["feature"] != "test-parse" {
		t.Errorf("expected feature 'test-parse', got %v", data["feature"])
	}
	decisions, ok := data["decisions"].([]interface{})
	if !ok || len(decisions) != 2 {
		t.Errorf("expected 2 decisions, got %v", data["decisions"])
	}
	blockers, ok := data["blockers"].([]interface{})
	if !ok || len(blockers) != 1 {
		t.Errorf("expected 1 blocker, got %v", data["blockers"])
	}
}

func TestParseMarkdownExport_NoFeatureName(t *testing.T) {
	md := "Just some random text\nwithout headers\n"
	data := parseMarkdownExport(md)
	if data != nil {
		t.Error("expected nil for markdown without feature header")
	}
}
