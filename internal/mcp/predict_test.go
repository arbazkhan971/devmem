package mcp

import (
	"context"
	"strings"
	"testing"
)

func TestHandlePredictBlocker_NoFeature(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	res, err := srv.handlePredictBlocker(ctx, newReq("memorx_predict_blocker", nil))
	if err != nil {
		t.Fatalf("handlePredictBlocker error: %v", err)
	}
	text := resultText(t, res)
	if !strings.Contains(text, "no active feature") && !strings.Contains(text, "Failed") {
		t.Errorf("expected error about no active feature, got:\n%s", text)
	}
}

func TestHandlePredictBlocker_WithFeature(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	_, err := srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name": "predict-test",
	}))
	if err != nil {
		t.Fatalf("handleStartFeature: %v", err)
	}

	// Add a blocker
	_, _ = srv.handleRemember(ctx, newReq("memorx_remember", map[string]interface{}{
		"content": "Blocked on API key provisioning",
		"type":    "blocker",
	}))

	res, err := srv.handlePredictBlocker(ctx, newReq("memorx_predict_blocker", nil))
	if err != nil {
		t.Fatalf("handlePredictBlocker error: %v", err)
	}
	text := resultText(t, res)
	if !strings.Contains(text, "Blocker Prediction") {
		t.Errorf("expected Blocker Prediction header, got:\n%s", text)
	}
	if !strings.Contains(text, "predict-test") {
		t.Errorf("expected feature name, got:\n%s", text)
	}
	if !strings.Contains(text, "Risk Level") {
		t.Errorf("expected Risk Level, got:\n%s", text)
	}
}

func TestHandlePredictBlocker_ByName(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	_, _ = srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name": "named-predict",
	}))

	res, err := srv.handlePredictBlocker(ctx, newReq("memorx_predict_blocker", map[string]interface{}{
		"feature": "named-predict",
	}))
	if err != nil {
		t.Fatalf("handlePredictBlocker error: %v", err)
	}
	text := resultText(t, res)
	if !strings.Contains(text, "named-predict") {
		t.Errorf("expected feature name, got:\n%s", text)
	}
}

func TestHandleRiskScore_Empty(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	res, err := srv.handleRiskScore(ctx, newReq("memorx_risk_score", nil))
	if err != nil {
		t.Fatalf("handleRiskScore error: %v", err)
	}
	text := resultText(t, res)
	if !strings.Contains(text, "Risk Scores") {
		t.Errorf("expected Risk Scores header, got:\n%s", text)
	}
}

func TestHandleRiskScore_WithFeatures(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	_, _ = srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name": "risk-feat-a",
	}))
	_, _ = srv.handleRemember(ctx, newReq("memorx_remember", map[string]interface{}{
		"content": "Blocked on credentials",
		"type":    "blocker",
	}))

	res, err := srv.handleRiskScore(ctx, newReq("memorx_risk_score", nil))
	if err != nil {
		t.Fatalf("handleRiskScore error: %v", err)
	}
	text := resultText(t, res)
	if !strings.Contains(text, "risk-feat-a") {
		t.Errorf("expected feature name in scores, got:\n%s", text)
	}
	if !strings.Contains(text, "/100") {
		t.Errorf("expected score out of 100, got:\n%s", text)
	}
}

func TestHandleBurndown_NoPlan(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	_, _ = srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name": "burndown-no-plan",
	}))

	res, err := srv.handleBurndown(ctx, newReq("memorx_burndown", nil))
	if err != nil {
		t.Fatalf("handleBurndown error: %v", err)
	}
	text := resultText(t, res)
	if !strings.Contains(text, "no active plan") && !strings.Contains(text, "Failed") {
		t.Errorf("expected no active plan message, got:\n%s", text)
	}
}

func TestHandleBurndown_WithPlan(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	_, _ = srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name": "burndown-test",
	}))
	_, _ = srv.handleSavePlan(ctx, newReq("memorx_save_plan", map[string]interface{}{
		"title": "Build Auth",
		"steps": []interface{}{
			map[string]interface{}{"title": "Step 1"},
			map[string]interface{}{"title": "Step 2"},
			map[string]interface{}{"title": "Step 3"},
		},
	}))

	res, err := srv.handleBurndown(ctx, newReq("memorx_burndown", nil))
	if err != nil {
		t.Fatalf("handleBurndown error: %v", err)
	}
	text := resultText(t, res)
	if !strings.Contains(text, "Burndown") {
		t.Errorf("expected Burndown header, got:\n%s", text)
	}
	if !strings.Contains(text, "burndown-test") {
		t.Errorf("expected feature name, got:\n%s", text)
	}
	if !strings.Contains(text, "/3") {
		t.Errorf("expected step count, got:\n%s", text)
	}
}

func TestHandleCompare_MissingParams(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	res, err := srv.handleCompare(ctx, newReq("memorx_compare", nil))
	if err != nil {
		t.Fatalf("handleCompare error: %v", err)
	}
	text := resultText(t, res)
	if !strings.Contains(text, "required") {
		t.Errorf("expected required error, got:\n%s", text)
	}
}

func TestHandleCompare_TwoFeatures(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	_, _ = srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name": "cmp-a",
	}))
	_, _ = srv.handleRemember(ctx, newReq("memorx_remember", map[string]interface{}{
		"content": "Decision A",
		"type":    "decision",
	}))
	_, _ = srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name": "cmp-b",
	}))
	_, _ = srv.handleRemember(ctx, newReq("memorx_remember", map[string]interface{}{
		"content": "Decision B1",
		"type":    "decision",
	}))
	_, _ = srv.handleRemember(ctx, newReq("memorx_remember", map[string]interface{}{
		"content": "Decision B2",
		"type":    "decision",
	}))

	res, err := srv.handleCompare(ctx, newReq("memorx_compare", map[string]interface{}{
		"feature_a": "cmp-a",
		"feature_b": "cmp-b",
	}))
	if err != nil {
		t.Fatalf("handleCompare error: %v", err)
	}
	text := resultText(t, res)
	if !strings.Contains(text, "cmp-a") || !strings.Contains(text, "cmp-b") {
		t.Errorf("expected both feature names, got:\n%s", text)
	}
	if !strings.Contains(text, "Compare") {
		t.Errorf("expected Compare header, got:\n%s", text)
	}
	if !strings.Contains(text, "Notes") {
		t.Errorf("expected Notes metric, got:\n%s", text)
	}
}

func TestHandleSummarizePeriod_Empty(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	res, err := srv.handleSummarizePeriod(ctx, newReq("memorx_summarize_period", nil))
	if err != nil {
		t.Fatalf("handleSummarizePeriod error: %v", err)
	}
	text := resultText(t, res)
	if !strings.Contains(text, "Summary") {
		t.Errorf("expected Summary header, got:\n%s", text)
	}
}

func TestHandleSummarizePeriod_WithData(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	_, _ = srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name": "period-test",
	}))
	_, _ = srv.handleRemember(ctx, newReq("memorx_remember", map[string]interface{}{
		"content": "Use Redis for caching",
		"type":    "decision",
	}))

	res, err := srv.handleSummarizePeriod(ctx, newReq("memorx_summarize_period", map[string]interface{}{
		"period": "week",
	}))
	if err != nil {
		t.Fatalf("handleSummarizePeriod error: %v", err)
	}
	text := resultText(t, res)
	if !strings.Contains(text, "period-test") {
		t.Errorf("expected feature name in summary, got:\n%s", text)
	}
}

func TestHandleSummarizePeriod_Today(t *testing.T) {
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	_, _ = srv.handleStartFeature(ctx, newReq("memorx_start_feature", map[string]interface{}{
		"name": "today-test",
	}))
	_, _ = srv.handleRemember(ctx, newReq("memorx_remember", map[string]interface{}{
		"content": "A note from today",
		"type":    "note",
	}))

	res, err := srv.handleSummarizePeriod(ctx, newReq("memorx_summarize_period", map[string]interface{}{
		"period": "today",
	}))
	if err != nil {
		t.Fatalf("handleSummarizePeriod error: %v", err)
	}
	text := resultText(t, res)
	if !strings.Contains(text, "today") {
		t.Errorf("expected 'today' in summary, got:\n%s", text)
	}
}
