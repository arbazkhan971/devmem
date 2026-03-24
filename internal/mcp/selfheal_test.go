package mcp

import (
	"context"
	"strings"
	"testing"
)

func TestHandleDeduplicate_Empty(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	res, err := srv.handleDeduplicate(ctx, newReq("memorx_deduplicate", nil))
	if err != nil {
		t.Fatalf("handleDeduplicate error: %v", err)
	}
	text := resultText(t, res)
	if !strings.Contains(text, "Deduplicate") {
		t.Errorf("expected Deduplicate header, got:\n%s", text)
	}
	if !strings.Contains(text, "No duplicates") {
		t.Errorf("expected no duplicates message, got:\n%s", text)
	}
}

func TestHandleDeduplicate_DryRun(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	_, _ = srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name": "dedup-test",
	}))
	// Create near-duplicate notes
	_, _ = srv.handleRemember(ctx, newReq("memorx_remember", map[string]interface{}{
		"content": "The authentication system uses JWT tokens for session management",
		"type":    "note",
	}))
	_, _ = srv.handleRemember(ctx, newReq("memorx_remember", map[string]interface{}{
		"content": "The authentication system uses JWT tokens for session management purposes",
		"type":    "note",
	}))

	res, err := srv.handleDeduplicate(ctx, newReq("memorx_deduplicate", map[string]interface{}{
		"dry_run": true,
	}))
	if err != nil {
		t.Fatalf("handleDeduplicate error: %v", err)
	}
	text := resultText(t, res)
	if !strings.Contains(text, "DRY RUN") {
		t.Errorf("expected DRY RUN mode, got:\n%s", text)
	}
}

func TestHandleDeduplicate_WithFeature(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	_, _ = srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name": "dedup-scoped",
	}))
	_, _ = srv.handleRemember(ctx, newReq("memorx_remember", map[string]interface{}{
		"content": "Note alpha",
		"type":    "note",
	}))

	res, err := srv.handleDeduplicate(ctx, newReq("memorx_deduplicate", map[string]interface{}{
		"feature": "dedup-scoped",
	}))
	if err != nil {
		t.Fatalf("handleDeduplicate error: %v", err)
	}
	text := resultText(t, res)
	if !strings.Contains(text, "Deduplicate") {
		t.Errorf("expected Deduplicate header, got:\n%s", text)
	}
}

func TestHandleIntegrityCheck_Clean(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	res, err := srv.handleIntegrityCheck(ctx, newReq("memorx_integrity_check", nil))
	if err != nil {
		t.Fatalf("handleIntegrityCheck error: %v", err)
	}
	text := resultText(t, res)
	if !strings.Contains(text, "Integrity Check") {
		t.Errorf("expected Integrity Check header, got:\n%s", text)
	}
	if !strings.Contains(text, "Integrity:") {
		t.Errorf("expected Integrity score, got:\n%s", text)
	}
}

func TestHandleIntegrityCheck_WithData(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	_, _ = srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name": "integrity-test",
	}))
	_, _ = srv.handleRemember(ctx, newReq("memorx_remember", map[string]interface{}{
		"content": "Test note for integrity",
		"type":    "note",
	}))

	res, err := srv.handleIntegrityCheck(ctx, newReq("memorx_integrity_check", map[string]interface{}{
		"fix": false,
	}))
	if err != nil {
		t.Fatalf("handleIntegrityCheck error: %v", err)
	}
	text := resultText(t, res)
	if !strings.Contains(text, "Integrity:") {
		t.Errorf("expected Integrity score, got:\n%s", text)
	}
	if !strings.Contains(text, "items checked") {
		t.Errorf("expected items checked count, got:\n%s", text)
	}
}

func TestHandleAutoLinkCode_Empty(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	res, err := srv.handleAutoLinkCode(ctx, newReq("memorx_auto_link_code", nil))
	if err != nil {
		t.Fatalf("handleAutoLinkCode error: %v", err)
	}
	text := resultText(t, res)
	if !strings.Contains(text, "Auto-Link Code") {
		t.Errorf("expected Auto-Link Code header, got:\n%s", text)
	}
}

func TestHandleAutoLinkCode_WithFilePaths(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	_, _ = srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name": "autolink-test",
	}))
	// Add notes that reference code files
	_, _ = srv.handleRemember(ctx, newReq("memorx_remember", map[string]interface{}{
		"content": "Modified internal/mcp/server.go to add new tool registration",
		"type":    "note",
	}))
	_, _ = srv.handleRemember(ctx, newReq("memorx_remember", map[string]interface{}{
		"content": "Updated cmd/main.go and internal/storage/db.go for migration support",
		"type":    "note",
	}))

	res, err := srv.handleAutoLinkCode(ctx, newReq("memorx_auto_link_code", map[string]interface{}{
		"feature": "autolink-test",
	}))
	if err != nil {
		t.Fatalf("handleAutoLinkCode error: %v", err)
	}
	text := resultText(t, res)
	if !strings.Contains(text, "Auto-link") {
		t.Errorf("expected Auto-link text, got:\n%s", text)
	}
}
