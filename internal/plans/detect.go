package plans

import (
	"regexp"
	"strings"
)

// planKeywords are words that indicate plan-like content.
var planKeywords = []string{
	"plan", "steps", "todo", "phase", "milestone", "implementation",
}

// numberedPatterns match common numbered list formats.
var numberedPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?m)^\s*\d+\.\s+(.+)`),  // "1. Title"
	regexp.MustCompile(`(?m)^\s*\d+\)\s+(.+)`),   // "1) Title"
	regexp.MustCompile(`(?m)^\s*-\s+Step:\s+(.+)`), // "- Step: Title"
}

// IsPlanLike returns true if content has 3+ numbered items AND contains
// at least one plan-related keyword.
func IsPlanLike(content string) bool {
	lower := strings.ToLower(content)

	// Check for plan keywords
	hasKeyword := false
	for _, kw := range planKeywords {
		if strings.Contains(lower, kw) {
			hasKeyword = true
			break
		}
	}
	if !hasKeyword {
		return false
	}

	// Count numbered items across all patterns
	count := countNumberedItems(content)
	return count >= 3
}

// countNumberedItems counts the total number of numbered list items found.
func countNumberedItems(content string) int {
	seen := make(map[int]bool) // track line indices to avoid double-counting
	lines := strings.Split(content, "\n")

	for _, pat := range numberedPatterns {
		for i, line := range lines {
			if seen[i] {
				continue
			}
			if pat.MatchString(line) {
				seen[i] = true
			}
		}
	}
	return len(seen)
}

// ParseSteps extracts numbered items as steps from text content.
// Supports formats: "1. Title", "1) Title", "- Step: Title"
func ParseSteps(content string) []StepInput {
	var steps []StepInput
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		for _, pat := range numberedPatterns {
			matches := pat.FindStringSubmatch(line)
			if len(matches) >= 2 {
				title := strings.TrimSpace(matches[1])
				if title != "" {
					steps = append(steps, StepInput{Title: title, Description: ""})
				}
				break // only match first pattern per line
			}
		}
	}
	return steps
}
