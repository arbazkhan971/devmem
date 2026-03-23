package git

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/arbazkhan971/memorx/internal/storage"
	"github.com/google/uuid"
)

type SyncResult struct {
	NewCommits int
	Commits    []StoredCommit
}

type StoredCommit struct {
	ID, Hash, Message, Author, IntentType, CommittedAt string
	IntentConfidence                                   float64
	FilesChanged                                       []FileChange
}

func SyncCommits(db *storage.DB, gitRoot, featureID, sessionID string, since time.Time) (*SyncResult, error) {
	commits, err := ReadCommits(gitRoot, since)
	if err != nil {
		return nil, fmt.Errorf("read commits: %w", err)
	}
	result := &SyncResult{}
	sessArg := sql.NullString{String: sessionID, Valid: sessionID != ""}
	for _, c := range commits {
		var existing string
		if err := db.Reader().QueryRow("SELECT id FROM commits WHERE hash = ?", c.Hash).Scan(&existing); err == nil {
			continue
		} else if err != sql.ErrNoRows {
			return nil, fmt.Errorf("check existing commit %s: %w", c.Hash, err)
		}
		paths := make([]string, len(c.FilesChanged))
		for i, fc := range c.FilesChanged {
			paths[i] = fc.Path
		}
		intentType, intentConf := ClassifyIntent(c.Message, paths)
		filesJSON, err := json.Marshal(c.FilesChanged)
		if err != nil {
			return nil, fmt.Errorf("marshal files for %s: %w", c.Hash, err)
		}
		id := uuid.New().String()
		res, err := db.Writer().Exec(
			`INSERT INTO commits (id, feature_id, session_id, hash, message, author, files_changed, intent_type, intent_confidence, committed_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, featureID, sessArg, c.Hash, c.Message, c.Author, string(filesJSON), intentType, intentConf, c.CommittedAt)
		if err != nil {
			return nil, fmt.Errorf("insert commit %s: %w", c.Hash, err)
		}
		rowid, _ := res.LastInsertId()
		if _, err = db.Writer().Exec(`INSERT INTO commits_fts(rowid, message) VALUES (?, ?)`, rowid, c.Message); err != nil {
			return nil, fmt.Errorf("insert commit FTS %s: %w", c.Hash, err)
		}
		result.Commits = append(result.Commits, StoredCommit{
			ID: id, Hash: c.Hash, Message: c.Message, Author: c.Author,
			IntentType: intentType, IntentConfidence: intentConf,
			FilesChanged: c.FilesChanged, CommittedAt: c.CommittedAt,
		})
		result.NewCommits++
	}
	return result, nil
}
