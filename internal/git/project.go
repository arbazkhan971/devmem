package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func FindGitRoot(dir string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("not a git repository: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func EnsureMemoryDir(gitRoot string) (string, error) {
	memDir := filepath.Join(gitRoot, ".memory")
	if err := os.MkdirAll(memDir, 0755); err != nil {
		return "", fmt.Errorf("create .memory dir: %w", err)
	}
	if err := ensureGitignore(gitRoot); err != nil {
		return "", fmt.Errorf("update .gitignore: %w", err)
	}
	return memDir, nil
}

func CurrentBranch(gitRoot string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = gitRoot
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("get current branch: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func ProjectName(gitRoot string) string {
	return filepath.Base(gitRoot)
}

func ensureGitignore(gitRoot string) error {
	path := filepath.Join(gitRoot, ".gitignore")
	content, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if strings.Contains(string(content), ".memory/") {
		return nil
	}
	entry := ".memory/\n"
	if len(content) > 0 && !strings.HasSuffix(string(content), "\n") {
		entry = "\n" + entry
	}
	return os.WriteFile(path, append(content, []byte(entry)...), 0644)
}
