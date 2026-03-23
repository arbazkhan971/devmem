package consolidation_test

import (
	"path/filepath"
	"testing"

	"github.com/arbaz/devmem/internal/consolidation"
	"github.com/arbaz/devmem/internal/storage"
)

// newTestDB creates a temp DB with migrations applied.
func newTestDB(t *testing.T) *storage.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := storage.NewDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := storage.Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return db
}

func TestNewEngine(t *testing.T) {
	db := newTestDB(t)
	cfg := consolidation.DefaultConfig()

	engine := consolidation.NewEngine(db, cfg)
	if engine == nil {
		t.Fatal("expected non-nil engine")
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := consolidation.DefaultConfig()

	if cfg.EntropyThreshold != 0.7 {
		t.Errorf("expected EntropyThreshold 0.7, got %f", cfg.EntropyThreshold)
	}
	if cfg.MaxUnsummarized != 20 {
		t.Errorf("expected MaxUnsummarized 20, got %d", cfg.MaxUnsummarized)
	}
	if cfg.MaxConflicts != 3 {
		t.Errorf("expected MaxConflicts 3, got %d", cfg.MaxConflicts)
	}
	if cfg.DecayHalfLifeDays != 14.0 {
		t.Errorf("expected DecayHalfLifeDays 14.0, got %f", cfg.DecayHalfLifeDays)
	}
}

func TestRunOnce_EmptyDB(t *testing.T) {
	db := newTestDB(t)
	engine := consolidation.NewEngine(db, consolidation.DefaultConfig())

	err := engine.RunOnce()
	if err != nil {
		t.Fatalf("RunOnce on empty DB: %v", err)
	}
}

func TestGetState_InitialState(t *testing.T) {
	db := newTestDB(t)
	engine := consolidation.NewEngine(db, consolidation.DefaultConfig())

	state, err := engine.GetState()
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}

	if state.EntropyScore != 0.0 {
		t.Errorf("expected initial entropy 0.0, got %f", state.EntropyScore)
	}
	if state.UnsummarizedCount != 0 {
		t.Errorf("expected initial unsummarized count 0, got %d", state.UnsummarizedCount)
	}
	if state.ConflictCount != 0 {
		t.Errorf("expected initial conflict count 0, got %d", state.ConflictCount)
	}
}

func TestGetState_AfterRunOnce(t *testing.T) {
	db := newTestDB(t)
	engine := consolidation.NewEngine(db, consolidation.DefaultConfig())

	err := engine.RunOnce()
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	state, err := engine.GetState()
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}

	if state.LastRunAt == "" {
		t.Error("expected non-empty LastRunAt after RunOnce")
	}
	if state.NextTriggerAt == "" {
		t.Error("expected non-empty NextTriggerAt after RunOnce")
	}
}

func TestStartStop(t *testing.T) {
	db := newTestDB(t)
	cfg := consolidation.DefaultConfig()
	cfg.IdleTimeout = 100 * 1000 * 1000 // very long to avoid firing during test

	engine := consolidation.NewEngine(db, cfg)

	engine.Start()
	// Starting again should be a no-op
	engine.Start()

	engine.Stop()
	// Stopping again should be a no-op
	engine.Stop()
}

func TestRunOnce_UpdatesConsolidationState(t *testing.T) {
	db := newTestDB(t)
	engine := consolidation.NewEngine(db, consolidation.DefaultConfig())

	// Get state before RunOnce
	stateBefore, err := engine.GetState()
	if err != nil {
		t.Fatalf("GetState before: %v", err)
	}
	if stateBefore.LastRunAt != "" {
		t.Errorf("expected empty LastRunAt before RunOnce, got %q", stateBefore.LastRunAt)
	}

	// Run consolidation
	if err := engine.RunOnce(); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	// Get state after RunOnce
	stateAfter, err := engine.GetState()
	if err != nil {
		t.Fatalf("GetState after: %v", err)
	}

	// last_run_at should be set
	if stateAfter.LastRunAt == "" {
		t.Error("expected non-empty LastRunAt after RunOnce")
	}

	// entropy_score should be a valid float (>= 0)
	if stateAfter.EntropyScore < 0 {
		t.Errorf("expected non-negative entropy score, got %f", stateAfter.EntropyScore)
	}

	// Verify direct DB read of entropy_score and last_run_at
	var dbEntropy float64
	var dbLastRun string
	err = db.Reader().QueryRow(
		`SELECT entropy_score, COALESCE(last_run_at, '') FROM consolidation_state WHERE id = 1`,
	).Scan(&dbEntropy, &dbLastRun)
	if err != nil {
		t.Fatalf("direct DB read: %v", err)
	}
	if dbLastRun == "" {
		t.Error("expected last_run_at to be set in DB")
	}
	if dbEntropy != stateAfter.EntropyScore {
		t.Errorf("DB entropy %f != state entropy %f", dbEntropy, stateAfter.EntropyScore)
	}
}

func TestApplyDecay_CountsStaleNotes(t *testing.T) {
	db := newTestDB(t)
	engine := consolidation.NewEngine(db, consolidation.DefaultConfig())

	featureID := "feat-decay-test"
	_, err := db.Writer().Exec(
		`INSERT INTO features (id, name, description) VALUES (?, ?, ?)`,
		featureID, "decay-test", "test",
	)
	if err != nil {
		t.Fatalf("create feature: %v", err)
	}

	// Insert a note older than 30 days with no outgoing links (stale)
	staleID := "stale-note-1"
	_, err = db.Writer().Exec(
		`INSERT INTO notes (id, feature_id, content, type, created_at, updated_at)
		 VALUES (?, ?, ?, ?, datetime('now', '-60 days'), datetime('now', '-60 days'))`,
		staleID, featureID, "old stale note", "note",
	)
	if err != nil {
		t.Fatalf("insert stale note: %v", err)
	}

	// Insert a recent note (not stale)
	recentID := "recent-note-1"
	_, err = db.Writer().Exec(
		`INSERT INTO notes (id, feature_id, content, type, created_at, updated_at)
		 VALUES (?, ?, ?, ?, datetime('now'), datetime('now'))`,
		recentID, featureID, "recent note", "note",
	)
	if err != nil {
		t.Fatalf("insert recent note: %v", err)
	}

	count, err := engine.ApplyDecay()
	if err != nil {
		t.Fatalf("ApplyDecay: %v", err)
	}
	// Should count only the stale note (older than 30 days, no outgoing links)
	if count != 1 {
		t.Errorf("expected 1 stale note, got %d", count)
	}

	// Now give the stale note an outgoing link — it should no longer count as stale
	linkID := "link-for-stale"
	_, err = db.Writer().Exec(
		`INSERT INTO memory_links (id, source_id, source_type, target_id, target_type, relationship, strength)
		 VALUES (?, ?, 'note', ?, 'note', 'related', 0.5)`,
		linkID, staleID, recentID,
	)
	if err != nil {
		t.Fatalf("create link: %v", err)
	}

	count2, err := engine.ApplyDecay()
	if err != nil {
		t.Fatalf("ApplyDecay second: %v", err)
	}
	if count2 != 0 {
		t.Errorf("expected 0 stale notes after linking, got %d", count2)
	}
}
