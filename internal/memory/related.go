package memory

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/arbaz/devmem/internal/search"
)

// RelatedResult holds all related memories for a topic.
type RelatedResult struct {
	Decisions []RelatedItem
	Facts     []RelatedItem
	Files     []RelatedItem
	Commits   []RelatedItem
}

// RelatedItem represents a single related memory.
type RelatedItem struct {
	ID, Type, Content string
	Relevance         float64
}

// FileDependency represents a file that often changes alongside another file.
type FileDependency struct {
	Path        string
	Occurrences int
}

// FindRelated finds all related memories for a topic by combining search,
// link traversal, and file tracking.
func (s *Store) FindRelated(engine *search.Engine, topic string, depth int) (*RelatedResult, error) {
	if depth <= 0 {
		depth = 2
	}
	result := &RelatedResult{}
	seen := make(map[string]bool)

	// 1. Search notes/facts/commits for the topic
	searchResults, err := engine.Search(topic, "all_features", []string{"notes", "commits", "facts"}, "", 20)
	if err != nil {
		return nil, fmt.Errorf("search related: %w", err)
	}

	for _, sr := range searchResults {
		key := sr.Type + ":" + sr.ID
		if seen[key] {
			continue
		}
		seen[key] = true

		item := RelatedItem{ID: sr.ID, Type: sr.Type, Content: sr.Content, Relevance: sr.Relevance}
		switch sr.Type {
		case "note":
			// Check if it's a decision type
			var noteType string
			s.db.Reader().QueryRow(`SELECT type FROM notes WHERE id = ?`, sr.ID).Scan(&noteType)
			if noteType == "decision" {
				result.Decisions = append(result.Decisions, item)
			}
		case "fact":
			result.Facts = append(result.Facts, item)
		case "commit":
			result.Commits = append(result.Commits, item)
		}

		// 2. Traverse links from each result up to depth
		linked, err := engine.TraverseLinks(sr.ID, sr.Type, depth)
		if err != nil {
			continue
		}
		for _, lm := range linked {
			lkey := lm.Type + ":" + lm.ID
			if seen[lkey] {
				continue
			}
			seen[lkey] = true

			content := s.loadLinkedContent(lm.ID, lm.Type)
			if content == "" {
				continue
			}
			linkedItem := RelatedItem{ID: lm.ID, Type: lm.Type, Content: content, Relevance: lm.Strength}
			switch lm.Type {
			case "note":
				var noteType string
				s.db.Reader().QueryRow(`SELECT type FROM notes WHERE id = ?`, lm.ID).Scan(&noteType)
				if noteType == "decision" {
					result.Decisions = append(result.Decisions, linkedItem)
				}
			case "fact":
				result.Facts = append(result.Facts, linkedItem)
			case "commit":
				result.Commits = append(result.Commits, linkedItem)
			}
		}
	}

	// 3. Check files_touched for related files
	files, err := s.findRelatedFiles(topic)
	if err == nil {
		for _, f := range files {
			result.Files = append(result.Files, RelatedItem{Type: "file", Content: f})
		}
	}

	return result, nil
}

// loadLinkedContent fetches the display content for a linked memory item.
func (s *Store) loadLinkedContent(id, memType string) string {
	r := s.db.Reader()
	var content string
	switch memType {
	case "note":
		r.QueryRow(`SELECT content FROM notes WHERE id = ?`, id).Scan(&content)
	case "fact":
		r.QueryRow(`SELECT subject || ' ' || predicate || ' ' || object FROM facts WHERE id = ?`, id).Scan(&content)
	case "commit":
		r.QueryRow(`SELECT message FROM commits WHERE id = ?`, id).Scan(&content)
	}
	return content
}

// findRelatedFiles searches files_touched for paths matching the topic.
func (s *Store) findRelatedFiles(topic string) ([]string, error) {
	pattern := "%" + topic + "%"
	rows, err := s.db.Reader().Query(
		`SELECT DISTINCT path FROM files_touched WHERE path LIKE ? ORDER BY first_seen DESC LIMIT 20`,
		pattern,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var paths []string
	for rows.Next() {
		var p string
		if rows.Scan(&p) == nil {
			paths = append(paths, p)
		}
	}
	return paths, rows.Err()
}

// FormatRelatedResult formats a RelatedResult into a human-readable string.
func FormatRelatedResult(r *RelatedResult) string {
	var b strings.Builder
	section := func(title string, items []RelatedItem) {
		if len(items) == 0 {
			return
		}
		fmt.Fprintf(&b, "**%s:**\n", title)
		for _, item := range items {
			content := item.Content
			if len(content) > 120 {
				content = content[:120] + "..."
			}
			content = strings.ReplaceAll(content, "\n", " ")
			fmt.Fprintf(&b, "- %s (%.2f)\n", content, item.Relevance)
		}
		b.WriteString("\n")
	}
	section("Related decisions", r.Decisions)
	section("Related facts", r.Facts)
	section("Related files", r.Files)
	section("Related commits", r.Commits)
	if b.Len() == 0 {
		return "No related memories found."
	}
	return b.String()
}

// FindDependencies finds files that often change together with the given file
// based on commit co-occurrence in the files_changed JSON column.
func (s *Store) FindDependencies(filePath string) ([]FileDependency, error) {
	// 1. Find all commits that modified this file
	rows, err := s.db.Reader().Query(
		`SELECT files_changed FROM commits WHERE files_changed LIKE ?`,
		"%"+filePath+"%",
	)
	if err != nil {
		return nil, fmt.Errorf("query commits for file: %w", err)
	}
	defer rows.Close()

	coOccurrence := make(map[string]int)
	for rows.Next() {
		var filesJSON string
		if err := rows.Scan(&filesJSON); err != nil {
			continue
		}
		// 2. Parse files_changed JSON
		var files []struct {
			Path   string `json:"Path"`
			Action string `json:"Action"`
		}
		if json.Unmarshal([]byte(filesJSON), &files) != nil {
			continue
		}

		// Check if this commit actually contains our file
		found := false
		for _, f := range files {
			if f.Path == filePath {
				found = true
				break
			}
		}
		if !found {
			continue
		}

		// 3. Count co-occurrences
		for _, f := range files {
			if f.Path != filePath {
				coOccurrence[f.Path]++
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate commits: %w", err)
	}

	// Convert to sorted slice (highest co-occurrence first)
	deps := make([]FileDependency, 0, len(coOccurrence))
	for path, count := range coOccurrence {
		deps = append(deps, FileDependency{Path: path, Occurrences: count})
	}
	// Insertion sort by Occurrences DESC
	for i := 1; i < len(deps); i++ {
		for j := i; j > 0 && deps[j].Occurrences > deps[j-1].Occurrences; j-- {
			deps[j], deps[j-1] = deps[j-1], deps[j]
		}
	}

	// Limit to top 20
	if len(deps) > 20 {
		deps = deps[:20]
	}
	return deps, nil
}

// FormatDependencies formats a list of file dependencies into a human-readable string.
func FormatDependencies(deps []FileDependency) string {
	if len(deps) == 0 {
		return "No file dependencies found (no commits reference this file)."
	}
	var b strings.Builder
	b.WriteString("**File dependencies (by commit co-occurrence):**\n")
	for _, d := range deps {
		times := "time"
		if d.Occurrences != 1 {
			times = "times"
		}
		fmt.Fprintf(&b, "- %s (changed together %d %s)\n", d.Path, d.Occurrences, times)
	}
	return b.String()
}
