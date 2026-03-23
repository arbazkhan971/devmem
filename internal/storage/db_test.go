package storage_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/arbazkhan971/memorx/internal/storage"
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

func TestNewDB_WriterAndReader(t *testing.T) {
	dir := t.TempDir()
	db, err := storage.NewDB(filepath.Join(dir, "memory.db"))
	if err != nil {
		t.Fatalf("NewDB failed: %v", err)
	}
	defer db.Close()

	if db.Writer() == nil {
		t.Fatal("writer is nil")
	}
	if db.Reader() == nil {
		t.Fatal("reader is nil")
	}
	if db.Path() == "" {
		t.Fatal("path is empty")
	}
}

func TestDB_Close_Idempotent(t *testing.T) {
	dir := t.TempDir()
	db, err := storage.NewDB(filepath.Join(dir, "memory.db"))
	if err != nil {
		t.Fatalf("NewDB failed: %v", err)
	}

	// First close should succeed
	err = db.Close()
	if err != nil {
		t.Fatalf("first Close failed: %v", err)
	}

	// Second close should not panic (it may return an error for already-closed DB,
	// but it should not panic)
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("second Close panicked: %v", r)
			}
		}()
		db.Close()
	}()
}

func TestNewDB_WALModePersistsAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "memory.db")

	db1, err := storage.NewDB(dbPath)
	if err != nil {
		t.Fatalf("first NewDB: %v", err)
	}
	db1.Close()

	db2, err := storage.NewDB(dbPath)
	if err != nil {
		t.Fatalf("second NewDB: %v", err)
	}
	defer db2.Close()

	var journalMode string
	err = db2.Reader().QueryRow("PRAGMA journal_mode").Scan(&journalMode)
	if err != nil {
		t.Fatalf("PRAGMA query: %v", err)
	}
	if journalMode != "wal" {
		t.Fatalf("expected WAL mode after reopen, got %s", journalMode)
	}
}

func TestNewDB_PathReturnsCorrectPath(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "subdir", "memory.db")

	db, err := storage.NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	if db.Path() != dbPath {
		t.Fatalf("expected path %s, got %s", dbPath, db.Path())
	}
}

func TestNewDB_ReopenExistingFile(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "reopen.db")

	db1, err := storage.NewDB(dbPath)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	storage.Migrate(db1)
	db1.Writer().Exec(`INSERT INTO features (id, name, status) VALUES ('f1', 'persist', 'active')`)
	db1.Close()

	db2, err := storage.NewDB(dbPath)
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	defer db2.Close()

	var name string
	err = db2.Reader().QueryRow(`SELECT name FROM features WHERE id = 'f1'`).Scan(&name)
	if err != nil {
		t.Fatalf("query after reopen: %v", err)
	}
	if name != "persist" {
		t.Errorf("expected 'persist', got %q", name)
	}
}

func TestNewDB_ForeignKeysEnabled(t *testing.T) {
	dir := t.TempDir()
	db, err := storage.NewDB(filepath.Join(dir, "memory.db"))
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	var fk int
	err = db.Writer().QueryRow("PRAGMA foreign_keys").Scan(&fk)
	if err != nil {
		t.Fatalf("PRAGMA foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Fatalf("expected foreign_keys=1, got %d", fk)
	}
}
