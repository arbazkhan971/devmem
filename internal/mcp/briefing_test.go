package mcp

import (
	"context"
	"strings"
	"testing"

	"github.com/arbaz/devmem/internal/memory"
)

func TestFormatBriefing_ActiveFeatureWithPlan(t *testing.T) {
	feature := &memory.Feature{
		ID:     "feat-123",
		Name:   "auth-v2",
		Branch: "feature/auth-v2",
		Status: "active",
	}
	ctx := &memory.Context{
		Feature: feature,
		Plan: &memory.PlanInfo{
			Title:         "Auth Migration",
			Status:        "active",
			TotalSteps:    7,
			CompletedStep: 4,
		},
		SessionHistory: []memory.Session{
			{ID: "sess-12345678", FeatureID: "feat-123", Tool: "claude-code", StartedAt: "2025-01-15 10:00:00"},
		},
		RecentNotes: []memory.Note{
			{Content: "Token refresh working, need to test expiry edge cases"},
		},
	}

	briefing := formatBriefing(ctx, feature)

	checks := []struct {
		desc, want string
	}{
		{"feature name", "auth-v2"},
		{"branch in brackets", "[feature/auth-v2]"},
		{"plan title", "Auth Migration"},
		{"plan progress", "4/7"},
		{"recent note", "Token refresh working"},
	}
	for _, c := range checks {
		if !strings.Contains(briefing, c.want) {
			t.Errorf("briefing should contain %s (%q), got:\n%s", c.desc, c.want, briefing)
		}
	}
}

func TestFormatBriefing_ActiveFeatureNoPlan(t *testing.T) {
	feature := &memory.Feature{
		ID:     "feat-456",
		Name:   "bugfix-login",
		Branch: "fix/login",
		Status: "active",
	}
	ctx := &memory.Context{
		Feature: feature,
		Plan:    nil, // no plan
		RecentNotes: []memory.Note{
			{Content: "Found the root cause in session middleware"},
		},
	}

	briefing := formatBriefing(ctx, feature)

	if !strings.Contains(briefing, "bugfix-login") {
		t.Errorf("briefing should contain feature name, got:\n%s", briefing)
	}
	if strings.Contains(briefing, "Plan:") {
		t.Errorf("briefing should NOT contain plan line when no plan exists, got:\n%s", briefing)
	}
	if !strings.Contains(briefing, "Found the root cause") {
		t.Errorf("briefing should contain recent note, got:\n%s", briefing)
	}
}

func TestFormatBriefing_NoActiveFeature(t *testing.T) {
	briefing := formatBriefing(&memory.Context{}, nil)

	if !strings.Contains(briefing, "No active feature") {
		t.Errorf("briefing with nil feature should say 'No active feature', got:\n%s", briefing)
	}
}

func TestFormatBriefing_IncludesLastSession(t *testing.T) {
	feature := &memory.Feature{
		ID:     "feat-789",
		Name:   "api-redesign",
		Status: "active",
	}
	ctx := &memory.Context{
		Feature: feature,
		SessionHistory: []memory.Session{
			{ID: "sess-abcdefgh", FeatureID: "feat-789", Tool: "cursor", StartedAt: "2025-06-10 14:30:00"},
		},
	}

	briefing := formatBriefing(ctx, feature)

	if !strings.Contains(briefing, "last:") {
		t.Errorf("briefing should include last session info, got:\n%s", briefing)
	}
	if !strings.Contains(briefing, "ago") {
		t.Errorf("briefing should include relative time, got:\n%s", briefing)
	}
}

func TestHandleBriefing_NoActiveFeature(t *testing.T) {
	srv, _ := setupTestServer(t)

	res, err := srv.handleBriefing(context.Background(), newReq("devmem_briefing", nil))
	if err != nil {
		t.Fatalf("handleBriefing error: %v", err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "No active feature") {
		t.Errorf("briefing without active feature should say so, got:\n%s", text)
	}
}

func TestHandleBriefing_WithActiveFeature(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	// Start a feature and add some data
	_, err := srv.handleStartFeature(ctx, newReq("devmem_start_feature", map[string]interface{}{
		"name":        "briefing-test",
		"description": "testing the briefing tool",
	}))
	if err != nil {
		t.Fatalf("handleStartFeature error: %v", err)
	}

	_, err = srv.handleRemember(ctx, newReq("devmem_remember", map[string]interface{}{
		"content": "Set up the database schema with migrations",
		"type":    "note",
	}))
	if err != nil {
		t.Fatalf("handleRemember error: %v", err)
	}

	res, err := srv.handleBriefing(ctx, newReq("devmem_briefing", nil))
	if err != nil {
		t.Fatalf("handleBriefing error: %v", err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "devmem:") {
		t.Errorf("briefing should start with 'devmem:', got:\n%s", text)
	}
	if !strings.Contains(text, "briefing-test") {
		t.Errorf("briefing should contain feature name, got:\n%s", text)
	}
	if !strings.Contains(text, "database schema") {
		t.Errorf("briefing should contain recent note content, got:\n%s", text)
	}
}

func TestFormatTimeAgo(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains string
	}{
		{"invalid format", "not-a-date", "not-a-date"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatTimeAgo(tt.input)
			if !strings.Contains(result, tt.contains) {
				t.Errorf("formatTimeAgo(%q) = %q, want to contain %q", tt.input, result, tt.contains)
			}
		})
	}
}
