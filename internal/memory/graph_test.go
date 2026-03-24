package memory_test

import (
	"strings"
	"testing"

	"github.com/arbazkhan971/memorx/internal/memory"
)

func TestMemoryGraph_EmptyDB(t *testing.T) {
	store := newTestStore(t)

	result, err := store.MemoryGraph("")
	if err != nil {
		t.Fatalf("MemoryGraph: %v", err)
	}
	if len(result.Nodes) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(result.Nodes))
	}
	if len(result.Edges) != 0 {
		t.Errorf("expected 0 edges, got %d", len(result.Edges))
	}
}

func TestMemoryGraph_WithLinks(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("graph-test", "Test feature")
	n1, _ := store.CreateNote(f.ID, "", "Authentication middleware", "decision")
	n2, _ := store.CreateNote(f.ID, "", "Rate limiting for auth", "note")
	fact, _ := store.CreateFact(f.ID, "", "auth", "uses", "JWT")

	store.CreateLink(n1.ID, "note", n2.ID, "note", "related", 0.8)
	store.CreateLink(n1.ID, "note", fact.ID, "fact", "implements", 0.7)

	result, err := store.MemoryGraph("")
	if err != nil {
		t.Fatalf("MemoryGraph: %v", err)
	}

	// CreateLink creates bidirectional links, so we get 4 edges (2 per call)
	if len(result.Nodes) < 3 {
		t.Errorf("expected at least 3 nodes, got %d", len(result.Nodes))
	}
	if len(result.Edges) < 2 {
		t.Errorf("expected at least 2 edges, got %d", len(result.Edges))
	}
}

func TestMemoryGraph_FilterByFeature(t *testing.T) {
	store := newTestStore(t)

	fa, _ := store.CreateFeature("graph-a", "Feature A")
	fb, _ := store.CreateFeature("graph-b", "Feature B")

	na1, _ := store.CreateNote(fa.ID, "", "Auth note A1", "note")
	na2, _ := store.CreateNote(fa.ID, "", "Auth note A2", "note")
	nb1, _ := store.CreateNote(fb.ID, "", "DB note B1", "note")
	nb2, _ := store.CreateNote(fb.ID, "", "DB note B2", "note")

	store.CreateLink(na1.ID, "note", na2.ID, "note", "related", 0.8)
	store.CreateLink(nb1.ID, "note", nb2.ID, "note", "related", 0.7)

	// Filter to feature A only
	result, err := store.MemoryGraph("graph-a")
	if err != nil {
		t.Fatalf("MemoryGraph(graph-a): %v", err)
	}

	// Should only include nodes from feature A
	if len(result.Nodes) < 2 {
		t.Errorf("expected at least 2 nodes for feature A, got %d", len(result.Nodes))
	}

	// Verify cluster contains feature A
	if result.Clusters["graph-a"] == 0 {
		t.Errorf("expected cluster 'graph-a' to have nodes, got clusters: %v", result.Clusters)
	}
}

func TestMemoryGraph_FeatureNotFound(t *testing.T) {
	store := newTestStore(t)

	_, err := store.MemoryGraph("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent feature")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestMemoryGraph_Clusters(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("cluster-test", "Cluster test")
	n1, _ := store.CreateNote(f.ID, "", "Note one", "note")
	n2, _ := store.CreateNote(f.ID, "", "Note two", "note")
	store.CreateLink(n1.ID, "note", n2.ID, "note", "related", 0.9)

	result, err := store.MemoryGraph("")
	if err != nil {
		t.Fatalf("MemoryGraph: %v", err)
	}

	if len(result.Clusters) == 0 {
		t.Error("expected at least one cluster")
	}
	if result.Clusters["cluster-test"] == 0 {
		t.Errorf("expected cluster-test to have nodes, got: %v", result.Clusters)
	}
}

func TestFormatGraph_Nil(t *testing.T) {
	out := memory.FormatGraph(nil, "summary")
	if out != "No graph data." {
		t.Errorf("expected nil message, got %q", out)
	}
}

func TestFormatGraph_Summary(t *testing.T) {
	result := &memory.GraphResult{
		Nodes: []memory.GraphNode{
			{ID: "1", Type: "note"},
			{ID: "2", Type: "note"},
			{ID: "3", Type: "fact"},
		},
		Edges: []memory.GraphEdge{
			{
				Source:       memory.GraphNode{ID: "1", Type: "note"},
				Target:       memory.GraphNode{ID: "2", Type: "note"},
				Relationship: "related",
				Strength:     0.8,
			},
			{
				Source:       memory.GraphNode{ID: "1", Type: "note"},
				Target:       memory.GraphNode{ID: "3", Type: "fact"},
				Relationship: "implements",
				Strength:     0.7,
			},
		},
		Clusters: map[string]int{"auth": 2, "tokens": 1},
	}

	out := memory.FormatGraph(result, "summary")
	if !strings.Contains(out, "3 nodes") {
		t.Errorf("expected '3 nodes' in output, got %q", out)
	}
	if !strings.Contains(out, "2 edges") {
		t.Errorf("expected '2 edges' in output, got %q", out)
	}
	if !strings.Contains(out, "auth(2)") {
		t.Errorf("expected 'auth(2)' in output, got %q", out)
	}
	if !strings.Contains(out, "tokens(1)") {
		t.Errorf("expected 'tokens(1)' in output, got %q", out)
	}
}

func TestFormatGraph_Detailed(t *testing.T) {
	result := &memory.GraphResult{
		Nodes: []memory.GraphNode{
			{ID: "1", Type: "note"},
			{ID: "2", Type: "fact"},
		},
		Edges: []memory.GraphEdge{
			{
				Source:       memory.GraphNode{ID: "1", Type: "note"},
				Target:       memory.GraphNode{ID: "2", Type: "fact"},
				Relationship: "related",
				Strength:     0.8,
			},
		},
		Clusters: map[string]int{"auth": 2},
	}

	out := memory.FormatGraph(result, "detailed")
	if !strings.Contains(out, "Memory Graph: 2 nodes, 1 edges") {
		t.Errorf("expected header in detailed output, got %q", out)
	}
	if !strings.Contains(out, "Clusters") {
		t.Errorf("expected clusters section in detailed output")
	}
	if !strings.Contains(out, "Node types") {
		t.Errorf("expected node types section in detailed output")
	}
	if !strings.Contains(out, "Edges") {
		t.Errorf("expected edges section in detailed output")
	}
}

func TestFormatGraph_EmptyGraph(t *testing.T) {
	result := &memory.GraphResult{
		Clusters: map[string]int{},
	}
	out := memory.FormatGraph(result, "summary")
	if !strings.Contains(out, "0 nodes") {
		t.Errorf("expected '0 nodes' in output, got %q", out)
	}
}
