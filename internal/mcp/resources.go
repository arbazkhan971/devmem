package mcp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/arbaz/devmem/internal/git"
	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// handleResourceActiveContext implements the devmem://context/active resource.
// Returns compact context for the currently active feature.
func (s *DevMemServer) handleResourceActiveContext(ctx context.Context, req mcplib.ReadResourceRequest) ([]mcplib.ResourceContents, error) {
	feature, err := s.store.GetActiveFeature()
	if err != nil {
		return []mcplib.ResourceContents{
			mcplib.TextResourceContents{
				URI:      "devmem://context/active",
				MIMEType: "text/plain",
				Text:     "No active feature.",
			},
		}, nil
	}

	ctxData, err := s.store.GetContext(feature.ID, "compact", nil)
	if err != nil {
		return []mcplib.ResourceContents{
			mcplib.TextResourceContents{
				URI:      "devmem://context/active",
				MIMEType: "text/plain",
				Text:     fmt.Sprintf("Error loading context: %v", err),
			},
		}, nil
	}

	return []mcplib.ResourceContents{
		mcplib.TextResourceContents{
			URI:      "devmem://context/active",
			MIMEType: "text/plain",
			Text:     formatContext(ctxData),
		},
	}, nil
}

// handleResourceRecentChanges implements the devmem://changes/recent resource.
// Returns commits since the last session ended.
func (s *DevMemServer) handleResourceRecentChanges(ctx context.Context, req mcplib.ReadResourceRequest) ([]mcplib.ResourceContents, error) {
	feature, err := s.store.GetActiveFeature()
	if err != nil {
		return []mcplib.ResourceContents{
			mcplib.TextResourceContents{
				URI:      "devmem://changes/recent",
				MIMEType: "text/plain",
				Text:     "No active feature.",
			},
		}, nil
	}

	// Find the last ended session to determine "since" time
	since := time.Now().AddDate(0, 0, -1) // default: last 24 hours
	sessions, err := s.store.ListSessions(feature.ID, 10)
	if err == nil {
		for _, sess := range sessions {
			if sess.EndedAt != "" {
				t, err := time.Parse("2006-01-02 15:04:05", sess.EndedAt)
				if err == nil {
					since = t
					break
				}
			}
		}
	}

	// Sync and get recent commits
	result, err := git.SyncCommits(s.db, s.gitRoot, feature.ID, s.currentSessionID, since)
	if err != nil {
		return []mcplib.ResourceContents{
			mcplib.TextResourceContents{
				URI:      "devmem://changes/recent",
				MIMEType: "text/plain",
				Text:     fmt.Sprintf("Error syncing commits: %v", err),
			},
		}, nil
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("# Recent changes (since %s)\n\n", since.Format(time.DateTime)))
	if result.NewCommits == 0 {
		b.WriteString("No new commits.\n")
	} else {
		b.WriteString(fmt.Sprintf("**%d new commits:**\n\n", result.NewCommits))
		for _, c := range result.Commits {
			b.WriteString(fmt.Sprintf("- `%s` %s [%s] by %s at %s\n",
				c.Hash[:7], c.Message, c.IntentType, c.Author, c.CommittedAt))
			if len(c.FilesChanged) > 0 {
				for _, f := range c.FilesChanged {
					b.WriteString(fmt.Sprintf("    %s %s\n", f.Action, f.Path))
				}
			}
		}
	}

	return []mcplib.ResourceContents{
		mcplib.TextResourceContents{
			URI:      "devmem://changes/recent",
			MIMEType: "text/plain",
			Text:     b.String(),
		},
	}, nil
}
