package memory

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// MemoryLink represents a bidirectional link between memory items.
type MemoryLink struct {
	ID           string
	SourceID     string
	SourceType   string
	TargetID     string
	TargetType   string
	Relationship string
	Strength     float64
	CreatedAt    string
}

const linkInsertSQL = `INSERT OR IGNORE INTO memory_links
	(id, source_id, source_type, target_id, target_type, relationship, strength, created_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

// CreateLink creates a bidirectional link between two memory items.
func (s *Store) CreateLink(sourceID, sourceType, targetID, targetType, relationship string, strength float64) error {
	now := time.Now().UTC().Format(time.DateTime)
	w := s.db.Writer()
	dirs := [2][4]string{
		{sourceID, sourceType, targetID, targetType},
		{targetID, targetType, sourceID, sourceType},
	}
	for _, d := range dirs {
		if _, err := w.Exec(linkInsertSQL,
			uuid.New().String(), d[0], d[1], d[2], d[3], relationship, strength, now,
		); err != nil {
			return fmt.Errorf("create link %s->%s: %w", d[0], d[2], err)
		}
	}
	return nil
}

// GetLinks returns all links where the given memory item is the source.
func (s *Store) GetLinks(memoryID, memoryType string) ([]MemoryLink, error) {
	rows, err := s.db.Reader().Query(
		`SELECT id, source_id, source_type, target_id, target_type, relationship, strength, created_at
		 FROM memory_links WHERE source_id = ? AND source_type = ?
		 ORDER BY strength DESC, created_at DESC`,
		memoryID, memoryType,
	)
	if err != nil {
		return nil, fmt.Errorf("get links: %w", err)
	}
	defer rows.Close()

	var links []MemoryLink
	for rows.Next() {
		var l MemoryLink
		if err := rows.Scan(&l.ID, &l.SourceID, &l.SourceType, &l.TargetID, &l.TargetType,
			&l.Relationship, &l.Strength, &l.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan link: %w", err)
		}
		links = append(links, l)
	}
	return links, rows.Err()
}

// ftsTarget defines a single FTS index to search during auto-linking.
type ftsTarget struct {
	typeName string // singular: "note", "fact", "commit"
	table    string // plural table name: "notes", "facts", "commits"
}

var autoLinkTargets = []ftsTarget{
	{"note", "notes"},
	{"fact", "facts"},
	{"commit", "commits"},
}

// AutoLink searches FTS indexes for content and creates "related" links
// for results above a BM25 rank threshold. Returns the count of links created.
func (s *Store) AutoLink(sourceID, sourceType, content string) (int, error) {
	count := 0
	for _, t := range autoLinkTargets {
		q := fmt.Sprintf(
			`SELECT t.id, '%s' as type, rank
			 FROM %s_fts fts JOIN %s t ON t.rowid = fts.rowid
			 WHERE %s_fts MATCH ? AND rank < -0.5
			 ORDER BY rank LIMIT 10`,
			t.typeName, t.table, t.table, t.table,
		)
		n, err := s.searchAndLink(sourceID, sourceType, content, q)
		if err != nil {
			return count, fmt.Errorf("autolink %s: %w", t.table, err)
		}
		count += n
	}
	return count, nil
}

// searchAndLink performs an FTS search and creates links for matching results.
func (s *Store) searchAndLink(sourceID, sourceType, content, query string) (int, error) {
	rows, err := s.db.Reader().Query(query, content)
	if err != nil {
		return 0, nil // FTS match errors are common with special characters
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var targetID, targetType string
		var rank float64
		if err := rows.Scan(&targetID, &targetType, &rank); err != nil {
			continue
		}
		if targetID == sourceID && targetType == sourceType {
			continue
		}
		strength := rankToStrength(rank)
		if err := s.CreateLink(sourceID, sourceType, targetID, targetType, "related", strength); err != nil {
			continue
		}
		count++
	}
	return count, rows.Err()
}

// rankToStrength converts a BM25 rank (negative, more negative = better) to a link strength.
func rankToStrength(rank float64) float64 {
	switch {
	case rank < -2.0:
		return 0.9
	case rank < -1.0:
		return 0.7
	default:
		return 0.5
	}
}
