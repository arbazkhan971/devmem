package plans_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/arbazkhan971/memorx/internal/plans"
)

func TestIsPlanLike_WithPlanContent(t *testing.T) {
	content := `Implementation plan for the new API:
1. Set up database schema
2. Implement REST endpoints
3. Write integration tests
4. Deploy to staging`

	if !plans.IsPlanLike(content) {
		t.Error("expected IsPlanLike to return true for plan content")
	}
}

func TestIsPlanLike_WithRegularText(t *testing.T) {
	content := `This is just a regular note about the project.
It discusses some architecture decisions and trade-offs.
Nothing particularly structured here.`

	if plans.IsPlanLike(content) {
		t.Error("expected IsPlanLike to return false for regular text")
	}
}

func TestIsPlanLike_NumberedListWithoutKeyword(t *testing.T) {
	content := `Here are some items:
1. First thing
2. Second thing
3. Third thing`

	if plans.IsPlanLike(content) {
		t.Error("expected IsPlanLike to return false for numbered list without plan keyword")
	}
}

func TestIsPlanLike_KeywordWithoutEnoughItems(t *testing.T) {
	content := `Here is the plan:
1. Only one step`

	if plans.IsPlanLike(content) {
		t.Error("expected IsPlanLike to return false with fewer than 3 items")
	}
}

func TestIsPlanLike_StepsKeyword(t *testing.T) {
	content := `Steps to complete:
1. Design the schema
2. Write migrations
3. Add indexes`

	if !plans.IsPlanLike(content) {
		t.Error("expected IsPlanLike to return true with 'steps' keyword")
	}
}

func TestIsPlanLike_TodoKeyword(t *testing.T) {
	content := `TODO items:
1. Fix the bug
2. Update docs
3. Release version`

	if !plans.IsPlanLike(content) {
		t.Error("expected IsPlanLike to return true with 'todo' keyword")
	}
}

func TestParseSteps_DotFormat(t *testing.T) {
	content := "1. First step\n2. Second step\n3. Third step"
	steps := plans.ParseSteps(content)

	if len(steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(steps))
	}
	if steps[0].Title != "First step" {
		t.Errorf("expected 'First step', got %q", steps[0].Title)
	}
	if steps[1].Title != "Second step" {
		t.Errorf("expected 'Second step', got %q", steps[1].Title)
	}
	if steps[2].Title != "Third step" {
		t.Errorf("expected 'Third step', got %q", steps[2].Title)
	}
}

func TestParseSteps_ParenFormat(t *testing.T) {
	content := "1) Create module\n2) Add dependencies\n3) Write tests"
	steps := plans.ParseSteps(content)

	if len(steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(steps))
	}
	if steps[0].Title != "Create module" {
		t.Errorf("expected 'Create module', got %q", steps[0].Title)
	}
	if steps[1].Title != "Add dependencies" {
		t.Errorf("expected 'Add dependencies', got %q", steps[1].Title)
	}
}

func TestParseSteps_DashStepFormat(t *testing.T) {
	content := "- Step: Initialize project\n- Step: Setup CI\n- Step: Deploy"
	steps := plans.ParseSteps(content)

	if len(steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(steps))
	}
	if steps[0].Title != "Initialize project" {
		t.Errorf("expected 'Initialize project', got %q", steps[0].Title)
	}
}

func TestParseSteps_MixedFormats(t *testing.T) {
	content := `1. First step
2) Second step
- Step: Third step
Some non-step text
3. Fourth step`

	steps := plans.ParseSteps(content)

	if len(steps) != 4 {
		t.Fatalf("expected 4 steps, got %d", len(steps))
	}
	if steps[0].Title != "First step" {
		t.Errorf("expected 'First step', got %q", steps[0].Title)
	}
	if steps[1].Title != "Second step" {
		t.Errorf("expected 'Second step', got %q", steps[1].Title)
	}
	if steps[2].Title != "Third step" {
		t.Errorf("expected 'Third step', got %q", steps[2].Title)
	}
	if steps[3].Title != "Fourth step" {
		t.Errorf("expected 'Fourth step', got %q", steps[3].Title)
	}
}

func TestParseSteps_MarkdownDashFormat(t *testing.T) {
	// Plain markdown dashes ("- Step one") are NOT one of the supported formats.
	// The supported formats are: "1. Title", "1) Title", "- Step: Title"
	// Plain "- item" should not match.
	content := "- Step one\n- Step two\n- Step three"
	steps := plans.ParseSteps(content)

	// These are plain dash items without "Step:" prefix, so they should NOT be parsed
	if len(steps) != 0 {
		t.Errorf("expected 0 steps for plain dash format (not supported), got %d", len(steps))
	}
}

func TestParseSteps_DashStepColonFormat(t *testing.T) {
	// "- Step: Title" IS a supported format
	content := "- Step: Design the API\n- Step: Implement handlers\n- Step: Write tests"
	steps := plans.ParseSteps(content)

	if len(steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(steps))
	}
	if steps[0].Title != "Design the API" {
		t.Errorf("expected 'Design the API', got %q", steps[0].Title)
	}
	if steps[1].Title != "Implement handlers" {
		t.Errorf("expected 'Implement handlers', got %q", steps[1].Title)
	}
	if steps[2].Title != "Write tests" {
		t.Errorf("expected 'Write tests', got %q", steps[2].Title)
	}
}

func TestParseSteps_EmptyContent(t *testing.T) {
	steps := plans.ParseSteps("")
	if len(steps) != 0 {
		t.Errorf("expected 0 steps for empty content, got %d", len(steps))
	}
}

func TestParseSteps_NoMatchingLines(t *testing.T) {
	content := "Just some regular text\nwith no numbered items\nat all."
	steps := plans.ParseSteps(content)
	if len(steps) != 0 {
		t.Errorf("expected 0 steps, got %d", len(steps))
	}
}

func TestIsPlanLike_OnlyKeywordsNoNumbers(t *testing.T) {
	// Has plan keywords but no numbered items at all
	content := `This is a plan for the implementation phase.
We need steps to complete the milestone.
But there are no numbered items here.`

	if plans.IsPlanLike(content) {
		t.Error("expected IsPlanLike to return false when keywords exist but no numbered items")
	}
}

func TestIsPlanLike_UnicodeContent(t *testing.T) {
	// Unicode content with plan structure
	content := `Implementation plan for the API:
1. Create the database schema with UTF-8 support (日本語テスト)
2. Add internationalization endpoints (données françaises)
3. Write tests for multi-language content (Ünïcödé)`

	if !plans.IsPlanLike(content) {
		t.Error("expected IsPlanLike to return true for plan with unicode content")
	}
}

func TestIsPlanLike_UnicodeContentNoPlan(t *testing.T) {
	// Unicode content without plan keywords
	content := `日本語のテキスト。
これは普通のメモです。
特にプランではありません。`

	if plans.IsPlanLike(content) {
		t.Error("expected IsPlanLike to return false for unicode text without plan keywords")
	}
}

func TestIsPlanLike_VariousEdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{"just whitespace", "   \n\t  ", false},
		{"keyword no list", "This is a plan", false},
		{"milestone with items", "Milestone items:\n1. Do A\n2. Do B\n3. Do C", true},
		{"phase with items", "Phase 1 breakdown:\n1. Gather requirements\n2. Design schema\n3. Build MVP", true},
		{"implementation with parens", "Implementation details:\n1) First\n2) Second\n3) Third", true},
		{"single line", "plan: 1. A 2. B 3. C", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := plans.IsPlanLike(tc.content)
			if got != tc.want {
				t.Errorf("IsPlanLike(%q) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

func TestParseSteps_WhitespaceOnly(t *testing.T) {
	steps := plans.ParseSteps("   \n\t  \n  ")
	if len(steps) != 0 {
		t.Errorf("expected 0 steps for whitespace, got %d", len(steps))
	}
}

func TestParseSteps_100PlusSteps(t *testing.T) {
	// Stress test: generate 150 numbered steps
	var lines []string
	for i := 1; i <= 150; i++ {
		lines = append(lines, fmt.Sprintf("%d. Step number %d", i, i))
	}
	content := strings.Join(lines, "\n")

	steps := plans.ParseSteps(content)
	if len(steps) != 150 {
		t.Fatalf("expected 150 steps, got %d", len(steps))
	}
	if steps[0].Title != "Step number 1" {
		t.Errorf("expected first step title 'Step number 1', got %q", steps[0].Title)
	}
	if steps[149].Title != "Step number 150" {
		t.Errorf("expected last step title 'Step number 150', got %q", steps[149].Title)
	}
}
