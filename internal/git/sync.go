package git

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/arbaz/devmem/internal/storage"
	"github.com/google/uuid"
)

// SyncResult contains the results of a commit sync operation.
type SyncResult struct {
	NewCommits int
	Commits    []StoredCommit
}

// StoredCommit represents a commit that has been stored in the database.
type StoredCommit struct {
	ID               string
	Hash             string
	Message          string
	Author           string
	IntentType       string
	IntentConfidence float64
	FilesChanged     []FileChange
	CommittedAt      string
}

// SyncCommits reads new commits from the git repository and stores them in the database.
// It skips commits that have already been synced (by hash).
func SyncCommits(db *storage.DB, gitRoot, featureID, sessionID string, since time.Time) (*SyncResult, error) {
	commits, err := ReadCommits(gitRoot, since)
	if err != nil {
		return nil, fmt.Errorf("read commits: %w", err)
	}

	result := &SyncResult{}

	for _, c := range commits {
		// Check if commit already exists
		var existing string
		err := db.Reader().QueryRow("SELECT id FROM commits WHERE hash = ?", c.Hash).Scan(&existing)
		if err == nil {
			// Already synced, skip
			continue
		}
		if err != sql.ErrNoRows {
			return nil, fmt.Errorf("check existing commit %s: %w", c.Hash, err)
		}

		// Classify intent
		filePaths := make([]string, len(c.FilesChanged))
		for i, fc := range c.FilesChanged {
			filePaths[i] = fc.Path
		}
		intentType, intentConf := ClassifyIntent(c.Message, filePaths)

		// Marshal files changed
		filesJSON, err := json.Marshal(c.FilesChanged)
		if err != nil {
			return nil, fmt.Errorf("marshal files for %s: %w", c.Hash, err)
		}

		id := uuid.New().String()

		// Insert into commits table
		var sessionArg interface{}
		if sessionID != "" {
			sessionArg = sessionID
		}

		_, err = db.Writer().Exec(`
			INSERT INTO commits (id, feature_id, session_id, hash, message, author, files_changed, intent_type, intent_confidence, committed_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, featureID, sessionArg, c.Hash, c.Message, c.Author, string(filesJSON),
			intentType, intentConf, c.CommittedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("insert commit %s: %w", c.Hash, err)
		}

		// Get the rowid for FTS sync
		var rowid int64
		err = db.Writer().QueryRow("SELECT rowid FROM commits WHERE id = ?", id).Scan(&rowid)
		if err != nil {
			return nil, fmt.Errorf("get rowid for commit %s: %w", c.Hash, err)
		}

		// Sync to FTS
		_, err = db.Writer().Exec(`INSERT INTO commits_fts(rowid, message) VALUES (?, ?)`, rowid, c.Message)
		if err != nil {
			return nil, fmt.Errorf("insert commit FTS %s: %w", c.Hash, err)
		}

		stored := StoredCommit{
			ID:               id,
			Hash:             c.Hash,
			Message:          c.Message,
			Author:           c.Author,
			IntentType:       intentType,
			IntentConfidence: intentConf,
			FilesChanged:     c.FilesChanged,
			CommittedAt:      c.CommittedAt,
		}

		result.Commits = append(result.Commits, stored)
		result.NewCommits++
	}

	return result, nil
}
