package mcp

import (
	"context"

	"github.com/arbazkhan971/memorx/internal/memory"
	mcplib "github.com/mark3labs/mcp-go/mcp"
)

func (s *DevMemServer) handleDeduplicate(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	feature := getStringArg(req, "feature", "")
	dryRun := getBoolArg(req, "dry_run", true)
	result, err := s.store.Deduplicate(feature, dryRun)
	if err != nil {
		return respondErr("Deduplication failed: %v", err)
	}
	return mcplib.NewToolResultText(memory.FormatDeduplicate(result)), nil
}

func (s *DevMemServer) handleIntegrityCheck(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	fix := getBoolArg(req, "fix", false)
	result, err := s.store.IntegrityCheck(fix)
	if err != nil {
		return respondErr("Integrity check failed: %v", err)
	}
	return mcplib.NewToolResultText(memory.FormatIntegrityCheck(result)), nil
}

func (s *DevMemServer) handleAutoLinkCode(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	feature := getStringArg(req, "feature", "")
	result, err := s.store.AutoLinkCode(feature)
	if err != nil {
		return respondErr("Auto-link failed: %v", err)
	}
	return mcplib.NewToolResultText(memory.FormatAutoLinkCode(result)), nil
}
