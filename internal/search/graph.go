package search

import "fmt"

// LinkedMemory represents a memory item discovered via link traversal.
type LinkedMemory struct {
	ID           string
	Type         string
	Relationship string
	Strength     float64
	Depth        int
}

// TraverseLinks follows memory_links from a starting memory, using a recursive
// CTE to discover connected items up to maxDepth hops away.
func (e *Engine) TraverseLinks(memoryID, memoryType string, maxDepth int) ([]LinkedMemory, error) {
	if maxDepth < 1 {
		maxDepth = 1
	}

	const query = `
WITH RECURSIVE connected AS (
    SELECT target_id, target_type, relationship, strength, 1 as depth
    FROM memory_links
    WHERE source_id = ? AND source_type = ?
    UNION ALL
    SELECT ml.target_id, ml.target_type, ml.relationship, ml.strength, c.depth + 1
    FROM memory_links ml
    JOIN connected c ON ml.source_id = c.target_id AND ml.source_type = c.target_type
    WHERE c.depth < ?
)
SELECT DISTINCT target_id, target_type, relationship, strength, depth
FROM connected
ORDER BY depth, strength DESC
`

	rows, err := e.db.Reader().Query(query, memoryID, memoryType, maxDepth)
	if err != nil {
		return nil, fmt.Errorf("traverse links: %w", err)
	}
	defer rows.Close()

	var results []LinkedMemory
	for rows.Next() {
		var lm LinkedMemory
		if err := rows.Scan(&lm.ID, &lm.Type, &lm.Relationship, &lm.Strength, &lm.Depth); err != nil {
			return nil, fmt.Errorf("scan linked memory: %w", err)
		}
		results = append(results, lm)
	}
	return results, rows.Err()
}
