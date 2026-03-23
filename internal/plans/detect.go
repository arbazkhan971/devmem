package plans

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// stepPattern matches numbered list formats: "1. Title", "1) Title", "- Step: Title"
var stepPattern = regexp.MustCompile(`(?m)^\s*(?:\d+[.)]\s+|-\s+Step:\s+)(.+)`)

// keywordPattern matches plan-related keywords (case-insensitive).
var keywordPattern = regexp.MustCompile(`(?i)\b(?:plan|steps|todo|phase|milestone|implementation)\b`)

// IsPlanLike returns true if content has 3+ numbered items AND contains
// at least one plan-related keyword.
func IsPlanLike(content string) bool {
	return keywordPattern.MatchString(content) && len(stepPattern.FindAllString(content, -1)) >= 3
}

// ParseSteps extracts numbered items as steps from text content.
// Supports formats: "1. Title", "1) Title", "- Step: Title"
func ParseSteps(content string) []StepInput {
	matches := stepPattern.FindAllStringSubmatch(content, -1)
	steps := make([]StepInput, 0, len(matches))
	for _, m := range matches {
		if title := strings.TrimSpace(m[1]); title != "" {
			steps = append(steps, StepInput{Title: title})
		}
	}
	return steps
}

// matchThreshold is the minimum Jaccard similarity for a commit-step match.
const matchThreshold = 0.3

// MatchCommitToSteps finds the best matching incomplete step for a commit message.
// Uses Jaccard similarity between the commit message and each pending/in_progress
// step title (and description). Returns nil if no match exceeds the threshold.
func (m *Manager) MatchCommitToSteps(commitMessage, featureID string) (*PlanStep, error) {
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
		if s.Status == "completed" || s.Status == "skipped" {
			continue
		}
		score := jaccard(commitWords, tokenize(s.Title))
		if s.Description != "" {
			if ds := jaccard(commitWords, tokenize(s.Description)); ds > score {
				score = ds
			}
		}
		if score > bestScore {
			bestScore, bestStep = score, s
		}
	}
	if bestScore < matchThreshold {
		return nil, nil
	}
	result := *bestStep
	return &result, nil
}

// tokenize splits text into a set of lowercase words.
func tokenize(text string) map[string]bool {
	words := make(map[string]bool)
	for _, w := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		words[w] = true
	}
	return words
}

// jaccard computes |A intersect B| / |A union B|.
func jaccard(a, b map[string]bool) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}
	inter := 0
	for w := range a {
		if b[w] {
			inter++
		}
	}
	if union := len(a) + len(b) - inter; union > 0 {
		return float64(inter) / float64(union)
	}
	return 0
}
