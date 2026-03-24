package mcp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/arbazkhan971/memorx/internal/memory"
	mcplib "github.com/mark3labs/mcp-go/mcp"
)

func (s *DevMemServer) handleTimeTravel(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	asOfStr, errRes := requireParam(req, "as_of")
	if errRes != nil {
		return errRes, nil
	}

	// Parse the as_of timestamp, supporting date-only, datetime, and RFC3339
	var asOf time.Time
	var parseErr error
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, asOfStr); err == nil {
			asOf = t
			parseErr = nil
			break
		} else {
			parseErr = err
		}
	}
	if parseErr != nil {
		return respondErr("Invalid as_of format %q (use ISO date like 2026-03-15 or 2026-03-15T10:30:00Z)", asOfStr)
	}

	// Resolve feature
	featureName := getStringArg(req, "feature", "")
	var featureID string
	if featureName != "" {
		f, err := s.store.GetFeature(featureName)
		if err != nil {
			return respondErr("Feature '%s' not found", featureName)
		}
		featureID = f.ID
	} else {
		f, errRes := s.requireActiveFeature()
		if errRes != nil {
			return errRes, nil
		}
		featureID = f.ID
	}

	result, err := s.store.TimeTravel(featureID, asOf)
	if err != nil {
		return respondErr("Time travel failed: %v", err)
	}

	// Build detailed output
	var b strings.Builder
	compact := memory.FormatTimeTravel(result)
	fmt.Fprintf(&b, "# Time Travel\n\n%s\n\n", compact)

	if len(result.ActiveFacts) > 0 {
		b.WriteString("## Facts\n\n")
		for _, f := range result.ActiveFacts {
			fmt.Fprintf(&b, "- %s **%s** %s\n", f.Subject, f.Predicate, f.Object)
		}
		b.WriteString("\n")
	}

	if len(result.NotesAtTime) > 0 {
		b.WriteString("## Notes\n\n")
		for _, n := range result.NotesAtTime {
			content := n.Content
			if len(content) > 120 {
				content = content[:120] + "..."
			}
			content = strings.ReplaceAll(content, "\n", " ")
			fmt.Fprintf(&b, "- [%s] %s (%s)\n", n.Type, content, n.CreatedAt)
		}
		b.WriteString("\n")
	}

	if result.PlanAtTime != nil {
		completed := 0
		for _, st := range result.PlanAtTime.Steps {
			if st.Status == "completed" {
				completed++
			}
		}
		fmt.Fprintf(&b, "## Plan: %s (%d/%d steps)\n\n", result.PlanAtTime.Title, completed, len(result.PlanAtTime.Steps))
		for _, st := range result.PlanAtTime.Steps {
			check := "[ ]"
			if st.Status == "completed" {
				check = "[x]"
			} else if st.Status == "in_progress" {
				check = "[-]"
			}
			fmt.Fprintf(&b, "- %s %s\n", check, st.Title)
		}
		b.WriteString("\n")
	}

	return mcplib.NewToolResultText(b.String()), nil
}

func (s *DevMemServer) handleReplay(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	sessionID := getStringArg(req, "session_id", "")

	replay, err := s.store.ReplaySession(sessionID)
	if err != nil {
		return respondErr("Replay failed: %v", err)
	}

	return mcplib.NewToolResultText(memory.FormatReplay(replay)), nil
}
