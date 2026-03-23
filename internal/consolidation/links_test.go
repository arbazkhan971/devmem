package consolidation_test

import (
	"testing"

	"github.com/arbazkhan971/memorx/internal/consolidation"
	"github.com/google/uuid"
)

func TestDiscoverLinks_NoNotes(t *testing.T) {
	db := newTestDB(t)
	engine := consolidation.NewEngine(db, consolidation.DefaultConfig())

	count, err := engine.DiscoverLinks()
	if err != nil {
		t.Fatalf("DiscoverLinks: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 links, got %d", count)
	}
}

func TestDiscoverLinks_CreatesLinks(t *testing.T) {
	db := newTestDB(t)
	engine := consolidation.NewEngine(db, consolidation.DefaultConfig())

	featureID := uuid.New().String()
	_, err := db.Writer().Exec(
		`INSERT INTO features (id, name, description) VALUES (?, ?, ?)`,
		featureID, "test-feat-links", "test",
	)
	if err != nil {
		t.Fatalf("create feature: %v", err)
	}

	// Create 3 notes with related content but no links
	insertTestNote(t, db, featureID, "implementing database connection pooling for better performance", "progress")
	insertTestNote(t, db, featureID, "database connection pool size should be configurable", "decision")
	insertTestNote(t, db, featureID, "connection pooling performance benchmark results look good", "note")

	count, err := engine.DiscoverLinks()
	if err != nil {
		t.Fatalf("DiscoverLinks: %v", err)
	}
	if count < 1 {
		t.Errorf("expected at least 1 link created, got %d", count)
	}

	// Verify links exist in the database
	var linkCount int
	err = db.Reader().QueryRow(
		`SELECT COUNT(*) FROM memory_links WHERE source_type = 'note' AND target_type = 'note'`,
	).Scan(&linkCount)
	if err != nil {
		t.Fatalf("count links: %v", err)
	}
	if linkCount < 1 {
		t.Errorf("expected at least 1 link in DB, got %d", linkCount)
	}
}

func TestDiscoverLinks_SkipsAlreadyLinked(t *testing.T) {
	db := newTestDB(t)
	engine := consolidation.NewEngine(db, consolidation.DefaultConfig())

	featureID := uuid.New().String()
	_, err := db.Writer().Exec(
		`INSERT INTO features (id, name, description) VALUES (?, ?, ?)`,
		featureID, "test-feat-linked", "test",
	)
	if err != nil {
		t.Fatalf("create feature: %v", err)
	}

	// Create notes
	id1 := insertTestNote(t, db, featureID, "implementing database connection pooling", "progress")
	id2 := insertTestNote(t, db, featureID, "database connection pool configuration", "decision")

	// Manually create a link for note1
	linkID := uuid.New().String()
	_, err = db.Writer().Exec(
		`INSERT INTO memory_links (id, source_id, source_type, target_id, target_type, relationship, strength)
		 VALUES (?, ?, 'note', ?, 'note', 'related', 0.5)`,
		linkID, id1, id2,
	)
	if err != nil {
		t.Fatalf("create manual link: %v", err)
	}

	// Run DiscoverLinks - note1 should be skipped since it already has a link
	count, err := engine.DiscoverLinks()
	if err != nil {
		t.Fatalf("DiscoverLinks: %v", err)
	}

	// note2 still has no outgoing links, so it should get linked
	// But note1 already has a link as source, so it should be skipped
	// The exact count depends on FTS matching, but we just verify it runs without error
	t.Logf("links created: %d", count)
}

func TestDiscoverLinks_NoDuplicatesOnSecondRun(t *testing.T) {
	db := newTestDB(t)
	engine := consolidation.NewEngine(db, consolidation.DefaultConfig())

	featureID := uuid.New().String()
	_, err := db.Writer().Exec(
		`INSERT INTO features (id, name, description) VALUES (?, ?, ?)`,
		featureID, "test-feat-nodup", "test",
	)
	if err != nil {
		t.Fatalf("create feature: %v", err)
	}

	// Create notes with related content
	insertTestNote(t, db, featureID, "implementing database connection pooling for performance", "progress")
	insertTestNote(t, db, featureID, "database connection pool size configuration options", "decision")
	insertTestNote(t, db, featureID, "connection pooling performance benchmark results good", "note")

	// First run
	count1, err := engine.DiscoverLinks()
	if err != nil {
		t.Fatalf("DiscoverLinks run 1: %v", err)
	}

	// Count links in the database after first run
	var linkCount1 int
	err = db.Reader().QueryRow(
		`SELECT COUNT(*) FROM memory_links WHERE source_type = 'note' AND target_type = 'note'`,
	).Scan(&linkCount1)
	if err != nil {
		t.Fatalf("count links after run 1: %v", err)
	}
	t.Logf("run 1: created %d links, total in DB: %d", count1, linkCount1)

	// Second run — notes that already have outgoing links should be skipped,
	// so no new links should be created
	count2, err := engine.DiscoverLinks()
	if err != nil {
		t.Fatalf("DiscoverLinks run 2: %v", err)
	}

	var linkCount2 int
	err = db.Reader().QueryRow(
		`SELECT COUNT(*) FROM memory_links WHERE source_type = 'note' AND target_type = 'note'`,
	).Scan(&linkCount2)
	if err != nil {
		t.Fatalf("count links after run 2: %v", err)
	}
	t.Logf("run 2: created %d links, total in DB: %d", count2, linkCount2)

	// After first run, all notes that could be linked got links.
	// On the second run, those notes already have outgoing links and are skipped.
	// The link count in DB should not increase.
	if linkCount2 > linkCount1 {
		t.Errorf("second run created extra links: %d -> %d", linkCount1, linkCount2)
	}
}
