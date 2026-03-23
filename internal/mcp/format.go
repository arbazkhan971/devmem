package mcp

import (
	"fmt"
	"strings"
	"time"

	"github.com/arbazkhan971/memorx/internal/memory"
	mcplib "github.com/mark3labs/mcp-go/mcp"
)

func optField(b *strings.Builder, cond bool, f string, a ...interface{}) {
	if cond {
		fmt.Fprintf(b, f, a...)
	}
}

func ctxSec[T any](b *strings.Builder, title string, items []T, fn func(T) string) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(b, "%s:", title)
	for _, item := range items {
		fmt.Fprintf(b, " %s;", fn(item))
	}
	b.WriteString("\n")
}

func formatContext(ctx *memory.Context) string {
	var b strings.Builder
	if ctx.Feature != nil {
		fmt.Fprintf(&b, "%s [%s]", ctx.Feature.Name, ctx.Feature.Status)
		optField(&b, ctx.Feature.Branch != "", " branch:%s", ctx.Feature.Branch)
		b.WriteString("\n")
	}
	if ctx.LastSessionSummary != "" {
		fmt.Fprintf(&b, "LastSession: %s\n", strings.ReplaceAll(ctx.LastSessionSummary, "\n", " "))
	}
	if ctx.Summary != "" {
		fmt.Fprintf(&b, "Summary: %s\n", strings.ReplaceAll(ctx.Summary, "\n", " "))
	}
	if ctx.Plan != nil {
		fmt.Fprintf(&b, "Plan: %s %d/%d\n", ctx.Plan.Title, ctx.Plan.CompletedStep, ctx.Plan.TotalSteps)
	}
	ctxSec(&b, "Commits", ctx.RecentCommits, func(c memory.CommitInfo) string {
		return fmt.Sprintf("%s %s", c.Hash[:min(7, len(c.Hash))], c.Message)
	})
	ctxSec(&b, "Notes", ctx.RecentNotes, func(n memory.Note) string {
		return fmt.Sprintf("[%s] %s", n.Type, truncate(n.Content, 100))
	})
	ctxSec(&b, "Facts", ctx.ActiveFacts, func(f memory.Fact) string {
		return fmt.Sprintf("%s %s %s", f.Subject, f.Predicate, f.Object)
	})
	ctxSec(&b, "Pinned", ctx.PinnedMemories, func(m memory.MemoryItem) string {
		return fmt.Sprintf("[%s] %s", m.Type, truncate(m.Content, 100))
	})
	ctxSec(&b, "Sessions", ctx.SessionHistory, func(s memory.Session) string {
		e := s.EndedAt
		if e == "" {
			e = "active"
		}
		return fmt.Sprintf("%s %s->%s %s", s.ID[:8], s.StartedAt, e, s.Tool)
	})
	ctxSec(&b, "Links", ctx.Links, func(l memory.MemoryLink) string {
		return fmt.Sprintf("%s:%s->%s:%s[%s,%.1f]", l.SourceType, l.SourceID[:8], l.TargetType, l.TargetID[:8], l.Relationship, l.Strength)
	})
	ctxSec(&b, "Files", ctx.FilesTouched, func(f string) string { return f })
	return b.String()
}

func (s *DevMemServer) exportMarkdown(feature *memory.Feature, ctx *memory.Context) (*mcplib.CallToolResult, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "# Feature: %s\n\n**Status:** %s\n", feature.Name, feature.Status)
	optField(&b, feature.Branch != "", "**Branch:** %s\n", feature.Branch)
	optField(&b, feature.Description != "", "**Description:** %s\n", feature.Description)
	fmt.Fprintf(&b, "**Created:** %s\n**Last Active:** %s\n\n", feature.CreatedAt, feature.LastActive)
	if ctx.Plan != nil {
		fmt.Fprintf(&b, "## Plan: %s\n\nProgress: %d/%d steps\n\n", ctx.Plan.Title, ctx.Plan.CompletedStep, ctx.Plan.TotalSteps)
		if ap, err := s.planManager.GetActivePlan(feature.ID); err == nil {
			ps, _ := s.planManager.GetPlanSteps(ap.ID)
			for _, st := range ps {
				c := map[string]string{"completed": "[x]", "in_progress": "[-]"}[st.Status]
				if c == "" {
					c = "[ ]"
				}
				fmt.Fprintf(&b, "- %s %s\n", c, st.Title)
			}
		}
		b.WriteString("\n")
	}
	for _, sec := range []struct{ t, h, e string }{
		{"decision", "Decisions", "_No decisions recorded._"}, {"progress", "Progress Notes", "_No progress notes._"}, {"blocker", "Blockers", ""},
	} {
		notes, _ := s.store.ListNotes(feature.ID, sec.t, 50)
		if len(notes) == 0 && sec.e == "" {
			continue
		}
		fmt.Fprintf(&b, "## %s\n\n", sec.h)
		if len(notes) == 0 {
			b.WriteString(sec.e + "\n\n")
		}
		for _, n := range notes {
			fmt.Fprintf(&b, "- **[%s]** %s\n", n.CreatedAt, n.Content)
		}
		b.WriteString("\n")
	}
	b.WriteString("## Facts (Current)\n\n")
	if len(ctx.ActiveFacts) == 0 {
		b.WriteString("_No facts recorded._\n\n")
	}
	for _, f := range ctx.ActiveFacts {
		fmt.Fprintf(&b, "- %s **%s** %s\n", f.Subject, f.Predicate, f.Object)
	}
	b.WriteString("\n## Commits\n\n")
	if len(ctx.RecentCommits) == 0 {
		b.WriteString("_No commits synced._\n\n")
	}
	for _, c := range ctx.RecentCommits {
		fmt.Fprintf(&b, "- `%s` %s (%s)\n", c.Hash[:min(7, len(c.Hash))], c.Message, c.CommittedAt)
	}
	b.WriteString("\n## Session History\n\n")
	for _, sess := range ctx.SessionHistory {
		end := sess.EndedAt
		if end == "" {
			end = "active"
		}
		fmt.Fprintf(&b, "- %s → %s (%s)\n", sess.StartedAt, end, sess.Tool)
	}
	return mcplib.NewToolResultText(b.String()), nil
}

func formatBriefing(ctx *memory.Context, feature *memory.Feature) string {
	if feature == nil {
		return "memorx: No active feature. Use memorx_start_feature to begin."
	}
	var lines []string
	fl := fmt.Sprintf("memorx: Welcome back! Active feature: %s", feature.Name)
	if feature.Branch != "" {
		fl += fmt.Sprintf(" [%s]", feature.Branch)
	}
	lines = append(lines, fl)
	if ctx.Plan != nil {
		lines = append(lines, fmt.Sprintf("memorx: plan: %s (%d/%d steps done)", ctx.Plan.Title, ctx.Plan.CompletedStep, ctx.Plan.TotalSteps))
	}
	if len(ctx.SessionHistory) > 0 {
		lines = append(lines, fmt.Sprintf("memorx: last: %s via %s", formatTimeAgo(ctx.SessionHistory[0].StartedAt), ctx.SessionHistory[0].Tool))
	}
	if len(ctx.RecentNotes) > 0 {
		lines = append(lines, fmt.Sprintf("memorx: recent: \"%s\"", truncate(ctx.RecentNotes[0].Content, 80)))
	}
	lines = append(lines, "memorx: tip: say \"where did I leave off?\" for full context")
	return strings.Join(lines, "\n")
}

func formatTimeAgo(datetime string) string {
	t, err := time.Parse(time.DateTime, datetime)
	if err != nil {
		if t, err = time.Parse(time.RFC3339, datetime); err != nil {
			return datetime
		}
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
