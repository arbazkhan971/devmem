package memory_test

import (
	"testing"

	"github.com/arbazkhan971/memorx/internal/memory"
)

func TestExtractFromText_Decisions(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{"decided", "We decided to use PostgreSQL for the database."},
		{"chose", "The team chose React over Vue for the frontend."},
		{"selected", "We selected AWS Lambda for serverless compute."},
		{"agreed", "Everyone agreed on the REST API approach."},
		{"went with", "We went with a monorepo structure."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := memory.ExtractFromText(tt.text)
			if len(items) == 0 {
				t.Fatalf("expected at least 1 item, got 0")
			}
			found := false
			for _, item := range items {
				if item.Type == "decision" {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected a decision item, got types: %v", itemTypes(items))
			}
		})
	}
}

func TestExtractFromText_Blockers(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{"blocked", "We are blocked on the API review."},
		{"stuck", "I'm stuck on the authentication flow."},
		{"cant", "We can't deploy until the CI is fixed."},
		{"waiting on", "We're waiting on the design team for mockups."},
		{"depends on", "This feature depends on the auth service being ready."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := memory.ExtractFromText(tt.text)
			if len(items) == 0 {
				t.Fatalf("expected at least 1 item, got 0")
			}
			found := false
			for _, item := range items {
				if item.Type == "blocker" {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected a blocker item, got types: %v", itemTypes(items))
			}
		})
	}
}

func TestExtractFromText_Progress(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{"done", "The migration is done."},
		{"completed", "We completed the API integration."},
		{"finished", "I finished the unit tests."},
		{"implemented", "We implemented the caching layer."},
		{"shipped", "We shipped the new dashboard."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := memory.ExtractFromText(tt.text)
			if len(items) == 0 {
				t.Fatalf("expected at least 1 item, got 0")
			}
			found := false
			for _, item := range items {
				if item.Type == "progress" {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected a progress item, got types: %v", itemTypes(items))
			}
		})
	}
}

func TestExtractFromText_Facts(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		subject   string
		predicate string
		object    string
	}{
		{"uses", "The backend uses Go for all services.", "backend", "uses", "Go for all services"},
		{"is", "The database is PostgreSQL.", "database", "is", "PostgreSQL"},
		{"runs on", "The app runs on Kubernetes.", "app", "runs on", "Kubernetes"},
		{"built with", "The frontend built with React.", "frontend", "built with", "React"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := memory.ExtractFromText(tt.text)
			if len(items) == 0 {
				t.Fatalf("expected at least 1 item, got 0")
			}
			var factItem *memory.ExtractedItem
			for i, item := range items {
				if item.Type == "fact" {
					factItem = &items[i]
					break
				}
			}
			if factItem == nil {
				t.Fatalf("expected a fact item, got types: %v", itemTypes(items))
			}
			if factItem.Fact == nil {
				t.Fatal("expected Fact triple to be non-nil")
			}
			if factItem.Fact.Predicate != tt.predicate {
				t.Errorf("expected predicate %q, got %q", tt.predicate, factItem.Fact.Predicate)
			}
		})
	}
}

func TestExtractFromText_NextSteps(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{"need to", "We need to add error handling."},
		{"should", "We should refactor the auth module."},
		{"will", "I will set up the CI pipeline tomorrow."},
		{"todo", "Todo: write integration tests."},
		{"next", "Next we tackle the payment flow."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := memory.ExtractFromText(tt.text)
			if len(items) == 0 {
				t.Fatalf("expected at least 1 item, got 0")
			}
			found := false
			for _, item := range items {
				if item.Type == "next_step" {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected a next_step item, got types: %v", itemTypes(items))
			}
		})
	}
}

func TestExtractFromText_MultiSentence(t *testing.T) {
	text := "We decided to use GraphQL. The API is REST-based for now. " +
		"I'm blocked on the auth service. We completed the database migration. " +
		"Next we need to add caching."

	items := memory.ExtractFromText(text)

	counts := make(map[string]int)
	for _, item := range items {
		counts[item.Type]++
	}

	if counts["decision"] < 1 {
		t.Errorf("expected at least 1 decision, got %d", counts["decision"])
	}
	if counts["blocker"] < 1 {
		t.Errorf("expected at least 1 blocker, got %d", counts["blocker"])
	}
	if counts["progress"] < 1 {
		t.Errorf("expected at least 1 progress, got %d", counts["progress"])
	}
	if counts["next_step"] < 1 {
		t.Errorf("expected at least 1 next_step, got %d", counts["next_step"])
	}
}

func TestExtractFromText_EmptyText(t *testing.T) {
	items := memory.ExtractFromText("")
	if len(items) != 0 {
		t.Errorf("expected 0 items from empty text, got %d", len(items))
	}
}

func TestExtractFromText_NoMatches(t *testing.T) {
	items := memory.ExtractFromText("Hello world. How are you?")
	// "Hello world" doesn't match any pattern. "How are you" might or might not.
	for _, item := range items {
		if item.Type == "decision" || item.Type == "blocker" || item.Type == "progress" {
			t.Errorf("unexpected type %q for generic text", item.Type)
		}
	}
}

func TestExtractFromText_NewlineSentences(t *testing.T) {
	text := "We decided to use SQLite\nThe migration is done\nNext we add tests"

	items := memory.ExtractFromText(text)
	counts := make(map[string]int)
	for _, item := range items {
		counts[item.Type]++
	}

	if counts["decision"] < 1 {
		t.Errorf("expected at least 1 decision from newline-separated text, got %d", counts["decision"])
	}
	if counts["progress"] < 1 {
		t.Errorf("expected at least 1 progress from newline-separated text, got %d", counts["progress"])
	}
}

func TestExtractFromText_FactTriplePopulated(t *testing.T) {
	text := "The project uses TypeScript."
	items := memory.ExtractFromText(text)
	if len(items) == 0 {
		t.Fatal("expected at least 1 item")
	}

	var factItem *memory.ExtractedItem
	for i, item := range items {
		if item.Type == "fact" {
			factItem = &items[i]
			break
		}
	}
	if factItem == nil {
		t.Fatal("expected a fact item")
	}
	if factItem.Fact == nil {
		t.Fatal("expected non-nil Fact triple")
	}
	if factItem.Fact.Subject == "" {
		t.Error("expected non-empty Subject")
	}
	if factItem.Fact.Predicate != "uses" {
		t.Errorf("expected predicate 'uses', got %q", factItem.Fact.Predicate)
	}
	if factItem.Fact.Object == "" {
		t.Error("expected non-empty Object")
	}
}

func itemTypes(items []memory.ExtractedItem) []string {
	var types []string
	for _, item := range items {
		types = append(types, item.Type)
	}
	return types
}
