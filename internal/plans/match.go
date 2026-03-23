package plans

import (
	"fmt"
	"strings"
	"unicode"
)

// matchThreshold is the minimum Jaccard similarity for a commit-step match.
const matchThreshold = 0.3

// MatchCommitToSteps finds the best matching incomplete step for a commit message.
// It uses Jaccard similarity (intersection / union of words) between the commit
// message and each pending/in_progress step title. Returns nil if no match
// exceeds the threshold.
func (m *Manager) MatchCommitToSteps(commitMessage string, featureID string) (*PlanStep, error) {
	plan, err := m.GetActivePlan(featureID)
	if err != nil {
		return nil, fmt.Errorf("get active plan: %w", err)
	}

	steps, err := m.GetPlanSteps(plan.ID)
	if err != nil {
		return nil, fmt.Errorf("get plan steps: %w", err)
	}

	commitWords := tokenize(commitMessage)
	if len(commitWords) == 0 {
		return nil, nil
	}

	var bestStep *PlanStep
	bestScore := 0.0

	for i := range steps {
		s := &steps[i]
		// Only consider incomplete steps
		if s.Status == "completed" || s.Status == "skipped" {
			continue
		}

		// Calculate Jaccard similarity with step title
		stepWords := tokenize(s.Title)
		if len(stepWords) == 0 {
			continue
		}

		score := jaccardSimilarity(commitWords, stepWords)

		// Also check description if available
		if s.Description != "" {
			descWords := tokenize(s.Description)
			descScore := jaccardSimilarity(commitWords, descWords)
			if descScore > score {
				score = descScore
			}
		}

		if score > bestScore {
			bestScore = score
			bestStep = s
		}
	}

	if bestScore < matchThreshold {
		return nil, nil
	}

	// Return a copy to avoid pointer to loop variable issues
	result := *bestStep
	return &result, nil
}

// tokenize splits text into lowercase words, removing punctuation.
func tokenize(text string) map[string]bool {
	words := make(map[string]bool)
	lower := strings.ToLower(text)

	var current strings.Builder
	for _, r := range lower {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current.WriteRune(r)
		} else {
			if current.Len() > 0 {
				words[current.String()] = true
				current.Reset()
			}
		}
	}
	if current.Len() > 0 {
		words[current.String()] = true
	}
	return words
}

// jaccardSimilarity computes |A intersect B| / |A union B|.
func jaccardSimilarity(a, b map[string]bool) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}

	intersection := 0
	for w := range a {
		if b[w] {
			intersection++
		}
	}

	union := len(a)
	for w := range b {
		if !a[w] {
			union++
		}
	}

	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}
