package storage

import (
	"database/sql"
	"fmt"
	"strings"
)

// Migrate applies all pending schema migrations.
func Migrate(db *DB) error {
	w := db.Writer()

	currentVersion := 0
	row := w.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='schema_version'")
	var name string
	if err := row.Scan(&name); err == nil {
		row = w.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_version")
		row.Scan(&currentVersion)
	}

	if currentVersion < 1 {
		if err := applyV1(w); err != nil {
			return fmt.Errorf("apply v1 migration: %w", err)
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

// execStatements splits sql by ";" and executes each statement.
// If ignoreExists is true, "already exists" errors are silently skipped
// (needed for FTS5 virtual tables which don't support IF NOT EXISTS).
func execStatements(tx *sql.Tx, schema string, ignoreExists bool) error {
	for _, stmt := range strings.Split(schema, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
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
