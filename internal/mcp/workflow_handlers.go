package mcp

import (
	"context"

	"github.com/arbazkhan971/memorx/internal/memory"
	mcplib "github.com/mark3labs/mcp-go/mcp"
)

func (s *DevMemServer) handleStandup(_ context.Context, _ mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	data, err := s.store.GetStandup()
	if err != nil {
		return respondErr("Failed to generate standup: %v", err)
	}
	return mcplib.NewToolResultText(memory.FormatStandup(data)), nil
}

func (s *DevMemServer) handleBranchContext(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	action := getStringArg(req, "action", "list")
	branch := getStringArg(req, "branch", "")

	switch action {
	case "save":
		if branch == "" {
			return respondErr("Parameter 'branch' is required for save action")
		}
		mapping, err := s.store.BranchContextSave(branch)
		if err != nil {
			return respondErr("Failed to save branch context: %v", err)
		}
		return mcplib.NewToolResultText(memory.FormatBranchContext("save", mapping, nil)), nil

	case "restore":
		if branch == "" {
			return respondErr("Parameter 'branch' is required for restore action")
		}
		mapping, err := s.store.BranchContextRestore(branch)
		if err != nil {
			return respondErr("Failed to restore branch context: %v", err)
		}
		return mcplib.NewToolResultText(memory.FormatBranchContext("restore", mapping, nil)), nil

	case "list":
		mappings, err := s.store.BranchContextList()
		if err != nil {
			return respondErr("Failed to list branch contexts: %v", err)
		}
		return mcplib.NewToolResultText(memory.FormatBranchContext("list", nil, mappings)), nil

	default:
		return respondErr("Invalid action %q. Use 'save', 'restore', or 'list'.", action)
	}
}
