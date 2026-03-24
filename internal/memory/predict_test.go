package memory

import (
	"strings"
	"testing"
)

func TestFormatBlockerPrediction(t *testing.T) {
	p := &BlockerPrediction{
		FeatureName:         "auth-v2",
		BlockerCount:        3,
		UnresolvedDeps:      2,
		TestCount:           0,
		DaysSinceLastCommit: 5,
		SimilarFeatureRisk:  80,
		RiskLevel:           "High",
		Explanation:         "High risk: auth-v2 has 2 unresolved dependencies and 0 tests.",
	}
	text := FormatBlockerPrediction(p)
	if !strings.Contains(text, "auth-v2") {
		t.Errorf("expected feature name, got:\n%s", text)
	}
	if !strings.Contains(text, "High") {
		t.Errorf("expected High risk level, got:\n%s", text)
	}
	if !strings.Contains(text, "Total blockers") {
		t.Errorf("expected Total blockers row, got:\n%s", text)
	}
}

func TestFormatRiskScores_Empty(t *testing.T) {
	text := FormatRiskScores(nil)
	if !strings.Contains(text, "No active features") {
		t.Errorf("expected empty message, got:\n%s", text)
	}
}

func TestFormatRiskScores_WithEntries(t *testing.T) {
	entries := []RiskEntry{
		{FeatureName: "alpha", Score: 85, Factors: []string{"-15 (plan 70%)"}},
		{FeatureName: "beta", Score: 30, Factors: []string{"-30 (2 blockers)", "-40 (plan 0%)"}},
	}
	text := FormatRiskScores(entries)
	if !strings.Contains(text, "alpha") || !strings.Contains(text, "beta") {
		t.Errorf("expected feature names, got:\n%s", text)
	}
	if !strings.Contains(text, "85/100") {
		t.Errorf("expected score for alpha, got:\n%s", text)
	}
	if !strings.Contains(text, "CRITICAL") {
		t.Errorf("expected CRITICAL for beta, got:\n%s", text)
	}
}

func TestFormatBurndown(t *testing.T) {
	d := &BurndownData{
		FeatureName:    "auth",
		TotalSteps:     10,
		CompletedSteps: 6,
		Velocity:       1.2,
		ETADate:        "Mar 28",
		Chart:          "\xe2\x96\x93\xe2\x96\x93\xe2\x96\x93\xe2\x96\x93\xe2\x96\x93\xe2\x96\x93\xe2\x96\x91\xe2\x96\x91\xe2\x96\x91\xe2\x96\x91 6/10 steps, 1.2/day, ETA: Mar 28",
	}
	text := FormatBurndown(d)
	if !strings.Contains(text, "Burndown: auth") {
		t.Errorf("expected Burndown header, got:\n%s", text)
	}
	if !strings.Contains(text, "6/10") {
		t.Errorf("expected step progress, got:\n%s", text)
	}
}

func TestFormatComparison(t *testing.T) {
	c := &FeatureComparison{
		FeatureA: ComparisonSide{Name: "alpha", NoteCount: 5, FactCount: 3, CommitCount: 10, SessionCount: 4, BlockerCount: 1, PlanProgress: "3/5", Status: "active", LastActive: "2026-01-15"},
		FeatureB: ComparisonSide{Name: "beta", NoteCount: 2, FactCount: 1, CommitCount: 3, SessionCount: 2, BlockerCount: 0, PlanProgress: "no plan", Status: "paused", LastActive: "2026-01-10"},
	}
	text := FormatComparison(c)
	if !strings.Contains(text, "alpha") || !strings.Contains(text, "beta") {
		t.Errorf("expected feature names, got:\n%s", text)
	}
	if !strings.Contains(text, "Compare:") {
		t.Errorf("expected Compare header, got:\n%s", text)
	}
}

func TestFormatPeriodSummary_Empty(t *testing.T) {
	s := &PeriodSummary{Period: "week"}
	text := FormatPeriodSummary(s)
	if !strings.Contains(text, "No activity") {
		t.Errorf("expected no activity message, got:\n%s", text)
	}
}

func TestFormatPeriodSummary_WithData(t *testing.T) {
	s := &PeriodSummary{
		Period:         "week",
		TotalCommits:   8,
		TotalDecisions: 3,
		TotalBlockers:  1,
		Features: []FeaturePeriodSummary{
			{Name: "auth-v2", Commits: 5, Decisions: 2, Blockers: 0, Notes: 7},
			{Name: "billing", Commits: 3, Decisions: 1, Blockers: 1, Notes: 4},
		},
	}
	text := FormatPeriodSummary(s)
	if !strings.Contains(text, "auth-v2") || !strings.Contains(text, "billing") {
		t.Errorf("expected feature names, got:\n%s", text)
	}
	if !strings.Contains(text, "8 commits") {
		t.Errorf("expected total commits, got:\n%s", text)
	}
}
