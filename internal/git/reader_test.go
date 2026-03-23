package git_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/arbaz/devmem/internal/git"
)

// initTestRepoWithCommits creates a temp git repo with several known commits.
func initTestRepoWithCommits(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	dir, _ = filepath.EvalSymlinks(dir)

	env := append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_AUTHOR_NAME=Test User",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test User",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("command %v failed: %v\n%s", args, err, out)
		}
	}

	run("git", "init")
	run("git", "config", "user.name", "Test User")
	run("git", "config", "user.email", "test@example.com")

	// Commit 1: add a file
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0644)
	run("git", "add", "main.go")
	run("git", "commit", "-m", "feat: add main.go")

	// Commit 2: add a test file
	os.WriteFile(filepath.Join(dir, "main_test.go"), []byte("package main\n"), 0644)
	run("git", "add", "main_test.go")
	run("git", "commit", "-m", "test: add initial tests")

	// Commit 3: modify main.go and add utils.go
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0644)
	os.WriteFile(filepath.Join(dir, "utils.go"), []byte("package main\n\nfunc helper() {}\n"), 0644)
	run("git", "add", "main.go", "utils.go")
	run("git", "commit", "-m", "implement helper function")

	// Commit 4: delete a file
	os.Remove(filepath.Join(dir, "utils.go"))
	run("git", "add", "utils.go")
	run("git", "commit", "-m", "fix: remove unused utils")

	return dir
}

func TestReadCommits_ReturnsAllCommits(t *testing.T) {
	dir := initTestRepoWithCommits(t)

	// Read all commits since a time well before now
	since := time.Now().Add(-1 * time.Hour)
	commits, err := git.ReadCommits(dir, since)
	if err != nil {
		t.Fatalf("ReadCommits: %v", err)
	}

	if len(commits) != 4 {
		t.Fatalf("expected 4 commits, got %d", len(commits))
	}
}

func TestReadCommits_ReturnsNewestFirst(t *testing.T) {
	dir := initTestRepoWithCommits(t)

	since := time.Now().Add(-1 * time.Hour)
	commits, err := git.ReadCommits(dir, since)
	if err != nil {
		t.Fatalf("ReadCommits: %v", err)
	}

	// Newest commit should be first
	if commits[0].Message != "fix: remove unused utils" {
		t.Fatalf("expected newest commit first, got: %s", commits[0].Message)
	}
	if commits[len(commits)-1].Message != "feat: add main.go" {
		t.Fatalf("expected oldest commit last, got: %s", commits[len(commits)-1].Message)
	}
}

func TestReadCommits_ParsesFields(t *testing.T) {
	dir := initTestRepoWithCommits(t)

	since := time.Now().Add(-1 * time.Hour)
	commits, err := git.ReadCommits(dir, since)
	if err != nil {
		t.Fatalf("ReadCommits: %v", err)
	}

	for _, c := range commits {
		if c.Hash == "" {
			t.Error("commit hash is empty")
		}
		if len(c.Hash) != 40 {
			t.Errorf("expected 40-char hash, got %d: %s", len(c.Hash), c.Hash)
		}
		if c.Message == "" {
			t.Error("commit message is empty")
		}
		if c.Author != "Test User" {
			t.Errorf("expected author 'Test User', got '%s'", c.Author)
		}
		if c.CommittedAt == "" {
			t.Error("committed_at is empty")
		}
	}
}

func TestReadCommits_FileChanges(t *testing.T) {
	dir := initTestRepoWithCommits(t)

	since := time.Now().Add(-1 * time.Hour)
	commits, err := git.ReadCommits(dir, since)
	if err != nil {
		t.Fatalf("ReadCommits: %v", err)
	}

	// Find commit "feat: add main.go" — should have 1 added file
	for _, c := range commits {
		if c.Message == "feat: add main.go" {
			if len(c.FilesChanged) != 1 {
				t.Fatalf("expected 1 file changed, got %d", len(c.FilesChanged))
			}
			if c.FilesChanged[0].Path != "main.go" {
				t.Errorf("expected main.go, got %s", c.FilesChanged[0].Path)
			}
			if c.FilesChanged[0].Action != "added" {
				t.Errorf("expected action 'added', got '%s'", c.FilesChanged[0].Action)
			}
			return
		}
	}
	t.Fatal("commit 'feat: add main.go' not found")
}

func TestReadCommits_FileChanges_Modified(t *testing.T) {
	dir := initTestRepoWithCommits(t)

	since := time.Now().Add(-1 * time.Hour)
	commits, err := git.ReadCommits(dir, since)
	if err != nil {
		t.Fatalf("ReadCommits: %v", err)
	}

	// Find "implement helper function" — should have 1 modified + 1 added
	for _, c := range commits {
		if c.Message == "implement helper function" {
			if len(c.FilesChanged) != 2 {
				t.Fatalf("expected 2 files changed, got %d", len(c.FilesChanged))
			}

			found := map[string]string{}
			for _, f := range c.FilesChanged {
				found[f.Path] = f.Action
			}
			if found["main.go"] != "modified" {
				t.Errorf("main.go: expected 'modified', got '%s'", found["main.go"])
			}
			if found["utils.go"] != "added" {
				t.Errorf("utils.go: expected 'added', got '%s'", found["utils.go"])
			}
			return
		}
	}
	t.Fatal("commit 'implement helper function' not found")
}

func TestReadCommits_FileChanges_Deleted(t *testing.T) {
	dir := initTestRepoWithCommits(t)

	since := time.Now().Add(-1 * time.Hour)
	commits, err := git.ReadCommits(dir, since)
	if err != nil {
		t.Fatalf("ReadCommits: %v", err)
	}

	// Find "fix: remove unused utils" — should have 1 deleted file
	for _, c := range commits {
		if c.Message == "fix: remove unused utils" {
			if len(c.FilesChanged) != 1 {
				t.Fatalf("expected 1 file changed, got %d", len(c.FilesChanged))
			}
			if c.FilesChanged[0].Path != "utils.go" {
				t.Errorf("expected utils.go, got %s", c.FilesChanged[0].Path)
			}
			if c.FilesChanged[0].Action != "deleted" {
				t.Errorf("expected action 'deleted', got '%s'", c.FilesChanged[0].Action)
			}
			return
		}
	}
	t.Fatal("commit 'fix: remove unused utils' not found")
}

func TestReadCommits_SinceFilter(t *testing.T) {
	dir := initTestRepoWithCommits(t)

	// Use a future time — should return no commits
	since := time.Now().Add(1 * time.Hour)
	commits, err := git.ReadCommits(dir, since)
	if err != nil {
		t.Fatalf("ReadCommits: %v", err)
	}

	if len(commits) != 0 {
		t.Fatalf("expected 0 commits with future since time, got %d", len(commits))
	}
}

func TestReadCommits_EmptyRepo(t *testing.T) {
	dir := t.TempDir()
	dir, _ = filepath.EvalSymlinks(dir)

	env := append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
	cmd := exec.Command("git", "init", dir)
	cmd.Env = env
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}

	since := time.Now().Add(-1 * time.Hour)
	commits, err := git.ReadCommits(dir, since)
	if err != nil {
		t.Fatalf("ReadCommits on empty repo: %v", err)
	}

	if len(commits) != 0 {
		t.Fatalf("expected 0 commits on empty repo, got %d", len(commits))
	}
}

func TestGetCurrentBranch(t *testing.T) {
	dir := initTestRepoWithCommits(t)

	branch, err := git.GetCurrentBranch(dir)
	if err != nil {
		t.Fatalf("GetCurrentBranch: %v", err)
	}

	// Default branch could be "main" or "master" depending on git config
	if branch == "" {
		t.Fatal("branch name is empty")
	}
}
