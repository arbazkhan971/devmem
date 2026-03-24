package memory

import (
	"strings"
	"testing"
)

func TestFormatStandup_Empty(t *testing.T) {
	d := &StandupData{}
	text := FormatStandup(d)
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
	if !strings.Contains(text, "no activity recorded") {
		t.Errorf("expected no activity message, got:\n%s", text)
	}
	if !strings.Contains(text, "None") {
		t.Errorf("expected None for blockers, got:\n%s", text)
	}
}

func TestFormatStandup_WithData(t *testing.T) {
	d := &StandupData{
		Yesterday: []StandupItem{
			{Feature: "auth", Content: "Implemented JWT validation"},
			{Feature: "auth", Content: "Added token refresh endpoint"},
		},
		Today: []StandupItem{
			{Feature: "auth", Content: "Write unit tests for token refresh"},
		},
		Blockers: []StandupItem{
			{Feature: "billing", Content: "Stripe API key not provisioned"},
		},
	}
	text := FormatStandup(d)
	if !strings.Contains(text, "JWT validation") {
		t.Errorf("expected yesterday content, got:\n%s", text)
	}
	if !strings.Contains(text, "unit tests") {
		t.Errorf("expected today content, got:\n%s", text)
	}
	if !strings.Contains(text, "Stripe API") {
		t.Errorf("expected blocker content, got:\n%s", text)
	}
}

func TestFormatBranchContext_Save(t *testing.T) {
	m := &BranchMapping{Branch: "feature/auth-v2", FeatureName: "auth-v2", SavedAt: "2026-03-25 10:00:00"}
	text := FormatBranchContext("save", m, nil)
	if !strings.Contains(text, "Saved") {
		t.Errorf("expected Saved, got:\n%s", text)
	}
	if !strings.Contains(text, "feature/auth-v2") {
		t.Errorf("expected branch name, got:\n%s", text)
	}
	if !strings.Contains(text, "auth-v2") {
		t.Errorf("expected feature name, got:\n%s", text)
	}
}

func TestFormatBranchContext_Restore(t *testing.T) {
	m := &BranchMapping{Branch: "feature/auth-v2", FeatureName: "auth-v2", SavedAt: "2026-03-25 10:00:00"}
	text := FormatBranchContext("restore", m, nil)
	if !strings.Contains(text, "Restored") {
		t.Errorf("expected Restored, got:\n%s", text)
	}
}

func TestFormatBranchContext_ListEmpty(t *testing.T) {
	text := FormatBranchContext("list", nil, nil)
	if !strings.Contains(text, "No branch mappings") {
		t.Errorf("expected no mappings message, got:\n%s", text)
	}
}

func TestFormatBranchContext_ListWithData(t *testing.T) {
	mappings := []BranchMapping{
		{Branch: "main", FeatureName: "core", SavedAt: "2026-03-25"},
		{Branch: "feature/auth", FeatureName: "auth", SavedAt: "2026-03-24"},
	}
	text := FormatBranchContext("list", nil, mappings)
	if !strings.Contains(text, "main") || !strings.Contains(text, "feature/auth") {
		t.Errorf("expected branch names, got:\n%s", text)
	}
	if !strings.Contains(text, "core") || !strings.Contains(text, "auth") {
		t.Errorf("expected feature names, got:\n%s", text)
	}
}
