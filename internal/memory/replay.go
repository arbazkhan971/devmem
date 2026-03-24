package memory

import (
	"database/sql"
	"fmt"
)

// SessionReplay holds the chronological replay of a session's events.
type SessionReplay struct {
	SessionID string
	Tool      string
	Feature   string
	StartedAt string
	EndedAt   string
	Events    []ReplayEvent
}

// ReplayEvent represents a single event that occurred during a session.
type ReplayEvent struct {
	Type      string // "note", "fact", "commit", "plan_step"
	Content   string
	Timestamp string
}

// ReplaySession replays a session's decisions step by step.
// If sessionID is empty, it replays the last completed session.
func (s *Store) ReplaySession(sessionID string) (*SessionReplay, error) {
	r := s.db.Reader()

	// If no session ID provided, find the last completed session
	if sessionID == "" {
		err := r.QueryRow(
			`SELECT id FROM sessions WHERE ended_at IS NOT NULL ORDER BY ended_at DESC LIMIT 1`,
		).Scan(&sessionID)
		if err != nil {
			return nil, fmt.Errorf("no completed sessions found")
		}
	}

	// Get session info
	var sess Session
	err := r.QueryRow(
		`SELECT `+sessionCols+` FROM sessions WHERE id = ?`, sessionID,
	).Scan(&sess.ID, &sess.FeatureID, &sess.Tool, &sess.StartedAt, &sess.EndedAt, &sess.Summary)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("session %q not found", sessionID)
	}
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}

	// Get feature name
	var featureName string
	r.QueryRow(`SELECT name FROM features WHERE id = ?`, sess.FeatureID).Scan(&featureName)

	replay := &SessionReplay{
		SessionID: sess.ID,
		Tool:      sess.Tool,
		Feature:   featureName,
		StartedAt: sess.StartedAt,
		EndedAt:   sess.EndedAt,
	}

	// Determine the time window for this session
	startTime := sess.StartedAt
	endTime := sess.EndedAt
	if endTime == "" {
		endTime = "9999-12-31 23:59:59" // open-ended session
	}

	// Get all notes created during this session
	noteRows, err := r.Query(
		`SELECT 'note' as type, '[' || type || '] ' || content, created_at FROM notes WHERE session_id = ? AND created_at >= ? AND created_at <= ? ORDER BY created_at`,
		sessionID, startTime, endTime,
	)
	if err == nil {
		replay.Events = append(replay.Events, scanReplayEvents(noteRows)...)
	}

	// Get all facts created during this session
	factRows, err := r.Query(
		`SELECT 'fact' as type, subject || ' ' || predicate || ' ' || object, recorded_at FROM facts WHERE session_id = ? AND recorded_at >= ? AND recorded_at <= ? ORDER BY recorded_at`,
		sessionID, startTime, endTime,
	)
	if err == nil {
		replay.Events = append(replay.Events, scanReplayEvents(factRows)...)
	}

	// Get all commits synced during this session
	commitRows, err := r.Query(
		`SELECT 'commit' as type, hash || ': ' || message, synced_at FROM commits WHERE session_id = ? AND synced_at >= ? AND synced_at <= ? ORDER BY synced_at`,
		sessionID, startTime, endTime,
	)
	if err == nil {
		replay.Events = append(replay.Events, scanReplayEvents(commitRows)...)
	}

	// Sort all events chronologically
	sortReplayEvents(replay.Events)

	return replay, nil
}

func scanReplayEvents(rows *sql.Rows) []ReplayEvent {
	defer rows.Close()
	var out []ReplayEvent
	for rows.Next() {
		var e ReplayEvent
		if rows.Scan(&e.Type, &e.Content, &e.Timestamp) == nil {
			out = append(out, e)
		}
	}
	return out
}

func sortReplayEvents(events []ReplayEvent) {
	// Simple insertion sort (usually a few dozen events max)
	for i := 1; i < len(events); i++ {
		for j := i; j > 0 && events[j].Timestamp < events[j-1].Timestamp; j-- {
			events[j], events[j-1] = events[j-1], events[j]
		}
	}
}

// FormatReplay produces a formatted output string for a SessionReplay.
func FormatReplay(r *SessionReplay) string {
	if r == nil {
		return "No session replay data."
	}

	ended := r.EndedAt
	if ended == "" {
		ended = "active"
	}

	header := fmt.Sprintf("# Session Replay: %s\n\n- Tool: %s\n- Feature: %s\n- Started: %s\n- Ended: %s\n- Events: %d\n\n",
		r.SessionID[:min(8, len(r.SessionID))], r.Tool, r.Feature, r.StartedAt, ended, len(r.Events))

	if len(r.Events) == 0 {
		return header + "No events recorded during this session."
	}

	result := header + "## Timeline\n\n"
	for _, e := range r.Events {
		content := e.Content
		if len(content) > 120 {
			content = content[:120] + "..."
		}
		result += fmt.Sprintf("- `%s` [%s] %s\n", e.Timestamp, e.Type, content)
	}

	return result
}

