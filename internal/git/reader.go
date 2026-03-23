package git

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Commit represents a single git commit with its metadata and changed files.
type Commit struct {
	Hash         string
	Message      string
	Author       string
	CommittedAt  string
	FilesChanged []FileChange
}

// FileChange represents a single file change in a commit.
type FileChange struct {
	Path   string
	Action string // "added", "modified", "deleted"
}

// ReadCommits reads git commits since the given time from the repository at gitRoot.
// It returns commits in reverse chronological order (newest first).
func ReadCommits(gitRoot string, since time.Time) ([]Commit, error) {
	sinceStr := since.Format(time.RFC3339)

	cmd := exec.Command("git", "log",
		"--since="+sinceStr,
		"--format=%H||%s||%an||%aI",
		"--no-merges",
	)
	cmd.Dir = gitRoot
	out, err := cmd.Output()
	if err != nil {
		// If there are no commits yet, git log returns an error
		if exitErr, ok := err.(*exec.ExitError); ok {
			// Empty repo or no matching commits — return empty
			_ = exitErr
			return nil, nil
		}
		return nil, fmt.Errorf("git log: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil, nil
	}

	var commits []Commit
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, "||", 4)
		if len(parts) != 4 {
			continue // skip malformed lines
		}

		c := Commit{
			Hash:        parts[0],
			Message:     parts[1],
			Author:      parts[2],
			CommittedAt: parts[3],
		}

		files, err := readFilesChanged(gitRoot, c.Hash)
		if err != nil {
			return nil, fmt.Errorf("read files for %s: %w", c.Hash, err)
		}
		c.FilesChanged = files

		commits = append(commits, c)
	}

	return commits, nil
}

// readFilesChanged returns the list of files changed in a given commit.
// The --root flag ensures we also get files for the initial commit (which has no parent).
func readFilesChanged(gitRoot, hash string) ([]FileChange, error) {
	cmd := exec.Command("git", "diff-tree", "--root", "--no-commit-id", "--name-status", "-r", hash)
	cmd.Dir = gitRoot
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff-tree: %w", err)
	}

	var files []FileChange
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		action := parseAction(parts[0])
		path := parts[1]
		// For renames (R100), the new path is the third field
		if strings.HasPrefix(parts[0], "R") && len(parts) >= 3 {
			path = parts[2]
		}

		files = append(files, FileChange{
			Path:   path,
			Action: action,
		})
	}

	return files, nil
}

// parseAction converts git status letters to human-readable action strings.
func parseAction(status string) string {
	switch {
	case status == "A":
		return "added"
	case status == "M":
		return "modified"
	case status == "D":
		return "deleted"
	case strings.HasPrefix(status, "R"):
		return "modified" // treat renames as modified
	case strings.HasPrefix(status, "C"):
		return "added" // treat copies as added
	default:
		return "modified"
	}
}

// GetCurrentBranch returns the current git branch name.
// This is an alias for CurrentBranch for API consistency.
func GetCurrentBranch(gitRoot string) (string, error) {
	return CurrentBranch(gitRoot)
}
