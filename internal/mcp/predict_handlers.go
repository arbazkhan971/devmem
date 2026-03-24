package mcp

import (
	"context"

	"github.com/arbazkhan971/memorx/internal/memory"
	mcplib "github.com/mark3labs/mcp-go/mcp"
)

func (s *DevMemServer) handlePredictBlocker(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	feature := getStringArg(req, "feature", "")
	pred, err := s.store.PredictBlocker(feature)
	if err != nil {
		return respondErr("Failed to predict blockers: %v", err)
	}
	return mcplib.NewToolResultText(memory.FormatBlockerPrediction(pred)), nil
}

func (s *DevMemServer) handleRiskScore(_ context.Context, _ mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	entries, err := s.store.GetRiskScores()
	if err != nil {
		return respondErr("Failed to get risk scores: %v", err)
	}
	return mcplib.NewToolResultText(memory.FormatRiskScores(entries)), nil
}

func (s *DevMemServer) handleBurndown(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	feature := getStringArg(req, "feature", "")
	data, err := s.store.GetBurndown(feature)
	if err != nil {
		return respondErr("Failed to generate burndown: %v", err)
	}
	return mcplib.NewToolResultText(memory.FormatBurndown(data)), nil
}

func (s *DevMemServer) handleCompare(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	featureA, errRes := requireParam(req, "feature_a")
	if errRes != nil {
		return errRes, nil
	}
	featureB, errRes := requireParam(req, "feature_b")
	if errRes != nil {
		return errRes, nil
	}
	cmp, err := s.store.CompareFeatures(featureA, featureB)
	if err != nil {
		return respondErr("Failed to compare features: %v", err)
	}
	return mcplib.NewToolResultText(memory.FormatComparison(cmp)), nil
}

func (s *DevMemServer) handleSummarizePeriod(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	period := getStringArg(req, "period", "week")
	summary, err := s.store.SummarizePeriod(period)
	if err != nil {
		return respondErr("Failed to summarize period: %v", err)
	}
	return mcplib.NewToolResultText(memory.FormatPeriodSummary(summary)), nil
}
