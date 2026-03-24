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
	if currentVersion < 4 {
		if err := applyV4(w); err != nil {
			return fmt.Errorf("apply v4 migration: %w", err)
		}
	}
	if currentVersion < 5 {
		if err := applyV5AIOptimization(w); err != nil {
			return fmt.Errorf("apply v5 migration: %w", err)
		}
	}
	return nil
}

func applyV5AIOptimization(w *sql.DB) error {
	tx, err := w.Begin()
	if err != nil { return fmt.Errorf("begin transaction: %w", err) }
	defer tx.Rollback()
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS prompt_memory (id TEXT PRIMARY KEY, feature_id TEXT, prompt TEXT NOT NULL, effectiveness TEXT DEFAULT 'unknown', outcome TEXT, created_at TEXT NOT NULL DEFAULT (datetime('now')))`,
		`CREATE TABLE IF NOT EXISTS token_usage (id TEXT PRIMARY KEY, session_id TEXT, tool_name TEXT NOT NULL, input_tokens INTEGER DEFAULT 0, output_tokens INTEGER DEFAULT 0, created_at TEXT NOT NULL DEFAULT (datetime('now')))`,
		`CREATE TABLE IF NOT EXISTS learnings (id TEXT PRIMARY KEY, feature_id TEXT, content TEXT NOT NULL, source_tool TEXT, created_at TEXT NOT NULL DEFAULT (datetime('now')))`,
	} {
		if _, err := tx.Exec(stmt); err != nil { return fmt.Errorf("exec: %w", err) }
	}
	if _, err := tx.Exec("INSERT OR IGNORE INTO schema_version (version) VALUES (5)"); err != nil {
		return fmt.Errorf("record version: %w", err)
	}
	return tx.Commit()
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

func applyV4(w *sql.DB) error {
	tx, err := w.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS files_touched (id TEXT PRIMARY KEY, feature_id TEXT NOT NULL, session_id TEXT, path TEXT NOT NULL, action TEXT DEFAULT 'modified', first_seen TEXT NOT NULL DEFAULT (datetime('now')), UNIQUE(feature_id, session_id, path))`,
		`CREATE INDEX IF NOT EXISTS idx_files_feature ON files_touched(feature_id)`,
		`CREATE INDEX IF NOT EXISTS idx_files_session ON files_touched(session_id)`,
		`CREATE TABLE IF NOT EXISTS project_map (id INTEGER PRIMARY KEY CHECK (id = 1), data TEXT NOT NULL DEFAULT '{}', scanned_at TEXT NOT NULL DEFAULT (datetime('now')))`,
	}
	for _, stmt := range stmts {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("exec: %w", err)
		}
	}
	// Add pinned column to notes and facts (idempotent)
	for _, alter := range []string{
		`ALTER TABLE notes ADD COLUMN pinned INTEGER DEFAULT 0`,
		`ALTER TABLE facts ADD COLUMN pinned INTEGER DEFAULT 0`,
	} {
		if _, err := tx.Exec(alter); err != nil && !strings.Contains(err.Error(), "duplicate column") {
			return fmt.Errorf("alter: %w", err)
		}
	}
	if _, err := tx.Exec("INSERT OR IGNORE INTO schema_version (version) VALUES (4)"); err != nil {
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
