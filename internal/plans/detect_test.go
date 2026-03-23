package plans_test

import (
	"testing"

	"github.com/arbaz/devmem/internal/plans"
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
