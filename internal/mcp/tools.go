package mcp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/arbaz/devmem/internal/git"
	"github.com/arbaz/devmem/internal/memory"
	"github.com/arbaz/devmem/internal/plans"
	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// getStringArg extracts a string argument from the request, returning fallback if missing.
func getStringArg(req mcplib.CallToolRequest, name, fallback string) string {
	args := req.GetArguments()
	if args == nil {
		return fallback
	}
	if v, ok := args[name].(string); ok && v != "" {
		return v
	}
	return fallback
}

// getStringSliceArg extracts a string slice argument from the request.
func getStringSliceArg(req mcplib.CallToolRequest, name string) []string {
	args := req.GetArguments()
	if args == nil {
		return nil
	}
	v, ok := args[name]
	if !ok {
		return nil
	}
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	var result []string
	for _, item := range arr {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

// handleStatus implements devmem_status.
func (s *DevMemServer) handleStatus(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	var b strings.Builder

	projectName := git.ProjectName(s.gitRoot)
	b.WriteString(fmt.Sprintf("# devmem status — %s\n\n", projectName))

	// Active feature
	feature, err := s.store.GetActiveFeature()
	if err != nil {
		b.WriteString("**Active feature:** none\n\n")
	} else {
		b.WriteString(fmt.Sprintf("**Active feature:** %s\n", feature.Name))
		b.WriteString(fmt.Sprintf("  - Status: %s\n", feature.Status))
		if feature.Branch != "" {
			b.WriteString(fmt.Sprintf("  - Branch: %s\n", feature.Branch))
		}
		if feature.Description != "" {
			b.WriteString(fmt.Sprintf("  - Description: %s\n", feature.Description))
		}
		b.WriteString(fmt.Sprintf("  - Last active: %s\n\n", feature.LastActive))

		// Active plan
		plan, err := s.planManager.GetActivePlan(feature.ID)
		if err == nil {
			steps, _ := s.planManager.GetPlanSteps(plan.ID)
			completed := 0
			for _, st := range steps {
				if st.Status == "completed" {
					completed++
				}
			}
			b.WriteString(fmt.Sprintf("**Active plan:** %s\n", plan.Title))
			b.WriteString(fmt.Sprintf("  - Progress: %d/%d steps completed\n\n", completed, len(steps)))
		} else {
			b.WriteString("**Active plan:** none\n\n")
		}
	}

	// Features count
	features, err := s.store.ListFeatures("all")
	if err == nil {
		active, paused, done := 0, 0, 0
		for _, f := range features {
			switch f.Status {
			case "active":
				active++
			case "paused":
				paused++
			case "done":
				done++
			}
		}
		b.WriteString(fmt.Sprintf("**Features:** %d total (%d active, %d paused, %d done)\n\n", len(features), active, paused, done))
	}

	// Current session
	if s.currentSessionID != "" {
		b.WriteString(fmt.Sprintf("**Session:** %s (active)\n", s.currentSessionID[:8]))
	}

	return mcplib.NewToolResultText(b.String()), nil
}

// handleListFeatures implements devmem_list_features.
func (s *DevMemServer) handleListFeatures(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	filter := getStringArg(req, "status_filter", "all")

	features, err := s.store.ListFeatures(filter)
	if err != nil {
		return mcplib.NewToolResultError(fmt.Sprintf("Failed to list features: %v", err)), nil
	}

	if len(features) == 0 {
		return mcplib.NewToolResultText("No features found."), nil
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("# Features (%s)\n\n", filter))
	for _, f := range features {
		b.WriteString(fmt.Sprintf("## %s [%s]\n", f.Name, f.Status))
		if f.Description != "" {
			b.WriteString(fmt.Sprintf("  %s\n", f.Description))
		}
		if f.Branch != "" {
			b.WriteString(fmt.Sprintf("  Branch: %s\n", f.Branch))
		}
		b.WriteString(fmt.Sprintf("  Last active: %s\n\n", f.LastActive))
	}

	return mcplib.NewToolResultText(b.String()), nil
}

// handleStartFeature implements devmem_start_feature.
func (s *DevMemServer) handleStartFeature(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	name := getStringArg(req, "name", "")
	if name == "" {
		return mcplib.NewToolResultError("Parameter 'name' is required"), nil
	}
	description := getStringArg(req, "description", "")

	// Check if feature already exists to determine action
	existing, _ := s.store.GetFeature(name)
	action := "created"
	if existing != nil {
		action = "resumed"
	}

	// End current session if one exists
	if s.currentSessionID != "" {
		_ = s.store.EndSession(s.currentSessionID)
		s.currentSessionID = ""
	}

	feature, err := s.store.StartFeature(name, description)
	if err != nil {
		return mcplib.NewToolResultError(fmt.Sprintf("Failed to start feature: %v", err)), nil
	}

	// Create new session
	sess, err := s.store.CreateSession(feature.ID, "mcp")
	if err == nil {
		s.currentSessionID = sess.ID
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("# Feature %s: %s\n\n", action, feature.Name))
	b.WriteString(fmt.Sprintf("- Status: %s\n", feature.Status))
	if feature.Branch != "" {
		b.WriteString(fmt.Sprintf("- Branch: %s\n", feature.Branch))
	}
	if feature.Description != "" {
		b.WriteString(fmt.Sprintf("- Description: %s\n", feature.Description))
	}

	// Get compact context
	ctxData, err := s.store.GetContext(feature.ID, "compact", nil)
	if err == nil {
		b.WriteString("\n---\n")
		b.WriteString(formatContext(ctxData))
	}

	return mcplib.NewToolResultText(b.String()), nil
}

// handleSwitchFeature implements devmem_switch_feature.
func (s *DevMemServer) handleSwitchFeature(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	name := getStringArg(req, "name", "")
	if name == "" {
		return mcplib.NewToolResultError("Parameter 'name' is required"), nil
	}

	// End current session
	if s.currentSessionID != "" {
		_ = s.store.EndSession(s.currentSessionID)
		s.currentSessionID = ""
	}

	feature, err := s.store.StartFeature(name, "")
	if err != nil {
		return mcplib.NewToolResultError(fmt.Sprintf("Failed to switch feature: %v", err)), nil
	}

	// Create new session
	sess, err := s.store.CreateSession(feature.ID, "mcp")
	if err == nil {
		s.currentSessionID = sess.ID
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("# Switched to feature: %s\n\n", feature.Name))
	b.WriteString(fmt.Sprintf("- Status: %s\n", feature.Status))
	if feature.Branch != "" {
		b.WriteString(fmt.Sprintf("- Branch: %s\n", feature.Branch))
	}

	// Get compact context
	ctxData, err := s.store.GetContext(feature.ID, "compact", nil)
	if err == nil {
		b.WriteString("\n---\n")
		b.WriteString(formatContext(ctxData))
	}

	return mcplib.NewToolResultText(b.String()), nil
}

// handleGetContext implements devmem_get_context.
func (s *DevMemServer) handleGetContext(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	tier := getStringArg(req, "tier", "standard")
	asOfStr := getStringArg(req, "as_of", "")

	feature, err := s.store.GetActiveFeature()
	if err != nil {
		return mcplib.NewToolResultError("No active feature. Use devmem_start_feature first."), nil
	}

	var asOf *time.Time
	if asOfStr != "" {
		t, err := time.Parse(time.RFC3339, asOfStr)
		if err != nil {
			// Try alternative format
			t, err = time.Parse("2006-01-02T15:04:05", asOfStr)
			if err != nil {
				return mcplib.NewToolResultError(fmt.Sprintf("Invalid as_of format: %v (use ISO 8601)", err)), nil
			}
		}
		asOf = &t
	}

	ctxData, err := s.store.GetContext(feature.ID, tier, asOf)
	if err != nil {
		return mcplib.NewToolResultError(fmt.Sprintf("Failed to get context: %v", err)), nil
	}

	return mcplib.NewToolResultText(formatContext(ctxData)), nil
}

// handleSync implements devmem_sync.
func (s *DevMemServer) handleSync(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	feature, err := s.store.GetActiveFeature()
	if err != nil {
		return mcplib.NewToolResultError("No active feature. Use devmem_start_feature first."), nil
	}

	sinceStr := getStringArg(req, "since", "")
	since := time.Now().AddDate(0, 0, -7) // default: last 7 days
	if sinceStr != "" {
		t, err := time.Parse(time.RFC3339, sinceStr)
		if err != nil {
			t, err = time.Parse("2006-01-02T15:04:05", sinceStr)
			if err != nil {
				return mcplib.NewToolResultError(fmt.Sprintf("Invalid since format: %v (use ISO 8601)", err)), nil
			}
		}
		since = t
	}

	result, err := git.SyncCommits(s.db, s.gitRoot, feature.ID, s.currentSessionID, since)
	if err != nil {
		return mcplib.NewToolResultError(fmt.Sprintf("Failed to sync commits: %v", err)), nil
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("# Sync complete\n\n"))
	b.WriteString(fmt.Sprintf("**New commits:** %d\n\n", result.NewCommits))

	// Match commits against plan steps
	planUpdates := 0
	for _, c := range result.Commits {
		b.WriteString(fmt.Sprintf("- `%s` %s [%s]\n", c.Hash[:7], c.Message, c.IntentType))

		// Try to match commit to plan steps
		matchedStep, err := s.planManager.MatchCommitToSteps(c.Message, feature.ID)
		if err == nil && matchedStep != nil {
			_ = s.planManager.UpdateStepStatus(matchedStep.ID, "completed")
			_ = s.planManager.LinkCommitToStep(matchedStep.ID, c.Hash)
			b.WriteString(fmt.Sprintf("  -> Completed plan step: %s\n", matchedStep.Title))
			planUpdates++
		}
	}

	if planUpdates > 0 {
		b.WriteString(fmt.Sprintf("\n**Plan steps completed:** %d\n", planUpdates))
	}

	return mcplib.NewToolResultText(b.String()), nil
}

// handleRemember implements devmem_remember.
func (s *DevMemServer) handleRemember(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	content := getStringArg(req, "content", "")
	if content == "" {
		return mcplib.NewToolResultError("Parameter 'content' is required"), nil
	}
	noteType := getStringArg(req, "type", "note")

	feature, err := s.store.GetActiveFeature()
	if err != nil {
		return mcplib.NewToolResultError("No active feature. Use devmem_start_feature first."), nil
	}

	// Create the note
	note, err := s.store.CreateNote(feature.ID, s.currentSessionID, content, noteType)
	if err != nil {
		return mcplib.NewToolResultError(fmt.Sprintf("Failed to save note: %v", err)), nil
	}

	// Auto-link
	linksCreated, _ := s.store.AutoLink(note.ID, "note", content)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("# Remembered (%s)\n\n", noteType))
	b.WriteString(fmt.Sprintf("- ID: %s\n", note.ID[:8]))
	b.WriteString(fmt.Sprintf("- Links created: %d\n", linksCreated))

	// Check if content looks like a plan
	if plans.IsPlanLike(content) {
		steps := plans.ParseSteps(content)
		if len(steps) > 0 {
			plan, err := s.planManager.CreatePlan(
				feature.ID, s.currentSessionID,
				fmt.Sprintf("Plan from note %s", note.ID[:8]),
				content, "devmem_remember", steps,
			)
			if err == nil {
				b.WriteString(fmt.Sprintf("\n**Auto-promoted to plan:** %s (%d steps)\n", plan.Title, len(steps)))
			}
		}
	}

	return mcplib.NewToolResultText(b.String()), nil
}

// handleSearch implements devmem_search.
func (s *DevMemServer) handleSearch(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	query := getStringArg(req, "query", "")
	if query == "" {
		return mcplib.NewToolResultError("Parameter 'query' is required"), nil
	}
	scope := getStringArg(req, "scope", "current_feature")
	types := getStringSliceArg(req, "types")

	var featureID string
	if scope == "current_feature" {
		feature, err := s.store.GetActiveFeature()
		if err != nil {
			return mcplib.NewToolResultError("No active feature. Use scope='all_features' or start a feature first."), nil
		}
		featureID = feature.ID
	}

	results, err := s.searchEngine.Search(query, scope, types, featureID, 20)
	if err != nil {
		return mcplib.NewToolResultError(fmt.Sprintf("Search failed: %v", err)), nil
	}

	if len(results) == 0 {
		return mcplib.NewToolResultText(fmt.Sprintf("No results found for: %s", query)), nil
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("# Search results for: %s\n\n", query))
	b.WriteString(fmt.Sprintf("Found %d results (scope: %s)\n\n", len(results), scope))

	for i, r := range results {
		b.WriteString(fmt.Sprintf("## %d. [%s] %s\n", i+1, r.Type, truncate(r.Content, 120)))
		if r.FeatureName != "" {
			b.WriteString(fmt.Sprintf("  Feature: %s\n", r.FeatureName))
		}
		b.WriteString(fmt.Sprintf("  Relevance: %.2f | Created: %s\n\n", r.Relevance, r.CreatedAt))
	}

	return mcplib.NewToolResultText(b.String()), nil
}

// handleSavePlan implements devmem_save_plan.
func (s *DevMemServer) handleSavePlan(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	title := getStringArg(req, "title", "")
	if title == "" {
		return mcplib.NewToolResultError("Parameter 'title' is required"), nil
	}
	content := getStringArg(req, "content", "")

	feature, err := s.store.GetActiveFeature()
	if err != nil {
		return mcplib.NewToolResultError("No active feature. Use devmem_start_feature first."), nil
	}

	// Parse steps from request arguments
	args := req.GetArguments()
	stepsRaw, ok := args["steps"]
	if !ok {
		return mcplib.NewToolResultError("Parameter 'steps' is required"), nil
	}

	stepsArr, ok := stepsRaw.([]interface{})
	if !ok {
		return mcplib.NewToolResultError("Parameter 'steps' must be an array of objects with 'title' and optional 'description'"), nil
	}

	var steps []plans.StepInput
	for _, item := range stepsArr {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		stepTitle, _ := m["title"].(string)
		stepDesc, _ := m["description"].(string)
		if stepTitle != "" {
			steps = append(steps, plans.StepInput{Title: stepTitle, Description: stepDesc})
		}
	}

	if len(steps) == 0 {
		return mcplib.NewToolResultError("At least one step with a 'title' is required"), nil
	}

	// Check for existing plan to report supersession
	var supersededInfo string
	oldPlan, err := s.planManager.GetActivePlan(feature.ID)
	if err == nil {
		oldSteps, _ := s.planManager.GetPlanSteps(oldPlan.ID)
		completed := 0
		for _, st := range oldSteps {
			if st.Status == "completed" {
				completed++
			}
		}
		supersededInfo = fmt.Sprintf("\n**Superseded:** %s (%d/%d steps completed)\n", oldPlan.Title, completed, len(oldSteps))
	}

	plan, err := s.planManager.CreatePlan(feature.ID, s.currentSessionID, title, content, "devmem_save_plan", steps)
	if err != nil {
		return mcplib.NewToolResultError(fmt.Sprintf("Failed to create plan: %v", err)), nil
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("# Plan saved: %s\n\n", plan.Title))
	b.WriteString(fmt.Sprintf("- ID: %s\n", plan.ID[:8]))
	b.WriteString(fmt.Sprintf("- Steps: %d\n", len(steps)))

	if supersededInfo != "" {
		b.WriteString(supersededInfo)
	}

	b.WriteString("\n**Steps:**\n")
	for i, st := range steps {
		b.WriteString(fmt.Sprintf("  %d. %s\n", i+1, st.Title))
	}

	return mcplib.NewToolResultText(b.String()), nil
}

// handleImportSession implements devmem_import_session.
// This is the key tool for bootstrapping memory from existing CLI sessions.
func (s *DevMemServer) handleImportSession(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	featureName := getStringArg(req, "feature_name", "")
	if featureName == "" {
		return mcplib.NewToolResultError("Parameter 'feature_name' is required"), nil
	}
	description := getStringArg(req, "description", "")

	// End current session if exists
	if s.currentSessionID != "" {
		_ = s.store.EndSession(s.currentSessionID)
	}

	// Start/resume the feature
	feature, err := s.store.StartFeature(featureName, description)
	if err != nil {
		return mcplib.NewToolResultError(fmt.Sprintf("Failed to start feature: %v", err)), nil
	}

	// Create a session for this import
	sess, err := s.store.CreateSession(feature.ID, "import")
	if err != nil {
		return mcplib.NewToolResultError(fmt.Sprintf("Failed to create session: %v", err)), nil
	}
	s.currentSessionID = sess.ID

	var b strings.Builder
	b.WriteString(fmt.Sprintf("# Importing session into: %s\n\n", featureName))

	imported := 0

	// Import decisions
	decisions := getStringSliceArg(req, "decisions")
	for _, d := range decisions {
		_, err := s.store.CreateNote(feature.ID, sess.ID, d, "decision")
		if err == nil {
			imported++
		}
	}
	if len(decisions) > 0 {
		b.WriteString(fmt.Sprintf("- Decisions imported: %d\n", len(decisions)))
	}

	// Import progress notes
	progressNotes := getStringSliceArg(req, "progress_notes")
	for _, p := range progressNotes {
		_, err := s.store.CreateNote(feature.ID, sess.ID, p, "progress")
		if err == nil {
			imported++
		}
	}
	if len(progressNotes) > 0 {
		b.WriteString(fmt.Sprintf("- Progress notes imported: %d\n", len(progressNotes)))
	}

	// Import blockers
	blockers := getStringSliceArg(req, "blockers")
	for _, bl := range blockers {
		_, err := s.store.CreateNote(feature.ID, sess.ID, bl, "blocker")
		if err == nil {
			imported++
		}
	}
	if len(blockers) > 0 {
		b.WriteString(fmt.Sprintf("- Blockers imported: %d\n", len(blockers)))
	}

	// Import next steps
	nextSteps := getStringSliceArg(req, "next_steps")
	for _, ns := range nextSteps {
		_, err := s.store.CreateNote(feature.ID, sess.ID, ns, "next_step")
		if err == nil {
			imported++
		}
	}
	if len(nextSteps) > 0 {
		b.WriteString(fmt.Sprintf("- Next steps imported: %d\n", len(nextSteps)))
	}

	// Import facts
	args := req.GetArguments()
	if factsRaw, ok := args["facts"]; ok {
		if factsArr, ok := factsRaw.([]interface{}); ok {
			factCount := 0
			for _, item := range factsArr {
				m, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				subject, _ := m["subject"].(string)
				predicate, _ := m["predicate"].(string)
				object, _ := m["object"].(string)
				if subject != "" && predicate != "" && object != "" {
					_, err := s.store.CreateFact(feature.ID, sess.ID, subject, predicate, object)
					if err == nil {
						factCount++
						imported++
					}
				}
			}
			if factCount > 0 {
				b.WriteString(fmt.Sprintf("- Facts imported: %d\n", factCount))
			}
		}
	}

	// Import plan
	planTitle := getStringArg(req, "plan_title", "")
	if planTitle != "" {
		if planStepsRaw, ok := args["plan_steps"]; ok {
			if planStepsArr, ok := planStepsRaw.([]interface{}); ok {
				var steps []plans.StepInput
				var completedStepTitles []string
				for _, item := range planStepsArr {
					m, ok := item.(map[string]interface{})
					if !ok {
						continue
					}
					title, _ := m["title"].(string)
					desc, _ := m["description"].(string)
					status, _ := m["status"].(string)
					if title != "" {
						steps = append(steps, plans.StepInput{Title: title, Description: desc})
						if status == "completed" {
							completedStepTitles = append(completedStepTitles, title)
						}
					}
				}
				if len(steps) > 0 {
					plan, err := s.planManager.CreatePlan(feature.ID, sess.ID, planTitle, "", "import", steps)
					if err == nil {
						// Mark completed steps
						planSteps, _ := s.planManager.GetPlanSteps(plan.ID)
						for _, ps := range planSteps {
							for _, ct := range completedStepTitles {
								if ps.Title == ct {
									_ = s.planManager.UpdateStepStatus(ps.ID, "completed")
								}
							}
						}
						b.WriteString(fmt.Sprintf("- Plan imported: %s (%d steps, %d completed)\n", planTitle, len(steps), len(completedStepTitles)))
						imported += len(steps)
					}
				}
			}
		}
	}

	// Auto-link all imported memories
	linksCreated := 0
	if imported > 0 {
		// Run auto-linking on the most recent notes
		notes, _ := s.store.ListNotes(feature.ID, "", imported)
		for _, n := range notes {
			count, _ := s.store.AutoLink(n.ID, "note", n.Content)
			linksCreated += count
		}
	}

	b.WriteString(fmt.Sprintf("\n**Total imported:** %d items, %d links created\n", imported, linksCreated))
	b.WriteString("\nMemory is now bootstrapped. Future sessions will have this context.")

	return mcplib.NewToolResultText(b.String()), nil
}

// handleExport implements devmem_export.
func (s *DevMemServer) handleExport(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	featureName := getStringArg(req, "feature_name", "")
	format := getStringArg(req, "format", "markdown")

	var feature *memory.Feature
	var err error

	if featureName != "" {
		feature, err = s.store.GetFeature(featureName)
		if err != nil {
			return mcplib.NewToolResultError(fmt.Sprintf("Feature '%s' not found", featureName)), nil
		}
	} else {
		feature, err = s.store.GetActiveFeature()
		if err != nil {
			return mcplib.NewToolResultError("No active feature. Specify feature_name or start a feature."), nil
		}
	}

	// Get full detailed context
	ctxData, err := s.store.GetContext(feature.ID, "detailed", nil)
	if err != nil {
		return mcplib.NewToolResultError(fmt.Sprintf("Failed to get context: %v", err)), nil
	}

	if format == "json" {
		return s.exportJSON(feature, ctxData)
	}
	return s.exportMarkdown(feature, ctxData)
}

func (s *DevMemServer) exportMarkdown(feature *memory.Feature, ctx *memory.Context) (*mcplib.CallToolResult, error) {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("# Feature: %s\n\n", feature.Name))
	b.WriteString(fmt.Sprintf("**Status:** %s\n", feature.Status))
	if feature.Branch != "" {
		b.WriteString(fmt.Sprintf("**Branch:** %s\n", feature.Branch))
	}
	if feature.Description != "" {
		b.WriteString(fmt.Sprintf("**Description:** %s\n", feature.Description))
	}
	b.WriteString(fmt.Sprintf("**Created:** %s\n", feature.CreatedAt))
	b.WriteString(fmt.Sprintf("**Last Active:** %s\n\n", feature.LastActive))

	// Plan
	if ctx.Plan != nil {
		b.WriteString(fmt.Sprintf("## Plan: %s\n\n", ctx.Plan.Title))
		b.WriteString(fmt.Sprintf("Progress: %d/%d steps\n\n", ctx.Plan.CompletedStep, ctx.Plan.TotalSteps))
		// Get plan steps via the active plan for this feature
		activePlan, err := s.planManager.GetActivePlan(feature.ID)
		if err == nil {
			planSteps, _ := s.planManager.GetPlanSteps(activePlan.ID)
			for _, st := range planSteps {
				check := "[ ]"
				if st.Status == "completed" {
					check = "[x]"
				} else if st.Status == "in_progress" {
					check = "[-]"
				}
				b.WriteString(fmt.Sprintf("- %s %s\n", check, st.Title))
			}
		}
		b.WriteString("\n")
	}

	// Decisions
	b.WriteString("## Decisions\n\n")
	decisions, _ := s.store.ListNotes(feature.ID, "decision", 50)
	if len(decisions) == 0 {
		b.WriteString("_No decisions recorded._\n\n")
	}
	for _, d := range decisions {
		b.WriteString(fmt.Sprintf("- **[%s]** %s\n", d.CreatedAt, d.Content))
	}
	b.WriteString("\n")

	// Facts
	b.WriteString("## Facts (Current)\n\n")
	if len(ctx.ActiveFacts) == 0 {
		b.WriteString("_No facts recorded._\n\n")
	}
	for _, f := range ctx.ActiveFacts {
		b.WriteString(fmt.Sprintf("- %s **%s** %s\n", f.Subject, f.Predicate, f.Object))
	}
	b.WriteString("\n")

	// Progress
	b.WriteString("## Progress Notes\n\n")
	progress, _ := s.store.ListNotes(feature.ID, "progress", 50)
	if len(progress) == 0 {
		b.WriteString("_No progress notes._\n\n")
	}
	for _, p := range progress {
		b.WriteString(fmt.Sprintf("- **[%s]** %s\n", p.CreatedAt, p.Content))
	}
	b.WriteString("\n")

	// Blockers
	blockers, _ := s.store.ListNotes(feature.ID, "blocker", 50)
	if len(blockers) > 0 {
		b.WriteString("## Blockers\n\n")
		for _, bl := range blockers {
			b.WriteString(fmt.Sprintf("- **[%s]** %s\n", bl.CreatedAt, bl.Content))
		}
		b.WriteString("\n")
	}

	// Commits
	b.WriteString("## Commits\n\n")
	if len(ctx.RecentCommits) == 0 {
		b.WriteString("_No commits synced._\n\n")
	}
	for _, c := range ctx.RecentCommits {
		b.WriteString(fmt.Sprintf("- `%s` %s (%s)\n", c.Hash[:min(7, len(c.Hash))], c.Message, c.CommittedAt))
	}
	b.WriteString("\n")

	// Sessions
	b.WriteString("## Session History\n\n")
	for _, sess := range ctx.SessionHistory {
		ended := "active"
		if sess.EndedAt != "" {
			ended = sess.EndedAt
		}
		b.WriteString(fmt.Sprintf("- %s → %s (%s)\n", sess.StartedAt, ended, sess.Tool))
	}

	return mcplib.NewToolResultText(b.String()), nil
}

func (s *DevMemServer) exportJSON(feature *memory.Feature, ctx *memory.Context) (*mcplib.CallToolResult, error) {
	// For JSON, just use the detailed context as formatted text
	// A full JSON serialization can be added later
	var b strings.Builder
	b.WriteString("{\n")
	b.WriteString(fmt.Sprintf("  \"feature\": \"%s\",\n", feature.Name))
	b.WriteString(fmt.Sprintf("  \"status\": \"%s\",\n", feature.Status))
	b.WriteString(fmt.Sprintf("  \"branch\": \"%s\",\n", feature.Branch))
	b.WriteString(fmt.Sprintf("  \"description\": \"%s\",\n", feature.Description))
	b.WriteString(fmt.Sprintf("  \"commits\": %d,\n", len(ctx.RecentCommits)))
	b.WriteString(fmt.Sprintf("  \"notes\": %d,\n", len(ctx.RecentNotes)))
	b.WriteString(fmt.Sprintf("  \"facts\": %d,\n", len(ctx.ActiveFacts)))
	b.WriteString(fmt.Sprintf("  \"sessions\": %d\n", len(ctx.SessionHistory)))
	b.WriteString("}")
	return mcplib.NewToolResultText(b.String()), nil
}

// formatContext formats a Context struct into readable markdown.
func formatContext(ctx *memory.Context) string {
	var b strings.Builder

	if ctx.Feature != nil {
		b.WriteString(fmt.Sprintf("## Context: %s [%s]\n\n", ctx.Feature.Name, ctx.Feature.Status))
		if ctx.Feature.Branch != "" {
			b.WriteString(fmt.Sprintf("Branch: %s\n", ctx.Feature.Branch))
		}
	}

	if ctx.Summary != "" {
		b.WriteString(fmt.Sprintf("\n### Summary\n%s\n", ctx.Summary))
	}

	if ctx.Plan != nil {
		b.WriteString(fmt.Sprintf("\n### Plan: %s\n", ctx.Plan.Title))
		b.WriteString(fmt.Sprintf("Progress: %d/%d steps completed\n", ctx.Plan.CompletedStep, ctx.Plan.TotalSteps))
	}

	if len(ctx.RecentCommits) > 0 {
		b.WriteString("\n### Recent Commits\n")
		for _, c := range ctx.RecentCommits {
			b.WriteString(fmt.Sprintf("- `%s` %s (%s)\n", c.Hash[:min(7, len(c.Hash))], c.Message, c.CommittedAt))
		}
	}

	if len(ctx.RecentNotes) > 0 {
		b.WriteString("\n### Recent Notes\n")
		for _, n := range ctx.RecentNotes {
			b.WriteString(fmt.Sprintf("- [%s] %s (%s)\n", n.Type, truncate(n.Content, 100), n.CreatedAt))
		}
	}

	if len(ctx.ActiveFacts) > 0 {
		b.WriteString("\n### Active Facts\n")
		for _, f := range ctx.ActiveFacts {
			b.WriteString(fmt.Sprintf("- %s %s %s\n", f.Subject, f.Predicate, f.Object))
		}
	}

	if len(ctx.SessionHistory) > 0 {
		b.WriteString("\n### Session History\n")
		for _, sess := range ctx.SessionHistory {
			ended := "active"
			if sess.EndedAt != "" {
				ended = sess.EndedAt
			}
			b.WriteString(fmt.Sprintf("- %s: %s -> %s (%s)\n", sess.ID[:8], sess.StartedAt, ended, sess.Tool))
		}
	}

	if len(ctx.Links) > 0 {
		b.WriteString("\n### Memory Links\n")
		for _, l := range ctx.Links {
			b.WriteString(fmt.Sprintf("- %s:%s -> %s:%s [%s, %.1f]\n",
				l.SourceType, l.SourceID[:8], l.TargetType, l.TargetID[:8], l.Relationship, l.Strength))
		}
	}

	if len(ctx.FilesTouched) > 0 {
		b.WriteString("\n### Files Touched\n")
		for _, f := range ctx.FilesTouched {
			b.WriteString(fmt.Sprintf("- %s\n", f))
		}
	}

	return b.String()
}

// truncate returns the first n characters of a string, adding "..." if truncated.
func truncate(s string, n int) string {
	// Replace newlines for single-line display
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// min returns the smaller of two ints.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
