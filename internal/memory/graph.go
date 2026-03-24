package memory

import (
	"database/sql"
	"fmt"
	"strings"
)

// GraphNode represents a node in the memory graph.
type GraphNode struct {
	ID   string
	Type string // note, fact, commit, plan, etc.
}

// GraphEdge represents an edge in the memory graph.
type GraphEdge struct {
	Source       GraphNode
	Target       GraphNode
	Relationship string
	Strength     float64
}

// GraphResult holds the complete graph analysis.
type GraphResult struct {
	Nodes    []GraphNode
	Edges    []GraphEdge
	Clusters map[string]int // cluster name -> node count
}

// MemoryGraph builds an adjacency graph of memory links for a feature.
// If feature is empty, it queries all links.
func (s *Store) MemoryGraph(feature string) (*GraphResult, error) {
	r := s.db.Reader()
	result := &GraphResult{
		Clusters: make(map[string]int),
	}

	nodeSet := make(map[string]GraphNode)  // "type:id" -> node
	edgeSet := make(map[string]bool)       // dedup key

	var rows *sql.Rows
	var err error

	if feature != "" {
		// Resolve feature name to ID
		var featureID string
		err = r.QueryRow(`SELECT id FROM features WHERE name = ?`, feature).Scan(&featureID)
		if err != nil {
			return nil, fmt.Errorf("feature %q not found", feature)
		}

		// Get all memory IDs belonging to this feature
		memoryIDs := make(map[string]bool)

		// Notes
		noteRows, err := r.Query(`SELECT id FROM notes WHERE feature_id = ?`, featureID)
		if err == nil {
			defer noteRows.Close()
			for noteRows.Next() {
				var id string
				if noteRows.Scan(&id) == nil {
					memoryIDs[id] = true
				}
			}
		}

		// Facts
		factRows, err := r.Query(`SELECT id FROM facts WHERE feature_id = ?`, featureID)
		if err == nil {
			defer factRows.Close()
			for factRows.Next() {
				var id string
				if factRows.Scan(&id) == nil {
					memoryIDs[id] = true
				}
			}
		}

		// Commits
		commitRows, err := r.Query(`SELECT id FROM commits WHERE feature_id = ?`, featureID)
		if err == nil {
			defer commitRows.Close()
			for commitRows.Next() {
				var id string
				if commitRows.Scan(&id) == nil {
					memoryIDs[id] = true
				}
			}
		}

		// Query links where source or target belongs to this feature
		rows, err = r.Query(
			`SELECT source_id, source_type, target_id, target_type, relationship, strength
			 FROM memory_links ORDER BY strength DESC`,
		)
		if err != nil {
			return nil, fmt.Errorf("query links: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var e GraphEdge
			if rows.Scan(&e.Source.ID, &e.Source.Type, &e.Target.ID, &e.Target.Type, &e.Relationship, &e.Strength) != nil {
				continue
			}
			// Filter: at least one side must belong to this feature
			if !memoryIDs[e.Source.ID] && !memoryIDs[e.Target.ID] {
				continue
			}
			addEdge(nodeSet, edgeSet, result, e)
		}
	} else {
		// No feature filter: return all links
		rows, err = r.Query(
			`SELECT source_id, source_type, target_id, target_type, relationship, strength
			 FROM memory_links ORDER BY strength DESC`,
		)
		if err != nil {
			return nil, fmt.Errorf("query links: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var e GraphEdge
			if rows.Scan(&e.Source.ID, &e.Source.Type, &e.Target.ID, &e.Target.Type, &e.Relationship, &e.Strength) != nil {
				continue
			}
			addEdge(nodeSet, edgeSet, result, e)
		}
	}

	// Build clusters: group nodes by their feature association
	s.buildClusters(r, nodeSet, result)

	// Convert nodeSet to slice
	for _, n := range nodeSet {
		result.Nodes = append(result.Nodes, n)
	}

	return result, nil
}

func addEdge(nodeSet map[string]GraphNode, edgeSet map[string]bool, result *GraphResult, e GraphEdge) {
	edgeKey := e.Source.Type + ":" + e.Source.ID + "->" + e.Target.Type + ":" + e.Target.ID
	if edgeSet[edgeKey] {
		return
	}
	edgeSet[edgeKey] = true
	result.Edges = append(result.Edges, e)

	srcKey := e.Source.Type + ":" + e.Source.ID
	tgtKey := e.Target.Type + ":" + e.Target.ID
	nodeSet[srcKey] = e.Source
	nodeSet[tgtKey] = e.Target
}

// buildClusters groups nodes by the feature they belong to.
func (s *Store) buildClusters(r *sql.DB, nodeSet map[string]GraphNode, result *GraphResult) {
	for _, node := range nodeSet {
		var featureName string
		switch node.Type {
		case "note":
			r.QueryRow(`SELECT f.name FROM features f JOIN notes n ON n.feature_id = f.id WHERE n.id = ?`, node.ID).Scan(&featureName)
		case "fact":
			r.QueryRow(`SELECT f.name FROM features f JOIN facts fa ON fa.feature_id = f.id WHERE fa.id = ?`, node.ID).Scan(&featureName)
		case "commit":
			r.QueryRow(`SELECT f.name FROM features f JOIN commits c ON c.feature_id = f.id WHERE c.id = ?`, node.ID).Scan(&featureName)
		}
		if featureName == "" {
			featureName = "unlinked"
		}
		result.Clusters[featureName]++
	}
}

// FormatGraph formats a GraphResult into a human-readable string.
func FormatGraph(r *GraphResult, format string) string {
	if r == nil {
		return "No graph data."
	}

	nodeCount := len(r.Nodes)
	edgeCount := len(r.Edges)

	if format == "detailed" {
		return formatGraphDetailed(r, nodeCount, edgeCount)
	}
	return formatGraphSummary(r, nodeCount, edgeCount)
}

func formatGraphSummary(r *GraphResult, nodeCount, edgeCount int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d nodes, %d edges.", nodeCount, edgeCount)

	if len(r.Clusters) > 0 {
		b.WriteString(" Clusters: ")
		first := true
		for name, count := range r.Clusters {
			if !first {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "%s(%d)", name, count)
			first = false
		}
	}

	return b.String()
}

func formatGraphDetailed(r *GraphResult, nodeCount, edgeCount int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "**Memory Graph: %d nodes, %d edges**\n\n", nodeCount, edgeCount)

	// Clusters
	if len(r.Clusters) > 0 {
		b.WriteString("**Clusters:**\n")
		for name, count := range r.Clusters {
			fmt.Fprintf(&b, "- %s: %d nodes\n", name, count)
		}
		b.WriteByte('\n')
	}

	// Node types breakdown
	typeCount := make(map[string]int)
	for _, n := range r.Nodes {
		typeCount[n.Type]++
	}
	if len(typeCount) > 0 {
		b.WriteString("**Node types:**\n")
		for t, c := range typeCount {
			fmt.Fprintf(&b, "- %s: %d\n", t, c)
		}
		b.WriteByte('\n')
	}

	// Edges
	if len(r.Edges) > 0 {
		b.WriteString("**Edges:**\n")
		limit := len(r.Edges)
		if limit > 20 {
			limit = 20
		}
		for _, e := range r.Edges[:limit] {
			fmt.Fprintf(&b, "- %s:%s -[%s (%.1f)]-> %s:%s\n",
				e.Source.Type, shortID(e.Source.ID),
				e.Relationship, e.Strength,
				e.Target.Type, shortID(e.Target.ID))
		}
		if len(r.Edges) > 20 {
			fmt.Fprintf(&b, "... and %d more edges\n", len(r.Edges)-20)
		}
	}

	return b.String()
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
