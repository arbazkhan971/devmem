package git

import (
	"path/filepath"
	"strings"
)

// Intent classification keyword maps.
var (
	conventionalPrefixes = map[string]string{
		"feat":     "feature",
		"fix":      "bugfix",
		"docs":     "docs",
		"test":     "test",
		"refactor": "refactor",
		"chore":    "cleanup",
		"ci":       "infra",
	}

	messageKeywords = map[string]string{
		// bugfix
		"fix":     "bugfix",
		"bug":     "bugfix",
		"patch":   "bugfix",
		"resolve": "bugfix",
		"crash":   "bugfix",
		"issue":   "bugfix",
		"error":   "bugfix",
		// feature
		"feat":      "feature",
		"add":       "feature",
		"implement": "feature",
		"support":   "feature",
		"introduce": "feature",
		"new":       "feature",
		// refactor
		"refactor":    "refactor",
		"clean":       "refactor",
		"rename":      "refactor",
		"simplify":    "refactor",
		"reorganize":  "refactor",
		"restructure": "refactor",
		// test
		"test":     "test",
		"spec":     "test",
		"coverage": "test",
		// docs
		"doc":     "docs",
		"readme":  "docs",
		"comment": "docs",
		"docs":    "docs",
		// infra
		"ci":     "infra",
		"cd":     "infra",
		"docker": "infra",
		"deploy": "infra",
		"infra":  "infra",
		"config": "infra",
		// cleanup
		"chore":   "cleanup",
		"cleanup": "cleanup",
		"lint":    "cleanup",
		"format":  "cleanup",
	}
)

// ClassifyIntent classifies a commit's intent based on its message and changed files.
// Returns the intent type and a confidence score between 0.0 and 1.0.
func ClassifyIntent(message string, files []string) (intentType string, confidence float64) {
	// 1. Check conventional commit prefixes (highest confidence: 0.9)
	if it, ok := checkConventionalPrefix(message); ok {
		return it, 0.9
	}

	// 2. Check message keywords (confidence: 0.8)
	if it, ok := checkMessageKeywords(message); ok {
		return it, 0.8
	}

	// 3. Check file path signals (confidence: 0.6)
	if it, ok := checkFileSignals(files); ok {
		return it, 0.6
	}

	// 4. Default
	return "unknown", 0.0
}

// checkConventionalPrefix checks for conventional commit prefixes like "feat:", "fix:", etc.
func checkConventionalPrefix(message string) (string, bool) {
	lower := strings.ToLower(strings.TrimSpace(message))

	for prefix, intentType := range conventionalPrefixes {
		// Check for "prefix:" or "prefix(scope):"
		if strings.HasPrefix(lower, prefix+":") || strings.HasPrefix(lower, prefix+"(") {
			return intentType, true
		}
	}

	return "", false
}

// checkMessageKeywords looks for intent keywords in the commit message.
func checkMessageKeywords(message string) (string, bool) {
	lower := strings.ToLower(message)
	words := tokenize(lower)

	for _, word := range words {
		if intentType, ok := messageKeywords[word]; ok {
			return intentType, true
		}
	}

	return "", false
}

// tokenize splits a message into lowercase words, stripping common punctuation.
func tokenize(message string) []string {
	// Replace common separators with spaces
	r := strings.NewReplacer(
		":", " ",
		"/", " ",
		"-", " ",
		"_", " ",
		"(", " ",
		")", " ",
		"[", " ",
		"]", " ",
		",", " ",
		".", " ",
		"!", " ",
		"#", " ",
	)
	cleaned := r.Replace(message)
	return strings.Fields(cleaned)
}

// checkFileSignals classifies intent based on the file paths changed.
// All files must match the same pattern for a classification to be made.
func checkFileSignals(files []string) (string, bool) {
	if len(files) == 0 {
		return "", false
	}

	// Check if all files are test files
	if allMatch(files, isTestFile) {
		return "test", true
	}

	// Check if all files are infra files
	if allMatch(files, isInfraFile) {
		return "infra", true
	}

	// Check if all files are doc files
	if allMatch(files, isDocFile) {
		return "docs", true
	}

	return "", false
}

func allMatch(files []string, predicate func(string) bool) bool {
	for _, f := range files {
		if !predicate(f) {
			return false
		}
	}
	return true
}

func isTestFile(path string) bool {
	base := filepath.Base(path)
	// Go test files
	if strings.HasSuffix(base, "_test.go") {
		return true
	}
	// JS/TS test files
	if strings.Contains(base, ".test.") || strings.Contains(base, ".spec.") {
		return true
	}
	return false
}

func isInfraFile(path string) bool {
	base := filepath.Base(path)
	lower := strings.ToLower(base)

	if lower == "dockerfile" || lower == "docker-compose.yml" || lower == "docker-compose.yaml" {
		return true
	}
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".yml" || ext == ".yaml" {
		return true
	}
	// .github/* paths
	if strings.HasPrefix(path, ".github/") || strings.HasPrefix(path, ".github"+string(filepath.Separator)) {
		return true
	}
	return false
}

func isDocFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".md" {
		return true
	}
	// docs/* directory
	if strings.HasPrefix(path, "docs/") || strings.HasPrefix(path, "docs"+string(filepath.Separator)) {
		return true
	}
	return false
}
