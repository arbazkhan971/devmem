package consolidation_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/arbaz/devmem/internal/consolidation"
	"github.com/arbaz/devmem/internal/storage"
	"github.com/google/uuid"
)

// insertTestNote inserts a note directly into the DB and syncs to FTS.
func insertTestNote(t *testing.T, db *storage.DB, featureID, content, noteType string) string {
	t.Helper()
	id := uuid.New().String()
	now := time.Now().UTC().Format(time.DateTime)
	w := db.Writer()

	_, err := w.Exec(
		`INSERT INTO notes (id, feature_id, content, type, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		id, featureID, content, noteType, now, now,
	)
	if err != nil {
		t.Fatalf("insert note: %v", err)
	}

	// Sync to FTS
	var rowID int64
	err = w.QueryRow(`SELECT rowid FROM notes WHERE id = ?`, id).Scan(&rowID)
	if err != nil {
		t.Fatalf("get note rowid: %v", err)
	}
	_, err = w.Exec(
		`INSERT INTO notes_fts(rowid, content, type) VALUES (?, ?, ?)`,
		rowID, content, noteType,
	)
	if err != nil {
		t.Fatalf("sync note to fts: %v", err)
	}
	_, err = w.Exec(
		`INSERT INTO notes_trigram(rowid, content) VALUES (?, ?)`,
		rowID, content,
	)
	if err != nil {
		t.Fatalf("sync note to trigram: %v", err)
	}

	return id
}

func TestGenerateSummaries_NotEnoughNotes(t *testing.T) {
	db := newTestDB(t)
	engine := consolidation.NewEngine(db, consolidation.DefaultConfig())

	featureID := uuid.New().String()
	_, err := db.Writer().Exec(
		`INSERT INTO features (id, name, description) VALUES (?, ?, ?)`,
		featureID, "test-feat", "test",
	)
	if err != nil {
		t.Fatalf("create feature: %v", err)
	}

	// Create only 5 notes (below threshold of 20)
	for i := 0; i < 5; i++ {
		insertTestNote(t, db, featureID, fmt.Sprintf("note %d", i), "note")
	}

	count, err := engine.GenerateSummaries(featureID)
	if err != nil {
		t.Fatalf("GenerateSummaries: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 summaries (not enough notes), got %d", count)
	}
}

func TestGenerateSummaries_CreatesGen0(t *testing.T) {
	db := newTestDB(t)
	engine := consolidation.NewEngine(db, consolidation.DefaultConfig())

	featureID := uuid.New().String()
	_, err := db.Writer().Exec(
		`INSERT INTO features (id, name, description) VALUES (?, ?, ?)`,
		featureID, "test-feat-sum", "test",
	)
	if err != nil {
		t.Fatalf("create feature: %v", err)
	}

	// Create 25 notes (above threshold of 20)
	for i := 0; i < 25; i++ {
		noteType := "note"
		switch {
		case i < 3:
			noteType = "decision"
		case i < 6:
			noteType = "blocker"
		case i < 12:
			noteType = "progress"
		}
		insertTestNote(t, db, featureID, fmt.Sprintf("note content number %d for feature testing", i), noteType)
	}

	count, err := engine.GenerateSummaries(featureID)
	if err != nil {
		t.Fatalf("GenerateSummaries: %v", err)
	}
	if count < 1 {
		t.Errorf("expected at least 1 summary created, got %d", count)
	}

	// Verify a gen-0 summary was created with the correct scope
	var summaryCount int
	err = db.Reader().QueryRow(
		`SELECT COUNT(*) FROM summaries WHERE scope = ? AND generation = 0`,
		"feature:"+featureID,
	).Scan(&summaryCount)
	if err != nil {
		t.Fatalf("count summaries: %v", err)
	}
	if summaryCount < 1 {
		t.Errorf("expected at least 1 gen-0 summary, got %d", summaryCount)
	}

	// Verify covers_from and covers_to are set
	var coversFrom, coversTo string
	err = db.Reader().QueryRow(
		`SELECT covers_from, covers_to FROM summaries WHERE scope = ? AND generation = 0 LIMIT 1`,
		"feature:"+featureID,
	).Scan(&coversFrom, &coversTo)
	if err != nil {
		t.Fatalf("get summary date range: %v", err)
	}
	if coversFrom == "" {
		t.Error("expected non-empty covers_from")
	}
	if coversTo == "" {
		t.Error("expected non-empty covers_to")
	}
	if coversFrom > coversTo {
		t.Errorf("covers_from (%s) should be <= covers_to (%s)", coversFrom, coversTo)
	}
}

func TestGenerateSummaries_CreatesGen1WhenEnoughGen0(t *testing.T) {
	db := newTestDB(t)
	engine := consolidation.NewEngine(db, consolidation.DefaultConfig())

	featureID := uuid.New().String()
	_, err := db.Writer().Exec(
		`INSERT INTO features (id, name, description) VALUES (?, ?, ?)`,
		featureID, "test-feat-gen1", "test",
	)
	if err != nil {
		t.Fatalf("create feature: %v", err)
	}

	scope := "feature:" + featureID

	// Manually insert 5 gen-0 summaries (the threshold for gen-1 creation)
	for i := 0; i < 5; i++ {
		sumID := uuid.New().String()
		coversFrom := fmt.Sprintf("2025-01-%02d 00:00:00", i*5+1)
		coversTo := fmt.Sprintf("2025-01-%02d 23:59:59", i*5+5)
		_, err := db.Writer().Exec(
			`INSERT INTO summaries (id, scope, content, generation, token_count, covers_from, covers_to, created_at)
			 VALUES (?, ?, ?, 0, 100, ?, ?, datetime('now'))`,
			sumID, scope, fmt.Sprintf("Gen-0 summary batch %d content", i), coversFrom, coversTo,
		)
		if err != nil {
			t.Fatalf("insert gen-0 summary %d: %v", i, err)
		}
	}

	// Verify we have 5 gen-0 summaries
	var gen0Count int
	err = db.Reader().QueryRow(
		`SELECT COUNT(*) FROM summaries WHERE scope = ? AND generation = 0`, scope,
	).Scan(&gen0Count)
	if err != nil {
		t.Fatalf("count gen-0: %v", err)
	}
	if gen0Count != 5 {
		t.Fatalf("expected 5 gen-0 summaries, got %d", gen0Count)
	}

	// Run GenerateSummaries — should create a gen-1 summary
	count, err := engine.GenerateSummaries(featureID)
	if err != nil {
		t.Fatalf("GenerateSummaries: %v", err)
	}
	if count < 1 {
		t.Errorf("expected at least 1 summary created (gen-1), got %d", count)
	}

	// Verify a gen-1 summary was created
	var gen1Count int
	err = db.Reader().QueryRow(
		`SELECT COUNT(*) FROM summaries WHERE scope = ? AND generation = 1`, scope,
	).Scan(&gen1Count)
	if err != nil {
		t.Fatalf("count gen-1: %v", err)
	}
	if gen1Count != 1 {
		t.Errorf("expected 1 gen-1 summary, got %d", gen1Count)
	}

	// Verify gen-1 summary has correct covers_from and covers_to spanning all gen-0s
	var coversFrom, coversTo string
	err = db.Reader().QueryRow(
		`SELECT covers_from, covers_to FROM summaries WHERE scope = ? AND generation = 1 LIMIT 1`, scope,
	).Scan(&coversFrom, &coversTo)
	if err != nil {
		t.Fatalf("get gen-1 date range: %v", err)
	}
	if coversFrom == "" || coversTo == "" {
		t.Error("expected non-empty covers_from and covers_to on gen-1 summary")
	}
	if coversFrom > coversTo {
		t.Errorf("gen-1 covers_from (%s) > covers_to (%s)", coversFrom, coversTo)
	}
}

func TestGenerateSummaries_EmptyFeature(t *testing.T) {
	db := newTestDB(t)
	engine := consolidation.NewEngine(db, consolidation.DefaultConfig())

	featureID := uuid.New().String()
	_, err := db.Writer().Exec(
		`INSERT INTO features (id, name, description) VALUES (?, ?, ?)`,
		featureID, "empty-feat", "test",
	)
	if err != nil {
		t.Fatalf("create feature: %v", err)
	}

	count, err := engine.GenerateSummaries(featureID)
	if err != nil {
		t.Fatalf("GenerateSummaries: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 summaries for empty feature, got %d", count)
	}
}
