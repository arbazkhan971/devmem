package mcp

import (
	"context"
	"strings"
	"testing"
)

func TestHandleStandup_Empty(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	res, err := srv.handleStandup(ctx, newReq("memorx_standup", nil))
	if err != nil {
		t.Fatalf("handleStandup error: %v", err)
	}
	text := resultText(t, res)
	if !strings.Contains(text, "Daily Standup") {
		t.Errorf("expected Daily Standup header, got:\n%s", text)
	}
	if !strings.Contains(text, "Yesterday") {
		t.Errorf("expected Yesterday section, got:\n%s", text)
	}
	if !strings.Contains(text, "Today") {
		t.Errorf("expected Today section, got:\n%s", text)
	}
	if !strings.Contains(text, "Blockers") {
		t.Errorf("expected Blockers section, got:\n%s", text)
	}
}

func TestHandleStandup_WithBlockers(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	_, _ = srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name": "standup-test",
	}))
	_, _ = srv.handleRemember(ctx, newReq("memorx_remember", map[string]interface{}{
		"content": "Waiting on API key",
		"type":    "blocker",
	}))

	res, err := srv.handleStandup(ctx, newReq("memorx_standup", nil))
	if err != nil {
		t.Fatalf("handleStandup error: %v", err)
	}
	text := resultText(t, res)
	if !strings.Contains(text, "Waiting on API key") {
		t.Errorf("expected blocker content in standup, got:\n%s", text)
	}
}

func TestHandleBranchContext_Save(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	_, _ = srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name": "branch-feat",
	}))

	res, err := srv.handleBranchContext(ctx, newReq("memorx_branch_context", map[string]interface{}{
		"action": "save",
		"branch": "feature/auth-v2",
	}))
	if err != nil {
		t.Fatalf("handleBranchContext save error: %v", err)
	}
	text := resultText(t, res)
	if !strings.Contains(text, "Branch Context Saved") {
		t.Errorf("expected Branch Context Saved, got:\n%s", text)
	}
	if !strings.Contains(text, "feature/auth-v2") {
		t.Errorf("expected branch name, got:\n%s", text)
	}
	if !strings.Contains(text, "branch-feat") {
		t.Errorf("expected feature name, got:\n%s", text)
	}
}

func TestHandleBranchContext_SaveRestore(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	_, _ = srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name": "feat-for-branch",
	}))
	_, _ = srv.handleBranchContext(ctx, newReq("memorx_branch_context", map[string]interface{}{
		"action": "save",
		"branch": "feature/test-branch",
	}))

	// Switch to a different feature
	_, _ = srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name": "other-feature",
	}))

	// Restore
	res, err := srv.handleBranchContext(ctx, newReq("memorx_branch_context", map[string]interface{}{
		"action": "restore",
		"branch": "feature/test-branch",
	}))
	if err != nil {
		t.Fatalf("handleBranchContext restore error: %v", err)
	}
	text := resultText(t, res)
	if !strings.Contains(text, "Restored") {
		t.Errorf("expected Restored message, got:\n%s", text)
	}
	if !strings.Contains(text, "feat-for-branch") {
		t.Errorf("expected restored feature name, got:\n%s", text)
	}
}

func TestHandleBranchContext_List(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	// List with no mappings
	res, err := srv.handleBranchContext(ctx, newReq("memorx_branch_context", map[string]interface{}{
		"action": "list",
	}))
	if err != nil {
		t.Fatalf("handleBranchContext list error: %v", err)
	}
	text := resultText(t, res)
	if !strings.Contains(text, "Branch Context Mappings") {
		t.Errorf("expected Branch Context Mappings header, got:\n%s", text)
	}
	if !strings.Contains(text, "No branch mappings") {
		t.Errorf("expected no mappings message, got:\n%s", text)
	}
}

func TestHandleBranchContext_ListWithData(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	_, _ = srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name": "list-feat",
	}))
	_, _ = srv.handleBranchContext(ctx, newReq("memorx_branch_context", map[string]interface{}{
		"action": "save",
		"branch": "main",
	}))

	res, err := srv.handleBranchContext(ctx, newReq("memorx_branch_context", map[string]interface{}{
		"action": "list",
	}))
	if err != nil {
		t.Fatalf("handleBranchContext list error: %v", err)
	}
	text := resultText(t, res)
	if !strings.Contains(text, "main") {
		t.Errorf("expected branch 'main' in list, got:\n%s", text)
	}
	if !strings.Contains(text, "list-feat") {
		t.Errorf("expected feature name in list, got:\n%s", text)
	}
}

func TestHandleBranchContext_SaveNoBranch(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	res, err := srv.handleBranchContext(ctx, newReq("memorx_branch_context", map[string]interface{}{
		"action": "save",
	}))
	if err != nil {
		t.Fatalf("handleBranchContext save error: %v", err)
	}
	text := resultText(t, res)
	if !strings.Contains(text, "required") {
		t.Errorf("expected required error, got:\n%s", text)
	}
}

func TestHandleBranchContext_RestoreNotFound(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	res, err := srv.handleBranchContext(ctx, newReq("memorx_branch_context", map[string]interface{}{
		"action": "restore",
		"branch": "nonexistent-branch",
	}))
	if err != nil {
		t.Fatalf("handleBranchContext restore error: %v", err)
	}
	text := resultText(t, res)
	if !strings.Contains(text, "Failed") || !strings.Contains(text, "no context saved") {
		t.Errorf("expected error about no context saved, got:\n%s", text)
	}
}

func TestHandleBranchContext_InvalidAction(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	res, err := srv.handleBranchContext(ctx, newReq("memorx_branch_context", map[string]interface{}{
		"action": "invalid",
	}))
	if err != nil {
		t.Fatalf("handleBranchContext error: %v", err)
	}
	text := resultText(t, res)
	if !strings.Contains(text, "Invalid action") {
		t.Errorf("expected invalid action error, got:\n%s", text)
	}
}
