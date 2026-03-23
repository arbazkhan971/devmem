package git

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type Commit struct {
	Hash         string
	Message      string
	Author       string
	CommittedAt  string
	FilesChanged []FileChange
}

type FileChange struct {
	Path   string
	Action string // "added", "modified", "deleted"
}

// ReadCommits returns commits since the given time in reverse chronological order.
func ReadCommits(gitRoot string, since time.Time) ([]Commit, error) {
	cmd := exec.Command("git", "log",
		"--since="+since.Format(time.RFC3339),
		"--format=%H||%s||%an||%aI",
		"--no-merges",
	)
	cmd.Dir = gitRoot
	out, err := cmd.Output()
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return nil, nil
		}
		return nil, fmt.Errorf("git log: %w", err)
	}

	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return nil, nil
	}

	var commits []Commit
	for _, line := range strings.Split(trimmed, "\n") {
		c, ok := parseCommitLine(line)
		if !ok {
			continue
		}
		if c.FilesChanged, err = readFilesChanged(gitRoot, c.Hash); err != nil {
			return nil, fmt.Errorf("read files for %s: %w", c.Hash, err)
		}
		commits = append(commits, c)
	}
	return commits, nil
}

func parseCommitLine(line string) (Commit, bool) {
	line = strings.TrimSpace(line)
	hash, rest, ok := strings.Cut(line, "||")
	if !ok {
		return Commit{}, false
	}
	msg, rest, ok := strings.Cut(rest, "||")
	if !ok {
		return Commit{}, false
	}
	author, date, ok := strings.Cut(rest, "||")
	if !ok {
		return Commit{}, false
	}
	return Commit{Hash: hash, Message: msg, Author: author, CommittedAt: date}, true
}

// readFilesChanged returns the files changed in a commit.
func readFilesChanged(gitRoot, hash string) ([]FileChange, error) {
	cmd := exec.Command("git", "diff-tree", "--root", "--no-commit-id", "--name-status", "-r", hash)
	cmd.Dir = gitRoot
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff-tree: %w", err)
	}

	var files []FileChange
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if parts := strings.Fields(line); len(parts) >= 2 {
			path := parts[1]
			if strings.HasPrefix(parts[0], "R") && len(parts) >= 3 {
				path = parts[2]
			}
			files = append(files, FileChange{Path: path, Action: parseAction(parts[0])})
		}
	}
	return files, nil
}

func parseAction(status string) string {
	switch {
	case status == "A":
		return "added"
	case status == "D":
		return "deleted"
	case strings.HasPrefix(status, "C"):
		return "added"
	default:
		return "modified"
	}
}
