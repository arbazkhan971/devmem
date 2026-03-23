# devmem Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a SOTA local MCP memory server in Go that gives any coding CLI persistent, project-scoped session/feature memory with bi-temporal facts, semantic diffs, plan tracking, and background consolidation.

**Architecture:** Single Go binary using stdio MCP transport. SQLite (WAL mode) for storage with FTS5 for search. tree-sitter for AST-level code diff analysis. Background goroutine for memory consolidation. Per-project `.memory/` directory auto-detected from git root.

**Tech Stack:** Go 1.26, modernc.org/sqlite (pure Go), mcp-go SDK, go-tree-sitter, SQLite FTS5

---

## File Structure

```
devmem/
├── cmd/devmem/main.go                    # Entry point, CLI flags, MCP server bootstrap
├── internal/
│   ├── storage/
│   │   ├── db.go                         # SQLite connection manager (WAL, single writer + readers)
│   │   ├── schema.go                     # All CREATE TABLE/INDEX/FTS5 statements
│   │   └── migrations.go                 # Schema version tracking + migration runner
│   ├── memory/
│   │   ├── features.go                   # Feature CRUD (create, list, get, update status)
│   │   ├── sessions.go                   # Session lifecycle (auto-create on connect, end on disconnect)
│   │   ├── facts.go                      # Bi-temporal fact operations (create, invalidate, query-as-of)
│   │   ├── notes.go                      # Note CRUD + plan-like content detection
│   │   ├── links.go                      # Memory link CRUD + auto-linking via FTS5 similarity
│   │   └── context.go                    # Tiered context assembly (compact/standard/detailed)
│   ├── git/
│   │   ├── project.go                    # Git root detection, .gitignore management
│   │   ├── reader.go                     # Git log, diff-tree, show via exec.Command
│   │   └── intent.go                     # Commit intent classification (keyword heuristics)
│   ├── search/
│   │   ├── engine.go                     # 3-layer search orchestration (FTS5 → trigram → fuzzy)
│   │   ├── scoring.go                    # BM25 * temporal_decay * type_weight * link_boost
│   │   └── graph.go                      # Recursive CTE link traversal (2-hop)
│   ├── plans/
│   │   ├── manager.go                    # Plan CRUD + bi-temporal versioning
│   │   ├── detect.go                     # Auto-detect plan-like content from notes
│   │   └── match.go                      # Commit-to-plan-step matching via FTS5 similarity
│   ├── semantic/
│   │   ├── diff.go                       # tree-sitter AST diff engine (3-phase matching)
│   │   ├── parser.go                     # Language-specific tree-sitter setup (Go, TS, JS)
│   │   └── entities.go                   # Code entity extraction from AST nodes
│   ├── consolidation/
│   │   ├── engine.go                     # Background goroutine, entropy trigger, consolidation loop
│   │   ├── contradictions.go             # Fact conflict detection + resolution
│   │   ├── decay.go                      # Memory relevance decay (14-day half-life)
│   │   ├── summarize.go                  # Recursive tiered summarization
│   │   └── links.go                      # Background link discovery for unlinked memories
│   └── mcp/
│       ├── server.go                     # MCP stdio server setup, tool/resource registration
│       ├── tools.go                      # All 9 tool handler implementations
│       └── resources.go                  # 2 resource handlers + notification emitters
├── go.mod
├── go.sum
├── Makefile
└── docs/specs/2026-03-23-devmem-design.md
```

---

## Chunk 1: Foundation — Go Module, SQLite, Schema, Git Detection

**What this builds:** A Go binary that initializes a SQLite database in `.memory/` at the git root of the current directory. No MCP yet — just the storage foundation.

### Task 1.1: Initialize Go module and dependencies

**Files:**
- Create: `go.mod`
- Create: `go.sum`
- Create: `Makefile`

- [ ] **Step 1: Initialize Go module**

```bash
cd /Users/arbaz/devmem
go mod init github.com/arbaz/devmem
```

- [ ] **Step 2: Add core dependencies**

```bash
go get modernc.org/sqlite
go get github.com/google/uuid
go get github.com/mark3labs/mcp-go
```

- [ ] **Step 3: Create Makefile**

```makefile
.PHONY: build test run clean

build:
	go build -o bin/devmem ./cmd/devmem

test:
	go test ./... -v -count=1

run: build
	./bin/devmem

clean:
	rm -rf bin/
```

- [ ] **Step 4: Commit**

```bash
git add go.mod go.sum Makefile
git commit -m "chore: initialize Go module with core dependencies"
```

---

### Task 1.2: SQLite connection manager

**Files:**
- Create: `internal/storage/db.go`
- Create: `internal/storage/db_test.go`

- [ ] **Step 1: Write failing test for DB initialization**

```go
// internal/storage/db_test.go
package storage_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/arbaz/devmem/internal/storage"
)

func TestNewDB_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "memory.db")

	db, err := storage.NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB failed: %v", err)
	}
	defer db.Close()

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatal("database file was not created")
	}
}

func TestNewDB_WALMode(t *testing.T) {
	dir := t.TempDir()
	db, err := storage.NewDB(filepath.Join(dir, "memory.db"))
	if err != nil {
		t.Fatalf("NewDB failed: %v", err)
	}
	defer db.Close()

	var journalMode string
	err = db.Reader().QueryRow("PRAGMA journal_mode").Scan(&journalMode)
	if err != nil {
		t.Fatalf("PRAGMA query failed: %v", err)
	}
	if journalMode != "wal" {
		t.Fatalf("expected WAL mode, got %s", journalMode)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/arbaz/devmem && go test ./internal/storage/ -v -run TestNewDB
```
Expected: FAIL — package doesn't exist yet.

- [ ] **Step 3: Implement DB connection manager**

```go
// internal/storage/db.go
package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// DB manages SQLite connections with WAL mode.
// Single writer + multiple readers for concurrent MCP client access.
type DB struct {
	writer *sql.DB
	reader *sql.DB
	path   string
}

func NewDB(dbPath string) (*DB, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	writer, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL&_foreign_keys=ON")
	if err != nil {
		return nil, fmt.Errorf("open writer: %w", err)
	}
	writer.SetMaxOpenConns(1)

	reader, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000&mode=ro&_foreign_keys=ON")
	if err != nil {
		writer.Close()
		return nil, fmt.Errorf("open reader: %w", err)
	}
	reader.SetMaxOpenConns(4)

	// Force WAL mode on writer
	if _, err := writer.Exec("PRAGMA journal_mode=WAL"); err != nil {
		writer.Close()
		reader.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}

	return &DB{writer: writer, reader: reader, path: dbPath}, nil
}

func (db *DB) Writer() *sql.DB { return db.writer }
func (db *DB) Reader() *sql.DB { return db.reader }
func (db *DB) Path() string    { return db.path }

func (db *DB) Close() error {
	var errs []error
	if err := db.reader.Close(); err != nil {
		errs = append(errs, err)
	}
	if err := db.writer.Close(); err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return fmt.Errorf("close errors: %v", errs)
	}
	return nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/storage/ -v -run TestNewDB
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/storage/db.go internal/storage/db_test.go
git commit -m "feat: add SQLite connection manager with WAL mode"
```

---

### Task 1.3: Schema definition and migration

**Files:**
- Create: `internal/storage/schema.go`
- Create: `internal/storage/migrations.go`
- Create: `internal/storage/schema_test.go`

- [ ] **Step 1: Write failing test for schema migration**

```go
// internal/storage/schema_test.go
package storage_test

import (
	"path/filepath"
	"testing"

	"github.com/arbaz/devmem/internal/storage"
)

func TestMigrate_CreatesAllTables(t *testing.T) {
	dir := t.TempDir()
	db, err := storage.NewDB(filepath.Join(dir, "memory.db"))
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	if err := storage.Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	expectedTables := []string{
		"features", "sessions", "facts", "notes",
		"plans", "plan_steps", "commits", "semantic_changes",
		"memory_links", "summaries", "consolidation_state",
	}
	for _, table := range expectedTables {
		var name string
		err := db.Reader().QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", table,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %s not found: %v", table, err)
		}
	}
}

func TestMigrate_CreatesFTSTables(t *testing.T) {
	dir := t.TempDir()
	db, err := storage.NewDB(filepath.Join(dir, "memory.db"))
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	if err := storage.Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	ftsTables := []string{"notes_fts", "commits_fts", "facts_fts", "plans_fts"}
	for _, table := range ftsTables {
		var name string
		err := db.Reader().QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", table,
		).Scan(&name)
		if err != nil {
			t.Errorf("FTS table %s not found: %v", table, err)
		}
	}
}

func TestMigrate_Idempotent(t *testing.T) {
	dir := t.TempDir()
	db, err := storage.NewDB(filepath.Join(dir, "memory.db"))
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	if err := storage.Migrate(db); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}
	if err := storage.Migrate(db); err != nil {
		t.Fatalf("second Migrate should be idempotent: %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/storage/ -v -run TestMigrate
```
Expected: FAIL

- [ ] **Step 3: Implement schema definitions**

Create `internal/storage/schema.go` with all CREATE TABLE IF NOT EXISTS statements from the spec (features, sessions, facts, notes, plans, plan_steps, commits, semantic_changes, memory_links, summaries, consolidation_state) plus FTS5 virtual tables and indexes.

- [ ] **Step 4: Implement migration runner**

Create `internal/storage/migrations.go` with a `Migrate(db *DB) error` function that executes schema in a transaction via the writer connection. Use a `schema_version` table to track applied migrations.

- [ ] **Step 5: Run tests**

```bash
go test ./internal/storage/ -v -run TestMigrate
```
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/storage/schema.go internal/storage/migrations.go internal/storage/schema_test.go
git commit -m "feat: add SQLite schema with bi-temporal facts, FTS5, and migration runner"
```

---

### Task 1.4: Git project detection

**Files:**
- Create: `internal/git/project.go`
- Create: `internal/git/project_test.go`

- [ ] **Step 1: Write failing test**

```go
// internal/git/project_test.go
package git_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/arbaz/devmem/internal/git"
)

func TestFindGitRoot_FromSubdir(t *testing.T) {
	dir := t.TempDir()
	// Initialize git repo
	cmd := exec.Command("git", "init", dir)
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}

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

func TestMemoryDir_CreatesAndGitignores(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.Command("git", "init", dir)
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}

	memDir, err := git.EnsureMemoryDir(dir)
	if err != nil {
		t.Fatalf("EnsureMemoryDir: %v", err)
	}

	if _, err := os.Stat(memDir); os.IsNotExist(err) {
		t.Fatal(".memory/ dir not created")
	}

	gitignore, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatal("expected .gitignore to exist")
	}
	if !contains(string(gitignore), ".memory/") {
		t.Fatal(".gitignore should contain .memory/")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/git/ -v -run TestFindGitRoot
```

- [ ] **Step 3: Implement git project detection**

Create `internal/git/project.go` with `FindGitRoot(dir string) (string, error)` using `git rev-parse --show-toplevel` and `EnsureMemoryDir(gitRoot string) (string, error)` that creates `.memory/` and appends to `.gitignore`.

- [ ] **Step 4: Run tests**

```bash
go test ./internal/git/ -v
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/git/project.go internal/git/project_test.go
git commit -m "feat: add git root detection and .memory/ directory setup"
```

---

### Task 1.5: Entry point that ties it together

**Files:**
- Create: `cmd/devmem/main.go`

- [ ] **Step 1: Create main.go**

```go
// cmd/devmem/main.go
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/arbaz/devmem/internal/git"
	"github.com/arbaz/devmem/internal/storage"
)

func main() {
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("get working directory: %v", err)
	}

	gitRoot, err := git.FindGitRoot(cwd)
	if err != nil {
		log.Fatalf("not a git repository: %v", err)
	}

	memDir, err := git.EnsureMemoryDir(gitRoot)
	if err != nil {
		log.Fatalf("setup memory directory: %v", err)
	}

	dbPath := memDir + "/memory.db"
	db, err := storage.NewDB(dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	if err := storage.Migrate(db); err != nil {
		log.Fatalf("migrate database: %v", err)
	}

	fmt.Fprintf(os.Stderr, "devmem: initialized at %s\n", memDir)
	// MCP server will be added in Chunk 2
}
```

- [ ] **Step 2: Build and verify**

```bash
cd /Users/arbaz/devmem && make build
./bin/devmem
```
Expected: prints "devmem: initialized at /Users/arbaz/devmem/.memory"

- [ ] **Step 3: Commit**

```bash
git add cmd/devmem/main.go
git commit -m "feat: add entry point with git detection and DB initialization"
```

---

## Chunk 2: Memory Core — Features, Sessions, Notes, Facts

**What this builds:** CRUD operations for features, sessions, notes, and bi-temporal facts. All with tests. No MCP layer yet.

### Task 2.1: Feature operations

**Files:**
- Create: `internal/memory/features.go`
- Create: `internal/memory/features_test.go`

- [ ] **Step 1: Write failing tests for feature CRUD**

Test: Create, Get, List, UpdateStatus, GetActive, StartFeature (create-or-resume + auto-pause logic).

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/memory/ -v -run TestFeature
```

- [ ] **Step 3: Implement feature operations**

`Store` struct wraps `*storage.DB`. Methods: `CreateFeature`, `GetFeature`, `ListFeatures`, `UpdateFeatureStatus`, `GetActiveFeature`, `StartFeature` (idempotent create-or-resume with auto-pause of current active).

- [ ] **Step 4: Run tests — PASS**

- [ ] **Step 5: Commit**

```bash
git commit -m "feat: add feature CRUD with create-or-resume logic"
```

---

### Task 2.2: Session lifecycle

**Files:**
- Create: `internal/memory/sessions.go`
- Create: `internal/memory/sessions_test.go`

- [ ] **Step 1: Write failing tests**

Test: CreateSession (auto-creates under active feature), EndSession, ListSessions, GetCurrentSession.

- [ ] **Step 2: Run — FAIL**
- [ ] **Step 3: Implement**
- [ ] **Step 4: Run — PASS**
- [ ] **Step 5: Commit**

```bash
git commit -m "feat: add session lifecycle management"
```

---

### Task 2.3: Bi-temporal facts

**Files:**
- Create: `internal/memory/facts.go`
- Create: `internal/memory/facts_test.go`

- [ ] **Step 1: Write failing tests**

Test: CreateFact, GetActiveFacts (WHERE invalid_at IS NULL), InvalidateFact, QueryAsOf(datetime) (bi-temporal query), auto-invalidation when contradicting fact inserted (same subject+predicate, different object).

- [ ] **Step 2: Run — FAIL**
- [ ] **Step 3: Implement bi-temporal fact operations**

Key method: `CreateFact` checks for existing facts with same subject+predicate. If found and object differs → invalidate old (set invalid_at=now), insert new. This is the contradiction resolution.

- [ ] **Step 4: Run — PASS**
- [ ] **Step 5: Commit**

```bash
git commit -m "feat: add bi-temporal facts with auto-contradiction resolution"
```

---

### Task 2.4: Notes with FTS5 sync

**Files:**
- Create: `internal/memory/notes.go`
- Create: `internal/memory/notes_test.go`

- [ ] **Step 1: Write failing tests**

Test: CreateNote, ListNotes (by feature, by type), FTS5 search on note content.

- [ ] **Step 2: Run — FAIL**
- [ ] **Step 3: Implement notes with FTS5 triggers**

On insert, also insert into `notes_fts` and `notes_trigram`. Use SQLite triggers or manual sync in Go.

- [ ] **Step 4: Run — PASS**
- [ ] **Step 5: Commit**

```bash
git commit -m "feat: add notes with FTS5 full-text indexing"
```

---

### Task 2.5: Memory links (A-MEM)

**Files:**
- Create: `internal/memory/links.go`
- Create: `internal/memory/links_test.go`

- [ ] **Step 1: Write failing tests**

Test: CreateLink, GetLinks (for a memory), FindRelated (FTS5 search + auto-link candidates), 2-hop graph traversal via recursive CTE.

- [ ] **Step 2: Run — FAIL**
- [ ] **Step 3: Implement**

`AutoLink(sourceID, sourceType, content string)`: runs FTS5 search across notes_fts, facts_fts, commits_fts. For each result above threshold → create bidirectional link with relationship="related".

- [ ] **Step 4: Run — PASS**
- [ ] **Step 5: Commit**

```bash
git commit -m "feat: add A-MEM style memory linking with auto-discovery"
```

---

### Task 2.6: Tiered context assembly

**Files:**
- Create: `internal/memory/context.go`
- Create: `internal/memory/context_test.go`

- [ ] **Step 1: Write failing tests**

Test: GetContext with tier=compact (~200 tokens), standard (~500), detailed (~1500). Verify each tier includes correct data levels.

- [ ] **Step 2: Run — FAIL**
- [ ] **Step 3: Implement**

`GetContext(featureID string, tier string, asOf *time.Time) (*Context, error)`: assembles context from summaries, plans, commits, notes, facts, links based on tier. Uses bi-temporal asOf for historical queries.

- [ ] **Step 4: Run — PASS**
- [ ] **Step 5: Commit**

```bash
git commit -m "feat: add tiered context assembly (compact/standard/detailed)"
```

---

## Chunk 3: Git Engine — Commit Sync, Intent Classification

### Task 3.1: Git log reader

**Files:**
- Create: `internal/git/reader.go`
- Create: `internal/git/reader_test.go`

- [ ] **Step 1: Write failing tests**

Test: ReadCommits(since time.Time) returns parsed commits with hash, message, author, date, files. Use a temp git repo with known commits.

- [ ] **Step 2: Run — FAIL**
- [ ] **Step 3: Implement using exec.Command("git", "log", ...)**
- [ ] **Step 4: Run — PASS**
- [ ] **Step 5: Commit**

```bash
git commit -m "feat: add git commit reader via exec"
```

---

### Task 3.2: Commit intent classification

**Files:**
- Create: `internal/git/intent.go`
- Create: `internal/git/intent_test.go`

- [ ] **Step 1: Write failing tests**

```go
func TestClassifyIntent(t *testing.T) {
	tests := []struct {
		message  string
		files    []string
		expected string
	}{
		{"fix: resolve auth crash on expired tokens", nil, "bugfix"},
		{"feat: add refresh token rotation", nil, "feature"},
		{"refactor: extract middleware", nil, "refactor"},
		{"test: add webhook integration tests", nil, "test"},
		{"docs: update API reference", nil, "docs"},
		{"chore: update Dockerfile", []string{"Dockerfile"}, "infra"},
		{"random message", nil, "unknown"},
	}
	for _, tt := range tests {
		intent, _ := git.ClassifyIntent(tt.message, tt.files)
		if intent != tt.expected {
			t.Errorf("ClassifyIntent(%q) = %s, want %s", tt.message, intent, tt.expected)
		}
	}
}
```

- [ ] **Step 2: Run — FAIL**
- [ ] **Step 3: Implement keyword + file path heuristics from spec section 7.2**
- [ ] **Step 4: Run — PASS**
- [ ] **Step 5: Commit**

```bash
git commit -m "feat: add commit intent classification via heuristics"
```

---

### Task 3.3: Commit sync into memory store

**Files:**
- Create: `internal/memory/commits.go`
- Create: `internal/memory/commits_test.go`

- [ ] **Step 1: Write failing tests**

Test: SyncCommits reads from git, stores in DB, classifies intent, auto-links to existing notes/facts, deduplicates (skip already-synced hashes).

- [ ] **Step 2: Run — FAIL**
- [ ] **Step 3: Implement**
- [ ] **Step 4: Run — PASS**
- [ ] **Step 5: Commit**

```bash
git commit -m "feat: add commit sync with intent classification and auto-linking"
```

---

## Chunk 4: Search Engine — FTS5, Trigram, Fuzzy, Graph

### Task 4.1: Three-layer search

**Files:**
- Create: `internal/search/engine.go`
- Create: `internal/search/scoring.go`
- Create: `internal/search/graph.go`
- Create: `internal/search/engine_test.go`

- [ ] **Step 1: Write failing tests**

Test: Search returns results from notes, facts, commits, plans. Test FTS5 layer finds exact matches. Test trigram layer finds partial matches. Test fuzzy layer corrects typos. Test scoring applies temporal_decay * type_weight * link_boost.

- [ ] **Step 2: Run — FAIL**
- [ ] **Step 3: Implement 3-layer search engine from spec section 6**
- [ ] **Step 4: Run — PASS**
- [ ] **Step 5: Commit**

```bash
git commit -m "feat: add 3-layer search engine (FTS5/trigram/fuzzy) with scoring"
```

---

## Chunk 5: Plan Engine

### Task 5.1: Plan CRUD with bi-temporal versioning

**Files:**
- Create: `internal/plans/manager.go`
- Create: `internal/plans/manager_test.go`

- [ ] **Step 1: Write failing tests**

Test: CreatePlan, GetActivePlan, SupersedePlan (invalidates old, copies completed steps), UpdateStepStatus, ListPlansForFeature (including superseded).

- [ ] **Step 2: Run — FAIL**
- [ ] **Step 3: Implement**
- [ ] **Step 4: Run — PASS**
- [ ] **Step 5: Commit**

```bash
git commit -m "feat: add plan manager with bi-temporal versioning"
```

---

### Task 5.2: Plan auto-detection and commit matching

**Files:**
- Create: `internal/plans/detect.go`
- Create: `internal/plans/match.go`
- Create: `internal/plans/detect_test.go`

- [ ] **Step 1: Write failing tests**

Test: IsPlanLike detects numbered lists with plan keywords. Test ParseSteps extracts step titles. Test MatchCommitToStep finds FTS5 matches between commit messages and step titles.

- [ ] **Step 2: Run — FAIL**
- [ ] **Step 3: Implement from spec sections 9.1 and 9.2**
- [ ] **Step 4: Run — PASS**
- [ ] **Step 5: Commit**

```bash
git commit -m "feat: add plan auto-detection and commit-to-step matching"
```

---

## Chunk 6: MCP Server — All 9 Tools + Resources

### Task 6.1: MCP server setup with stdio transport

**Files:**
- Create: `internal/mcp/server.go`

- [ ] **Step 1: Implement MCP server**

Use `mcp-go` SDK to create stdio server. Register all 9 tools with names, descriptions, and JSON schemas. Register 2 resources.

- [ ] **Step 2: Update main.go to start MCP server**

Replace the placeholder print with actual MCP server loop.

- [ ] **Step 3: Build and verify server starts**

```bash
make build && echo '{"jsonrpc":"2.0","method":"initialize","id":1,"params":{"capabilities":{}}}' | ./bin/devmem
```
Expected: JSON-RPC response with tool list.

- [ ] **Step 4: Commit**

```bash
git commit -m "feat: add MCP stdio server with tool/resource registration"
```

---

### Task 6.2: Implement all tool handlers

**Files:**
- Create: `internal/mcp/tools.go`
- Create: `internal/mcp/resources.go`

- [ ] **Step 1: Implement devmem_status handler**

Calls `store.GetActiveFeature()`, `store.GetActivePlan()`, `consolidation.GetState()`. Returns JSON.

- [ ] **Step 2: Implement devmem_list_features handler**

Calls `store.ListFeatures()` with optional status filter. Enriches with intent breakdown from commits.

- [ ] **Step 3: Implement devmem_start_feature handler**

Calls `store.StartFeature(name, description)`. Creates session. Returns compact context.

- [ ] **Step 4: Implement devmem_switch_feature handler**

Pauses current, activates target, creates session, returns compact context.

- [ ] **Step 5: Implement devmem_get_context handler**

Calls `store.GetContext(featureID, tier, asOf)`. Returns tiered response.

- [ ] **Step 6: Implement devmem_sync handler**

Calls git reader → stores commits → classifies intent → matches plan steps → auto-links.

- [ ] **Step 7: Implement devmem_remember handler**

Stores note → extracts facts → checks contradictions → auto-links → detects plan-like content.

- [ ] **Step 8: Implement devmem_search handler**

Calls search engine with query, scope, types, asOf.

- [ ] **Step 9: Implement devmem_save_plan handler**

Calls plan manager → supersedes old plan → matches existing commits to steps.

- [ ] **Step 10: Implement resource handlers**

`devmem://context/active` returns compact context. `devmem://changes/recent` returns commits since last session.

- [ ] **Step 11: Build and integration test**

```bash
make build
claude mcp add -s project --transport stdio devmem -- /Users/arbaz/devmem/bin/devmem
```
Test each tool manually via Claude Code.

- [ ] **Step 12: Commit**

```bash
git commit -m "feat: implement all 9 MCP tool handlers and 2 resources"
```

---

## Chunk 7: Semantic Diff Engine (tree-sitter)

### Task 7.1: tree-sitter AST parsing

**Files:**
- Create: `internal/semantic/parser.go`
- Create: `internal/semantic/entities.go`
- Create: `internal/semantic/parser_test.go`

- [ ] **Step 1: Write failing tests**

Test: ParseFile extracts top-level entities (functions, structs, methods) from Go source. Test with known Go file content.

- [ ] **Step 2: Run — FAIL**
- [ ] **Step 3: Add tree-sitter dependency and implement parser**

```bash
go get github.com/tree-sitter/go-tree-sitter
go get github.com/tree-sitter/tree-sitter-go
go get github.com/tree-sitter/tree-sitter-javascript
go get github.com/tree-sitter/tree-sitter-typescript
```

Implement `ParseFile(content []byte, lang string) ([]Entity, error)`.

- [ ] **Step 4: Run — PASS**
- [ ] **Step 5: Commit**

```bash
git commit -m "feat: add tree-sitter AST parsing for Go/TS/JS"
```

---

### Task 7.2: Semantic diff engine

**Files:**
- Create: `internal/semantic/diff.go`
- Create: `internal/semantic/diff_test.go`

- [ ] **Step 1: Write failing tests**

Test: DiffEntities with known before/after entity lists. Verify added/modified/deleted/renamed detection via 3-phase matching.

- [ ] **Step 2: Run — FAIL**
- [ ] **Step 3: Implement 3-phase entity matching from spec section 7.3**
- [ ] **Step 4: Run — PASS**
- [ ] **Step 5: Commit**

```bash
git commit -m "feat: add semantic diff engine with 3-phase entity matching"
```

---

### Task 7.3: Wire semantic diffs into commit sync

**Files:**
- Modify: `internal/memory/commits.go`
- Modify: `internal/mcp/tools.go` (devmem_sync handler)

- [ ] **Step 1: Update SyncCommits to call semantic diff for each commit's changed files**
- [ ] **Step 2: Store results in semantic_changes table**
- [ ] **Step 3: Include semantic changes in devmem_get_context responses**
- [ ] **Step 4: Run all tests — PASS**
- [ ] **Step 5: Commit**

```bash
git commit -m "feat: wire semantic diffs into commit sync pipeline"
```

---

## Chunk 8: Consolidation Engine

### Task 8.1: Contradiction detection

**Files:**
- Create: `internal/consolidation/engine.go`
- Create: `internal/consolidation/contradictions.go`
- Create: `internal/consolidation/contradictions_test.go`

- [ ] **Step 1: Write failing tests**

Test: DetectContradictions finds facts with same subject+predicate but different object. Test auto-resolution keeps most recent.

- [ ] **Step 2: Run — FAIL**
- [ ] **Step 3: Implement from spec section 8.2**
- [ ] **Step 4: Run — PASS**
- [ ] **Step 5: Commit**

```bash
git commit -m "feat: add contradiction detection and auto-resolution"
```

---

### Task 8.2: Memory decay

**Files:**
- Create: `internal/consolidation/decay.go`
- Create: `internal/consolidation/decay_test.go`

- [ ] **Step 1: Write failing test**

Test: ApplyDecay reduces relevance scores for old, unaccessed memories. Memories accessed recently are not decayed.

- [ ] **Step 2: Run — FAIL**
- [ ] **Step 3: Implement 14-day half-life decay**
- [ ] **Step 4: Run — PASS**
- [ ] **Step 5: Commit**

```bash
git commit -m "feat: add memory relevance decay (14-day half-life)"
```

---

### Task 8.3: Recursive summarization

**Files:**
- Create: `internal/consolidation/summarize.go`
- Create: `internal/consolidation/summarize_test.go`

- [ ] **Step 1: Write failing test**

Test: When >20 unsummarized notes exist, GenerateSummary creates a gen-0 summary concatenating key points. Test gen-1 merges 5+ gen-0s.

- [ ] **Step 2: Run — FAIL**
- [ ] **Step 3: Implement heuristic summarization (V1: concatenate + truncate, no LLM)**
- [ ] **Step 4: Run — PASS**
- [ ] **Step 5: Commit**

```bash
git commit -m "feat: add recursive tiered summarization"
```

---

### Task 8.4: Background link discovery

**Files:**
- Create: `internal/consolidation/links.go`
- Create: `internal/consolidation/links_test.go`

- [ ] **Step 1: Write failing test**

Test: DiscoverLinks finds unlinked notes created since last run, searches for related memories, creates links.

- [ ] **Step 2: Run — FAIL**
- [ ] **Step 3: Implement**
- [ ] **Step 4: Run — PASS**
- [ ] **Step 5: Commit**

```bash
git commit -m "feat: add background link discovery"
```

---

### Task 8.5: Consolidation engine loop

**Files:**
- Modify: `internal/consolidation/engine.go`
- Create: `internal/consolidation/engine_test.go`

- [ ] **Step 1: Write failing tests**

Test: Engine runs when entropy exceeds threshold. Test it calls contradictions, decay, summarize, and link discovery in sequence. Test it updates consolidation_state.

- [ ] **Step 2: Run — FAIL**
- [ ] **Step 3: Implement background goroutine with entropy trigger**
- [ ] **Step 4: Run — PASS**
- [ ] **Step 5: Wire into main.go (start goroutine after DB init)**
- [ ] **Step 6: Commit**

```bash
git commit -m "feat: add consolidation engine with entropy-triggered background loop"
```

---

## Chunk 9: current.json Snapshot + Polish

### Task 9.1: JSON snapshot writer

**Files:**
- Create: `internal/memory/snapshot.go`
- Create: `internal/memory/snapshot_test.go`

- [ ] **Step 1: Write failing test**

Test: WriteSnapshot creates valid JSON at `.memory/current.json` with project name, active feature, plan progress, features list, consolidation state.

- [ ] **Step 2: Run — FAIL**
- [ ] **Step 3: Implement — called after every write operation**
- [ ] **Step 4: Run — PASS**
- [ ] **Step 5: Commit**

```bash
git commit -m "feat: add current.json human-readable snapshot"
```

---

### Task 9.2: End-to-end integration test

**Files:**
- Create: `tests/integration_test.go`

- [ ] **Step 1: Write integration test**

Full lifecycle: init DB → create feature → create session → add notes → add facts (with contradiction) → sync commits → save plan → search → get context at all tiers → verify current.json.

- [ ] **Step 2: Run — PASS**
- [ ] **Step 3: Commit**

```bash
git commit -m "test: add end-to-end integration test"
```

---

### Task 9.3: README and install instructions

**Files:**
- Create: `README.md`

- [ ] **Step 1: Write README with:**

- What devmem is (one paragraph)
- Install: `go install github.com/arbaz/devmem/cmd/devmem@latest`
- Setup for Claude Code, Cursor, Windsurf (3 examples)
- Tool reference table
- Architecture diagram (ASCII)

- [ ] **Step 2: Commit**

```bash
git commit -m "docs: add README with install and setup instructions"
```

---

## Phase Summary

| Chunk | What | Tasks | Key Deliverable |
|-------|------|-------|-----------------|
| 1 | Foundation | 5 | Go binary that inits SQLite at git root |
| 2 | Memory Core | 6 | Features, sessions, facts, notes, links, context |
| 3 | Git Engine | 3 | Commit sync with intent classification |
| 4 | Search | 1 | 3-layer FTS5/trigram/fuzzy search with scoring |
| 5 | Plans | 2 | Plan CRUD, auto-detection, commit matching |
| 6 | MCP Server | 2 | All 9 tools + 2 resources, usable from Claude Code |
| 7 | Semantic Diffs | 3 | tree-sitter AST diffing for Go/TS/JS |
| 8 | Consolidation | 5 | Background engine: contradictions, decay, summarize, links |
| 9 | Polish | 3 | current.json, integration tests, README |

**Total: 30 tasks, ~150 steps**

After Chunk 6, devmem is fully usable. Chunks 7-8 add the SOTA differentiators. Chunk 9 polishes for release.
