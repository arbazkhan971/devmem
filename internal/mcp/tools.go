package mcp

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/arbazkhan971/memorx/internal/git"
	"github.com/arbazkhan971/memorx/internal/memory"
	"github.com/arbazkhan971/memorx/internal/plans"
	"github.com/arbazkhan971/memorx/internal/search"
	mcplib "github.com/mark3labs/mcp-go/mcp"
)

func respond(f string, a ...interface{}) (*mcplib.CallToolResult, error) {
	return mcplib.NewToolResultText(fmt.Sprintf(f, a...)), nil
}
func respondErr(f string, a ...interface{}) (*mcplib.CallToolResult, error) {
	return mcplib.NewToolResultError(fmt.Sprintf(f, a...)), nil
}
func (s *DevMemServer) requireActiveFeature() (*memory.Feature, *mcplib.CallToolResult) {
	f, err := s.store.GetActiveFeature()
	if err != nil {
		return nil, mcplib.NewToolResultError("No active feature. Use memorx_start_feature first.")
	}
	return f, nil
}
func requireParam(req mcplib.CallToolRequest, name string) (string, *mcplib.CallToolResult) {
	if v := getStringArg(req, name, ""); v != "" {
		return v, nil
	}
	return "", mcplib.NewToolResultError(fmt.Sprintf("Parameter '%s' is required", name))
}
func mdTable(h1, h2 string, rows [][2]string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "| %s | %s |\n|--------|-------|\n", h1, h2)
	for _, r := range rows {
		fmt.Fprintf(&b, "| %s | %s |\n", r[0], r[1])
	}
	return b.String()
}
func getBoolArg(req mcplib.CallToolRequest, name string, fb bool) bool {
	if a := req.GetArguments(); a != nil {
		if v, ok := a[name].(bool); ok {
			return v
		}
	}
	return fb
}
func getStringArg(req mcplib.CallToolRequest, name, fb string) string {
	if a := req.GetArguments(); a != nil {
		if v, ok := a[name].(string); ok && v != "" {
			return v
		}
	}
	return fb
}
func getStringSliceArg(req mcplib.CallToolRequest, name string) []string {
	if a := req.GetArguments(); a != nil {
		if arr, ok := a[name].([]interface{}); ok {
			var r []string
			for _, item := range arr {
				if s, ok := item.(string); ok {
					r = append(r, s)
				}
			}
			return r
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

func (s *DevMemServer) handleStatus(_ context.Context, _ mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "# memorx status — %s\n\n", git.ProjectName(s.gitRoot))
	feature, err := s.store.GetActiveFeature()
	if err != nil {
		b.WriteString("**Active feature:** none\n\n")
	} else {
		fmt.Fprintf(&b, "**Active feature:** %s\n  - Status: %s\n", feature.Name, feature.Status)
		optField(&b, feature.Branch != "", "  - Branch: %s\n", feature.Branch)
		optField(&b, feature.Description != "", "  - Description: %s\n", feature.Description)
		fmt.Fprintf(&b, "  - Last active: %s\n\n", feature.LastActive)
		if plan, err := s.planManager.GetActivePlan(feature.ID); err == nil {
			steps, _ := s.planManager.GetPlanSteps(plan.ID)
			fmt.Fprintf(&b, "**Active plan:** %s\n  - Progress: %d/%d steps completed\n\n", plan.Title, countCompleted(steps), len(steps))
		} else {
			b.WriteString("**Active plan:** none\n\n")
		}
	}
	if features, err := s.store.ListFeatures("all"); err == nil {
		c := map[string]int{}
		for _, f := range features {
			c[f.Status]++
		}
		fmt.Fprintf(&b, "**Features:** %d total (%d active, %d paused, %d done)\n\n", len(features), c["active"], c["paused"], c["done"])
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
		optField(&b, f.Description != "", "  %s\n", f.Description)
		optField(&b, f.Branch != "", "  Branch: %s\n", f.Branch)
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
	ctxText := ""
	if ctxData, err := s.store.GetContext(feature.ID, "compact", nil); err == nil {
		ctxText = formatContext(ctxData)
	}
	return feature, ctxText, nil
}

func (s *DevMemServer) featureResponse(header string, feature *memory.Feature, ctxText string) (*mcplib.CallToolResult, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s: %s\n\n- Status: %s\n", header, feature.Name, feature.Status)
	optField(&b, feature.Branch != "", "- Branch: %s\n", feature.Branch)
	optField(&b, feature.Description != "", "- Description: %s\n", feature.Description)
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
	pu := 0
	for _, c := range result.Commits {
		fmt.Fprintf(&b, "- `%s` %s [%s]\n", c.Hash[:7], c.Message, c.IntentType)
		if ms, err := s.planManager.MatchCommitToSteps(c.Message, feature.ID); err == nil && ms != nil {
			_ = s.planManager.UpdateStepStatus(ms.ID, "completed")
			_ = s.planManager.LinkCommitToStep(ms.ID, c.Hash)
			fmt.Fprintf(&b, "  -> Completed plan step: %s\n", ms.Title)
			pu++
		}
	}
	if pu > 0 {
		fmt.Fprintf(&b, "\n**Plan steps completed:** %d\n", pu)
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
	lc, _ := s.store.AutoLink(note.ID, "note", content)
	resp := fmt.Sprintf("# Remembered (%s)\n\n- ID: %s\n- Links created: %d\n", noteType, note.ID[:8], lc)
	if plans.IsPlanLike(content) {
		if steps := plans.ParseSteps(content); len(steps) > 0 {
			if plan, err := s.planManager.CreatePlan(feature.ID, s.currentSessionID, fmt.Sprintf("Plan from note %s", note.ID[:8]), content, "memorx_remember", steps); err == nil {
				resp += fmt.Sprintf("\n**Auto-promoted to plan:** %s (%d steps)\n", plan.Title, len(steps))
			}
		}
	}
	return mcplib.NewToolResultText(resp), nil
}

func (s *DevMemServer) handleSearch(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	query, errRes := requireParam(req, "query")
	if errRes != nil {
		return errRes, nil
	}
	scope, types := getStringArg(req, "scope", "current_feature"), getStringSliceArg(req, "types")
	var featureID string
	if scope == "current_feature" {
		f, err := s.store.GetActiveFeature()
		if err != nil {
			return respondErr("No active feature. Use scope='all_features' or start a feature first.")
		}
		featureID = f.ID
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

func (s *DevMemServer) handleHistory(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	query, errRes := requireParam(req, "query")
	if errRes != nil {
		return errRes, nil
	}

	daysBack := 30
	if a := req.GetArguments(); a != nil {
		if v, ok := a["days_back"].(float64); ok && v > 0 {
			daysBack = int(v)
		}
	}

	types := getStringSliceArg(req, "types")
	// Map user-friendly type names to internal table names
	if len(types) > 0 {
		mapped := make([]string, 0, len(types))
		typeMap := map[string]string{
			"decisions": "notes",
			"progress":  "notes",
			"blockers":  "notes",
			"facts":     "facts",
			"commits":   "commits",
			"notes":     "notes",
			"plans":     "plans",
		}
		seen := map[string]bool{}
		for _, t := range types {
			if m, ok := typeMap[t]; ok && !seen[m] {
				mapped = append(mapped, m)
				seen[m] = true
			}
		}
		if len(mapped) > 0 {
			types = mapped
		}
	}

	since := time.Now().UTC().AddDate(0, 0, -daysBack)
	opts := &search.SearchOpts{Since: &since}
	results, err := s.searchEngine.SearchWithOpts(query, "all_features", types, "", 50, opts)
	if err != nil {
		return respondErr("History search failed: %v", err)
	}
	if len(results) == 0 {
		return respond("No history found for %q in the last %d days.", query, daysBack)
	}

	// Sort chronologically (oldest first)
	search.SortByTimeAsc(results)

	var b strings.Builder
	fmt.Fprintf(&b, "# History: %q (last %d days, %d results)\n\n", query, daysBack, len(results))
	for _, r := range results {
		feat := ""
		if r.FeatureName != "" {
			feat = fmt.Sprintf(" [%s]", r.FeatureName)
		}
		fmt.Fprintf(&b, "- **%s** %s — %s%s\n", r.CreatedAt, r.Type, truncate(r.Content, 120), feat)
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
	superseded := ""
	if old, err := s.planManager.GetActivePlan(feature.ID); err == nil {
		os, _ := s.planManager.GetPlanSteps(old.ID)
		superseded = fmt.Sprintf("\n**Superseded:** %s (%d/%d steps completed)\n", old.Title, countCompleted(os), len(os))
	}
	plan, err := s.planManager.CreatePlan(feature.ID, s.currentSessionID, title, getStringArg(req, "content", ""), "memorx_save_plan", steps)
	if err != nil {
		return respondErr("Failed to create plan: %v", err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Plan saved: %s\n\n- ID: %s\n- Steps: %d\n%s\n**Steps:**\n", plan.Title, plan.ID[:8], len(steps), superseded)
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
	for _, nt := range []struct{ a, t, l string }{
		{"decisions", "decision", "Decisions"}, {"progress_notes", "progress", "Progress notes"},
		{"blockers", "blocker", "Blockers"}, {"next_steps", "next_step", "Next steps"},
	} {
		notes := getStringSliceArg(req, nt.a)
		for _, n := range notes {
			if _, err := s.store.CreateNote(feature.ID, sess.ID, n, nt.t); err == nil {
				imported++
			}
		}
		if len(notes) > 0 {
			fmt.Fprintf(&b, "- %s imported: %d\n", nt.l, len(notes))
		}
	}
	if factsArr, ok := req.GetArguments()["facts"].([]interface{}); ok {
		fc := 0
		for _, item := range factsArr {
			if m, ok := item.(map[string]interface{}); ok {
				subj, _ := m["subject"].(string)
				pred, _ := m["predicate"].(string)
				obj, _ := m["object"].(string)
				if subj != "" && pred != "" && obj != "" {
					if _, err := s.store.CreateFact(feature.ID, sess.ID, subj, pred, obj); err == nil {
						fc++
						imported++
					}
				}
			}
		}
		if fc > 0 {
			fmt.Fprintf(&b, "- Facts imported: %d\n", fc)
		}
	}
	if pt := getStringArg(req, "plan_title", ""); pt != "" {
		if pa, ok := req.GetArguments()["plan_steps"].([]interface{}); ok {
			imported += s.importPlan(&b, feature.ID, sess.ID, pt, pa)
		}
	}
	lc := 0
	if imported > 0 {
		notes, _ := s.store.ListNotes(feature.ID, "", imported)
		for _, n := range notes {
			c, _ := s.store.AutoLink(n.ID, "note", n.Content)
			lc += c
		}
	}
	fmt.Fprintf(&b, "\n**Total imported:** %d items, %d links created\n\nMemory is now bootstrapped. Future sessions will have this context.", imported, lc)
	return mcplib.NewToolResultText(b.String()), nil
}

func (s *DevMemServer) importPlan(b *strings.Builder, fID, sID, title string, raw []interface{}) int {
	var steps []plans.StepInput
	var done []string
	for _, item := range raw {
		if m, ok := item.(map[string]interface{}); ok {
			t, _ := m["title"].(string)
			d, _ := m["description"].(string)
			st, _ := m["status"].(string)
			if t != "" {
				steps = append(steps, plans.StepInput{Title: t, Description: d})
				if st == "completed" {
					done = append(done, t)
				}
			}
		}
	}
	if len(steps) == 0 {
		return 0
	}
	plan, err := s.planManager.CreatePlan(fID, sID, title, "", "import", steps)
	if err != nil {
		return 0
	}
	ps, _ := s.planManager.GetPlanSteps(plan.ID)
	for _, p := range ps {
		for _, ct := range done {
			if p.Title == ct {
				_ = s.planManager.UpdateStepStatus(p.ID, "completed")
			}
		}
	}
	fmt.Fprintf(b, "- Plan imported: %s (%d steps, %d completed)\n", title, len(steps), len(done))
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
	lc, _ := s.store.AutoLink(note.ID, "note", summary)
	sid := s.currentSessionID
	s.currentSessionID = ""
	return respond("# Session ended\n\n- Session: %s\n- Summary saved: %s\n- Progress note created: %s\n- Links created: %d\n\nThe next session will see this summary in its context.", sid[:8], truncate(summary, 80), note.ID[:8], lc)
}

func (s *DevMemServer) handleExport(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	fn := getStringArg(req, "feature_name", "")
	var feature *memory.Feature
	var err error
	if fn != "" {
		if feature, err = s.store.GetFeature(fn); err != nil {
			return respondErr("Feature '%s' not found", fn)
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

func (s *DevMemServer) handleAnalytics(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	if fn := getStringArg(req, "feature", ""); fn != "" {
		f, err := s.store.GetFeature(fn)
		if err != nil {
			return respondErr("Feature '%s' not found", fn)
		}
		a, err := s.store.GetFeatureAnalytics(f.ID)
		if err != nil {
			return respondErr("Failed to get analytics: %v", err)
		}
		var b strings.Builder
		fmt.Fprintf(&b, "# Feature Analytics: %s\n\n**Age:** %d days (last active %d days ago)\n**Avg session duration:** %s\n\n## Activity Counts\n\n", a.Name, a.DaysSinceCreated, a.DaysSinceLastActive, a.AvgSessionDuration)
		b.WriteString(mdTable("Metric", "Count", [][2]string{
			{"Sessions", itoa(a.SessionCount)}, {"Commits", itoa(a.CommitCount)}, {"Notes", itoa(a.NoteCount)},
			{"Decisions", itoa(a.DecisionCount)}, {"Blockers", itoa(a.BlockerCount)},
			{"Facts (active)", itoa(a.ActiveFactCount)}, {"Facts (invalidated)", itoa(a.InvalidatedFactCount)},
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
	optField(&b, a.MostActiveFeature != "", "\n**Most active feature:** %s\n", a.MostActiveFeature)
	optField(&b, a.MostBlockedFeature != "", "**Most blocked feature:** %s\n", a.MostBlockedFeature)
	if len(a.RecentActivity) > 0 {
		b.WriteString("\n## Recent Activity\n\n")
		for _, act := range a.RecentActivity {
			fmt.Fprintf(&b, "- %s\n", act)
		}
	}
	return mcplib.NewToolResultText(b.String()), nil
}

func (s *DevMemServer) handleHealth(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	fn := getStringArg(req, "feature", "")
	fID, errRes := s.resolveFeatureID(fn)
	if errRes != nil {
		return errRes, nil
	}
	h, err := s.store.GetMemoryHealth(fID)
	if err != nil {
		return respondErr("Failed to get memory health: %v", err)
	}
	scope := "All Features"
	if fn != "" {
		scope = fn
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
		for _, s := range h.Suggestions {
			fmt.Fprintf(&b, "- %s\n", s)
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
	fID, errRes := s.resolveFeatureID(getStringArg(req, "feature", ""))
	if errRes != nil {
		return errRes, nil
	}
	switch what {
	case "stale_facts":
		d, err := s.store.ForgetStaleFacts(fID)
		if err != nil {
			return respondErr("Failed to forget %s: %v", what, err)
		}
		return respond("Deleted %d stale facts (invalidated 30+ days ago).", d)
	case "stale_notes":
		d, err := s.store.ForgetStaleNotes(fID)
		if err != nil {
			return respondErr("Failed to forget %s: %v", what, err)
		}
		return respond("Deleted %d stale notes (60+ days old, no links).", d)
	case "completed_features":
		d, err := s.store.ForgetCompletedFeatures()
		if err != nil {
			return respondErr("Failed to forget %s: %v", what, err)
		}
		return respond("Deleted %d completed features (done 90+ days ago).", d)
	default:
		typ, err := s.store.ForgetByID(what)
		if err != nil {
			return respondErr("Failed to forget: %v", err)
		}
		return respond("Deleted %s with ID %s.", typ, what)
	}
}

func (s *DevMemServer) handleManage(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	action, errRes := requireParam(req, "action")
	if errRes != nil {
		return errRes, nil
	}

	switch action {
	case "list":
		feature, errRes := s.requireActiveFeature()
		if errRes != nil {
			return errRes, nil
		}
		filterStr := getStringArg(req, "filter", "all")
		limit := getIntArg(req, "limit", 20)
		filter := memory.MemoryFilter{Limit: limit}
		switch filterStr {
		case "notes":
			filter.Type = "notes"
		case "facts":
			filter.Type = "facts"
		case "pinned":
			pinned := true
			filter.Pinned = &pinned
		}
		items, err := s.store.ListMemories(feature.ID, filter)
		if err != nil {
			return respondErr("Failed to list memories: %v", err)
		}
		if len(items) == 0 {
			return respond("No memories found (filter: %s).", filterStr)
		}
		var b strings.Builder
		fmt.Fprintf(&b, "# Memories (%d, filter: %s)\n\n", len(items), filterStr)
		for _, m := range items {
			pin := ""
			if m.Pinned {
				pin = " [PINNED]"
			}
			fmt.Fprintf(&b, "- **[%s]** `%s` %s%s\n  %s\n", m.Type, m.ID[:8], m.CreatedAt, pin, truncate(m.Content, 120))
		}
		return mcplib.NewToolResultText(b.String()), nil

	case "pin":
		id, errRes := requireParam(req, "id")
		if errRes != nil {
			return errRes, nil
		}
		memType := s.detectMemoryType(id)
		if memType == "" {
			return respondErr("Memory with ID %q not found in notes or facts.", id)
		}
		if err := s.store.PinMemory(id, memType); err != nil {
			return respondErr("Failed to pin: %v", err)
		}
		return respond("Pinned %s `%s`. It will now always appear in context.", memType, id[:min(8, len(id))])

	case "unpin":
		id, errRes := requireParam(req, "id")
		if errRes != nil {
			return errRes, nil
		}
		memType := s.detectMemoryType(id)
		if memType == "" {
			return respondErr("Memory with ID %q not found in notes or facts.", id)
		}
		if err := s.store.UnpinMemory(id, memType); err != nil {
			return respondErr("Failed to unpin: %v", err)
		}
		return respond("Unpinned %s `%s`.", memType, id[:min(8, len(id))])

	case "delete":
		id, errRes := requireParam(req, "id")
		if errRes != nil {
			return errRes, nil
		}
		typ, err := s.store.ForgetByID(id)
		if err != nil {
			return respondErr("Failed to delete: %v", err)
		}
		return respond("Deleted %s `%s`.", typ, id[:min(8, len(id))])

	default:
		return respondErr("Unknown action %q. Use: list, pin, unpin, or delete.", action)
	}
}

// detectMemoryType checks if an ID belongs to a note or fact.
func (s *DevMemServer) detectMemoryType(id string) string {
	r := s.db.Reader()
	var dummy string
	if r.QueryRow(`SELECT id FROM notes WHERE id = ?`, id).Scan(&dummy) == nil {
		return "note"
	}
	if r.QueryRow(`SELECT id FROM facts WHERE id = ?`, id).Scan(&dummy) == nil {
		return "fact"
	}
	return ""
}

func getIntArg(req mcplib.CallToolRequest, name string, fb int) int {
	if a := req.GetArguments(); a != nil {
		if v, ok := a[name].(float64); ok {
			return int(v)
		}
	}
	return fb
}

func (s *DevMemServer) handleGenerateRules(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	output, dryRun := getStringArg(req, "output", ""), getBoolArg(req, "dry_run", false)
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

func (s *DevMemServer) handleProjectMap(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	rescan := getBoolArg(req, "rescan", false)

	if !rescan {
		if pm, err := s.store.GetProjectMap(); err == nil {
			return mcplib.NewToolResultText(memory.FormatProjectMap(pm)), nil
		}
	}

	pm, err := s.store.ScanProject(s.gitRoot)
	if err != nil {
		return respondErr("Failed to scan project: %v", err)
	}
	return mcplib.NewToolResultText(memory.FormatProjectMap(pm)), nil
}

func (s *DevMemServer) handleBriefing(_ context.Context, _ mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	feature, err := s.store.GetActiveFeature()
	if err != nil {
		return mcplib.NewToolResultText("memorx: No active feature. Use memorx_start_feature to begin."), nil
	}
	ctxData, err := s.store.GetContext(feature.ID, "standard", nil)
	if err != nil {
		return respondErr("Failed to load context: %v", err)
	}
	sessions, _ := s.store.ListSessions(feature.ID, 5)
	ctxData.SessionHistory = sessions
	return mcplib.NewToolResultText(formatBriefing(ctxData, feature)), nil
}

func (s *DevMemServer) handleSnapshot(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	content, errRes := requireParam(req, "content")
	if errRes != nil {
		return errRes, nil
	}
	feature, errRes := s.requireActiveFeature()
	if errRes != nil {
		return errRes, nil
	}
	snapshotType := getStringArg(req, "type", "pre_compaction")
	if err := s.store.SaveSnapshot(feature.ID, s.currentSessionID, content, snapshotType); err != nil {
		return respondErr("Failed to save snapshot: %v", err)
	}
	return respond("# Snapshot saved (%s)\n\nContext preserved for feature: %s\nContent length: %d chars\n\nThis context can be recovered later with memorx_recover.", snapshotType, feature.Name, len(content))
}

func (s *DevMemServer) handleRecover(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	query, errRes := requireParam(req, "query")
	if errRes != nil {
		return errRes, nil
	}
	feature, errRes := s.requireActiveFeature()
	if errRes != nil {
		return errRes, nil
	}
	limit := 3
	if a := req.GetArguments(); a != nil {
		if v, ok := a["limit"].(float64); ok && v > 0 {
			limit = int(v)
		}
	}
	matches, err := s.store.RecoverContext(feature.ID, query, limit)
	if err != nil {
		return respondErr("Failed to recover context: %v", err)
	}
	if len(matches) == 0 {
		return respond("No matching snapshots found for: %s", query)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Recovered context (%d matches)\n\n", len(matches))
	for i, m := range matches {
		fmt.Fprintf(&b, "## Match %d [%s] — %s\n\n%s\n\n", i+1, m.SnapshotType, m.CreatedAt, m.Content)
	}
	return mcplib.NewToolResultText(b.String()), nil
}

func (s *DevMemServer) handleTrackFiles(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	feature, err := s.requireActiveFeature()
	if err != nil {
		return respondErr("No active feature")
	}
	files := getStringSliceArg(req, "files")
	if len(files) == 0 {
		return respondErr("files array is required")
	}
	action := getStringArg(req, "action", "modified")
	tracked := 0
	for _, f := range files {
		if err := s.store.TrackFile(feature.ID, s.currentSessionID, f, action); err == nil {
			tracked++
		}
	}
	return respond("tracked %d files (%s) in %s", tracked, action, feature.Name)
}

func (s *DevMemServer) handleRelated(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	topic, errRes := requireParam(req, "topic")
	if errRes != nil {
		return errRes, nil
	}
	depth := 2
	if a := req.GetArguments(); a != nil {
		if v, ok := a["depth"].(float64); ok && v > 0 {
			depth = int(v)
		}
	}
	result, err := s.store.FindRelated(s.searchEngine, topic, depth)
	if err != nil {
		return respondErr("Failed to find related memories: %v", err)
	}
	return mcplib.NewToolResultText(memory.FormatRelatedResult(result)), nil
}

func (s *DevMemServer) handleDependencies(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	file, errRes := requireParam(req, "file")
	if errRes != nil {
		return errRes, nil
	}
	deps, err := s.store.FindDependencies(file)
	if err != nil {
		return respondErr("Failed to find dependencies: %v", err)
	}
	return mcplib.NewToolResultText(memory.FormatDependencies(deps)), nil
}

func (s *DevMemServer) handleDiff(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	feature, errRes := s.requireActiveFeature()
	if errRes != nil {
		return errRes, nil
	}

	var since time.Time
	if sinceStr := getStringArg(req, "since", ""); sinceStr != "" {
		// Try date-only format first, then full timestamps.
		if t, err := time.Parse("2006-01-02", sinceStr); err == nil {
			since = t
		} else {
			t, errR := parseTimestamp(sinceStr, "since")
			if errR != nil {
				return errR, nil
			}
			since = t
		}
	} else {
		// Default: last session end time.
		t, err := s.store.GetLastSessionEndTime(feature.ID)
		if err != nil {
			return respondErr("Failed to get last session time: %v", err)
		}
		if t.IsZero() {
			// No previous session — show everything since feature creation.
			if ct, err := time.Parse(time.DateTime, feature.CreatedAt); err == nil {
				since = ct
			} else {
				since = time.Now().UTC().Add(-24 * time.Hour)
			}
		} else {
			since = t
		}
	}

	diff, err := s.store.GetDiff(feature.ID, since)
	if err != nil {
		return respondErr("Failed to compute diff: %v", err)
	}

	return mcplib.NewToolResultText(formatDiff(diff, since)), nil
}

func formatDiff(diff *memory.MemoryDiff, since time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Since last session (%s):\n", formatTimeAgo(since.Format(time.DateTime)))

	// Facts line.
	fmt.Fprintf(&b, "+%d facts", len(diff.NewFacts))
	if len(diff.InvalidatedFacts) > 0 {
		fmt.Fprintf(&b, ", -%d invalidated", len(diff.InvalidatedFacts))
	}

	// Notes line with type breakdown.
	if len(diff.NewNotes) > 0 {
		typeCounts := map[string]int{}
		for _, n := range diff.NewNotes {
			typeCounts[n.Type]++
		}
		fmt.Fprintf(&b, " | +%d notes", len(diff.NewNotes))
		if len(typeCounts) > 0 {
			b.WriteString(" (")
			first := true
			for typ, count := range typeCounts {
				if !first {
					b.WriteString(", ")
				}
				fmt.Fprintf(&b, "%d %s", count, typ)
				first = false
			}
			b.WriteString(")")
		}
	} else {
		b.WriteString(" | +0 notes")
	}

	// Commits.
	fmt.Fprintf(&b, " | +%d commits\n", diff.NewCommits)

	// Plan + links + files line.
	fmt.Fprintf(&b, "plan: %s", diff.PlanDelta)
	fmt.Fprintf(&b, " | +%d links", diff.NewLinks)
	fmt.Fprintf(&b, " | +%d files", len(diff.NewFiles))
	b.WriteString("\n")

	return b.String()
}

