package bench

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

// makeResult is a helper to create a Result with the given fields.
func makeResult(id, ability string, passed bool, score float64, latencyMs int64, missed, falsePos []string) Result {
	return Result{
		ScenarioID:     id,
		Ability:        ability,
		Passed:         passed,
		Score:          score,
		LatencyMs:      latencyMs,
		MissedTerms:    missed,
		FalsePositives: falsePos,
	}
}

func TestGenerateReport_AllPassing(t *testing.T) {
	results := []Result{
		makeResult("sc-001", "Session Continuity", true, 1.0, 2, nil, nil),
		makeResult("sc-002", "Session Continuity", true, 1.0, 3, nil, nil),
		makeResult("sc-003", "Decision Recall", true, 1.0, 1, nil, nil),
		makeResult("sc-004", "Decision Recall", true, 1.0, 4, nil, nil),
		makeResult("sc-005", "Plan Tracking", true, 1.0, 2, nil, nil),
	}

	r := GenerateReport(results)

	if r.TotalScenarios != 5 {
		t.Errorf("TotalScenarios = %d, want 5", r.TotalScenarios)
	}
	if r.TotalPassed != 5 {
		t.Errorf("TotalPassed = %d, want 5", r.TotalPassed)
	}
	if r.OverallScore != 100.0 {
		t.Errorf("OverallScore = %.1f, want 100.0", r.OverallScore)
	}
	if r.OverallAccuracy != 100.0 {
		t.Errorf("OverallAccuracy = %.1f, want 100.0", r.OverallAccuracy)
	}
	if len(r.Failures) != 0 {
		t.Errorf("Failures = %d, want 0", len(r.Failures))
	}

	// Check ability scores.
	sc, ok := r.AbilityScores["Session Continuity"]
	if !ok {
		t.Fatal("missing ability: Session Continuity")
	}
	if sc.Score != 100.0 {
		t.Errorf("Session Continuity Score = %.1f, want 100.0", sc.Score)
	}
	if sc.Accuracy != 100.0 {
		t.Errorf("Session Continuity Accuracy = %.1f, want 100.0", sc.Accuracy)
	}
	if sc.Total != 2 {
		t.Errorf("Session Continuity Total = %d, want 2", sc.Total)
	}
}

func TestGenerateReport_MixedResults(t *testing.T) {
	results := []Result{
		makeResult("sc-001", "Session Continuity", true, 1.0, 2, nil, nil),
		makeResult("sc-002", "Session Continuity", true, 0.9, 3, nil, nil),
		makeResult("sc-003", "Session Continuity", false, 0.6, 5, []string{"session history"}, nil),
		makeResult("sc-004", "Decision Recall", true, 1.0, 1, nil, nil),
		makeResult("sc-005", "Decision Recall", true, 0.8, 4, nil, nil),
		makeResult("sc-006", "Decision Recall", false, 0.5, 10, []string{"rationale"}, []string{"unrelated"}),
	}

	r := GenerateReport(results)

	if r.TotalScenarios != 6 {
		t.Errorf("TotalScenarios = %d, want 6", r.TotalScenarios)
	}
	if r.TotalPassed != 4 {
		t.Errorf("TotalPassed = %d, want 4", r.TotalPassed)
	}

	// Overall score: avg of [1.0, 0.9, 0.6, 1.0, 0.8, 0.5] = 4.8/6 = 0.8 * 100 = 80.0
	expectedScore := (4.8 / 6.0) * 100
	if math.Abs(r.OverallScore-expectedScore) > 0.01 {
		t.Errorf("OverallScore = %.2f, want %.2f", r.OverallScore, expectedScore)
	}

	// Accuracy: 4/6 = 66.67%
	expectedAccuracy := (4.0 / 6.0) * 100
	if math.Abs(r.OverallAccuracy-expectedAccuracy) > 0.01 {
		t.Errorf("OverallAccuracy = %.2f, want %.2f", r.OverallAccuracy, expectedAccuracy)
	}

	// Check failures: sc-003 (0.6) and sc-006 (0.5) should be failures.
	if len(r.Failures) != 2 {
		t.Fatalf("Failures = %d, want 2", len(r.Failures))
	}
	if r.Failures[0].ScenarioID != "sc-003" {
		t.Errorf("Failures[0].ScenarioID = %s, want sc-003", r.Failures[0].ScenarioID)
	}
	if r.Failures[1].ScenarioID != "sc-006" {
		t.Errorf("Failures[1].ScenarioID = %s, want sc-006", r.Failures[1].ScenarioID)
	}

	// Check Session Continuity ability.
	sc := r.AbilityScores["Session Continuity"]
	// avg score: (1.0+0.9+0.6)/3 * 100 = 83.33
	scExpected := ((1.0 + 0.9 + 0.6) / 3.0) * 100
	if math.Abs(sc.Score-scExpected) > 0.01 {
		t.Errorf("Session Continuity Score = %.2f, want %.2f", sc.Score, scExpected)
	}
	// accuracy: 2/3 * 100 = 66.67
	scAccExpected := (2.0 / 3.0) * 100
	if math.Abs(sc.Accuracy-scAccExpected) > 0.01 {
		t.Errorf("Session Continuity Accuracy = %.2f, want %.2f", sc.Accuracy, scAccExpected)
	}
}

func TestGenerateReport_Empty(t *testing.T) {
	r := GenerateReport(nil)

	if r.TotalScenarios != 0 {
		t.Errorf("TotalScenarios = %d, want 0", r.TotalScenarios)
	}
	if r.TotalPassed != 0 {
		t.Errorf("TotalPassed = %d, want 0", r.TotalPassed)
	}
	if r.OverallScore != 0 {
		t.Errorf("OverallScore = %.1f, want 0.0", r.OverallScore)
	}
	if r.OverallAccuracy != 0 {
		t.Errorf("OverallAccuracy = %.1f, want 0.0", r.OverallAccuracy)
	}
	if r.AvgLatencyMs != 0 {
		t.Errorf("AvgLatencyMs = %d, want 0", r.AvgLatencyMs)
	}
	if r.P95LatencyMs != 0 {
		t.Errorf("P95LatencyMs = %d, want 0", r.P95LatencyMs)
	}
	if len(r.Failures) != 0 {
		t.Errorf("Failures = %d, want 0", len(r.Failures))
	}
	if r.AbilityScores == nil {
		t.Error("AbilityScores should not be nil")
	}
}

func TestPrintMarkdown_ContainsHeaders(t *testing.T) {
	results := []Result{
		makeResult("sc-001", "Session Continuity", true, 1.0, 2, nil, nil),
		makeResult("sc-002", "Decision Recall", false, 0.5, 5, []string{"rationale"}, nil),
	}

	r := GenerateReport(results)
	md := r.PrintMarkdown()

	expected := []string{
		"# memorX Benchmark Report",
		"## Overall",
		"## Ability Breakdown",
		"## Failures",
		"| Metric | Value |",
		"| Ability | Score | Accuracy | Scenarios |",
		"| Scenarios |",
		"| Passed |",
		"| Overall Score |",
		"| Accuracy |",
		"| Avg Latency |",
		"| P95 Latency |",
		"Session Continuity",
		"Decision Recall",
		"sc-002",
	}

	for _, s := range expected {
		if !strings.Contains(md, s) {
			t.Errorf("PrintMarkdown missing expected string: %q", s)
		}
	}
}

func TestPrintMarkdown_NoFailuresSection_WhenAllPass(t *testing.T) {
	results := []Result{
		makeResult("sc-001", "Session Continuity", true, 1.0, 2, nil, nil),
	}

	r := GenerateReport(results)
	md := r.PrintMarkdown()

	if strings.Contains(md, "## Failures") {
		t.Error("PrintMarkdown should not contain Failures section when all pass")
	}
}

func TestP95Latency(t *testing.T) {
	// Create 20 results with latencies 1..20.
	results := make([]Result, 20)
	for i := 0; i < 20; i++ {
		results[i] = makeResult(
			"sc-"+strings.Repeat("0", 2)+string(rune('a'+i)),
			"Test",
			true,
			1.0,
			int64(i+1),
			nil,
			nil,
		)
	}

	r := GenerateReport(results)

	// P95 of [1..20]: rank = (95*20)/100 = 19, so sorted[19] = 20.
	if r.P95LatencyMs != 20 {
		t.Errorf("P95LatencyMs = %d, want 20", r.P95LatencyMs)
	}

	// Avg latency: (1+2+...+20)/20 = 210/20 = 10 (integer division).
	if r.AvgLatencyMs != 10 {
		t.Errorf("AvgLatencyMs = %d, want 10", r.AvgLatencyMs)
	}
}

func TestP95Latency_SmallSet(t *testing.T) {
	results := []Result{
		makeResult("sc-001", "Test", true, 1.0, 100, nil, nil),
		makeResult("sc-002", "Test", true, 1.0, 200, nil, nil),
		makeResult("sc-003", "Test", true, 1.0, 300, nil, nil),
	}

	r := GenerateReport(results)

	// P95 of [100, 200, 300]: rank = (95*3)/100 = 2, so sorted[2] = 300.
	if r.P95LatencyMs != 300 {
		t.Errorf("P95LatencyMs = %d, want 300", r.P95LatencyMs)
	}
}

func TestP95Latency_SingleResult(t *testing.T) {
	results := []Result{
		makeResult("sc-001", "Test", true, 1.0, 42, nil, nil),
	}

	r := GenerateReport(results)

	// P95 of [42]: rank = (95*1)/100 = 0, so sorted[0] = 42.
	if r.P95LatencyMs != 42 {
		t.Errorf("P95LatencyMs = %d, want 42", r.P95LatencyMs)
	}
}

func TestPrintJSON_ValidJSON(t *testing.T) {
	results := []Result{
		makeResult("sc-001", "Session Continuity", true, 1.0, 2, nil, nil),
		makeResult("sc-002", "Decision Recall", false, 0.5, 5, []string{"rationale"}, nil),
	}

	r := GenerateReport(results)
	jsonStr := r.PrintJSON()

	var parsed Report
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		t.Fatalf("PrintJSON produced invalid JSON: %v", err)
	}

	if parsed.TotalScenarios != 2 {
		t.Errorf("parsed TotalScenarios = %d, want 2", parsed.TotalScenarios)
	}
	if parsed.TotalPassed != 1 {
		t.Errorf("parsed TotalPassed = %d, want 1", parsed.TotalPassed)
	}
}
