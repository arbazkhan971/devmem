package memory

import (
	"database/sql"
	"fmt"
	"math"
	"strings"
	"time"
)

// BlockerPrediction holds the result of predicting blockers for a feature.
type BlockerPrediction struct {
	FeatureName          string
	BlockerCount         int
	UnresolvedDeps       int
	TestCount            int
	DaysSinceLastCommit  int
	SimilarFeatureRisk   float64
	RiskLevel            string // "High", "Medium", "Low"
	Explanation          string
}

// RiskEntry holds risk scoring data for a single feature.
type RiskEntry struct {
	FeatureName    string
	Score          int
	DaysInactive   int
	BlockerCount   int
	PlanPct        int
	StaleFacts     int
	OrphanNotes    int
	Factors        []string
}

// BurndownData holds burndown chart data for a feature's plan.
type BurndownData struct {
	FeatureName    string
	TotalSteps     int
	CompletedSteps int
	StepDates      []StepCompletion
	Velocity       float64 // steps per day
	ETADate        string
	Chart          string  // ASCII burndown
}

// StepCompletion records when a plan step was completed.
type StepCompletion struct {
	StepTitle   string
	CompletedAt string
}

// FeatureComparison holds side-by-side comparison data.
type FeatureComparison struct {
	FeatureA, FeatureB ComparisonSide
}

// ComparisonSide holds stats for one side of a comparison.
type ComparisonSide struct {
	Name         string
	NoteCount    int
	FactCount    int
	CommitCount  int
	SessionCount int
	BlockerCount int
	PlanProgress string // "3/5" or "no plan"
	Status       string
	LastActive   string
}

// PeriodSummary holds summarized activity for a time period.
type PeriodSummary struct {
	Period          string
	Features        []FeaturePeriodSummary
	TotalCommits    int
	TotalDecisions  int
	TotalBlockers   int
	BlockersResolved int
}

// FeaturePeriodSummary holds per-feature activity in a period.
type FeaturePeriodSummary struct {
	Name       string
	Commits    int
	Decisions  int
	Blockers   int
	Notes      int
}

// PredictBlocker analyzes patterns to predict if a feature will hit a blocker.
func (s *Store) PredictBlocker(featureName string) (*BlockerPrediction, error) {
	r := s.db.Reader()

	var featureID, fName string
	if featureName != "" {
		f, err := s.GetFeature(featureName)
		if err != nil {
			return nil, fmt.Errorf("feature %q not found", featureName)
		}
		featureID = f.ID
		fName = f.Name
	} else {
		f, err := s.GetActiveFeature()
		if err != nil {
			return nil, fmt.Errorf("no active feature")
		}
		featureID = f.ID
		fName = f.Name
	}

	pred := &BlockerPrediction{FeatureName: fName}

	// Count blockers for this feature
	pred.BlockerCount = countRows(r, `SELECT COUNT(*) FROM notes WHERE feature_id = ? AND type = 'blocker'`, featureID)

	// Count tests
	pred.TestCount = countRows(r, `SELECT COUNT(*) FROM test_results WHERE feature_id = ?`, featureID)

	// Days since last commit
	var lastCommitAt sql.NullString
	r.QueryRow(`SELECT MAX(committed_at) FROM commits WHERE feature_id = ?`, featureID).Scan(&lastCommitAt)
	if lastCommitAt.Valid {
		if t, err := time.Parse(time.DateTime, lastCommitAt.String); err == nil {
			pred.DaysSinceLastCommit = int(math.Floor(time.Since(t).Hours() / 24))
		}
	}

	// Count unresolved dependencies (blockers without a matching resolved note)
	pred.UnresolvedDeps = countRows(r,
		`SELECT COUNT(*) FROM notes WHERE feature_id = ? AND type = 'blocker'
		 AND id NOT IN (
		   SELECT source_id FROM memory_links WHERE relationship = 'resolves' AND source_type = 'note'
		   UNION
		   SELECT target_id FROM memory_links WHERE relationship = 'resolves' AND target_type = 'note'
		 )`, featureID)

	// Check similar features: how many features with similar blocker/test patterns had blockers?
	totalOtherFeatures := countRows(r, `SELECT COUNT(*) FROM features WHERE id != ?`, featureID)
	featuresWithBlockers := countRows(r,
		`SELECT COUNT(DISTINCT f.id) FROM features f
		 JOIN notes n ON n.feature_id = f.id AND n.type = 'blocker'
		 WHERE f.id != ?`, featureID)

	if totalOtherFeatures > 0 {
		pred.SimilarFeatureRisk = math.Round(float64(featuresWithBlockers) / float64(totalOtherFeatures) * 100)
	}

	// Calculate risk level
	riskScore := 0
	var reasons []string
	if pred.UnresolvedDeps > 0 {
		riskScore += pred.UnresolvedDeps * 20
		reasons = append(reasons, fmt.Sprintf("%d unresolved dependencies", pred.UnresolvedDeps))
	}
	if pred.TestCount == 0 {
		riskScore += 25
		reasons = append(reasons, "0 tests")
	}
	if pred.DaysSinceLastCommit > 3 {
		riskScore += 15
		reasons = append(reasons, fmt.Sprintf("%d days since last commit", pred.DaysSinceLastCommit))
	}
	if pred.BlockerCount > 2 {
		riskScore += 20
		reasons = append(reasons, fmt.Sprintf("%d total blockers", pred.BlockerCount))
	}

	switch {
	case riskScore >= 40:
		pred.RiskLevel = "High"
	case riskScore >= 20:
		pred.RiskLevel = "Medium"
	default:
		pred.RiskLevel = "Low"
	}

	if len(reasons) == 0 {
		reasons = append(reasons, "no risk factors detected")
	}
	pctStr := ""
	if pred.SimilarFeatureRisk > 0 {
		pctStr = fmt.Sprintf(" Similar features had blockers %.0f%% of the time.", pred.SimilarFeatureRisk)
	}
	pred.Explanation = fmt.Sprintf("%s risk: %s has %s.%s",
		pred.RiskLevel, fName, strings.Join(reasons, " and "), pctStr)

	return pred, nil
}

// FormatBlockerPrediction formats a blocker prediction as markdown.
func FormatBlockerPrediction(p *BlockerPrediction) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Blocker Prediction: %s\n\n", p.FeatureName)
	fmt.Fprintf(&b, "**Risk Level:** %s\n\n", p.RiskLevel)
	fmt.Fprintf(&b, "%s\n\n", p.Explanation)
	fmt.Fprintf(&b, "| Factor | Value |\n|--------|-------|\n")
	fmt.Fprintf(&b, "| Total blockers | %d |\n", p.BlockerCount)
	fmt.Fprintf(&b, "| Unresolved deps | %d |\n", p.UnresolvedDeps)
	fmt.Fprintf(&b, "| Tests | %d |\n", p.TestCount)
	fmt.Fprintf(&b, "| Days since commit | %d |\n", p.DaysSinceLastCommit)
	return b.String()
}

// GetRiskScores scores every active feature for risk (0-100).
func (s *Store) GetRiskScores() ([]RiskEntry, error) {
	r := s.db.Reader()

	features, err := s.ListFeatures("active")
	if err != nil {
		return nil, err
	}

	var results []RiskEntry
	for _, feat := range features {
		entry := RiskEntry{FeatureName: feat.Name, Score: 100}
		var factors []string

		// Days inactive
		if t, err := time.Parse(time.DateTime, feat.LastActive); err == nil {
			entry.DaysInactive = int(math.Floor(time.Since(t).Hours() / 24))
		}
		if entry.DaysInactive > 0 {
			penalty := entry.DaysInactive * 5
			entry.Score -= penalty
			factors = append(factors, fmt.Sprintf("-%d (inactive %dd)", penalty, entry.DaysInactive))
		}

		// Blockers
		entry.BlockerCount = countRows(r, `SELECT COUNT(*) FROM notes WHERE feature_id = ? AND type = 'blocker'`, feat.ID)
		if entry.BlockerCount > 0 {
			penalty := entry.BlockerCount * 15
			entry.Score -= penalty
			factors = append(factors, fmt.Sprintf("-%d (%d blockers)", penalty, entry.BlockerCount))
		}

		// Plan completion %
		var planID string
		if r.QueryRow(`SELECT id FROM plans WHERE feature_id = ? AND status = 'active' AND invalid_at IS NULL LIMIT 1`, feat.ID).Scan(&planID) == nil {
			total := countRows(r, `SELECT COUNT(*) FROM plan_steps WHERE plan_id = ?`, planID)
			completed := countRows(r, `SELECT COUNT(*) FROM plan_steps WHERE plan_id = ? AND status = 'completed'`, planID)
			if total > 0 {
				entry.PlanPct = completed * 100 / total
			}
		} else {
			entry.PlanPct = 0
		}
		planPenalty := (100 - entry.PlanPct) / 2
		if planPenalty > 0 {
			entry.Score -= planPenalty
			factors = append(factors, fmt.Sprintf("-%d (plan %d%%)", planPenalty, entry.PlanPct))
		}

		// Stale facts (facts with low confidence or old)
		entry.StaleFacts = countRows(r,
			`SELECT COUNT(*) FROM facts WHERE feature_id = ? AND invalid_at IS NOT NULL`, feat.ID)
		if entry.StaleFacts > 0 {
			penalty := entry.StaleFacts * 3
			entry.Score -= penalty
			factors = append(factors, fmt.Sprintf("-%d (%d stale facts)", penalty, entry.StaleFacts))
		}

		// Orphan notes (notes not linked to anything)
		totalNotes := countRows(r, `SELECT COUNT(*) FROM notes WHERE feature_id = ?`, feat.ID)
		linkedNotes := countRows(r,
			`SELECT COUNT(DISTINCT source_id) FROM memory_links WHERE source_type = 'note'
			 AND source_id IN (SELECT id FROM notes WHERE feature_id = ?)`, feat.ID)
		entry.OrphanNotes = totalNotes - linkedNotes
		if entry.OrphanNotes < 0 {
			entry.OrphanNotes = 0
		}

		entry.Factors = factors
		if entry.Score < 0 {
			entry.Score = 0
		}
		results = append(results, entry)
	}

	return results, nil
}

// FormatRiskScores formats risk scores as markdown.
func FormatRiskScores(entries []RiskEntry) string {
	var b strings.Builder
	b.WriteString("# Risk Scores (Active Features)\n\n")
	if len(entries) == 0 {
		b.WriteString("No active features to score.\n")
		return b.String()
	}
	for _, e := range entries {
		emoji := "OK"
		if e.Score < 40 {
			emoji = "CRITICAL"
		} else if e.Score < 70 {
			emoji = "WARNING"
		}
		fmt.Fprintf(&b, "**%s**: %d/100 [%s]\n", e.FeatureName, e.Score, emoji)
		if len(e.Factors) > 0 {
			fmt.Fprintf(&b, "  Factors: %s\n", strings.Join(e.Factors, ", "))
		}
	}
	return b.String()
}

// GetBurndown generates burndown chart data from plan velocity.
func (s *Store) GetBurndown(featureName string) (*BurndownData, error) {
	r := s.db.Reader()

	var featureID, fName string
	if featureName != "" {
		f, err := s.GetFeature(featureName)
		if err != nil {
			return nil, fmt.Errorf("feature %q not found", featureName)
		}
		featureID = f.ID
		fName = f.Name
	} else {
		f, err := s.GetActiveFeature()
		if err != nil {
			return nil, fmt.Errorf("no active feature")
		}
		featureID = f.ID
		fName = f.Name
	}

	// Find active plan
	var planID, planCreatedAt string
	if err := r.QueryRow(
		`SELECT id, created_at FROM plans WHERE feature_id = ? AND status = 'active' AND invalid_at IS NULL ORDER BY created_at DESC LIMIT 1`,
		featureID,
	).Scan(&planID, &planCreatedAt); err != nil {
		return nil, fmt.Errorf("no active plan for %s", fName)
	}

	totalSteps := countRows(r, `SELECT COUNT(*) FROM plan_steps WHERE plan_id = ?`, planID)
	completedSteps := countRows(r, `SELECT COUNT(*) FROM plan_steps WHERE plan_id = ? AND status = 'completed'`, planID)

	// Get completed step dates
	stepDates := scanRows(r,
		`SELECT title, COALESCE(completed_at, '') FROM plan_steps WHERE plan_id = ? AND status = 'completed' ORDER BY completed_at ASC`,
		[]any{planID},
		func(rows *sql.Rows) (StepCompletion, error) {
			var sc StepCompletion
			return sc, rows.Scan(&sc.StepTitle, &sc.CompletedAt)
		},
	)

	data := &BurndownData{
		FeatureName:    fName,
		TotalSteps:     totalSteps,
		CompletedSteps: completedSteps,
		StepDates:      stepDates,
	}

	// Calculate velocity
	planStart, err := time.Parse(time.DateTime, planCreatedAt)
	if err != nil {
		planStart = time.Now().AddDate(0, 0, -1) // fallback
	}
	daysActive := time.Since(planStart).Hours() / 24
	if daysActive < 1 {
		daysActive = 1
	}
	data.Velocity = math.Round(float64(completedSteps)/daysActive*10) / 10

	// ETA
	remaining := totalSteps - completedSteps
	if data.Velocity > 0 && remaining > 0 {
		daysLeft := math.Ceil(float64(remaining) / data.Velocity)
		eta := time.Now().AddDate(0, 0, int(daysLeft))
		data.ETADate = eta.Format("Jan 2")
	} else if remaining == 0 {
		data.ETADate = "Done!"
	} else {
		data.ETADate = "N/A (no velocity)"
	}

	// ASCII burndown chart
	width := 10
	filled := 0
	if totalSteps > 0 {
		filled = completedSteps * width / totalSteps
	}
	empty := width - filled
	chart := strings.Repeat("\xe2\x96\x93", filled) + strings.Repeat("\xe2\x96\x91", empty)
	data.Chart = fmt.Sprintf("%s %d/%d steps, %.1f/day, ETA: %s", chart, completedSteps, totalSteps, data.Velocity, data.ETADate)

	return data, nil
}

// FormatBurndown formats burndown data as markdown.
func FormatBurndown(d *BurndownData) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Burndown: %s\n\n", d.FeatureName)
	fmt.Fprintf(&b, "%s\n\n", d.Chart)
	if len(d.StepDates) > 0 {
		b.WriteString("**Completed steps:**\n")
		for _, sd := range d.StepDates {
			date := sd.CompletedAt
			if len(date) > 10 {
				date = date[:10]
			}
			fmt.Fprintf(&b, "- %s (%s)\n", sd.StepTitle, date)
		}
	}
	return b.String()
}

// CompareFeatures compares two features side by side.
func (s *Store) CompareFeatures(nameA, nameB string) (*FeatureComparison, error) {
	r := s.db.Reader()

	loadSide := func(name string) (ComparisonSide, error) {
		f, err := s.GetFeature(name)
		if err != nil {
			return ComparisonSide{}, fmt.Errorf("feature %q not found", name)
		}
		side := ComparisonSide{
			Name:       f.Name,
			Status:     f.Status,
			LastActive: f.LastActive,
		}
		side.NoteCount = countRows(r, `SELECT COUNT(*) FROM notes WHERE feature_id = ?`, f.ID)
		side.FactCount = countRows(r, `SELECT COUNT(*) FROM facts WHERE feature_id = ? AND invalid_at IS NULL`, f.ID)
		side.CommitCount = countRows(r, `SELECT COUNT(*) FROM commits WHERE feature_id = ?`, f.ID)
		side.SessionCount = countRows(r, `SELECT COUNT(*) FROM sessions WHERE feature_id = ?`, f.ID)
		side.BlockerCount = countRows(r, `SELECT COUNT(*) FROM notes WHERE feature_id = ? AND type = 'blocker'`, f.ID)

		var planID string
		if r.QueryRow(`SELECT id FROM plans WHERE feature_id = ? AND status = 'active' AND invalid_at IS NULL LIMIT 1`, f.ID).Scan(&planID) == nil {
			total := countRows(r, `SELECT COUNT(*) FROM plan_steps WHERE plan_id = ?`, planID)
			completed := countRows(r, `SELECT COUNT(*) FROM plan_steps WHERE plan_id = ? AND status = 'completed'`, planID)
			side.PlanProgress = fmt.Sprintf("%d/%d", completed, total)
		} else {
			side.PlanProgress = "no plan"
		}
		return side, nil
	}

	sideA, err := loadSide(nameA)
	if err != nil {
		return nil, err
	}
	sideB, err := loadSide(nameB)
	if err != nil {
		return nil, err
	}

	return &FeatureComparison{FeatureA: sideA, FeatureB: sideB}, nil
}

// FormatComparison formats a feature comparison as markdown.
func FormatComparison(c *FeatureComparison) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Compare: %s vs %s\n\n", c.FeatureA.Name, c.FeatureB.Name)
	fmt.Fprintf(&b, "| Metric | %s | %s |\n|--------|--------|--------|\n", c.FeatureA.Name, c.FeatureB.Name)
	fmt.Fprintf(&b, "| Status | %s | %s |\n", c.FeatureA.Status, c.FeatureB.Status)
	fmt.Fprintf(&b, "| Notes | %d | %d |\n", c.FeatureA.NoteCount, c.FeatureB.NoteCount)
	fmt.Fprintf(&b, "| Facts | %d | %d |\n", c.FeatureA.FactCount, c.FeatureB.FactCount)
	fmt.Fprintf(&b, "| Commits | %d | %d |\n", c.FeatureA.CommitCount, c.FeatureB.CommitCount)
	fmt.Fprintf(&b, "| Sessions | %d | %d |\n", c.FeatureA.SessionCount, c.FeatureB.SessionCount)
	fmt.Fprintf(&b, "| Blockers | %d | %d |\n", c.FeatureA.BlockerCount, c.FeatureB.BlockerCount)
	fmt.Fprintf(&b, "| Plan | %s | %s |\n", c.FeatureA.PlanProgress, c.FeatureB.PlanProgress)
	fmt.Fprintf(&b, "| Last active | %s | %s |\n", c.FeatureA.LastActive, c.FeatureB.LastActive)
	return b.String()
}

// SummarizePeriod summarizes activity across all features for a time period.
func (s *Store) SummarizePeriod(period string) (*PeriodSummary, error) {
	r := s.db.Reader()

	var since time.Time
	now := time.Now().UTC()
	switch period {
	case "today":
		since = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	case "month":
		since = now.AddDate(0, -1, 0)
	default: // "week"
		since = now.AddDate(0, 0, -7)
		period = "week"
	}
	sinceStr := since.Format(time.DateTime)

	summary := &PeriodSummary{Period: period}

	// Get all features with activity in the period
	type featureActivity struct {
		id, name string
	}
	feats := scanRows(r,
		`SELECT DISTINCT f.id, f.name FROM features f
		 WHERE f.id IN (
		   SELECT feature_id FROM notes WHERE created_at >= ?
		   UNION SELECT feature_id FROM commits WHERE committed_at >= ?
		   UNION SELECT feature_id FROM sessions WHERE started_at >= ?
		 ) ORDER BY f.name`,
		[]any{sinceStr, sinceStr, sinceStr},
		func(rows *sql.Rows) (featureActivity, error) {
			var fa featureActivity
			return fa, rows.Scan(&fa.id, &fa.name)
		},
	)

	for _, feat := range feats {
		fps := FeaturePeriodSummary{Name: feat.name}
		fps.Commits = countRows(r, `SELECT COUNT(*) FROM commits WHERE feature_id = ? AND committed_at >= ?`, feat.id, sinceStr)
		fps.Decisions = countRows(r, `SELECT COUNT(*) FROM notes WHERE feature_id = ? AND type = 'decision' AND created_at >= ?`, feat.id, sinceStr)
		fps.Blockers = countRows(r, `SELECT COUNT(*) FROM notes WHERE feature_id = ? AND type = 'blocker' AND created_at >= ?`, feat.id, sinceStr)
		fps.Notes = countRows(r, `SELECT COUNT(*) FROM notes WHERE feature_id = ? AND created_at >= ?`, feat.id, sinceStr)

		summary.TotalCommits += fps.Commits
		summary.TotalDecisions += fps.Decisions
		summary.TotalBlockers += fps.Blockers
		summary.Features = append(summary.Features, fps)
	}

	return summary, nil
}

// FormatPeriodSummary formats a period summary as markdown.
func FormatPeriodSummary(s *PeriodSummary) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Summary: This %s\n\n", s.Period)
	if len(s.Features) == 0 {
		b.WriteString("No activity recorded in this period.\n")
		return b.String()
	}
	fmt.Fprintf(&b, "**%d commits, %d decisions, %d blockers** across %d features\n\n",
		s.TotalCommits, s.TotalDecisions, s.TotalBlockers, len(s.Features))
	for _, f := range s.Features {
		parts := []string{}
		if f.Commits > 0 {
			parts = append(parts, fmt.Sprintf("%d commits", f.Commits))
		}
		if f.Decisions > 0 {
			parts = append(parts, fmt.Sprintf("%d decisions", f.Decisions))
		}
		if f.Blockers > 0 {
			parts = append(parts, fmt.Sprintf("%d blockers", f.Blockers))
		}
		if f.Notes > 0 && f.Notes != f.Decisions+f.Blockers {
			parts = append(parts, fmt.Sprintf("%d notes", f.Notes))
		}
		detail := "no activity"
		if len(parts) > 0 {
			detail = strings.Join(parts, ", ")
		}
		fmt.Fprintf(&b, "- **%s**: %s\n", f.Name, detail)
	}
	return b.String()
}
