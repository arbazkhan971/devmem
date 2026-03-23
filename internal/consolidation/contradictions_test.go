package consolidation_test

import (
	"testing"
	"time"

	"github.com/arbazkhan971/memorx/internal/consolidation"
	"github.com/google/uuid"
)

func TestDetectContradictions_NoConflicts(t *testing.T) {
	db := newTestDB(t)
	engine := consolidation.NewEngine(db, consolidation.DefaultConfig())

	count, err := engine.DetectContradictions()
	if err != nil {
		t.Fatalf("DetectContradictions: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 invalidated, got %d", count)
	}
}

func TestDetectContradictions_ResolvesConflict(t *testing.T) {
	db := newTestDB(t)
	engine := consolidation.NewEngine(db, consolidation.DefaultConfig())

	// Create a feature first
	featureID := uuid.New().String()
	_, err := db.Writer().Exec(
		`INSERT INTO features (id, name, description) VALUES (?, ?, ?)`,
		featureID, "test-feat", "test",
	)
	if err != nil {
		t.Fatalf("create feature: %v", err)
	}

	// Insert two active facts with same subject+predicate but different objects
	// Older fact
	now := time.Now().UTC()
	oldTime := now.Add(-time.Hour).Format(time.DateTime)
	id1 := uuid.New().String()
	_, err = db.Writer().Exec(
		`INSERT INTO facts (id, feature_id, subject, predicate, object, valid_at, recorded_at, confidence)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 1.0)`,
		id1, featureID, "database", "uses", "PostgreSQL", oldTime, oldTime,
	)
	if err != nil {
		t.Fatalf("insert fact 1: %v", err)
	}

	// Newer fact (same subject+predicate, different object)
	newTime := now.Format(time.DateTime)
	id2 := uuid.New().String()
	_, err = db.Writer().Exec(
		`INSERT INTO facts (id, feature_id, subject, predicate, object, valid_at, recorded_at, confidence)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 1.0)`,
		id2, featureID, "database", "uses", "SQLite", newTime, newTime,
	)
	if err != nil {
		t.Fatalf("insert fact 2: %v", err)
	}

	// Run contradiction detection
	count, err := engine.DetectContradictions()
	if err != nil {
		t.Fatalf("DetectContradictions: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 invalidated, got %d", count)
	}

	// Verify only the newer fact remains active
	var activeCount int
	err = db.Reader().QueryRow(
		`SELECT COUNT(*) FROM facts WHERE subject = 'database' AND predicate = 'uses' AND invalid_at IS NULL`,
	).Scan(&activeCount)
	if err != nil {
		t.Fatalf("count active: %v", err)
	}
	if activeCount != 1 {
		t.Errorf("expected 1 active fact, got %d", activeCount)
	}

	// Verify the remaining active fact is the newer one
	var activeID string
	err = db.Reader().QueryRow(
		`SELECT id FROM facts WHERE subject = 'database' AND predicate = 'uses' AND invalid_at IS NULL`,
	).Scan(&activeID)
	if err != nil {
		t.Fatalf("get active fact: %v", err)
	}
	if activeID != id2 {
		t.Errorf("expected active fact ID %q (newer), got %q", id2, activeID)
	}

	// Verify the older fact has invalid_at set
	var invalidAt string
	err = db.Reader().QueryRow(
		`SELECT COALESCE(invalid_at, '') FROM facts WHERE id = ?`, id1,
	).Scan(&invalidAt)
	if err != nil {
		t.Fatalf("get old fact: %v", err)
	}
	if invalidAt == "" {
		t.Error("expected old fact to have invalid_at set")
	}
}

func TestDetectContradictions_ThreeConflictingFacts(t *testing.T) {
	db := newTestDB(t)
	engine := consolidation.NewEngine(db, consolidation.DefaultConfig())
	featureID := uuid.New().String()
	db.Writer().Exec(`INSERT INTO features (id, name, description) VALUES (?, ?, ?)`, featureID, "three-conflict", "test")
	now := time.Now().UTC()
	for i, obj := range []string{"A", "B", "C"} {
		id := uuid.New().String()
		ts := now.Add(time.Duration(i) * time.Minute).Format(time.DateTime)
		db.Writer().Exec(`INSERT INTO facts (id, feature_id, subject, predicate, object, valid_at, recorded_at, confidence) VALUES (?, ?, 'x', 'is', ?, ?, ?, 1.0)`, id, featureID, obj, ts, ts)
	}
	count, err := engine.DetectContradictions()
	if err != nil {
		t.Fatalf("DetectContradictions: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 invalidated (keep newest C), got %d", count)
	}
}

func TestDetectContradictions_MultipleGroups(t *testing.T) {
	db := newTestDB(t)
	engine := consolidation.NewEngine(db, consolidation.DefaultConfig())

	featureID := uuid.New().String()
	_, err := db.Writer().Exec(
		`INSERT INTO features (id, name, description) VALUES (?, ?, ?)`,
		featureID, "test-feat-multi", "test",
	)
	if err != nil {
		t.Fatalf("create feature: %v", err)
	}

	now := time.Now().UTC()

	// Conflict group 1: database uses
	for i, obj := range []string{"PostgreSQL", "SQLite", "MySQL"} {
		id := uuid.New().String()
		ts := now.Add(time.Duration(i) * time.Minute).Format(time.DateTime)
		_, err = db.Writer().Exec(
			`INSERT INTO facts (id, feature_id, subject, predicate, object, valid_at, recorded_at, confidence)
			 VALUES (?, ?, ?, ?, ?, ?, ?, 1.0)`,
			id, featureID, "database", "uses", obj, ts, ts,
		)
		if err != nil {
			t.Fatalf("insert fact: %v", err)
		}
	}

	// Conflict group 2: api framework
	for i, obj := range []string{"Gin", "Echo"} {
		id := uuid.New().String()
		ts := now.Add(time.Duration(i) * time.Minute).Format(time.DateTime)
		_, err = db.Writer().Exec(
			`INSERT INTO facts (id, feature_id, subject, predicate, object, valid_at, recorded_at, confidence)
			 VALUES (?, ?, ?, ?, ?, ?, ?, 1.0)`,
			id, featureID, "api", "framework", obj, ts, ts,
		)
		if err != nil {
			t.Fatalf("insert fact: %v", err)
		}
	}

	count, err := engine.DetectContradictions()
	if err != nil {
		t.Fatalf("DetectContradictions: %v", err)
	}
	// Group 1: 3 facts, 2 invalidated. Group 2: 2 facts, 1 invalidated. Total: 3
	if count != 3 {
		t.Errorf("expected 3 invalidated, got %d", count)
	}

	// Verify 2 active facts remain (one per group)
	var activeCount int
	err = db.Reader().QueryRow(
		`SELECT COUNT(*) FROM facts WHERE invalid_at IS NULL`,
	).Scan(&activeCount)
	if err != nil {
		t.Fatalf("count active: %v", err)
	}
	if activeCount != 2 {
		t.Errorf("expected 2 active facts, got %d", activeCount)
	}
}
