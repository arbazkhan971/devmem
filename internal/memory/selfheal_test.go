package memory

import (
	"strings"
	"testing"
)

func TestWordOverlap(t *testing.T) {
	tests := []struct {
		a, b string
		min  float64
	}{
		{"the quick brown fox", "the quick brown fox", 1.0},
		{"the quick brown fox", "the slow red dog", 0.2},
		{"hello world", "goodbye world", 0.5},
		{"", "something", 0.0},
		{"a", "", 0.0},
	}
	for _, tt := range tests {
		overlap := wordOverlap(tt.a, tt.b)
		if overlap < tt.min {
			t.Errorf("wordOverlap(%q, %q) = %.2f, want >= %.2f", tt.a, tt.b, overlap, tt.min)
		}
	}
}

func TestFilePathPattern(t *testing.T) {
	tests := []struct {
		input   string
		want    []string
	}{
		{"Modified internal/mcp/server.go to add tools", []string{"internal/mcp/server.go"}},
		{"Updated cmd/main.go and internal/storage/db.go", []string{"cmd/main.go", "internal/storage/db.go"}},
		{"No files mentioned here", nil},
		{"Check config.yaml and style.css", []string{"config.yaml", "style.css"}},
		{"The file app.ts has issues", []string{"app.ts"}},
	}
	for _, tt := range tests {
		matches := filePathPattern.FindAllStringSubmatch(tt.input, -1)
		var found []string
		for _, m := range matches {
			if len(m) >= 2 {
				found = append(found, m[1])
			}
		}
		if len(found) != len(tt.want) {
			t.Errorf("filePathPattern on %q: got %v, want %v", tt.input, found, tt.want)
			continue
		}
		for i, f := range found {
			if f != tt.want[i] {
				t.Errorf("filePathPattern on %q[%d]: got %q, want %q", tt.input, i, f, tt.want[i])
			}
		}
	}
}

func TestFormatDeduplicate_NoDuplicates(t *testing.T) {
	d := &DeduplicateResult{DryRun: true}
	text := FormatDeduplicate(d)
	if !strings.Contains(text, "No duplicates") {
		t.Errorf("expected no duplicates message, got:\n%s", text)
	}
}

func TestFormatDeduplicate_WithDuplicates(t *testing.T) {
	d := &DeduplicateResult{
		DryRun:   true,
		TotalDups: 1,
		Groups: []DuplicateGroup{
			{
				NoteIDs:    []string{"aaaa1111-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "bbbb2222-bbbb-bbbb-bbbb-bbbbbbbbbbbb"},
				Previews:   []string{"The auth uses JWT", "The auth uses JWT tokens"},
				Similarity: 0.85,
			},
		},
	}
	text := FormatDeduplicate(d)
	if !strings.Contains(text, "DRY RUN") {
		t.Errorf("expected DRY RUN, got:\n%s", text)
	}
	if !strings.Contains(text, "1 duplicate") {
		t.Errorf("expected duplicate count, got:\n%s", text)
	}
}

func TestFormatIntegrityCheck_Clean(t *testing.T) {
	r := &IntegrityResult{ScorePercent: 100, TotalChecked: 50}
	text := FormatIntegrityCheck(r)
	if !strings.Contains(text, "100%") {
		t.Errorf("expected 100%%, got:\n%s", text)
	}
	if !strings.Contains(text, "All clear") {
		t.Errorf("expected All clear, got:\n%s", text)
	}
}

func TestFormatIntegrityCheck_WithIssues(t *testing.T) {
	r := &IntegrityResult{
		ScorePercent:   92,
		TotalChecked:   100,
		BrokenLinks:    2,
		OrphanSessions: 1,
		FixedCount:     3,
	}
	text := FormatIntegrityCheck(r)
	if !strings.Contains(text, "92%") {
		t.Errorf("expected 92%%, got:\n%s", text)
	}
	if !strings.Contains(text, "2 broken link") {
		t.Errorf("expected broken links, got:\n%s", text)
	}
	if !strings.Contains(text, "Fixed: 3") {
		t.Errorf("expected fixed count, got:\n%s", text)
	}
}

func TestFormatAutoLinkCode_Empty(t *testing.T) {
	r := &AutoLinkCodeResult{}
	text := FormatAutoLinkCode(r)
	if !strings.Contains(text, "No file references") {
		t.Errorf("expected no file references message, got:\n%s", text)
	}
}

func TestFormatAutoLinkCode_WithLinks(t *testing.T) {
	r := &AutoLinkCodeResult{LinkedNotes: 15, LinkedFiles: 8, NewLinks: 20}
	text := FormatAutoLinkCode(r)
	if !strings.Contains(text, "15 notes") {
		t.Errorf("expected note count, got:\n%s", text)
	}
	if !strings.Contains(text, "8 code files") {
		t.Errorf("expected file count, got:\n%s", text)
	}
}
