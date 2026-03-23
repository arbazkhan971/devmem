package memory

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// GenerateOnboarding produces a comprehensive onboarding markdown document
// combining project_map, all decisions, all active facts, active plans,
// and recent session summaries. If feature is non-empty, scopes to that feature.
func (s *Store) GenerateOnboarding(feature string) (string, error) {
	var b strings.Builder
	r := s.db.Reader()

	// Project name from project_map (fall back to "Unknown Project")
	projectName := "Unknown Project"
	pm, pmErr := s.GetProjectMap()
	if pmErr == nil && pm != nil {
		projectName = pm.Root
		if idx := strings.LastIndex(pm.Root, "/"); idx >= 0 {
			projectName = pm.Root[idx+1:]
		}
	}

	b.WriteString(fmt.Sprintf("# Project Onboarding: %s\n\n", projectName))

	// Tech stack from project map
	if pm != nil && len(pm.Languages) > 0 {
		b.WriteString("## Tech Stack\n\n")
		for lang, count := range pm.Languages {
			fmt.Fprintf(&b, "- %s (%d files)\n", lang, count)
		}
		b.WriteString("\n")
	}

	// Architecture from project map
	if pm != nil {
		b.WriteString("## Architecture\n\n")
		if len(pm.Directories) > 0 {
			b.WriteString("**Directories:** ")
			b.WriteString(strings.Join(pm.Directories, ", "))
			b.WriteString("\n\n")
		}
		if len(pm.KeyFiles) > 0 {
			b.WriteString("**Key Files:**\n")
			for _, f := range pm.KeyFiles {
				fmt.Fprintf(&b, "- `%s` (%s, %s)\n", f.Path, f.Language, f.Role)
			}
			b.WriteString("\n")
		}
	}

	// Determine feature scope
	var featureIDs []string
	var featureNames []string
	if feature != "" {
		f, err := s.GetFeature(feature)
		if err != nil {
			return "", fmt.Errorf("feature %q not found: %w", feature, err)
		}
		featureIDs = append(featureIDs, f.ID)
		featureNames = append(featureNames, f.Name)
	} else {
		features, err := s.ListFeatures("all")
		if err != nil {
			return "", fmt.Errorf("list features: %w", err)
		}
		for _, f := range features {
			featureIDs = append(featureIDs, f.ID)
			featureNames = append(featureNames, f.Name)
		}
	}

	// Key Decisions (all features, newest first)
	b.WriteString("## Key Decisions\n\n")
	decisionCount := 0
	for i, fid := range featureIDs {
		notes, err := s.ListNotes(fid, "decision", 50)
		if err != nil {
			continue
		}
		for _, n := range notes {
			if decisionCount == 0 || len(featureIDs) > 1 {
				if decisionCount == 0 || featureNames[i] != "" {
					// Include feature name prefix when showing multiple features
				}
			}
			prefix := ""
			if len(featureIDs) > 1 {
				prefix = fmt.Sprintf("[%s] ", featureNames[i])
			}
			fmt.Fprintf(&b, "- %s%s\n", prefix, strings.ReplaceAll(n.Content, "\n", " "))
			decisionCount++
		}
	}
	if decisionCount == 0 {
		b.WriteString("_No decisions recorded yet._\n")
	}
	b.WriteString("\n")

	// Active Facts
	b.WriteString("## Known Facts\n\n")
	factCount := 0
	for _, fid := range featureIDs {
		facts, err := s.GetActiveFacts(fid)
		if err != nil {
			continue
		}
		for _, f := range facts {
			fmt.Fprintf(&b, "- %s %s %s\n", f.Subject, f.Predicate, f.Object)
			factCount++
		}
	}
	if factCount == 0 {
		b.WriteString("_No facts recorded yet._\n")
	}
	b.WriteString("\n")

	// Current Work (active features + plans)
	b.WriteString("## Current Work\n\n")
	activeFeatures, _ := s.ListFeatures("active")
	if len(activeFeatures) == 0 {
		b.WriteString("_No active features._\n")
	}
	for _, f := range activeFeatures {
		fmt.Fprintf(&b, "### %s", f.Name)
		if f.Branch != "" {
			fmt.Fprintf(&b, " (`%s`)", f.Branch)
		}
		b.WriteString("\n")
		if f.Description != "" {
			fmt.Fprintf(&b, "%s\n", f.Description)
		}
		if pi := s.loadPlanInfo(r, f.ID); pi != nil {
			fmt.Fprintf(&b, "- Plan: %s (%d/%d steps completed)\n", pi.Title, pi.CompletedStep, pi.TotalSteps)
		}
		b.WriteString("\n")
	}

	// Known Issues (blockers)
	b.WriteString("## Known Issues\n\n")
	blockerCount := 0
	for i, fid := range featureIDs {
		notes, err := s.ListNotes(fid, "blocker", 50)
		if err != nil {
			continue
		}
		for _, n := range notes {
			prefix := ""
			if len(featureIDs) > 1 {
				prefix = fmt.Sprintf("[%s] ", featureNames[i])
			}
			fmt.Fprintf(&b, "- %s%s\n", prefix, strings.ReplaceAll(n.Content, "\n", " "))
			blockerCount++
		}
	}
	if blockerCount == 0 {
		b.WriteString("_No blockers._\n")
	}
	b.WriteString("\n")

	// Recent Changes (last 10 commits across all features)
	b.WriteString("## Recent Changes\n\n")
	commits := scanRows(r,
		`SELECT hash, message, author, committed_at FROM commits ORDER BY committed_at DESC LIMIT 10`,
		nil,
		func(rows *sql.Rows) (CommitInfo, error) {
			var c CommitInfo
			return c, rows.Scan(&c.Hash, &c.Message, &c.Author, &c.CommittedAt)
		},
	)
	if len(commits) == 0 {
		b.WriteString("_No commits synced._\n")
	}
	for _, c := range commits {
		hash := c.Hash
		if len(hash) > 7 {
			hash = hash[:7]
		}
		fmt.Fprintf(&b, "- `%s` %s (%s)\n", hash, c.Message, c.CommittedAt)
	}
	b.WriteString("\n")

	// Recent Session Summaries
	b.WriteString("## Recent Sessions\n\n")
	sessions := scanRows(r,
		`SELECT `+sessionCols+` FROM sessions WHERE summary != '' AND ended_at IS NOT NULL ORDER BY ended_at DESC LIMIT 10`,
		nil,
		func(rows *sql.Rows) (Session, error) { return scanSession(rows) },
	)
	if len(sessions) == 0 {
		b.WriteString("_No session summaries available._\n")
	}
	for _, sess := range sessions {
		fmt.Fprintf(&b, "- **%s**: %s\n", sess.StartedAt, strings.ReplaceAll(sess.Summary, "\n", " "))
	}
	b.WriteString("\n")

	return b.String(), nil
}

// GenerateChangelog produces a changelog grouped by feature and time.
// days is the number of days to look back; format is "markdown" or "slack".
func (s *Store) GenerateChangelog(days int, format string) (string, error) {
	if days <= 0 {
		days = 7
	}
	if format == "" {
		format = "markdown"
	}
	r := s.db.Reader()

	since := time.Now().UTC().AddDate(0, 0, -days).Format(time.DateTime)
	now := time.Now().UTC()

	// Date range for display
	startDate := now.AddDate(0, 0, -days).Format("Jan 2")
	endDate := now.Format("Jan 2")

	// Get all commits in the time range with feature info
	type commitWithFeature struct {
		FeatureName string
		Hash        string
		Message     string
		IntentType  string
		CommittedAt string
	}
	commits := scanRows(r,
		`SELECT COALESCE(f.name, 'unlinked'), c.hash, c.message, COALESCE(c.intent_type, 'unknown'), c.committed_at
		 FROM commits c
		 LEFT JOIN features f ON c.feature_id = f.id
		 WHERE c.committed_at >= ?
		 ORDER BY c.committed_at DESC`,
		[]any{since},
		func(rows *sql.Rows) (commitWithFeature, error) {
			var c commitWithFeature
			return c, rows.Scan(&c.FeatureName, &c.Hash, &c.Message, &c.IntentType, &c.CommittedAt)
		},
	)

	// Get decisions in the time range with feature info
	type noteWithFeature struct {
		FeatureName string
		Content     string
		Type        string
		CreatedAt   string
	}
	decisions := scanRows(r,
		`SELECT COALESCE(f.name, 'unlinked'), n.content, n.type, n.created_at
		 FROM notes n
		 LEFT JOIN features f ON n.feature_id = f.id
		 WHERE n.type = 'decision' AND n.created_at >= ?
		 ORDER BY n.created_at DESC`,
		[]any{since},
		func(rows *sql.Rows) (noteWithFeature, error) {
			var n noteWithFeature
			return n, rows.Scan(&n.FeatureName, &n.Content, &n.Type, &n.CreatedAt)
		},
	)

	// Group commits by feature
	featureCommits := map[string][]commitWithFeature{}
	featureOrder := []string{}
	for _, c := range commits {
		if _, exists := featureCommits[c.FeatureName]; !exists {
			featureOrder = append(featureOrder, c.FeatureName)
		}
		featureCommits[c.FeatureName] = append(featureCommits[c.FeatureName], c)
	}

	// Count decisions per feature
	featureDecisionCount := map[string]int{}
	for _, d := range decisions {
		featureDecisionCount[d.FeatureName]++
	}

	// Also add features that have decisions but no commits
	for _, d := range decisions {
		if _, exists := featureCommits[d.FeatureName]; !exists {
			featureOrder = append(featureOrder, d.FeatureName)
			featureCommits[d.FeatureName] = nil
		}
	}

	var b strings.Builder

	if format == "slack" {
		fmt.Fprintf(&b, "*Changelog (%s - %s)*\n\n", startDate, endDate)
		if len(featureOrder) == 0 {
			b.WriteString("_No changes this period._\n")
			return b.String(), nil
		}
		for _, feat := range featureOrder {
			fmt.Fprintf(&b, "*%s*\n", feat)
			for _, c := range featureCommits[feat] {
				prefix := intentPrefix(c.IntentType)
				fmt.Fprintf(&b, "  %s %s\n", prefix, c.Message)
			}
			if dc := featureDecisionCount[feat]; dc > 0 {
				fmt.Fprintf(&b, "  %d decision(s) made\n", dc)
			}
			b.WriteString("\n")
		}
	} else {
		fmt.Fprintf(&b, "## Changelog (%s - %s)\n\n", startDate, endDate)
		if len(featureOrder) == 0 {
			b.WriteString("_No changes this period._\n")
			return b.String(), nil
		}
		for _, feat := range featureOrder {
			fmt.Fprintf(&b, "### %s\n\n", feat)
			for _, c := range featureCommits[feat] {
				prefix := intentPrefix(c.IntentType)
				fmt.Fprintf(&b, "- %s: %s\n", prefix, c.Message)
			}
			if dc := featureDecisionCount[feat]; dc > 0 {
				fmt.Fprintf(&b, "- %d decision(s) made\n", dc)
			}
			b.WriteString("\n")
		}
	}

	return b.String(), nil
}

// intentPrefix maps an intent type to a conventional changelog prefix.
func intentPrefix(intentType string) string {
	switch intentType {
	case "feature":
		return "feat"
	case "bugfix":
		return "fix"
	case "refactor":
		return "refactor"
	case "test":
		return "test"
	case "docs":
		return "docs"
	case "chore":
		return "chore"
	default:
		return intentType
	}
}

// ImportSharedMemory reads an exported memorX JSON/markdown and imports
// features, notes, facts, and plans. Returns counts of imported items.
type ImportResult struct {
	Features  int
	Notes     int
	Facts     int
	Plans     int
	Errors    []string
}

func (s *Store) ImportSharedMemory(data map[string]interface{}) (*ImportResult, error) {
	result := &ImportResult{}

	// Extract feature info
	featureName, _ := data["feature"].(string)
	if featureName == "" {
		featureName, _ = data["name"].(string)
	}
	if featureName == "" {
		return nil, fmt.Errorf("imported data must contain a 'feature' or 'name' field")
	}

	description, _ := data["description"].(string)
	status, _ := data["status"].(string)
	if status == "" {
		status = "paused"
	}

	// Create or get feature
	feature, err := s.GetFeature(featureName)
	if err != nil {
		feature, err = s.CreateFeature(featureName, description)
		if err != nil {
			return nil, fmt.Errorf("create feature %q: %w", featureName, err)
		}
		result.Features++
	}

	// Import notes (decisions, progress, blockers, etc.)
	for _, noteSection := range []struct {
		key      string
		noteType string
	}{
		{"decisions", "decision"},
		{"progress_notes", "progress"},
		{"blockers", "blocker"},
		{"next_steps", "next_step"},
		{"notes", "note"},
	} {
		if arr, ok := data[noteSection.key].([]interface{}); ok {
			for _, item := range arr {
				content, _ := item.(string)
				if content == "" {
					continue
				}
				if _, err := s.CreateNote(feature.ID, "", content, noteSection.noteType); err != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("note import: %v", err))
				} else {
					result.Notes++
				}
			}
		}
	}

	// Import facts
	if factsArr, ok := data["facts"].([]interface{}); ok {
		for _, item := range factsArr {
			m, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			subj, _ := m["subject"].(string)
			pred, _ := m["predicate"].(string)
			obj, _ := m["object"].(string)
			if subj == "" || pred == "" || obj == "" {
				continue
			}
			if _, err := s.CreateFact(feature.ID, "", subj, pred, obj); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("fact import: %v", err))
			} else {
				result.Facts++
			}
		}
	}

	// Set status if specified
	if status != "" && status != feature.Status {
		_ = s.UpdateFeatureStatus(featureName, status)
	}

	return result, nil
}
