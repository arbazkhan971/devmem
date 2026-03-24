package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/arbazkhan971/memorx/internal/memory"
	mcplib "github.com/mark3labs/mcp-go/mcp"
)

func (s *DevMemServer) handleErrorLog(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	feature, errRes := s.requireActiveFeature(); if errRes != nil { return errRes, nil }
	message, errRes := requireParam(req, "message"); if errRes != nil { return errRes, nil }
	entry, err := s.store.LogError(feature.ID, s.currentSessionID, message, getStringArg(req, "file", ""), getStringArg(req, "cause", ""), getStringArg(req, "resolution", ""))
	if err != nil { return respondErr("Failed to log error: %v", err) }
	var b strings.Builder
	fmt.Fprintf(&b, "# Error logged\n\n- ID: %s\n- Message: %s\n", entry.ID[:8], truncate(message, 100))
	if entry.FilePath != "" { fmt.Fprintf(&b, "- File: %s\n", entry.FilePath) }
	if entry.Cause != "" { fmt.Fprintf(&b, "- Cause: %s\n", entry.Cause) }
	if entry.Resolution != "" { fmt.Fprintf(&b, "- Resolution: %s\n- Status: RESOLVED\n", entry.Resolution) } else { b.WriteString("- Status: UNRESOLVED\n") }
	return mcplib.NewToolResultText(b.String()), nil
}
func (s *DevMemServer) handleErrorSearch(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	query, errRes := requireParam(req, "query"); if errRes != nil { return errRes, nil }
	errors, err := s.store.SearchErrors(query); if err != nil { return respondErr("Error search failed: %v", err) }
	return mcplib.NewToolResultText(memory.FormatErrorSearch(errors)), nil
}
func (s *DevMemServer) handleDebugContext(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	topic, errRes := requireParam(req, "topic"); if errRes != nil { return errRes, nil }
	dc, err := s.store.GetDebugContext(s.searchEngine, topic); if err != nil { return respondErr("Failed to load debug context: %v", err) }
	return mcplib.NewToolResultText(memory.FormatDebugContext(dc, topic)), nil
}
func (s *DevMemServer) handleTestMemory(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	feature, errRes := s.requireActiveFeature(); if errRes != nil { return errRes, nil }
	testName, errRes := requireParam(req, "test_name"); if errRes != nil { return errRes, nil }
	result, err := s.store.RecordTestResult(feature.ID, s.currentSessionID, testName, getBoolArg(req, "passed", false), getStringArg(req, "error_message", ""))
	if err != nil { return respondErr("Failed to record test result: %v", err) }
	history, _ := s.store.GetTestHistory(testName, 10)
	return mcplib.NewToolResultText(memory.FormatTestMemory(result, history)), nil
}
func (s *DevMemServer) handleGlobalSearch(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	query, errRes := requireParam(req, "query"); if errRes != nil { return errRes, nil }
	results, err := memory.GlobalSearch(query); if err != nil { return respondErr("Global search failed: %v", err) }
	return mcplib.NewToolResultText(memory.FormatGlobalSearch(results)), nil
}
func (s *DevMemServer) handleGlobalPatterns(_ context.Context, _ mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	patterns, err := memory.DetectPatterns(); if err != nil { return respondErr("Pattern detection failed: %v", err) }
	return mcplib.NewToolResultText(memory.FormatPatterns(patterns)), nil
}
func (s *DevMemServer) handleTemplate(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	feature, errRes := s.requireActiveFeature(); if errRes != nil { return errRes, nil }
	action, errRes := requireParam(req, "action"); if errRes != nil { return errRes, nil }
	name, errRes := requireParam(req, "name"); if errRes != nil { return errRes, nil }
	switch action {
	case "save":
		if err := s.store.SaveTemplate(feature.ID, name); err != nil { return respondErr("Failed to save template: %v", err) }
		return respond("# Template saved: %s\n\nExported decisions, facts, and plan steps from feature %q.", name, feature.Name)
	case "apply":
		td, err := s.store.ApplyTemplate(feature.ID, s.currentSessionID, name); if err != nil { return respondErr("Failed to apply template: %v", err) }
		return respond("# Template applied: %s\n\n- Decisions: %d\n- Facts: %d\n- Plan steps: %d", name, len(td.Decisions), len(td.Facts), len(td.PlanSteps))
	default: return respondErr("Unknown action %q", action)
	}
}
func (s *DevMemServer) handleLinkProjects(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	projectPath, errRes := requireParam(req, "project_path"); if errRes != nil { return errRes, nil }
	lp, err := s.store.LinkProject(projectPath, getStringArg(req, "relationship", "related")); if err != nil { return respondErr("Failed to link project: %v", err) }
	all, _ := s.store.ListLinkedProjects()
	var b strings.Builder
	fmt.Fprintf(&b, "# Project linked: %s [%s]\n\nPath: %s\n\n", lp.ProjectName, lp.Relationship, lp.ProjectPath)
	if len(all) > 1 { b.WriteString("## All linked projects\n"); for _, p := range all { fmt.Fprintf(&b, "- %s (%s)\n", p.ProjectName, p.Relationship) } }
	return mcplib.NewToolResultText(b.String()), nil
}
