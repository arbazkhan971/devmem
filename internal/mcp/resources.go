package mcp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/arbazkhan971/memorx/internal/git"
	mcplib "github.com/mark3labs/mcp-go/mcp"
)

func textResource(uri, text string) ([]mcplib.ResourceContents, error) {
	return []mcplib.ResourceContents{mcplib.TextResourceContents{URI: uri, MIMEType: "text/plain", Text: text}}, nil
}

func (s *DevMemServer) handleResourceActiveContext(_ context.Context, _ mcplib.ReadResourceRequest) ([]mcplib.ResourceContents, error) {
	const uri = "memorx://context/active"
	feature, err := s.store.GetActiveFeature()
	if err != nil {
		return textResource(uri, "No active feature.")
	}
	ctxData, err := s.store.GetContext(feature.ID, "compact", nil)
	if err != nil {
		return textResource(uri, fmt.Sprintf("Error loading context: %v", err))
	}
	return textResource(uri, formatContext(ctxData))
}

func (s *DevMemServer) handleResourceRecentChanges(_ context.Context, _ mcplib.ReadResourceRequest) ([]mcplib.ResourceContents, error) {
	const uri = "memorx://changes/recent"
	feature, err := s.store.GetActiveFeature()
	if err != nil {
		return textResource(uri, "No active feature.")
	}
	since := time.Now().AddDate(0, 0, -1)
	if sessions, err := s.store.ListSessions(feature.ID, 10); err == nil {
		for _, sess := range sessions {
			if sess.EndedAt != "" {
				if t, err := time.Parse("2006-01-02 15:04:05", sess.EndedAt); err == nil {
					since = t
					break
				}
			}
		}
	}
	result, err := git.SyncCommits(s.db, s.gitRoot, feature.ID, s.currentSessionID, since)
	if err != nil {
		return textResource(uri, fmt.Sprintf("Error syncing commits: %v", err))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Recent changes (since %s)\n\n", since.Format(time.DateTime))
	if result.NewCommits == 0 {
		b.WriteString("No new commits.\n")
	} else {
		fmt.Fprintf(&b, "**%d new commits:**\n\n", result.NewCommits)
		for _, c := range result.Commits {
			fmt.Fprintf(&b, "- `%s` %s [%s] by %s at %s\n", c.Hash[:7], c.Message, c.IntentType, c.Author, c.CommittedAt)
			for _, f := range c.FilesChanged {
				fmt.Fprintf(&b, "    %s %s\n", f.Action, f.Path)
			}
		}
	}
	return textResource(uri, b.String())
}
