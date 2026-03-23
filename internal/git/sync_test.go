package git_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/arbazkhan971/memorx/internal/git"
	"github.com/arbazkhan971/memorx/internal/storage"
	"github.com/google/uuid"
)

// setupSyncTest creates a temp git repo with commits and a temp database with schema.
// Returns the git root, DB, and a feature ID.
func setupSyncTest(t *testing.T) (string, *storage.DB, string, string) {
	t.Helper()

	// Create git repo with commits
	gitRoot := initTestRepoWithCommits(t)

	// Create temp database
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	db, err := storage.NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// Run migrations
	if err := storage.Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// Create a feature
	featureID := uuid.New().String()
	_, err = db.Writer().Exec(
		"INSERT INTO features (id, name, description, status) VALUES (?, ?, ?, ?)",
		featureID, "test-feature", "A test feature", "active",
	)
	if err != nil {
		t.Fatalf("insert feature: %v", err)
	}

	// Create a session
	sessionID := uuid.New().String()
	_, err = db.Writer().Exec(
		"INSERT INTO sessions (id, feature_id, tool) VALUES (?, ?, ?)",
		sessionID, featureID, "test",
	)
	if err != nil {
		t.Fatalf("insert session: %v", err)
	}

	return gitRoot, db, featureID, sessionID
}

func TestSyncCommits_SyncsAllCommits(t *testing.T) {
	gitRoot, db, featureID, sessionID := setupSyncTest(t)

	since := time.Now().Add(-1 * time.Hour)
	result, err := git.SyncCommits(db, gitRoot, featureID, sessionID, since)
	if err != nil {
		t.Fatalf("SyncCommits: %v", err)
	}

	if result.NewCommits != 4 {
		t.Fatalf("expected 4 new commits, got %d", result.NewCommits)
	}
	if len(result.Commits) != 4 {
		t.Fatalf("expected 4 commits in result, got %d", len(result.Commits))
	}
}

func TestSyncCommits_StoresInDB(t *testing.T) {
	gitRoot, db, featureID, sessionID := setupSyncTest(t)

	since := time.Now().Add(-1 * time.Hour)
	_, err := git.SyncCommits(db, gitRoot, featureID, sessionID, since)
	if err != nil {
		t.Fatalf("SyncCommits: %v", err)
	}

	// Verify commits are in the database
	var count int
	err = db.Reader().QueryRow("SELECT COUNT(*) FROM commits WHERE feature_id = ?", featureID).Scan(&count)
	if err != nil {
		t.Fatalf("query commits: %v", err)
	}
	if count != 4 {
		t.Fatalf("expected 4 commits in DB, got %d", count)
	}
}

func TestSyncCommits_SkipsDuplicates(t *testing.T) {
	gitRoot, db, featureID, sessionID := setupSyncTest(t)

	since := time.Now().Add(-1 * time.Hour)

	// First sync
	result1, err := git.SyncCommits(db, gitRoot, featureID, sessionID, since)
	if err != nil {
		t.Fatalf("first SyncCommits: %v", err)
	}
	if result1.NewCommits != 4 {
		t.Fatalf("expected 4 new commits on first sync, got %d", result1.NewCommits)
	}

	// Second sync should skip all
	result2, err := git.SyncCommits(db, gitRoot, featureID, sessionID, since)
	if err != nil {
		t.Fatalf("second SyncCommits: %v", err)
	}
	if result2.NewCommits != 0 {
		t.Fatalf("expected 0 new commits on second sync, got %d", result2.NewCommits)
	}
}

func TestSyncCommits_ClassifiesIntent(t *testing.T) {
	gitRoot, db, featureID, sessionID := setupSyncTest(t)

	since := time.Now().Add(-1 * time.Hour)
	result, err := git.SyncCommits(db, gitRoot, featureID, sessionID, since)
	if err != nil {
		t.Fatalf("SyncCommits: %v", err)
	}

	// Check that intents were classified
	for _, c := range result.Commits {
		switch c.Message {
		case "feat: add main.go":
			if c.IntentType != "feature" {
				t.Errorf("expected feature for %q, got %s", c.Message, c.IntentType)
			}
			if c.IntentConfidence != 0.9 {
				t.Errorf("expected 0.9 confidence for %q, got %f", c.Message, c.IntentConfidence)
			}
		case "test: add initial tests":
			if c.IntentType != "test" {
				t.Errorf("expected test for %q, got %s", c.Message, c.IntentType)
			}
		case "fix: remove unused utils":
			if c.IntentType != "bugfix" {
				t.Errorf("expected bugfix for %q, got %s", c.Message, c.IntentType)
			}
		case "implement helper function":
			if c.IntentType != "feature" {
				t.Errorf("expected feature for %q, got %s", c.Message, c.IntentType)
			}
			if c.IntentConfidence != 0.8 {
				t.Errorf("expected 0.8 confidence for %q, got %f", c.Message, c.IntentConfidence)
			}
		}
	}
}

func TestSyncCommits_StoresFilesChangedJSON(t *testing.T) {
	gitRoot, db, featureID, sessionID := setupSyncTest(t)

	since := time.Now().Add(-1 * time.Hour)
	_, err := git.SyncCommits(db, gitRoot, featureID, sessionID, since)
	if err != nil {
		t.Fatalf("SyncCommits: %v", err)
	}

	// Query a commit and check files_changed JSON
	rows, err := db.Reader().Query("SELECT hash, message, files_changed FROM commits WHERE feature_id = ?", featureID)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()

	found := false
	for rows.Next() {
		var hash, message, filesJSON string
		if err := rows.Scan(&hash, &message, &filesJSON); err != nil {
			t.Fatalf("scan: %v", err)
		}

		if message == "feat: add main.go" {
			found = true
			var files []git.FileChange
			if err := json.Unmarshal([]byte(filesJSON), &files); err != nil {
				t.Fatalf("unmarshal files_changed: %v", err)
			}
			if len(files) != 1 {
				t.Fatalf("expected 1 file, got %d", len(files))
			}
			if files[0].Path != "main.go" {
				t.Errorf("expected main.go, got %s", files[0].Path)
			}
			if files[0].Action != "added" {
				t.Errorf("expected added, got %s", files[0].Action)
			}
		}
	}
	if !found {
		t.Fatal("commit 'feat: add main.go' not found in DB")
	}
}

func TestSyncCommits_PopulatesFTS(t *testing.T) {
	gitRoot, db, featureID, sessionID := setupSyncTest(t)

	since := time.Now().Add(-1 * time.Hour)
	_, err := git.SyncCommits(db, gitRoot, featureID, sessionID, since)
	if err != nil {
		t.Fatalf("SyncCommits: %v", err)
	}

	// Search FTS for "helper"
	var count int
	err = db.Reader().QueryRow(
		"SELECT COUNT(*) FROM commits_fts WHERE commits_fts MATCH ?", "helper",
	).Scan(&count)
	if err != nil {
		t.Fatalf("FTS query: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 FTS match for 'helper', got %d", count)
	}
}

func TestSyncCommits_EmptySessionID(t *testing.T) {
	gitRoot, db, featureID, _ := setupSyncTest(t)

	since := time.Now().Add(-1 * time.Hour)
	result, err := git.SyncCommits(db, gitRoot, featureID, "", since)
	if err != nil {
		t.Fatalf("SyncCommits with empty session: %v", err)
	}

	if result.NewCommits != 4 {
		t.Fatalf("expected 4 commits, got %d", result.NewCommits)
	}

	// Verify session_id is NULL in DB
	var sessionID *string
	err = db.Reader().QueryRow("SELECT session_id FROM commits LIMIT 1").Scan(&sessionID)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if sessionID != nil {
		t.Fatalf("expected NULL session_id, got %s", *sessionID)
	}
}

func TestSyncCommits_NewCommitsAfterSync(t *testing.T) {
	gitRoot, db, featureID, sessionID := setupSyncTest(t)

	since := time.Now().Add(-1 * time.Hour)

	// First sync
	result1, err := git.SyncCommits(db, gitRoot, featureID, sessionID, since)
	if err != nil {
		t.Fatalf("first sync: %v", err)
	}
	if result1.NewCommits != 4 {
		t.Fatalf("expected 4 commits, got %d", result1.NewCommits)
	}

	// Add a new commit
	env := append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_AUTHOR_NAME=Test User",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test User",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)

	os.WriteFile(filepath.Join(gitRoot, "new.go"), []byte("package main\n"), 0644)
	cmd := exec.Command("git", "add", "new.go")
	cmd.Dir = gitRoot
	cmd.Env = env
	cmd.Run()

	cmd = exec.Command("git", "commit", "-m", "docs: add new file")
	cmd.Dir = gitRoot
	cmd.Env = env
	cmd.Run()

	// Second sync should pick up the new commit
	result2, err := git.SyncCommits(db, gitRoot, featureID, sessionID, since)
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if result2.NewCommits != 1 {
		t.Fatalf("expected 1 new commit, got %d", result2.NewCommits)
	}
	if result2.Commits[0].Message != "docs: add new file" {
		t.Errorf("expected 'docs: add new file', got %s", result2.Commits[0].Message)
	}
}

func TestSyncCommits_CommitMatchesPlanStep(t *testing.T) {
	gitRoot, db, featureID, sessionID := setupSyncTest(t)

	// Create a plan with steps that match commit messages
	planID := uuid.New().String()
	_, err := db.Writer().Exec(
		"INSERT INTO plans (id, feature_id, title, content, status) VALUES (?, ?, ?, ?, ?)",
		planID, featureID, "Implementation Plan", "Plan for implementing features", "active",
	)
	if err != nil {
		t.Fatalf("insert plan: %v", err)
	}

	stepID := uuid.New().String()
	_, err = db.Writer().Exec(
		"INSERT INTO plan_steps (id, plan_id, step_number, title, description, status) VALUES (?, ?, ?, ?, ?, ?)",
		stepID, planID, 1, "Add main entry point", "Create main.go with the main function", "pending",
	)
	if err != nil {
		t.Fatalf("insert plan step: %v", err)
	}

	// Sync commits
	since := time.Now().Add(-1 * time.Hour)
	result, err := git.SyncCommits(db, gitRoot, featureID, sessionID, since)
	if err != nil {
		t.Fatalf("SyncCommits: %v", err)
	}

	if result.NewCommits != 4 {
		t.Fatalf("expected 4 new commits, got %d", result.NewCommits)
	}

	// Verify the commit "feat: add main.go" is stored and can be queried
	// alongside the plan step for the same feature
	var commitMsg string
	err = db.Reader().QueryRow(`
		SELECT c.message FROM commits c
		JOIN plan_steps ps ON ps.plan_id IN (SELECT id FROM plans WHERE feature_id = c.feature_id)
		WHERE c.message LIKE '%add main%'
		AND ps.title LIKE '%Add main%'
		LIMIT 1
	`).Scan(&commitMsg)
	if err != nil {
		t.Fatalf("query commit matching plan step: %v", err)
	}
	if commitMsg != "feat: add main.go" {
		t.Errorf("expected commit message 'feat: add main.go', got '%s'", commitMsg)
	}

	// Verify the synced commit has correct intent classification
	var intentType string
	err = db.Reader().QueryRow(
		"SELECT intent_type FROM commits WHERE message = ?", "feat: add main.go",
	).Scan(&intentType)
	if err != nil {
		t.Fatalf("query intent_type: %v", err)
	}
	if intentType != "feature" {
		t.Errorf("expected intent_type 'feature', got '%s'", intentType)
	}
}
