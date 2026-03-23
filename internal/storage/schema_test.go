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
		"schema_version",
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

	ftsTables := []string{"notes_fts", "commits_fts", "facts_fts", "plans_fts", "notes_trigram", "commits_trigram"}
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

func TestMigrate_ConsolidationStateInitialized(t *testing.T) {
	dir := t.TempDir()
	db, err := storage.NewDB(filepath.Join(dir, "memory.db"))
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	if err := storage.Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	var entropy float64
	err = db.Reader().QueryRow("SELECT entropy_score FROM consolidation_state WHERE id = 1").Scan(&entropy)
	if err != nil {
		t.Fatalf("consolidation_state not initialized: %v", err)
	}
	if entropy != 0.0 {
		t.Fatalf("expected entropy 0.0, got %f", entropy)
	}
}

func TestMigrate_PreExistingDataNotCorrupted(t *testing.T) {
	dir := t.TempDir()
	db, err := storage.NewDB(filepath.Join(dir, "memory.db"))
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	// First migration
	if err := storage.Migrate(db); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}

	// Insert some data
	_, err = db.Writer().Exec(
		`INSERT INTO features (id, name, description, status) VALUES ('f1', 'my-feature', 'A description', 'active')`,
	)
	if err != nil {
		t.Fatalf("insert feature: %v", err)
	}
	_, err = db.Writer().Exec(
		`INSERT INTO notes (id, feature_id, content, type) VALUES ('n1', 'f1', 'important note', 'decision')`,
	)
	if err != nil {
		t.Fatalf("insert note: %v", err)
	}
	_, err = db.Writer().Exec(
		`INSERT INTO facts (id, feature_id, subject, predicate, object) VALUES ('fact1', 'f1', 'api', 'uses', 'REST')`,
	)
	if err != nil {
		t.Fatalf("insert fact: %v", err)
	}

	// Run migration again (should be idempotent)
	if err := storage.Migrate(db); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}

	// Verify feature data is intact
	var featureName, featureDesc, featureStatus string
	err = db.Reader().QueryRow(
		`SELECT name, description, status FROM features WHERE id = 'f1'`,
	).Scan(&featureName, &featureDesc, &featureStatus)
	if err != nil {
		t.Fatalf("read feature after re-migrate: %v", err)
	}
	if featureName != "my-feature" {
		t.Errorf("feature name corrupted: expected 'my-feature', got %q", featureName)
	}
	if featureDesc != "A description" {
		t.Errorf("feature description corrupted: expected 'A description', got %q", featureDesc)
	}
	if featureStatus != "active" {
		t.Errorf("feature status corrupted: expected 'active', got %q", featureStatus)
	}

	// Verify note data is intact
	var noteContent, noteType string
	err = db.Reader().QueryRow(
		`SELECT content, type FROM notes WHERE id = 'n1'`,
	).Scan(&noteContent, &noteType)
	if err != nil {
		t.Fatalf("read note after re-migrate: %v", err)
	}
	if noteContent != "important note" {
		t.Errorf("note content corrupted: expected 'important note', got %q", noteContent)
	}
	if noteType != "decision" {
		t.Errorf("note type corrupted: expected 'decision', got %q", noteType)
	}

	// Verify fact data is intact
	var factSubject, factPredicate, factObject string
	err = db.Reader().QueryRow(
		`SELECT subject, predicate, object FROM facts WHERE id = 'fact1'`,
	).Scan(&factSubject, &factPredicate, &factObject)
	if err != nil {
		t.Fatalf("read fact after re-migrate: %v", err)
	}
	if factSubject != "api" || factPredicate != "uses" || factObject != "REST" {
		t.Errorf("fact corrupted: got %s/%s/%s", factSubject, factPredicate, factObject)
	}
}
