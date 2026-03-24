package memory

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// StandupData holds the data for a daily standup.
type StandupData struct {
	Yesterday []StandupItem
	Today     []StandupItem
	Blockers  []StandupItem
}

// StandupItem represents a single standup line item.
type StandupItem struct {
	Feature string
	Content string
}

// BranchMapping holds a mapping between a git branch and a feature.
type BranchMapping struct {
	Branch      string
	FeatureName string
	SavedAt     string
}

// GetStandup generates daily standup data from yesterday's sessions and notes.
func (s *Store) GetStandup() (*StandupData, error) {
	r := s.db.Reader()

	now := time.Now().UTC()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	yesterdayStart := todayStart.AddDate(0, 0, -1)
	todayStr := todayStart.Format(time.DateTime)
	yesterdayStr := yesterdayStart.Format(time.DateTime)

	standup := &StandupData{}

	// Yesterday: notes from yesterday (not blockers, not next_step)
	type noteItem struct {
		featureName, content string
	}
	yesterdayNotes := scanRows(r,
		`SELECT COALESCE(f.name, 'unknown'), n.content
		 FROM notes n
		 LEFT JOIN features f ON n.feature_id = f.id
		 WHERE n.created_at >= ? AND n.created_at < ?
		 AND n.type NOT IN ('blocker', 'next_step')
		 ORDER BY n.created_at ASC`,
		[]any{yesterdayStr, todayStr},
		func(rows *sql.Rows) (noteItem, error) {
			var ni noteItem
			return ni, rows.Scan(&ni.featureName, &ni.content)
		},
	)
	for _, ni := range yesterdayNotes {
		standup.Yesterday = append(standup.Yesterday, StandupItem{
			Feature: ni.featureName,
			Content: truncateStr(ni.content, 120),
		})
	}

	// Also include session summaries from yesterday
	type sessItem struct {
		featureName, summary string
	}
	yesterdaySessions := scanRows(r,
		`SELECT COALESCE(f.name, 'unknown'), COALESCE(s.summary, '')
		 FROM sessions s
		 LEFT JOIN features f ON s.feature_id = f.id
		 WHERE s.ended_at >= ? AND s.ended_at < ?
		 AND s.summary IS NOT NULL AND s.summary != ''
		 ORDER BY s.ended_at ASC`,
		[]any{yesterdayStr, todayStr},
		func(rows *sql.Rows) (sessItem, error) {
			var si sessItem
			return si, rows.Scan(&si.featureName, &si.summary)
		},
	)
	for _, si := range yesterdaySessions {
		standup.Yesterday = append(standup.Yesterday, StandupItem{
			Feature: si.featureName,
			Content: truncateStr(si.summary, 120),
		})
	}

	// Today: next_step notes (most recent per feature)
	nextSteps := scanRows(r,
		`SELECT COALESCE(f.name, 'unknown'), n.content
		 FROM notes n
		 LEFT JOIN features f ON n.feature_id = f.id
		 WHERE n.type = 'next_step'
		 AND n.id IN (
		   SELECT id FROM notes WHERE type = 'next_step'
		   GROUP BY feature_id
		   HAVING created_at = MAX(created_at)
		 )
		 ORDER BY n.created_at DESC LIMIT 10`,
		nil,
		func(rows *sql.Rows) (noteItem, error) {
			var ni noteItem
			return ni, rows.Scan(&ni.featureName, &ni.content)
		},
	)
	for _, ni := range nextSteps {
		standup.Today = append(standup.Today, StandupItem{
			Feature: ni.featureName,
			Content: truncateStr(ni.content, 120),
		})
	}

	// Blockers: active blockers across all features
	blockers := scanRows(r,
		`SELECT COALESCE(f.name, 'unknown'), n.content
		 FROM notes n
		 LEFT JOIN features f ON n.feature_id = f.id
		 WHERE n.type = 'blocker'
		 ORDER BY n.created_at DESC LIMIT 10`,
		nil,
		func(rows *sql.Rows) (noteItem, error) {
			var ni noteItem
			return ni, rows.Scan(&ni.featureName, &ni.content)
		},
	)
	for _, ni := range blockers {
		standup.Blockers = append(standup.Blockers, StandupItem{
			Feature: ni.featureName,
			Content: truncateStr(ni.content, 120),
		})
	}

	return standup, nil
}

// FormatStandup formats standup data as markdown.
func FormatStandup(d *StandupData) string {
	var b strings.Builder
	b.WriteString("# Daily Standup\n\n")

	b.WriteString("## Yesterday\n")
	if len(d.Yesterday) == 0 {
		b.WriteString("- (no activity recorded)\n")
	}
	for _, item := range d.Yesterday {
		fmt.Fprintf(&b, "- [%s] %s\n", item.Feature, item.Content)
	}

	b.WriteString("\n## Today\n")
	if len(d.Today) == 0 {
		b.WriteString("- (no planned next steps)\n")
	}
	for _, item := range d.Today {
		fmt.Fprintf(&b, "- [%s] %s\n", item.Feature, item.Content)
	}

	b.WriteString("\n## Blockers\n")
	if len(d.Blockers) == 0 {
		b.WriteString("- None\n")
	}
	for _, item := range d.Blockers {
		fmt.Fprintf(&b, "- [%s] %s\n", item.Feature, item.Content)
	}

	return b.String()
}

// BranchContextSave saves a mapping between a git branch and the active feature.
func (s *Store) BranchContextSave(branch string) (*BranchMapping, error) {
	if branch == "" {
		return nil, fmt.Errorf("branch name is required")
	}

	f, err := s.GetActiveFeature()
	if err != nil {
		return nil, fmt.Errorf("no active feature to associate with branch")
	}

	w := s.db.Writer()
	now := time.Now().UTC().Format(time.DateTime)

	_, err = w.Exec(
		`INSERT INTO branch_context (id, branch, feature_name, saved_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(branch) DO UPDATE SET feature_name = excluded.feature_name, saved_at = excluded.saved_at`,
		uuid.New().String(), branch, f.Name, now,
	)
	if err != nil {
		return nil, fmt.Errorf("save branch context: %w", err)
	}

	return &BranchMapping{Branch: branch, FeatureName: f.Name, SavedAt: now}, nil
}

// BranchContextRestore restores context for a given branch by switching to the associated feature.
func (s *Store) BranchContextRestore(branch string) (*BranchMapping, error) {
	if branch == "" {
		return nil, fmt.Errorf("branch name is required")
	}

	r := s.db.Reader()
	var featureName, savedAt string
	err := r.QueryRow(
		`SELECT feature_name, saved_at FROM branch_context WHERE branch = ?`, branch,
	).Scan(&featureName, &savedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("no context saved for branch %q", branch)
	}
	if err != nil {
		return nil, fmt.Errorf("query branch context: %w", err)
	}

	// Switch to the feature
	_, err = s.StartFeature(featureName, "")
	if err != nil {
		return nil, fmt.Errorf("restore feature %q: %w", featureName, err)
	}

	return &BranchMapping{Branch: branch, FeatureName: featureName, SavedAt: savedAt}, nil
}

// BranchContextList returns all branch-to-feature mappings.
func (s *Store) BranchContextList() ([]BranchMapping, error) {
	r := s.db.Reader()
	return collectRows(r,
		`SELECT branch, feature_name, saved_at FROM branch_context ORDER BY saved_at DESC`,
		nil,
		func(rows *sql.Rows) (BranchMapping, error) {
			var m BranchMapping
			return m, rows.Scan(&m.Branch, &m.FeatureName, &m.SavedAt)
		},
	)
}

// FormatBranchContext formats branch context operation results as markdown.
func FormatBranchContext(action string, mapping *BranchMapping, mappings []BranchMapping) string {
	var b strings.Builder

	switch action {
	case "save":
		fmt.Fprintf(&b, "# Branch Context Saved\n\nBranch '%s' -> feature '%s'\n", mapping.Branch, mapping.FeatureName)

	case "restore":
		fmt.Fprintf(&b, "# Branch Context Restored\n\nBranch '%s' -> feature '%s' (restored)\n", mapping.Branch, mapping.FeatureName)

	case "list":
		b.WriteString("# Branch Context Mappings\n\n")
		if len(mappings) == 0 {
			b.WriteString("No branch mappings saved.\n")
		} else {
			for _, m := range mappings {
				fmt.Fprintf(&b, "- %s -> %s (saved %s)\n", m.Branch, m.FeatureName, m.SavedAt)
			}
		}
	}

	return b.String()
}
