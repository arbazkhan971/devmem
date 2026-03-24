package mcp

import (
	"context"

	"github.com/arbazkhan971/memorx/internal/memory"
	mcplib "github.com/mark3labs/mcp-go/mcp"
)

func (s *DevMemServer) handleReviewContext(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	files := getStringSliceArg(req, "files")
	if len(files) == 0 {
		return respondErr("Parameter 'files' is required (array of file paths)")
	}

	contexts, err := s.store.GetReviewContext(files)
	if err != nil {
		return respondErr("Failed to get review context: %v", err)
	}

	return mcplib.NewToolResultText(memory.FormatReviewContext(contexts)), nil
}

func (s *DevMemServer) handleReviewRisk(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	files := getStringSliceArg(req, "files")
	if len(files) == 0 {
		return respondErr("Parameter 'files' is required (array of file paths)")
	}

	risks, err := s.store.GetReviewRisk(files)
	if err != nil {
		return respondErr("Failed to assess risk: %v", err)
	}

	return mcplib.NewToolResultText(memory.FormatReviewRisk(risks)), nil
}

func (s *DevMemServer) handleReviewChecklist(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	featureName := getStringArg(req, "feature", "")
	var featureID, name string

	if featureName != "" {
		f, err := s.store.GetFeature(featureName)
		if err != nil {
			return respondErr("Feature '%s' not found", featureName)
		}
		featureID = f.ID
		name = f.Name
	} else {
		f, errRes := s.requireActiveFeature()
		if errRes != nil {
			return errRes, nil
		}
		featureID = f.ID
		name = f.Name
	}

	items, err := s.store.GenerateReviewChecklist(featureID)
	if err != nil {
		return respondErr("Failed to generate checklist: %v", err)
	}

	return mcplib.NewToolResultText(memory.FormatReviewChecklist(items, name)), nil
}
