package mcp

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/arbaz/devmem/internal/git"
	"github.com/arbaz/devmem/internal/memory"
	"github.com/arbaz/devmem/internal/plans"
	mcplib "github.com/mark3labs/mcp-go/mcp"
)

func respond(format string, args ...interface{}) (*mcplib.CallToolResult, error) {
	return mcplib.NewToolResultText(fmt.Sprintf(format, args...)), nil
}
func respondErr(format string, args ...interface{}) (*mcplib.CallToolResult, error) {
	return mcplib.NewToolResultError(fmt.Sprintf(format, args...)), nil
}

func (s *DevMemServer) requireActiveFeature() (*memory.Feature, *mcplib.CallToolResult) {
	f, err := s.store.GetActiveFeature()
	if err != nil {
		return nil, mcplib.NewToolResultError("No active feature. Use devmem_start_feature first.")
	}
	return f, nil
}

func requireParam(req mcplib.CallToolRequest, name string) (string, *mcplib.CallToolResult) {
	if v := getStringArg(req, name, ""); v != "" {
		return v, nil
	}
	return "", mcplib.NewToolResultError(fmt.Sprintf("Parameter '%s' is required", name))
}

func mdTable(header1, header2 string, rows [][2]string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "| %s | %s |\n|--------|-------|\n", header1, header2)
	for _, r := range rows {
		fmt.Fprintf(&b, "| %s | %s |\n", r[0], r[1])
	}
	return b.String()
}

func getBoolArg(req mcplib.CallToolRequest, name string, fallback bool) bool {
	if args := req.GetArguments(); args != nil {
		if v, ok := args[name].(bool); ok {
			return v
		}
	}
	return fallback
}

func getStringArg(req mcplib.CallToolRequest, name, fallback string) string {
	if args := req.GetArguments(); args != nil {
		if v, ok := args[name].(string); ok && v != "" {
			return v
		}
	}
	return fallback
}

func getStringSliceArg(req mcplib.CallToolRequest, name string) []string {
	if args := req.GetArguments(); args != nil {
		if arr, ok := args[name].([]interface{}); ok {
			var result []string
			for _, item := range arr {
				if s, ok := item.(string); ok {
					result = append(result, s)
				}
			}
			return result
		}
	}
	return nil
}

func itoa(n int) string { return fmt.Sprintf("%d", n) }

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func countCompleted(steps []plans.PlanStep) int {
	n := 0
	for _, st := range steps {
		if st.Status == "completed" {
			n++
		}
	}
	return n
}

func parseTimestamp(s, errCtx string) (time.Time, *mcplib.CallToolResult) {
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, mcplib.NewToolResultError(fmt.Sprintf("Invalid %s format (use ISO 8601)", errCtx))
}

func parseStepInputs(arr []interface{}) []plans.StepInput {
	var steps []plans.StepInput
	for _, item := range arr {
		if m, ok := item.(map[string]interface{}); ok {
			if t, _ := m["title"].(string); t != "" {
				d, _ := m["description"].(string)
				steps = append(steps, plans.StepInput{Title: t, Description: d})
			}
		}
	}
	return steps
}

func (s *DevMemServer) resolveFeatureID(name string) (string, *mcplib.CallToolResult) {
	if name == "" {
		return "", nil
	}
	f, err := s.store.GetFeature(name)
	if err != nil {
		return "", mcplib.NewToolResultError(fmt.Sprintf("Feature '%s' not found", name))
	}
	return f.ID, nil
}

// --- Handlers ---

func (s *DevMemServer) handleStatus(_ context.Context, _ mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "# devmem status — %s\n\n", git.ProjectName(s.gitRoot))
	feature, err := s.store.GetActiveFeature()
	if err != nil {
		b.WriteString("**Active feature:** none\n\n")
	} else {
		fmt.Fprintf(&b, "**Active feature:** %s\n  - Status: %s\n", feature.Name, feature.Status)
		if feature.Branch != "" {
			fmt.Fprintf(&b, "  - Branch: %s\n", feature.Branch)
		}
		if feature.Description != "" {
			fmt.Fprintf(&b, "  - Description: %s\n", feature.Description)
		}
		fmt.Fprintf(&b, "  - Last active: %s\n\n", feature.LastActive)
		if plan, err := s.planManager.GetActivePlan(feature.ID); err == nil {
			steps, _ := s.planManager.GetPlanSteps(plan.ID)
			fmt.Fprintf(&b, "**Active plan:** %s\n  - Progress: %d/%d steps completed\n\n", plan.Title, countCompleted(steps), len(steps))
		} else {
			b.WriteString("**Active plan:** none\n\n")
		}
	}
	if features, err := s.store.ListFeatures("all"); err == nil {
		counts := map[string]int{}
		for _, f := range features {
			counts[f.Status]++
		}
		fmt.Fprintf(&b, "**Features:** %d total (%d active, %d paused, %d done)\n\n",
			len(features), counts["active"], counts["paused"], counts["done"])
	}
	if s.currentSessionID != "" {
		fmt.Fprintf(&b, "**Session:** %s (active)\n", s.currentSessionID[:8])
	}
	return mcplib.NewToolResultText(b.String()), nil
}

func (s *DevMemServer) handleListFeatures(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	filter := getStringArg(req, "status_filter", "all")
	features, err := s.store.ListFeatures(filter)
	if err != nil {
		return respondErr("Failed to list features: %v", err)
	}
	if len(features) == 0 {
		return respond("No features found.")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Features (%s)\n\n", filter)
	for _, f := range features {
		fmt.Fprintf(&b, "## %s [%s]\n", f.Name, f.Status)
		if f.Description != "" {
			fmt.Fprintf(&b, "  %s\n", f.Description)
		}
		if f.Branch != "" {
			fmt.Fprintf(&b, "  Branch: %s\n", f.Branch)
		}
		fmt.Fprintf(&b, "  Last active: %s\n\n", f.LastActive)
	}
	return mcplib.NewToolResultText(b.String()), nil
}

func (s *DevMemServer) activateFeature(name, description string) (*memory.Feature, string, error) {
	if s.currentSessionID != "" {
		_ = s.store.EndSession(s.currentSessionID)
		s.currentSessionID = ""
	}
	feature, err := s.store.StartFeature(name, description)
	if err != nil {
		return nil, "", err
	}
	if sess, err := s.store.CreateSession(feature.ID, "mcp"); err == nil {
		s.currentSessionID = sess.ID
	}
	var ctxText string
	if ctxData, err := s.store.GetContext(feature.ID, "compact", nil); err == nil {
		ctxText = formatContext(ctxData)
	}
	return feature, ctxText, nil
}

func (s *DevMemServer) featureResponse(header string, feature *memory.Feature, ctxText string) (*mcplib.CallToolResult, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s: %s\n\n- Status: %s\n", header, feature.Name, feature.Status)
	if feature.Branch != "" {
		fmt.Fprintf(&b, "- Branch: %s\n", feature.Branch)
	}
	if feature.Description != "" {
		fmt.Fprintf(&b, "- Description: %s\n", feature.Description)
	}
	if ctxText != "" {
		b.WriteString("\n---\n")
		b.WriteString(ctxText)
	}
	return mcplib.NewToolResultText(b.String()), nil
}

func (s *DevMemServer) handleStartFeature(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	name, errRes := requireParam(req, "name")
	if errRes != nil {
		return errRes, nil
	}
	existing, _ := s.store.GetFeature(name)
	action := "Feature created"
	if existing != nil {
		action = "Feature resumed"
	}
	feature, ctxText, err := s.activateFeature(name, getStringArg(req, "description", ""))
	if err != nil {
		return respondErr("Failed to start feature: %v", err)
	}
	return s.featureResponse(action, feature, ctxText)
}

func (s *DevMemServer) handleSwitchFeature(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	name, errRes := requireParam(req, "name")
	if errRes != nil {
		return errRes, nil
	}
	feature, ctxText, err := s.activateFeature(name, "")
	if err != nil {
		return respondErr("Failed to switch feature: %v", err)
	}
	return s.featureResponse("Switched to feature", feature, ctxText)
}

func (s *DevMemServer) handleGetContext(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	feature, errRes := s.requireActiveFeature()
	if errRes != nil {
		return errRes, nil
	}
	tier := getStringArg(req, "tier", "standard")
	var asOf *time.Time
	if asOfStr := getStringArg(req, "as_of", ""); asOfStr != "" {
		t, errR := parseTimestamp(asOfStr, "as_of")
		if errR != nil {
			return errR, nil
		}
		asOf = &t
	}
	ctxData, err := s.store.GetContext(feature.ID, tier, asOf)
	if err != nil {
		return respondErr("Failed to get context: %v", err)
	}
	return mcplib.NewToolResultText(formatContext(ctxData)), nil
}

func (s *DevMemServer) handleSync(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	feature, errRes := s.requireActiveFeature()
	if errRes != nil {
		return errRes, nil
	}
	since := time.Now().AddDate(0, 0, -7)
	if sinceStr := getStringArg(req, "since", ""); sinceStr != "" {
		t, errR := parseTimestamp(sinceStr, "since")
		if errR != nil {
			return errR, nil
		}
		since = t
	}
	result, err := git.SyncCommits(s.db, s.gitRoot, feature.ID, s.currentSessionID, since)
	if err != nil {
		return respondErr("Failed to sync commits: %v", err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Sync complete\n\n**New commits:** %d\n\n", result.NewCommits)
	planUpdates := 0
	for _, c := range result.Commits {
		fmt.Fprintf(&b, "- `%s` %s [%s]\n", c.Hash[:7], c.Message, c.IntentType)
		if ms, err := s.planManager.MatchCommitToSteps(c.Message, feature.ID); err == nil && ms != nil {
			_ = s.planManager.UpdateStepStatus(ms.ID, "completed")
			_ = s.planManager.LinkCommitToStep(ms.ID, c.Hash)
			fmt.Fprintf(&b, "  -> Completed plan step: %s\n", ms.Title)
			planUpdates++
		}
	}
	if planUpdates > 0 {
		fmt.Fprintf(&b, "\n**Plan steps completed:** %d\n", planUpdates)
	}
	return mcplib.NewToolResultText(b.String()), nil
}

func (s *DevMemServer) handleRemember(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	content, errRes := requireParam(req, "content")
	if errRes != nil {
		return errRes, nil
	}
	feature, errRes := s.requireActiveFeature()
	if errRes != nil {
		return errRes, nil
	}
	noteType := getStringArg(req, "type", "note")
	note, err := s.store.CreateNote(feature.ID, s.currentSessionID, content, noteType)
	if err != nil {
		return respondErr("Failed to save note: %v", err)
	}
	linksCreated, _ := s.store.AutoLink(note.ID, "note", content)
	var b strings.Builder
	fmt.Fprintf(&b, "# Remembered (%s)\n\n- ID: %s\n- Links created: %d\n", noteType, note.ID[:8], linksCreated)
	if plans.IsPlanLike(content) {
		if steps := plans.ParseSteps(content); len(steps) > 0 {
			plan, err := s.planManager.CreatePlan(feature.ID, s.currentSessionID,
				fmt.Sprintf("Plan from note %s", note.ID[:8]), content, "devmem_remember", steps)
			if err == nil {
				fmt.Fprintf(&b, "\n**Auto-promoted to plan:** %s (%d steps)\n", plan.Title, len(steps))
			}
		}
	}
	return mcplib.NewToolResultText(b.String()), nil
}

func (s *DevMemServer) handleSearch(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	query, errRes := requireParam(req, "query")
	if errRes != nil {
		return errRes, nil
	}
	scope := getStringArg(req, "scope", "current_feature")
	types := getStringSliceArg(req, "types")
	var featureID string
	if scope == "current_feature" {
		feature, err := s.store.GetActiveFeature()
		if err != nil {
			return respondErr("No active feature. Use scope='all_features' or start a feature first.")
		}
		featureID = feature.ID
	}
	results, err := s.searchEngine.Search(query, scope, types, featureID, 20)
	if err != nil {
		return respondErr("Search failed: %v", err)
	}
	if len(results) == 0 {
		return respond("No results found for: %s", query)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Search results for %q (%d, scope:%s)\n", query, len(results), scope)
	for _, r := range results {
		feat := ""
		if r.FeatureName != "" {
			feat = " " + r.FeatureName
		}
		fmt.Fprintf(&b, "[%s] %q (%.2f)%s\n", r.Type, truncate(r.Content, 100), r.Relevance, feat)
	}
	return mcplib.NewToolResultText(b.String()), nil
}

func (s *DevMemServer) handleSavePlan(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	title, errRes := requireParam(req, "title")
	if errRes != nil {
		return errRes, nil
	}
	feature, errRes := s.requireActiveFeature()
	if errRes != nil {
		return errRes, nil
	}
	args := req.GetArguments()
	stepsRaw, ok := args["steps"]
	if !ok {
		return mcplib.NewToolResultError("Parameter 'steps' is required"), nil
	}
	stepsArr, ok := stepsRaw.([]interface{})
	if !ok {
		return mcplib.NewToolResultError("Parameter 'steps' must be an array of objects with 'title' and optional 'description'"), nil
	}
	steps := parseStepInputs(stepsArr)
	if len(steps) == 0 {
		return mcplib.NewToolResultError("At least one step with a 'title' is required"), nil
	}
	var supersededInfo string
	if oldPlan, err := s.planManager.GetActivePlan(feature.ID); err == nil {
		oldSteps, _ := s.planManager.GetPlanSteps(oldPlan.ID)
		supersededInfo = fmt.Sprintf("\n**Superseded:** %s (%d/%d steps completed)\n", oldPlan.Title, countCompleted(oldSteps), len(oldSteps))
	}
	plan, err := s.planManager.CreatePlan(feature.ID, s.currentSessionID, title, getStringArg(req, "content", ""), "devmem_save_plan", steps)
	if err != nil {
		return respondErr("Failed to create plan: %v", err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Plan saved: %s\n\n- ID: %s\n- Steps: %d\n%s\n**Steps:**\n", plan.Title, plan.ID[:8], len(steps), supersededInfo)
	for i, st := range steps {
		fmt.Fprintf(&b, "  %d. %s\n", i+1, st.Title)
	}
	return mcplib.NewToolResultText(b.String()), nil
}

func (s *DevMemServer) handleImportSession(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	featureName, errRes := requireParam(req, "feature_name")
	if errRes != nil {
		return errRes, nil
	}
	if s.currentSessionID != "" {
		_ = s.store.EndSession(s.currentSessionID)
	}
	feature, err := s.store.StartFeature(featureName, getStringArg(req, "description", ""))
	if err != nil {
		return respondErr("Failed to start feature: %v", err)
	}
	sess, err := s.store.CreateSession(feature.ID, "import")
	if err != nil {
		return respondErr("Failed to create session: %v", err)
	}
	s.currentSessionID = sess.ID
	var b strings.Builder
	fmt.Fprintf(&b, "# Importing session into: %s\n\n", featureName)
	imported := 0
	for _, nt := range []struct{ arg, noteType, label string }{
		{"decisions", "decision", "Decisions"},
		{"progress_notes", "progress", "Progress notes"},
		{"blockers", "blocker", "Blockers"},
		{"next_steps", "next_step", "Next steps"},
	} {
		notes := getStringSliceArg(req, nt.arg)
		for _, n := range notes {
			if _, err := s.store.CreateNote(feature.ID, sess.ID, n, nt.noteType); err == nil {
				imported++
			}
		}
		if len(notes) > 0 {
			fmt.Fprintf(&b, "- %s imported: %d\n", nt.label, len(notes))
		}
	}
	args := req.GetArguments()
	if factsArr, ok := args["facts"].([]interface{}); ok {
		factCount := 0
		for _, item := range factsArr {
			if m, ok := item.(map[string]interface{}); ok {
				subj, _ := m["subject"].(string)
				pred, _ := m["predicate"].(string)
				obj, _ := m["object"].(string)
				if subj != "" && pred != "" && obj != "" {
					if _, err := s.store.CreateFact(feature.ID, sess.ID, subj, pred, obj); err == nil {
						factCount++
						imported++
					}
				}
			}
		}
		if factCount > 0 {
			fmt.Fprintf(&b, "- Facts imported: %d\n", factCount)
		}
	}
	if planTitle := getStringArg(req, "plan_title", ""); planTitle != "" {
		if planStepsArr, ok := args["plan_steps"].([]interface{}); ok {
			imported += s.importPlan(&b, feature.ID, sess.ID, planTitle, planStepsArr)
		}
	}
	linksCreated := 0
	if imported > 0 {
		notes, _ := s.store.ListNotes(feature.ID, "", imported)
		for _, n := range notes {
			count, _ := s.store.AutoLink(n.ID, "note", n.Content)
			linksCreated += count
		}
	}
	fmt.Fprintf(&b, "\n**Total imported:** %d items, %d links created\n\nMemory is now bootstrapped. Future sessions will have this context.", imported, linksCreated)
	return mcplib.NewToolResultText(b.String()), nil
}

func (s *DevMemServer) importPlan(b *strings.Builder, featureID, sessionID, planTitle string, planStepsArr []interface{}) int {
	var steps []plans.StepInput
	var completedTitles []string
	for _, item := range planStepsArr {
		if m, ok := item.(map[string]interface{}); ok {
			t, _ := m["title"].(string)
			d, _ := m["description"].(string)
			st, _ := m["status"].(string)
			if t != "" {
				steps = append(steps, plans.StepInput{Title: t, Description: d})
				if st == "completed" {
					completedTitles = append(completedTitles, t)
				}
			}
		}
	}
	if len(steps) == 0 {
		return 0
	}
	plan, err := s.planManager.CreatePlan(featureID, sessionID, planTitle, "", "import", steps)
	if err != nil {
		return 0
	}
	planSteps, _ := s.planManager.GetPlanSteps(plan.ID)
	for _, ps := range planSteps {
		for _, ct := range completedTitles {
			if ps.Title == ct {
				_ = s.planManager.UpdateStepStatus(ps.ID, "completed")
			}
		}
	}
	fmt.Fprintf(b, "- Plan imported: %s (%d steps, %d completed)\n", planTitle, len(steps), len(completedTitles))
	return len(steps)
}

func (s *DevMemServer) handleEndSession(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	summary, errRes := requireParam(req, "summary")
	if errRes != nil {
		return errRes, nil
	}
	if s.currentSessionID == "" {
		return mcplib.NewToolResultError("No active session to end."), nil
	}
	feature, errRes := s.requireActiveFeature()
	if errRes != nil {
		return errRes, nil
	}
	if err := s.store.EndSessionWithSummary(s.currentSessionID, summary); err != nil {
		return respondErr("Failed to end session: %v", err)
	}
	note, err := s.store.CreateNote(feature.ID, s.currentSessionID, summary, "progress")
	if err != nil {
		return respondErr("Failed to create progress note: %v", err)
	}
	linksCreated, _ := s.store.AutoLink(note.ID, "note", summary)
	sessionID := s.currentSessionID
	s.currentSessionID = ""
	return respond("# Session ended\n\n- Session: %s\n- Summary saved: %s\n- Progress note created: %s\n- Links created: %d\n\nThe next session will see this summary in its context.",
		sessionID[:8], truncate(summary, 80), note.ID[:8], linksCreated)
}

func (s *DevMemServer) handleExport(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	featureName := getStringArg(req, "feature_name", "")
	var feature *memory.Feature
	var err error
	if featureName != "" {
		if feature, err = s.store.GetFeature(featureName); err != nil {
			return respondErr("Feature '%s' not found", featureName)
		}
	} else if feature, err = s.store.GetActiveFeature(); err != nil {
		return mcplib.NewToolResultError("No active feature. Specify feature_name or start a feature."), nil
	}
	ctx, err := s.store.GetContext(feature.ID, "detailed", nil)
	if err != nil {
		return respondErr("Failed to get context: %v", err)
	}
	if getStringArg(req, "format", "markdown") == "json" {
		return respond("{\n  \"feature\": \"%s\",\n  \"status\": \"%s\",\n  \"branch\": \"%s\",\n  \"description\": \"%s\",\n  \"commits\": %d,\n  \"notes\": %d,\n  \"facts\": %d,\n  \"sessions\": %d\n}",
			feature.Name, feature.Status, feature.Branch, feature.Description,
			len(ctx.RecentCommits), len(ctx.RecentNotes), len(ctx.ActiveFacts), len(ctx.SessionHistory))
	}
	return s.exportMarkdown(feature, ctx)
}

func (s *DevMemServer) exportMarkdown(feature *memory.Feature, ctx *memory.Context) (*mcplib.CallToolResult, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "# Feature: %s\n\n**Status:** %s\n", feature.Name, feature.Status)
	if feature.Branch != "" {
		fmt.Fprintf(&b, "**Branch:** %s\n", feature.Branch)
	}
	if feature.Description != "" {
		fmt.Fprintf(&b, "**Description:** %s\n", feature.Description)
	}
	fmt.Fprintf(&b, "**Created:** %s\n**Last Active:** %s\n\n", feature.CreatedAt, feature.LastActive)
	if ctx.Plan != nil {
		fmt.Fprintf(&b, "## Plan: %s\n\nProgress: %d/%d steps\n\n", ctx.Plan.Title, ctx.Plan.CompletedStep, ctx.Plan.TotalSteps)
		if activePlan, err := s.planManager.GetActivePlan(feature.ID); err == nil {
			planSteps, _ := s.planManager.GetPlanSteps(activePlan.ID)
			checks := map[string]string{"completed": "[x]", "in_progress": "[-]"}
			for _, st := range planSteps {
				c := checks[st.Status]
				if c == "" {
					c = "[ ]"
				}
				fmt.Fprintf(&b, "- %s %s\n", c, st.Title)
			}
		}
		b.WriteString("\n")
	}
	for _, sec := range []struct{ noteType, title, emptyMsg string }{
		{"decision", "Decisions", "_No decisions recorded._"},
		{"progress", "Progress Notes", "_No progress notes._"},
		{"blocker", "Blockers", ""},
	} {
		notes, _ := s.store.ListNotes(feature.ID, sec.noteType, 50)
		if len(notes) == 0 && sec.emptyMsg == "" {
			continue
		}
		fmt.Fprintf(&b, "## %s\n\n", sec.title)
		if len(notes) == 0 {
			b.WriteString(sec.emptyMsg + "\n\n")
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

func writeContextSection[T any](b *strings.Builder, title string, items []T, format func(T) string) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(b, "%s:", title)
	for _, item := range items {
		fmt.Fprintf(b, " %s;", format(item))
	}
	b.WriteString("\n")
}

func formatContext(ctx *memory.Context) string {
	var b strings.Builder
	if ctx.Feature != nil {
		fmt.Fprintf(&b, "%s [%s]", ctx.Feature.Name, ctx.Feature.Status)
		if ctx.Feature.Branch != "" {
			fmt.Fprintf(&b, " branch:%s", ctx.Feature.Branch)
		}
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
	writeContextSection(&b, "Commits", ctx.RecentCommits, func(c memory.CommitInfo) string {
		return fmt.Sprintf("%s %s", c.Hash[:min(7, len(c.Hash))], c.Message)
	})
	writeContextSection(&b, "Notes", ctx.RecentNotes, func(n memory.Note) string {
		return fmt.Sprintf("[%s] %s", n.Type, truncate(n.Content, 100))
	})
	writeContextSection(&b, "Facts", ctx.ActiveFacts, func(f memory.Fact) string {
		return fmt.Sprintf("%s %s %s", f.Subject, f.Predicate, f.Object)
	})
	writeContextSection(&b, "Sessions", ctx.SessionHistory, func(sess memory.Session) string {
		end := sess.EndedAt
		if end == "" {
			end = "active"
		}
		return fmt.Sprintf("%s %s->%s %s", sess.ID[:8], sess.StartedAt, end, sess.Tool)
	})
	writeContextSection(&b, "Links", ctx.Links, func(l memory.MemoryLink) string {
		return fmt.Sprintf("%s:%s->%s:%s[%s,%.1f]", l.SourceType, l.SourceID[:8], l.TargetType, l.TargetID[:8], l.Relationship, l.Strength)
	})
	writeContextSection(&b, "Files", ctx.FilesTouched, func(f string) string { return f })
	return b.String()
}

func (s *DevMemServer) handleAnalytics(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	if featureName := getStringArg(req, "feature", ""); featureName != "" {
		return s.handleFeatureAnalytics(featureName)
	}
	return s.handleProjectAnalytics()
}

func (s *DevMemServer) handleFeatureAnalytics(featureName string) (*mcplib.CallToolResult, error) {
	feature, err := s.store.GetFeature(featureName)
	if err != nil {
		return respondErr("Feature '%s' not found", featureName)
	}
	a, err := s.store.GetFeatureAnalytics(feature.ID)
	if err != nil {
		return respondErr("Failed to get analytics: %v", err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Feature Analytics: %s\n\n**Age:** %d days (last active %d days ago)\n**Avg session duration:** %s\n\n## Activity Counts\n\n",
		a.Name, a.DaysSinceCreated, a.DaysSinceLastActive, a.AvgSessionDuration)
	b.WriteString(mdTable("Metric", "Count", [][2]string{
		{"Sessions", itoa(a.SessionCount)}, {"Commits", itoa(a.CommitCount)},
		{"Notes", itoa(a.NoteCount)}, {"Decisions", itoa(a.DecisionCount)},
		{"Blockers", itoa(a.BlockerCount)}, {"Facts (active)", itoa(a.ActiveFactCount)},
		{"Facts (invalidated)", itoa(a.InvalidatedFactCount)},
	}))
	fmt.Fprintf(&b, "\n## Plan Progress\n\n%s\n", a.PlanProgress)
	if len(a.IntentBreakdown) > 0 {
		b.WriteString("\n## Commit Intent Breakdown\n\n")
		var rows [][2]string
		for intent, count := range a.IntentBreakdown {
			rows = append(rows, [2]string{intent, itoa(count)})
		}
		b.WriteString(mdTable("Intent", "Count", rows))
	}
	return mcplib.NewToolResultText(b.String()), nil
}

func (s *DevMemServer) handleProjectAnalytics() (*mcplib.CallToolResult, error) {
	a, err := s.store.GetProjectAnalytics()
	if err != nil {
		return respondErr("Failed to get analytics: %v", err)
	}
	var b strings.Builder
	b.WriteString("# Project Analytics\n\n## Features\n\n")
	b.WriteString(mdTable("Status", "Count", [][2]string{
		{"Total", itoa(a.TotalFeatures)}, {"Active", itoa(a.ActiveFeatures)},
		{"Paused", itoa(a.PausedFeatures)}, {"Done", itoa(a.DoneFeatures)},
	}))
	b.WriteString("\n## Totals\n\n")
	b.WriteString(mdTable("Metric", "Count", [][2]string{
		{"Sessions", itoa(a.TotalSessions)}, {"Commits", itoa(a.TotalCommits)},
		{"Notes", itoa(a.TotalNotes)}, {"Facts", itoa(a.TotalFacts)},
	}))
	if a.MostActiveFeature != "" {
		fmt.Fprintf(&b, "\n**Most active feature:** %s\n", a.MostActiveFeature)
	}
	if a.MostBlockedFeature != "" {
		fmt.Fprintf(&b, "**Most blocked feature:** %s\n", a.MostBlockedFeature)
	}
	if len(a.RecentActivity) > 0 {
		b.WriteString("\n## Recent Activity\n\n")
		for _, activity := range a.RecentActivity {
			fmt.Fprintf(&b, "- %s\n", activity)
		}
	}
	return mcplib.NewToolResultText(b.String()), nil
}

func (s *DevMemServer) handleHealth(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	featureName := getStringArg(req, "feature", "")
	featureID, errRes := s.resolveFeatureID(featureName)
	if errRes != nil {
		return errRes, nil
	}
	h, err := s.store.GetMemoryHealth(featureID)
	if err != nil {
		return respondErr("Failed to get memory health: %v", err)
	}
	scope := "All Features"
	if featureName != "" {
		scope = featureName
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Memory Health: %s\n\n**Score: %.0f/100**\n\n## Metrics\n\n", scope, h.Score)
	b.WriteString(mdTable("Metric", "Count", [][2]string{
		{"Total memories", itoa(h.TotalMemories)}, {"Active facts", itoa(h.ActiveFacts)},
		{"Stale facts", itoa(h.StaleFactCount)}, {"Conflicts", itoa(h.ConflictCount)},
		{"Orphan notes", itoa(h.OrphanNoteCount)}, {"Stale notes", itoa(h.StaleNoteCount)},
		{"Summaries", itoa(h.SummaryCount)},
	}))
	if len(h.Suggestions) > 0 {
		b.WriteString("\n## Suggestions\n\n")
		for _, suggestion := range h.Suggestions {
			fmt.Fprintf(&b, "- %s\n", suggestion)
		}
	} else {
		b.WriteString("\nMemory is healthy. No issues found.\n")
	}
	return mcplib.NewToolResultText(b.String()), nil
}

func (s *DevMemServer) handleForget(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	what, errRes := requireParam(req, "what")
	if errRes != nil {
		return errRes, nil
	}
	featureID, errRes := s.resolveFeatureID(getStringArg(req, "feature", ""))
	if errRes != nil {
		return errRes, nil
	}
	var deleted int
	var err error
	var msg string
	switch what {
	case "stale_facts":
		deleted, err = s.store.ForgetStaleFacts(featureID)
		msg = fmt.Sprintf("Deleted %d stale facts (invalidated 30+ days ago).", deleted)
	case "stale_notes":
		deleted, err = s.store.ForgetStaleNotes(featureID)
		msg = fmt.Sprintf("Deleted %d stale notes (60+ days old, no links).", deleted)
	case "completed_features":
		deleted, err = s.store.ForgetCompletedFeatures()
		msg = fmt.Sprintf("Deleted %d completed features (done 90+ days ago).", deleted)
	default:
		typ, err := s.store.ForgetByID(what)
		if err != nil {
			return respondErr("Failed to forget: %v", err)
		}
		return respond("Deleted %s with ID %s.", typ, what)
	}
	if err != nil {
		return respondErr("Failed to forget %s: %v", what, err)
	}
	return respond("%s", msg)
}

func (s *DevMemServer) handleGenerateRules(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	output := getStringArg(req, "output", "")
	dryRun := getBoolArg(req, "dry_run", false)
	content, err := s.store.GenerateAgentsMD()
	if err != nil {
		return respondErr("Failed to generate AGENTS.md: %v", err)
	}
	if dryRun {
		return respond("# Preview (dry run)\n\n%s", content)
	}
	if output == "" {
		output = s.gitRoot + "/AGENTS.md"
	}
	if err := os.WriteFile(output, []byte(content), 0644); err != nil {
		return respondErr("Failed to write %s: %v", output, err)
	}
	return respond("Generated %s from memory.\n\n%s", output, content)
}

func formatBriefing(ctx *memory.Context, feature *memory.Feature) string {
	if feature == nil {
		return "devmem: No active feature. Use devmem_start_feature to begin."
	}
	var lines []string
	featureLine := fmt.Sprintf("devmem: Welcome back! Active feature: %s", feature.Name)
	if feature.Branch != "" {
		featureLine += fmt.Sprintf(" [%s]", feature.Branch)
	}
	lines = append(lines, featureLine)
	if ctx.Plan != nil {
		lines = append(lines, fmt.Sprintf("devmem: plan: %s (%d/%d steps done)", ctx.Plan.Title, ctx.Plan.CompletedStep, ctx.Plan.TotalSteps))
	}
	if len(ctx.SessionHistory) > 0 {
		last := ctx.SessionHistory[0]
		lines = append(lines, fmt.Sprintf("devmem: last: %s via %s", formatTimeAgo(last.StartedAt), last.Tool))
	}
	if len(ctx.RecentNotes) > 0 {
		lines = append(lines, fmt.Sprintf("devmem: recent: \"%s\"", truncate(ctx.RecentNotes[0].Content, 80)))
	}
	lines = append(lines, "devmem: tip: say \"where did I leave off?\" for full context")
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

func (s *DevMemServer) handleBriefing(_ context.Context, _ mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	feature, err := s.store.GetActiveFeature()
	if err != nil {
		return mcplib.NewToolResultText("devmem: No active feature. Use devmem_start_feature to begin."), nil
	}
	ctxData, err := s.store.GetContext(feature.ID, "standard", nil)
	if err != nil {
		return respondErr("Failed to load context: %v", err)
	}
	sessions, _ := s.store.ListSessions(feature.ID, 5)
	ctxData.SessionHistory = sessions
	return mcplib.NewToolResultText(formatBriefing(ctxData, feature)), nil
}
