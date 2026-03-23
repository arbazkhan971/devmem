package mcp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/arbaz/devmem/internal/memory"
	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// formatBriefing generates a compact single-line "welcome back" briefing.
// All info on one line to minimize token usage across sessions.
func formatBriefing(ctx *memory.Context, feature *memory.Feature) string {
	if feature == nil {
		return "devmem: No active feature. Use devmem_start_feature to begin."
	}

	var parts []string

	// Feature name + branch
	if feature.Branch != "" {
		parts = append(parts, fmt.Sprintf("%s [%s]", feature.Name, feature.Branch))
	} else {
		parts = append(parts, feature.Name)
	}

	// Plan progress
	if ctx.Plan != nil {
		parts = append(parts, fmt.Sprintf("plan:%s %d/%d", ctx.Plan.Title, ctx.Plan.CompletedStep, ctx.Plan.TotalSteps))
	}

	// Last session
	if len(ctx.SessionHistory) > 0 {
		lastSession := ctx.SessionHistory[0]
		parts = append(parts, fmt.Sprintf("last:%s", formatTimeAgo(lastSession.StartedAt)))
	}

	// Most recent note (truncated)
	if len(ctx.RecentNotes) > 0 {
		content := strings.ReplaceAll(ctx.RecentNotes[0].Content, "\n", " ")
		if len(content) > 60 {
			content = content[:60] + "..."
		}
		parts = append(parts, fmt.Sprintf("\"%s\"", content))
	}

	return "devmem: " + strings.Join(parts, " | ")
}

// formatTimeAgo converts a datetime string to a human-readable "X ago" format.
func formatTimeAgo(datetime string) string {
	t, err := time.Parse(time.DateTime, datetime)
	if err != nil {
		// Try RFC3339 as fallback
		t, err = time.Parse(time.RFC3339, datetime)
		if err != nil {
			return datetime
		}
	}

	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1m ago"
		}
		return fmt.Sprintf("%dm ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1h ago"
		}
		return fmt.Sprintf("%dh ago", h)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1d ago"
		}
		return fmt.Sprintf("%dd ago", days)
	}
}

// handleBriefing implements the devmem_briefing tool.
func (s *DevMemServer) handleBriefing(_ context.Context, _ mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	feature, err := s.store.GetActiveFeature()
	if err != nil {
		return mcplib.NewToolResultText("devmem: No active feature. Use devmem_start_feature to begin."), nil
	}

	// Use standard tier so we get notes + sessions for the briefing
	ctxData, err := s.store.GetContext(feature.ID, "standard", nil)
	if err != nil {
		return mcplib.NewToolResultError(fmt.Sprintf("Failed to load context: %v", err)), nil
	}

	// Also load session history for the briefing (standard tier doesn't include sessions)
	sessions, _ := s.store.ListSessions(feature.ID, 5)
	ctxData.SessionHistory = sessions

	return mcplib.NewToolResultText(formatBriefing(ctxData, feature)), nil
}
