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
