package git_test

import (
	"testing"

	"github.com/arbaz/devmem/internal/git"
)

func TestClassifyIntent_ConventionalCommits(t *testing.T) {
	tests := []struct {
		message    string
		wantType   string
		wantConf   float64
	}{
		{"feat: add user authentication", "feature", 0.9},
		{"feat(auth): add login flow", "feature", 0.9},
		{"fix: resolve null pointer crash", "bugfix", 0.9},
		{"fix(api): handle empty response", "bugfix", 0.9},
		{"docs: update API reference", "docs", 0.9},
		{"test: add unit tests for parser", "test", 0.9},
		{"refactor: extract validation logic", "refactor", 0.9},
		{"chore: update dependencies", "cleanup", 0.9},
		{"ci: add GitHub Actions workflow", "infra", 0.9},
	}

	for _, tc := range tests {
		t.Run(tc.message, func(t *testing.T) {
			intentType, confidence := git.ClassifyIntent(tc.message, nil)
			if intentType != tc.wantType {
				t.Errorf("ClassifyIntent(%q) type = %s, want %s", tc.message, intentType, tc.wantType)
			}
			if confidence != tc.wantConf {
				t.Errorf("ClassifyIntent(%q) confidence = %f, want %f", tc.message, confidence, tc.wantConf)
			}
		})
	}
}

func TestClassifyIntent_MessageKeywords_Bugfix(t *testing.T) {
	messages := []string{
		"Fix broken login flow",
		"Resolve crash on startup",
		"Patch security vulnerability",
		"Bug in parser fixed",
		"Handle error in database connection",
		"Fix issue with file upload",
	}

	for _, msg := range messages {
		t.Run(msg, func(t *testing.T) {
			intentType, confidence := git.ClassifyIntent(msg, nil)
			if intentType != "bugfix" {
				t.Errorf("ClassifyIntent(%q) = %s, want bugfix", msg, intentType)
			}
			if confidence != 0.8 {
				t.Errorf("ClassifyIntent(%q) confidence = %f, want 0.8", msg, confidence)
			}
		})
	}
}

func TestClassifyIntent_MessageKeywords_Feature(t *testing.T) {
	messages := []string{
		"Add user profile page",
		"Implement search functionality",
		"Support for dark mode",
		"Introduce rate limiting",
		"New caching layer",
	}

	for _, msg := range messages {
		t.Run(msg, func(t *testing.T) {
			intentType, confidence := git.ClassifyIntent(msg, nil)
			if intentType != "feature" {
				t.Errorf("ClassifyIntent(%q) = %s, want feature", msg, intentType)
			}
			if confidence != 0.8 {
				t.Errorf("ClassifyIntent(%q) confidence = %f, want 0.8", msg, confidence)
			}
		})
	}
}

func TestClassifyIntent_MessageKeywords_Refactor(t *testing.T) {
	messages := []string{
		"Refactor database layer",
		"Clean up error handling",
		"Rename variables for clarity",
		"Simplify authentication flow",
		"Reorganize project structure",
		"Restructure API handlers",
	}

	for _, msg := range messages {
		t.Run(msg, func(t *testing.T) {
			intentType, confidence := git.ClassifyIntent(msg, nil)
			if intentType != "refactor" {
				t.Errorf("ClassifyIntent(%q) = %s, want refactor", msg, intentType)
			}
			if confidence != 0.8 {
				t.Errorf("ClassifyIntent(%q) confidence = %f, want 0.8", msg, confidence)
			}
		})
	}
}

func TestClassifyIntent_MessageKeywords_Test(t *testing.T) {
	messages := []string{
		"Test user service thoroughly",
		"Improve spec coverage",
		"Increase coverage to 90%",
	}

	for _, msg := range messages {
		t.Run(msg, func(t *testing.T) {
			intentType, confidence := git.ClassifyIntent(msg, nil)
			if intentType != "test" {
				t.Errorf("ClassifyIntent(%q) = %s, want test", msg, intentType)
			}
			if confidence != 0.8 {
				t.Errorf("ClassifyIntent(%q) confidence = %f, want 0.8", msg, confidence)
			}
		})
	}
}

func TestClassifyIntent_MessageKeywords_Docs(t *testing.T) {
	messages := []string{
		"Update doc strings",
		"README updated for API",
		"Comment explaining algorithm",
		"Update docs for new features",
	}

	for _, msg := range messages {
		t.Run(msg, func(t *testing.T) {
			intentType, confidence := git.ClassifyIntent(msg, nil)
			if intentType != "docs" {
				t.Errorf("ClassifyIntent(%q) = %s, want docs", msg, intentType)
			}
			if confidence != 0.8 {
				t.Errorf("ClassifyIntent(%q) confidence = %f, want 0.8", msg, confidence)
			}
		})
	}
}

func TestClassifyIntent_MessageKeywords_Infra(t *testing.T) {
	messages := []string{
		"Update CI pipeline",
		"Docker image updated",
		"Deploy to staging",
		"Update infra configuration",
		"Update config values",
	}

	for _, msg := range messages {
		t.Run(msg, func(t *testing.T) {
			intentType, confidence := git.ClassifyIntent(msg, nil)
			if intentType != "infra" {
				t.Errorf("ClassifyIntent(%q) = %s, want infra", msg, intentType)
			}
			if confidence != 0.8 {
				t.Errorf("ClassifyIntent(%q) confidence = %f, want 0.8", msg, confidence)
			}
		})
	}
}

func TestClassifyIntent_MessageKeywords_Cleanup(t *testing.T) {
	messages := []string{
		"Cleanup dead code",
		"Lint fixes",
		"Format all Go files",
		"Run cleanup on old data",
	}

	for _, msg := range messages {
		t.Run(msg, func(t *testing.T) {
			intentType, confidence := git.ClassifyIntent(msg, nil)
			if intentType != "cleanup" {
				t.Errorf("ClassifyIntent(%q) = %s, want cleanup", msg, intentType)
			}
			if confidence != 0.8 {
				t.Errorf("ClassifyIntent(%q) confidence = %f, want 0.8", msg, confidence)
			}
		})
	}
}

func TestClassifyIntent_FileSignals_TestFiles(t *testing.T) {
	files := []string{
		"internal/git/reader_test.go",
		"internal/git/intent_test.go",
	}

	intentType, confidence := git.ClassifyIntent("update test suite", files)
	// "test" keyword should match first at 0.8
	if intentType != "test" {
		t.Errorf("expected test, got %s", intentType)
	}
	if confidence != 0.8 {
		t.Errorf("expected 0.8, got %f", confidence)
	}

	// Now test with a non-keyword message — should fall through to file signals
	intentType, confidence = git.ClassifyIntent("minor changes", files)
	if intentType != "test" {
		t.Errorf("expected test from file signals, got %s", intentType)
	}
	if confidence != 0.6 {
		t.Errorf("expected 0.6 from file signals, got %f", confidence)
	}
}

func TestClassifyIntent_FileSignals_InfraFiles(t *testing.T) {
	tests := []struct {
		name  string
		files []string
	}{
		{"Dockerfile", []string{"Dockerfile"}},
		{"yaml", []string{"docker-compose.yml", "config.yaml"}},
		{"github", []string{".github/workflows/ci.yml"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			intentType, confidence := git.ClassifyIntent("minor changes", tc.files)
			if intentType != "infra" {
				t.Errorf("expected infra, got %s", intentType)
			}
			if confidence != 0.6 {
				t.Errorf("expected 0.6, got %f", confidence)
			}
		})
	}
}

func TestClassifyIntent_FileSignals_DocFiles(t *testing.T) {
	tests := []struct {
		name  string
		files []string
	}{
		{"markdown", []string{"README.md", "CHANGELOG.md"}},
		{"docs dir", []string{"docs/api.md", "docs/guide.md"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			intentType, confidence := git.ClassifyIntent("minor changes", tc.files)
			if intentType != "docs" {
				t.Errorf("expected docs, got %s", intentType)
			}
			if confidence != 0.6 {
				t.Errorf("expected 0.6, got %f", confidence)
			}
		})
	}
}

func TestClassifyIntent_FileSignals_MixedFilesNoMatch(t *testing.T) {
	// Mixed files should not match any file signal pattern
	files := []string{"main.go", "main_test.go"}

	intentType, confidence := git.ClassifyIntent("minor changes", files)
	if intentType != "unknown" {
		t.Errorf("expected unknown for mixed files, got %s", intentType)
	}
	if confidence != 0.0 {
		t.Errorf("expected 0.0, got %f", confidence)
	}
}

func TestClassifyIntent_Default_Unknown(t *testing.T) {
	intentType, confidence := git.ClassifyIntent("v1.2.3", nil)
	if intentType != "unknown" {
		t.Errorf("expected unknown, got %s", intentType)
	}
	if confidence != 0.0 {
		t.Errorf("expected 0.0, got %f", confidence)
	}
}

func TestClassifyIntent_ConventionalOverridesKeyword(t *testing.T) {
	// "fix:" prefix should take priority (0.9) over "add" keyword (0.8)
	intentType, confidence := git.ClassifyIntent("fix: add validation", nil)
	if intentType != "bugfix" {
		t.Errorf("expected bugfix (conventional), got %s", intentType)
	}
	if confidence != 0.9 {
		t.Errorf("expected 0.9, got %f", confidence)
	}
}

func TestClassifyIntent_CaseInsensitive(t *testing.T) {
	intentType, _ := git.ClassifyIntent("FIX: uppercase prefix", nil)
	if intentType != "bugfix" {
		t.Errorf("expected bugfix for uppercase, got %s", intentType)
	}

	intentType, _ = git.ClassifyIntent("REFACTOR database layer", nil)
	if intentType != "refactor" {
		t.Errorf("expected refactor for uppercase keyword, got %s", intentType)
	}
}

func TestClassifyIntent_JSTestFiles(t *testing.T) {
	files := []string{
		"src/components/Button.test.tsx",
		"src/utils/helpers.spec.js",
	}

	intentType, confidence := git.ClassifyIntent("minor changes", files)
	if intentType != "test" {
		t.Errorf("expected test for JS test files, got %s", intentType)
	}
	if confidence != 0.6 {
		t.Errorf("expected 0.6, got %f", confidence)
	}
}

func TestClassifyIntent_EmptyMessage(t *testing.T) {
	intentType, confidence := git.ClassifyIntent("", nil)
	if intentType != "unknown" {
		t.Errorf("ClassifyIntent(\"\") type = %s, want unknown", intentType)
	}
	if confidence != 0.0 {
		t.Errorf("ClassifyIntent(\"\") confidence = %f, want 0.0", confidence)
	}
}

func TestClassifyIntent_WhitespaceOnly(t *testing.T) {
	tests := []struct {
		name    string
		message string
	}{
		{"single space", " "},
		{"multiple spaces", "   "},
		{"tab", "\t"},
		{"newline", "\n"},
		{"mixed whitespace", " \t\n  \t"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			intentType, confidence := git.ClassifyIntent(tc.message, nil)
			if intentType != "unknown" {
				t.Errorf("ClassifyIntent(%q) type = %s, want unknown", tc.message, intentType)
			}
			if confidence != 0.0 {
				t.Errorf("ClassifyIntent(%q) confidence = %f, want 0.0", tc.message, confidence)
			}
		})
	}
}

func TestClassifyIntent_MultipleKeywords_FirstMatchWins(t *testing.T) {
	// The tokenizer splits the message into words and iterates over them in order.
	// The first matching keyword determines the intent type.
	tests := []struct {
		name     string
		message  string
		wantType string
		wantConf float64
	}{
		{
			name:     "fix before add",
			message:  "Fix add user validation",
			wantType: "bugfix",
			wantConf: 0.8,
		},
		{
			name:     "add before fix",
			message:  "Add fix for broken tests",
			wantType: "feature",
			wantConf: 0.8,
		},
		{
			name:     "refactor before test",
			message:  "Refactor test helper utilities",
			wantType: "refactor",
			wantConf: 0.8,
		},
		{
			name:     "test before deploy",
			message:  "Test deploy pipeline changes",
			wantType: "test",
			wantConf: 0.8,
		},
		{
			name:     "doc before cleanup",
			message:  "Doc cleanup for API reference",
			wantType: "docs",
			wantConf: 0.8,
		},
		{
			name:     "cleanup before new",
			message:  "Cleanup new unused imports",
			wantType: "cleanup",
			wantConf: 0.8,
		},
		{
			name:     "conventional prefix overrides all keywords",
			message:  "feat: fix refactor test doc deploy cleanup",
			wantType: "feature",
			wantConf: 0.9,
		},
		{
			name:     "fix prefix overrides add keyword",
			message:  "fix: add new feature support",
			wantType: "bugfix",
			wantConf: 0.9,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			intentType, confidence := git.ClassifyIntent(tc.message, nil)
			if intentType != tc.wantType {
				t.Errorf("ClassifyIntent(%q) type = %s, want %s", tc.message, intentType, tc.wantType)
			}
			if confidence != tc.wantConf {
				t.Errorf("ClassifyIntent(%q) confidence = %f, want %f", tc.message, confidence, tc.wantConf)
			}
		})
	}
}
