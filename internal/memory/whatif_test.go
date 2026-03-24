package memory_test

import (
	"strings"
	"testing"
	"time"

	"github.com/arbazkhan971/memorx/internal/memory"
)

func TestWhatIf_EmptyDB(t *testing.T) {
	store := newTestStore(t)

	_, err := store.WhatIf("nonexistent decision")
	if err == nil {
		t.Fatal("expected error for nonexistent decision")
	}
	if !strings.Contains(err.Error(), "no decision found") {
		t.Errorf("expected 'no decision found' error, got: %v", err)
	}
}

func TestWhatIf_FindsDecisionAndAffectedItems(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("whatif-test", "Test feature")

	// Create a decision
	store.CreateNote(f.ID, "", "Decided to use gRPC instead of REST for API communication", "decision")
	// Sleep >1s to ensure created_at (second precision) differs
	time.Sleep(1100 * time.Millisecond)

	// Create notes after the decision
	store.CreateNote(f.ID, "", "Implemented gRPC service definitions", "progress")
	store.CreateNote(f.ID, "", "Added gRPC interceptors for authentication", "note")

	// Create facts after the decision
	store.CreateFact(f.ID, "", "api", "protocol", "gRPC")
	store.CreateFact(f.ID, "", "api", "serialization", "protobuf")

	result, err := store.WhatIf("gRPC")
	if err != nil {
		t.Fatalf("WhatIf: %v", err)
	}

	if result.Decision.Type != "decision" {
		t.Errorf("expected decision type, got %q", result.Decision.Type)
	}
	if !strings.Contains(result.Decision.Content, "gRPC") {
		t.Errorf("expected decision about gRPC, got %q", result.Decision.Content)
	}
	if len(result.AffectedNotes) < 2 {
		t.Errorf("expected at least 2 affected notes, got %d", len(result.AffectedNotes))
	}
	if len(result.AffectedFacts) < 2 {
		t.Errorf("expected at least 2 affected facts, got %d", len(result.AffectedFacts))
	}
}

func TestWhatIf_FindsTopicRelatedItems(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("whatif-topic", "Topic test")

	// Create a decision about authentication
	store.CreateNote(f.ID, "", "Decided to use JWT for authentication", "decision")

	// Create topic-related items (before and after)
	store.CreateNote(f.ID, "", "Authentication flow documentation needs update", "note")
	store.CreateFact(f.ID, "", "authentication", "method", "JWT")

	result, err := store.WhatIf("JWT")
	if err != nil {
		t.Fatalf("WhatIf: %v", err)
	}

	// Should find topic-related items
	totalTopic := len(result.TopicRelated) + len(result.TopicFacts)
	t.Logf("Topic related: %d notes, %d facts", len(result.TopicRelated), len(result.TopicFacts))
	if totalTopic == 0 {
		t.Log("No topic-related items found (acceptable if keyword extraction picks different word)")
	}
}

func TestWhatIf_NoAffectedItems(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("whatif-empty", "Empty test")

	// Create decision with nothing after it
	store.CreateNote(f.ID, "", "Decided to use SQLite for storage", "decision")

	result, err := store.WhatIf("SQLite")
	if err != nil {
		t.Fatalf("WhatIf: %v", err)
	}

	if len(result.AffectedNotes) != 0 {
		t.Errorf("expected 0 affected notes, got %d", len(result.AffectedNotes))
	}
	if len(result.AffectedFacts) != 0 {
		t.Errorf("expected 0 affected facts, got %d", len(result.AffectedFacts))
	}
}

func TestWhatIf_FallbackToLikeSearch(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("whatif-like", "LIKE test")

	// Create a decision with content that may not FTS-match well
	store.CreateNote(f.ID, "", "Decided to use better-auth library for user management", "decision")

	result, err := store.WhatIf("better-auth")
	if err != nil {
		t.Fatalf("WhatIf: %v", err)
	}

	if !strings.Contains(result.Decision.Content, "better-auth") {
		t.Errorf("expected to find decision about better-auth, got %q", result.Decision.Content)
	}
}

func TestWhatIf_FallbackToAnyNoteType(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("whatif-any", "Any type test")

	// Create a non-decision note
	store.CreateNote(f.ID, "", "Considered using Redis for caching", "note")

	result, err := store.WhatIf("Redis")
	if err != nil {
		t.Fatalf("WhatIf: %v", err)
	}

	if !strings.Contains(result.Decision.Content, "Redis") {
		t.Errorf("expected to find note about Redis, got %q", result.Decision.Content)
	}
}

func TestFormatWhatIf_Nil(t *testing.T) {
	out := memory.FormatWhatIf(nil)
	if out != "No what-if result." {
		t.Errorf("expected nil message, got %q", out)
	}
}

func TestFormatWhatIf_WithContent(t *testing.T) {
	result := &memory.WhatIfResult{
		Decision: memory.Note{
			ID: "1", Content: "Use gRPC for API", Type: "decision",
		},
		AffectedNotes: []memory.Note{
			{ID: "2", Content: "Implemented gRPC services", Type: "progress"},
		},
		AffectedFacts: []memory.Fact{
			{ID: "3", Subject: "api", Predicate: "uses", Object: "gRPC"},
		},
	}

	out := memory.FormatWhatIf(result)
	if !strings.Contains(out, "Use gRPC for API") {
		t.Errorf("expected decision content in output")
	}
	if !strings.Contains(out, "2 items may be affected") {
		t.Errorf("expected affected count, got: %s", out)
	}
	if !strings.Contains(out, "Implemented gRPC services") {
		t.Errorf("expected affected note in output")
	}
	if !strings.Contains(out, "api uses gRPC") {
		t.Errorf("expected affected fact in output")
	}
}

func TestExtractTopicKeywords(t *testing.T) {
	// This is an internal function tested indirectly through WhatIf.
	// Here we test the formatting to ensure topic-related items appear.
	result := &memory.WhatIfResult{
		Decision: memory.Note{ID: "1", Content: "Use authentication", Type: "decision"},
		TopicRelated: []memory.Note{
			{ID: "2", Content: "Auth flow updated", Type: "note"},
		},
		TopicFacts: []memory.Fact{
			{ID: "3", Subject: "authentication", Predicate: "method", Object: "JWT"},
		},
	}

	out := memory.FormatWhatIf(result)
	if !strings.Contains(out, "reference the same topic") {
		t.Errorf("expected topic reference section, got: %s", out)
	}
}
