package mcp

import (
	"context"

	"github.com/arbazkhan971/memorx/internal/memory"
	mcplib "github.com/mark3labs/mcp-go/mcp"
)

func (s *DevMemServer) handleFocusTime(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	feature := getStringArg(req, "feature", "")
	days := getIntArg(req, "days", 7)

	entries, err := s.store.GetFocusTime(feature, days)
	if err != nil {
		return respondErr("Failed to get focus time: %v", err)
	}

	return mcplib.NewToolResultText(memory.FormatFocusTime(entries, days)), nil
}

func (s *DevMemServer) handleVelocity(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	feature := getStringArg(req, "feature", "")

	entries, err := s.store.GetVelocity(feature)
	if err != nil {
		return respondErr("Failed to get velocity: %v", err)
	}

	return mcplib.NewToolResultText(memory.FormatVelocity(entries)), nil
}

func (s *DevMemServer) handleInterruptions(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	days := getIntArg(req, "days", 7)

	report, err := s.store.GetInterruptions(days)
	if err != nil {
		return respondErr("Failed to analyze interruptions: %v", err)
	}

	return mcplib.NewToolResultText(memory.FormatInterruptions(report)), nil
}

func (s *DevMemServer) handleWeeklyReport(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	days := getIntArg(req, "days", 7)

	report, err := s.store.GetWeeklyReport(days)
	if err != nil {
		return respondErr("Failed to generate weekly report: %v", err)
	}

	return mcplib.NewToolResultText(memory.FormatWeeklyReport(report)), nil
}
