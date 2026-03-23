package git_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arbazkhan971/memorx/internal/git"
)

func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	// Resolve symlinks (macOS /var -> /private/var)
	dir, _ = filepath.EvalSymlinks(dir)
	cmd := exec.Command("git", "init", dir)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}
	return dir
}

func TestFindGitRoot_FromRoot(t *testing.T) {
	dir := initTestRepo(t)
	root, err := git.FindGitRoot(dir)
	if err != nil {
		t.Fatalf("FindGitRoot: %v", err)
	}
	if root != dir {
		t.Fatalf("expected %s, got %s", dir, root)
	}
}

func TestFindGitRoot_FromSubdir(t *testing.T) {
	dir := initTestRepo(t)
	subdir := filepath.Join(dir, "src", "pkg")
	os.MkdirAll(subdir, 0755)

	root, err := git.FindGitRoot(subdir)
	if err != nil {
		t.Fatalf("FindGitRoot: %v", err)
	}
	if root != dir {
		t.Fatalf("expected %s, got %s", dir, root)
	}
}

func TestFindGitRoot_NotGitRepo(t *testing.T) {
	dir := t.TempDir()
	_, err := git.FindGitRoot(dir)
	if err == nil {
		t.Fatal("expected error for non-git directory")
	}
}

func TestEnsureMemoryDir_Creates(t *testing.T) {
	dir := initTestRepo(t)
	memDir, err := git.EnsureMemoryDir(dir)
	if err != nil {
		t.Fatalf("EnsureMemoryDir: %v", err)
	}

	expected := filepath.Join(dir, ".memory")
	if memDir != expected {
		t.Fatalf("expected %s, got %s", expected, memDir)
	}

	if _, err := os.Stat(memDir); os.IsNotExist(err) {
		t.Fatal(".memory/ dir not created")
	}
}

func TestEnsureMemoryDir_AddsGitignore(t *testing.T) {
	dir := initTestRepo(t)
	_, err := git.EnsureMemoryDir(dir)
	if err != nil {
		t.Fatalf("EnsureMemoryDir: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatal("expected .gitignore to exist")
	}
	if !strings.Contains(string(content), ".memory/") {
		t.Fatal(".gitignore should contain .memory/")
	}
}

func TestEnsureMemoryDir_Idempotent(t *testing.T) {
	dir := initTestRepo(t)
	git.EnsureMemoryDir(dir)
	git.EnsureMemoryDir(dir) // second call

	content, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatal("expected .gitignore to exist")
	}
	// Should only appear once
	count := strings.Count(string(content), ".memory/")
	if count != 1 {
		t.Fatalf("expected .memory/ to appear once, got %d times", count)
	}
}

func TestProjectName(t *testing.T) {
	name := git.ProjectName("/Users/arbaz/Lineupx/LineupX-NextJS")
	if name != "LineupX-NextJS" {
		t.Fatalf("expected LineupX-NextJS, got %s", name)
	}
}

func TestProjectName_VariousPaths(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/home/user/projects/my-app", "my-app"},
		{"/tmp/single", "single"},
		{".", "."},
		{"/", "/"},
		{"/deep/nested/path/to/repo", "repo"},
	}
	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			got := git.ProjectName(tc.path)
			if got != tc.want {
				t.Errorf("ProjectName(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

func TestCurrentBranch_DetachedHEAD(t *testing.T) {
	dir := initTestRepo(t)
	env := append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=t@t.com",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=t@t.com",
	)
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}

	os.WriteFile(filepath.Join(dir, "f.txt"), []byte("x"), 0644)
	run("git", "add", "f.txt")
	run("git", "commit", "-m", "initial")

	// Detach HEAD
	run("git", "checkout", "--detach")

	branch, err := git.CurrentBranch(dir)
	if err != nil {
		t.Fatalf("CurrentBranch on detached HEAD: %v", err)
	}
	if branch != "HEAD" {
		t.Errorf("expected 'HEAD' on detached HEAD, got %q", branch)
	}
}

func TestFindGitRoot_SymlinkedPath(t *testing.T) {
	dir := initTestRepo(t)
	// Finding root from the root itself should work
	root, err := git.FindGitRoot(dir)
	if err != nil {
		t.Fatalf("FindGitRoot: %v", err)
	}
	if root == "" {
		t.Fatal("expected non-empty root")
	}
}
