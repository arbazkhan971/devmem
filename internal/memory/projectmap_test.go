package memory

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/arbazkhan971/memorx/internal/storage"
)

func setupTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, ".memory", "memory.db")
	db, err := storage.NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := storage.Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return NewStore(db), dir
}

func TestSaveAndGetProjectMap(t *testing.T) {
	store, _ := setupTestStore(t)

	pm := &ProjectMap{
		Root:      "/tmp/test-project",
		FileCount: 42,
		Languages: map[string]int{"go": 30, "typescript": 12},
		KeyFiles: []ProjectFile{
			{Path: "main.go", Language: "go", Role: "entry"},
			{Path: "go.mod", Language: "", Role: "config"},
		},
		Directories: []string{"cmd", "internal", "pkg"},
		ScannedAt:   "2025-01-15 10:00:00",
	}

	if err := store.SaveProjectMap(pm); err != nil {
		t.Fatalf("SaveProjectMap: %v", err)
	}

	got, err := store.GetProjectMap()
	if err != nil {
		t.Fatalf("GetProjectMap: %v", err)
	}

	if got.Root != pm.Root {
		t.Errorf("Root: got %q, want %q", got.Root, pm.Root)
	}
	if got.FileCount != pm.FileCount {
		t.Errorf("FileCount: got %d, want %d", got.FileCount, pm.FileCount)
	}
	if got.Languages["go"] != 30 {
		t.Errorf("Languages[go]: got %d, want 30", got.Languages["go"])
	}
	if got.Languages["typescript"] != 12 {
		t.Errorf("Languages[typescript]: got %d, want 12", got.Languages["typescript"])
	}
	if len(got.KeyFiles) != 2 {
		t.Errorf("KeyFiles: got %d, want 2", len(got.KeyFiles))
	}
	if len(got.Directories) != 3 {
		t.Errorf("Directories: got %d, want 3", len(got.Directories))
	}
}

func TestSaveProjectMap_Upsert(t *testing.T) {
	store, _ := setupTestStore(t)

	pm1 := &ProjectMap{
		Root:      "/tmp/project1",
		FileCount: 10,
		Languages: map[string]int{"go": 10},
		ScannedAt: "2025-01-15 10:00:00",
	}
	if err := store.SaveProjectMap(pm1); err != nil {
		t.Fatalf("SaveProjectMap first: %v", err)
	}

	pm2 := &ProjectMap{
		Root:      "/tmp/project2",
		FileCount: 20,
		Languages: map[string]int{"python": 20},
		ScannedAt: "2025-01-16 10:00:00",
	}
	if err := store.SaveProjectMap(pm2); err != nil {
		t.Fatalf("SaveProjectMap second: %v", err)
	}

	got, err := store.GetProjectMap()
	if err != nil {
		t.Fatalf("GetProjectMap: %v", err)
	}
	if got.FileCount != 20 {
		t.Errorf("FileCount should be updated: got %d, want 20", got.FileCount)
	}
	if got.Languages["python"] != 20 {
		t.Errorf("Languages should be updated to python:20, got %v", got.Languages)
	}
}

func TestGetProjectMap_Empty(t *testing.T) {
	store, _ := setupTestStore(t)

	_, err := store.GetProjectMap()
	if err == nil {
		t.Fatal("GetProjectMap should error when no map exists")
	}
}

func TestIdentifyKeyFiles(t *testing.T) {
	files := []string{
		"main.go",
		"go.mod",
		"cmd/server/main.go",
		"internal/handler.go",
		"internal/handler_test.go",
		"Dockerfile",
		"pkg/util.go",
	}
	// Create a temp dir so Dockerfile stat works
	gitRoot := t.TempDir()
	os.WriteFile(filepath.Join(gitRoot, "Dockerfile"), []byte("FROM scratch"), 0644)

	keyFiles := identifyKeyFiles(files, gitRoot)

	roles := map[string]bool{}
	for _, f := range keyFiles {
		roles[f.Role] = true
	}
	if !roles["entry"] {
		t.Error("expected at least one entry file")
	}
	if !roles["config"] {
		t.Error("expected at least one config file")
	}
	if !roles["test"] {
		t.Error("expected at least one test file")
	}
	if !roles["infra"] {
		t.Error("expected at least one infra file (Dockerfile)")
	}
}

func TestLanguageFromExt(t *testing.T) {
	cases := []struct {
		ext  string
		want string
	}{
		{".go", "go"},
		{".ts", "typescript"},
		{".tsx", "typescript"},
		{".js", "javascript"},
		{".py", "python"},
		{".rs", "rust"},
		{".java", "java"},
		{".txt", ""},
		{".md", ""},
	}
	for _, tc := range cases {
		if got := languageFromExt(tc.ext); got != tc.want {
			t.Errorf("languageFromExt(%q) = %q, want %q", tc.ext, got, tc.want)
		}
	}
}

func TestListTopLevelDirs(t *testing.T) {
	dir := t.TempDir()
	for _, d := range []string{"cmd", "internal", "pkg", ".git", "node_modules", ".hidden"} {
		os.Mkdir(filepath.Join(dir, d), 0755)
	}
	// Create a regular file too (should not appear).
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644)

	dirs := listTopLevelDirs(dir)
	want := []string{"cmd", "internal", "pkg"}
	if len(dirs) != len(want) {
		t.Fatalf("listTopLevelDirs: got %v, want %v", dirs, want)
	}
	for i, d := range dirs {
		if d != want[i] {
			t.Errorf("listTopLevelDirs[%d]: got %q, want %q", i, d, want[i])
		}
	}
}

func TestFormatProjectMap(t *testing.T) {
	pm := &ProjectMap{
		Root:      "/home/user/myproject",
		FileCount: 42,
		Languages: map[string]int{"go": 30, "typescript": 12},
		KeyFiles: []ProjectFile{
			{Path: "main.go", Language: "go", Role: "entry"},
			{Path: "go.mod", Language: "", Role: "config"},
		},
		Directories: []string{"cmd", "internal", "pkg"},
		ScannedAt:   "2025-01-15 10:00:00",
	}

	output := FormatProjectMap(pm)

	checks := []string{
		"# Project Map: myproject",
		"**Files:** 42",
		"go: 30 files",
		"typescript: 12 files",
		"cmd/",
		"internal/",
		"pkg/",
		"`main.go`",
		"`go.mod`",
		"Entry",
		"Config",
	}
	for _, c := range checks {
		if !containsStr(output, c) {
			t.Errorf("FormatProjectMap output should contain %q, got:\n%s", c, output)
		}
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestScanProject(t *testing.T) {
	store, dir := setupTestStore(t)

	// Set up a fake git repo structure.
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	os.MkdirAll(filepath.Join(dir, "cmd"), 0755)
	os.MkdirAll(filepath.Join(dir, "internal"), 0755)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644)
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0644)
	os.WriteFile(filepath.Join(dir, "cmd", "server.go"), []byte("package main"), 0644)
	os.WriteFile(filepath.Join(dir, "internal", "handler.go"), []byte("package internal"), 0644)
	os.WriteFile(filepath.Join(dir, "internal", "handler_test.go"), []byte("package internal"), 0644)

	// Initialize a git repo so git ls-files works.
	if err := runGit(dir, "init"); err != nil {
		t.Fatalf("git init: %v", err)
	}
	if err := runGit(dir, "add", "."); err != nil {
		t.Fatalf("git add: %v", err)
	}

	pm, err := store.ScanProject(dir)
	if err != nil {
		t.Fatalf("ScanProject: %v", err)
	}

	if pm.FileCount == 0 {
		t.Error("ScanProject should find at least some files")
	}
	if pm.Root != dir {
		t.Errorf("Root: got %q, want %q", pm.Root, dir)
	}
	if pm.Languages["go"] == 0 {
		t.Error("ScanProject should detect Go files")
	}
	if len(pm.Directories) == 0 {
		t.Error("ScanProject should find directories")
	}

	// Verify it's persisted.
	got, err := store.GetProjectMap()
	if err != nil {
		t.Fatalf("GetProjectMap after scan: %v", err)
	}
	if got.FileCount != pm.FileCount {
		t.Errorf("persisted FileCount: got %d, want %d", got.FileCount, pm.FileCount)
	}
}

func runGit(dir string, args ...string) error {
	c := exec.Command("git", args...)
	c.Dir = dir
	c.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@test.com",
	)
	return c.Run()
}
