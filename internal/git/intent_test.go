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

func TestClassifyIntent_ConfidenceLevelsCorrect(t *testing.T) {
	// Conventional prefix should always be 0.9
	_, conf := git.ClassifyIntent("feat: something", nil)
	if conf != 0.9 {
		t.Errorf("conventional prefix confidence should be 0.9, got %f", conf)
	}

	// Keyword match should always be 0.8
	_, conf = git.ClassifyIntent("Add new feature", nil)
	if conf != 0.8 {
		t.Errorf("keyword confidence should be 0.8, got %f", conf)
	}

	// File signal should always be 0.6
	_, conf = git.ClassifyIntent("changes", []string{"test_helper_test.go"})
	if conf != 0.6 {
		t.Errorf("file signal confidence should be 0.6, got %f", conf)
	}

	// No match should be 0.0
	_, conf = git.ClassifyIntent("v1.0.0", nil)
	if conf != 0.0 {
		t.Errorf("no match confidence should be 0.0, got %f", conf)
	}
}

func TestClassifyIntent_AllFileTypes_NoFiles(t *testing.T) {
	// Empty files list should not match any file signal
	intentType, _ := git.ClassifyIntent("changes", []string{})
	if intentType != "unknown" {
		t.Errorf("expected unknown for empty files, got %s", intentType)
	}
}

func TestClassifyIntent_ConfidenceTiers(t *testing.T) {
	// Conventional prefix = 0.9
	_, c1 := git.ClassifyIntent("feat: new", nil)
	if c1 != 0.9 {
		t.Errorf("conventional: expected 0.9, got %f", c1)
	}
	// Keyword = 0.8
	_, c2 := git.ClassifyIntent("Add login page", nil)
	if c2 != 0.8 {
		t.Errorf("keyword: expected 0.8, got %f", c2)
	}
	// File signal = 0.6
	_, c3 := git.ClassifyIntent("changes", []string{"foo_test.go"})
	if c3 != 0.6 {
		t.Errorf("file signal: expected 0.6, got %f", c3)
	}
	// Unknown = 0.0
	_, c4 := git.ClassifyIntent("v2.0.0", nil)
	if c4 != 0.0 {
		t.Errorf("unknown: expected 0.0, got %f", c4)
	}
}

func TestClassifyIntent_AllCategories(t *testing.T) {
	tests := []struct {
		message  string
		files    []string
		wantType string
	}{
		{"feat: add API", nil, "feature"},
		{"fix: null pointer", nil, "bugfix"},
		{"docs: update readme", nil, "docs"},
		{"test: add coverage", nil, "test"},
		{"refactor: extract method", nil, "refactor"},
		{"chore: update deps", nil, "cleanup"},
		{"ci: add pipeline", nil, "infra"},
		{"v2.0.0-rc1", nil, "unknown"},
	}
	for _, tc := range tests {
		t.Run(tc.wantType, func(t *testing.T) {
			got, _ := git.ClassifyIntent(tc.message, tc.files)
			if got != tc.wantType {
				t.Errorf("ClassifyIntent(%q) = %s, want %s", tc.message, got, tc.wantType)
			}
		})
	}
}

func TestIntentKeyword_Fix(t *testing.T) {
	got, c := git.ClassifyIntent("Fix broken login", nil)
	if got != "bugfix" || c != 0.8 { t.Errorf("got %s/%f", got, c) }
}
func TestIntentKeyword_Bug(t *testing.T) {
	got, c := git.ClassifyIntent("Bug in authentication", nil)
	if got != "bugfix" || c != 0.8 { t.Errorf("got %s/%f", got, c) }
}
func TestIntentKeyword_Patch(t *testing.T) {
	got, c := git.ClassifyIntent("Patch memory leak", nil)
	if got != "bugfix" || c != 0.8 { t.Errorf("got %s/%f", got, c) }
}
func TestIntentKeyword_Add(t *testing.T) {
	got, c := git.ClassifyIntent("Add export feature", nil)
	if got != "feature" || c != 0.8 { t.Errorf("got %s/%f", got, c) }
}
func TestIntentKeyword_Implement(t *testing.T) {
	got, c := git.ClassifyIntent("Implement caching", nil)
	if got != "feature" || c != 0.8 { t.Errorf("got %s/%f", got, c) }
}
func TestIntentKeyword_New(t *testing.T) {
	got, c := git.ClassifyIntent("New dashboard page", nil)
	if got != "feature" || c != 0.8 { t.Errorf("got %s/%f", got, c) }
}
func TestIntentKeyword_Refactor(t *testing.T) {
	got, c := git.ClassifyIntent("Refactor the handler", nil)
	if got != "refactor" || c != 0.8 { t.Errorf("got %s/%f", got, c) }
}
func TestIntentKeyword_Clean(t *testing.T) {
	got, c := git.ClassifyIntent("Clean up dead code", nil)
	if got != "refactor" || c != 0.8 { t.Errorf("got %s/%f", got, c) }
}
func TestIntentKeyword_Rename(t *testing.T) {
	got, c := git.ClassifyIntent("Rename variables", nil)
	if got != "refactor" || c != 0.8 { t.Errorf("got %s/%f", got, c) }
}
func TestIntentKeyword_Test(t *testing.T) {
	got, c := git.ClassifyIntent("Test the parser", nil)
	if got != "test" || c != 0.8 { t.Errorf("got %s/%f", got, c) }
}
func TestIntentKeyword_Spec(t *testing.T) {
	got, c := git.ClassifyIntent("Spec for auth module", nil)
	if got != "test" || c != 0.8 { t.Errorf("got %s/%f", got, c) }
}
func TestIntentKeyword_Doc(t *testing.T) {
	got, c := git.ClassifyIntent("Doc for API endpoints", nil)
	if got != "docs" || c != 0.8 { t.Errorf("got %s/%f", got, c) }
}
func TestIntentKeyword_CI(t *testing.T) {
	got, c := git.ClassifyIntent("CI pipeline update", nil)
	if got != "infra" || c != 0.8 { t.Errorf("got %s/%f", got, c) }
}
func TestIntentKeyword_Deploy(t *testing.T) {
	got, c := git.ClassifyIntent("Deploy new version", nil)
	if got != "infra" || c != 0.8 { t.Errorf("got %s/%f", got, c) }
}
func TestIntentKeyword_Lint(t *testing.T) {
	got, c := git.ClassifyIntent("Lint all files", nil)
	if got != "cleanup" || c != 0.8 { t.Errorf("got %s/%f", got, c) }
}

func TestConventionalCommit_Feat(t *testing.T) {
	got, c := git.ClassifyIntent("feat: add auth", nil)
	if got != "feature" || c != 0.9 { t.Errorf("got %s/%f", got, c) }
}
func TestConventionalCommit_Fix(t *testing.T) {
	got, c := git.ClassifyIntent("fix: null deref", nil)
	if got != "bugfix" || c != 0.9 { t.Errorf("got %s/%f", got, c) }
}
func TestConventionalCommit_Docs(t *testing.T) {
	got, c := git.ClassifyIntent("docs: update readme", nil)
	if got != "docs" || c != 0.9 { t.Errorf("got %s/%f", got, c) }
}
func TestConventionalCommit_Test(t *testing.T) {
	got, c := git.ClassifyIntent("test: add coverage", nil)
	if got != "test" || c != 0.9 { t.Errorf("got %s/%f", got, c) }
}
func TestConventionalCommit_Refactor(t *testing.T) {
	got, c := git.ClassifyIntent("refactor: extract fn", nil)
	if got != "refactor" || c != 0.9 { t.Errorf("got %s/%f", got, c) }
}
func TestConventionalCommit_Chore(t *testing.T) {
	got, c := git.ClassifyIntent("chore: bump deps", nil)
	if got != "cleanup" || c != 0.9 { t.Errorf("got %s/%f", got, c) }
}
func TestConventionalCommit_CI(t *testing.T) {
	got, c := git.ClassifyIntent("ci: add workflow", nil)
	if got != "infra" || c != 0.9 { t.Errorf("got %s/%f", got, c) }
}
func TestConventionalCommit_Unknown(t *testing.T) {
	got, c := git.ClassifyIntent("v1.0.0 release", nil)
	if got != "unknown" || c != 0.0 { t.Errorf("got %s/%f", got, c) }
}

func TestIntentKeywords(t *testing.T) {
	for _, tc := range []struct{ name, message, wantType string }{
		{"fix_keyword", "Fix the broken login", "bugfix"},
		{"bug_keyword", "Bug in authentication", "bugfix"},
		{"patch_keyword", "Patch memory leak", "bugfix"},
		{"add_keyword", "Add export feature", "feature"},
		{"implement_keyword", "Implement caching", "feature"},
		{"new_keyword", "New dashboard page", "feature"},
		{"refactor_keyword", "Refactor the handler", "refactor"},
		{"clean_keyword", "Clean up dead code", "refactor"},
		{"rename_keyword", "Rename variables", "refactor"},
		{"test_keyword", "Test the parser", "test"},
		{"spec_keyword", "Spec for auth module", "test"},
		{"doc_keyword", "Doc for API endpoints", "docs"},
		{"ci_keyword", "CI pipeline update", "infra"},
		{"deploy_keyword", "Deploy new version", "infra"},
		{"lint_keyword", "Lint all files", "cleanup"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, conf := git.ClassifyIntent(tc.message, nil)
			if got != tc.wantType { t.Errorf("got %s, want %s", got, tc.wantType) }
			if conf != 0.8 { t.Errorf("got %f, want 0.8", conf) }
		})
	}
}

func TestConventionalCommits(t *testing.T) {
	for _, tc := range []struct{ name, msg, wantType string; wantConf float64 }{
		{"feat_prefix", "feat: add auth", "feature", 0.9},
		{"fix_prefix", "fix: null deref", "bugfix", 0.9},
		{"docs_prefix", "docs: update readme", "docs", 0.9},
		{"test_prefix", "test: add coverage", "test", 0.9},
		{"refactor_prefix", "refactor: extract fn", "refactor", 0.9},
		{"chore_prefix", "chore: bump deps", "cleanup", 0.9},
		{"ci_prefix", "ci: add workflow", "infra", 0.9},
		{"unknown_prefix", "v1.0.0 release", "unknown", 0.0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, conf := git.ClassifyIntent(tc.msg, nil)
			if got != tc.wantType { t.Errorf("got %s, want %s", got, tc.wantType) }
			if conf != tc.wantConf { t.Errorf("got %f, want %f", conf, tc.wantConf) }
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
