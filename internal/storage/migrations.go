package storage

import (
	"database/sql"
	"fmt"
	"strings"
)

func Migrate(db *DB) error {
	w := db.Writer()
	currentVersion := 0
	var name string
	if err := w.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='schema_version'").Scan(&name); err == nil {
		w.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_version").Scan(&currentVersion)
	}
	if currentVersion < 1 {
		if err := applyV1(w); err != nil {
			return fmt.Errorf("apply v1 migration: %w", err)
		}
	}
	if currentVersion < 2 {
		if err := applyV2(w); err != nil {
			return fmt.Errorf("apply v2 migration: %w", err)
		}
	}
	if currentVersion < 3 {
		if err := applyV3(w); err != nil {
			return fmt.Errorf("apply v3 migration: %w", err)
		}
	}
	return nil
}

func applyV1(w *sql.DB) error {
	tx, err := w.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()
	if err := execStatements(tx, schemaV1, false); err != nil {
		return err
	}
	if err := execStatements(tx, ftsSchemaV1, true); err != nil {
		return err
	}
	if _, err := tx.Exec("INSERT OR IGNORE INTO schema_version (version) VALUES (1)"); err != nil {
		return fmt.Errorf("record version: %w", err)
	}
	return tx.Commit()
}

func applyV2(w *sql.DB) error {
	tx, err := w.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()
	_, err = tx.Exec(`ALTER TABLE sessions ADD COLUMN summary TEXT DEFAULT ''`)
	if err != nil && !strings.Contains(err.Error(), "duplicate column") {
		return fmt.Errorf("add summary column: %w", err)
	}
	if _, err := tx.Exec("INSERT OR IGNORE INTO schema_version (version) VALUES (2)"); err != nil {
		return fmt.Errorf("record version: %w", err)
	}
	return tx.Commit()
}

func applyV3(w *sql.DB) error {
	tx, err := w.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS context_snapshots (
			id TEXT PRIMARY KEY,
			feature_id TEXT NOT NULL,
			session_id TEXT,
			content TEXT NOT NULL,
			snapshot_type TEXT DEFAULT 'pre_compaction',
			created_at TEXT NOT NULL DEFAULT (datetime('now')))`,
		`CREATE INDEX IF NOT EXISTS idx_context_snapshots_feature ON context_snapshots(feature_id)`,
		`CREATE INDEX IF NOT EXISTS idx_context_snapshots_type ON context_snapshots(snapshot_type)`,
	}
	for _, stmt := range stmts {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("exec: %w", err)
		}
	}
	// FTS table for context_snapshots
	if _, err := tx.Exec(`CREATE VIRTUAL TABLE IF NOT EXISTS context_snapshots_fts USING fts5(content, snapshot_type, tokenize='porter unicode61')`); err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			return fmt.Errorf("create context_snapshots_fts: %w", err)
		}
	}
	if _, err := tx.Exec("INSERT OR IGNORE INTO schema_version (version) VALUES (3)"); err != nil {
		return fmt.Errorf("record version: %w", err)
	}
	return tx.Commit()
}

func execStatements(tx *sql.Tx, schema string, ignoreExists bool) error {
	for _, stmt := range strings.Split(schema, ";") {
		if stmt = strings.TrimSpace(stmt); stmt == "" {
			continue
		}
		if _, err := tx.Exec(stmt); err != nil {
			if ignoreExists && strings.Contains(err.Error(), "already exists") {
				continue
			}
			return fmt.Errorf("exec statement: %w\nstatement: %s", err, stmt)
		}
	}
	return nil
}
