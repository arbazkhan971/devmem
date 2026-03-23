package memory

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

type MemoryLink struct {
	ID, SourceID, SourceType, TargetID, TargetType, Relationship string
	Strength                                                     float64
	CreatedAt                                                    string
}

const linkInsertSQL = `INSERT OR IGNORE INTO memory_links
	(id, source_id, source_type, target_id, target_type, relationship, strength, created_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

func (s *Store) CreateLink(sourceID, sourceType, targetID, targetType, relationship string, strength float64) error {
	now := time.Now().UTC().Format(time.DateTime)
	w := s.db.Writer()
	for _, d := range [2][4]string{
		{sourceID, sourceType, targetID, targetType},
		{targetID, targetType, sourceID, sourceType},
	} {
		if _, err := w.Exec(linkInsertSQL, uuid.New().String(), d[0], d[1], d[2], d[3], relationship, strength, now); err != nil {
			return fmt.Errorf("create link %s->%s: %w", d[0], d[2], err)
		}
	}
	return nil
}

func (s *Store) GetLinks(memoryID, memoryType string) ([]MemoryLink, error) {
	rows, err := s.db.Reader().Query(
		`SELECT id, source_id, source_type, target_id, target_type, relationship, strength, created_at
		 FROM memory_links WHERE source_id = ? AND source_type = ? ORDER BY strength DESC, created_at DESC`,
		memoryID, memoryType,
	)
	if err != nil {
		return nil, fmt.Errorf("get links: %w", err)
	}
	defer rows.Close()
	var out []MemoryLink
	for rows.Next() {
		var l MemoryLink
		if err := rows.Scan(&l.ID, &l.SourceID, &l.SourceType, &l.TargetID, &l.TargetType, &l.Relationship, &l.Strength, &l.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan link: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

type ftsTarget struct {
	typeName, table string
}

var autoLinkTargets = []ftsTarget{{"note", "notes"}, {"fact", "facts"}, {"commit", "commits"}}

func (s *Store) AutoLink(sourceID, sourceType, content string) (int, error) {
	count := 0
	for _, t := range autoLinkTargets {
		q := fmt.Sprintf(
			`SELECT t.id, '%s' as type, rank FROM %s_fts fts JOIN %s t ON t.rowid = fts.rowid
			 WHERE %s_fts MATCH ? AND rank < -0.5 ORDER BY rank LIMIT 10`,
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

func (s *Store) searchAndLink(sourceID, sourceType, content, query string) (int, error) {
	rows, err := s.db.Reader().Query(query, content)
	if err != nil {
		return 0, nil
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var targetID, targetType string
		var rank float64
		if rows.Scan(&targetID, &targetType, &rank) != nil {
			continue
		}
		if targetID == sourceID && targetType == sourceType {
			continue
		}
		strength := 0.5
		if rank < -2.0 {
			strength = 0.9
		} else if rank < -1.0 {
			strength = 0.7
		}
		if s.CreateLink(sourceID, sourceType, targetID, targetType, "related", strength) != nil {
			continue
		}
		count++
	}
	return count, rows.Err()
}
