package memory

import (
	"database/sql"
	"fmt"
	"time"
)

type Context struct {
	Feature            *Feature
	Summary            string
	LastSessionSummary string
	Plan               *PlanInfo
	RecentCommits      []CommitInfo
	RecentNotes        []Note
	ActiveFacts        []Fact
	Links              []MemoryLink
	SessionHistory     []Session
	FilesTouched       []string
}

type PlanInfo struct {
	Title         string
	Status        string
	TotalSteps    int
	CompletedStep int
}

type CommitInfo struct {
	Hash, Message, Author, CommittedAt string
}

type tierCfg struct {
	commits, notes, sessions int
	facts, links, files      bool
}

var tiers = map[string]tierCfg{
	"compact":  {commits: 1},
	"standard": {commits: 5, notes: 3, facts: true},
	"detailed": {commits: 100, notes: 100, facts: true, sessions: 100, links: true, files: true},
}

func (s *Store) GetContext(featureID, tier string, asOf *time.Time) (*Context, error) {
	tc, ok := tiers[tier]
	if !ok {
		return nil, fmt.Errorf("unknown context tier: %q (valid: compact, standard, detailed)", tier)
	}
	r := s.db.Reader()
	feature, err := scanFeature(r.QueryRow("SELECT "+featureCols+" FROM features WHERE id = ?", featureID))
	if err != nil {
		return nil, fmt.Errorf("get feature for context: %w", err)
	}
	ctx := &Context{Feature: feature, Plan: s.loadPlanInfo(r, featureID)}

	var summary string
	if r.QueryRow(`SELECT content FROM summaries WHERE scope = ? ORDER BY generation DESC, created_at DESC LIMIT 1`, featureID).Scan(&summary) == nil {
		ctx.Summary = summary
	}
	var lastSess string
	if r.QueryRow(`SELECT COALESCE(summary, '') FROM sessions WHERE feature_id = ? AND ended_at IS NOT NULL AND summary != '' ORDER BY ended_at DESC LIMIT 1`, featureID).Scan(&lastSess) == nil && lastSess != "" {
		ctx.LastSessionSummary = lastSess
	}

	ctx.RecentCommits = s.loadRecentCommits(r, featureID, tc.commits)
	if tc.notes > 0 {
		ctx.RecentNotes, _ = s.ListNotes(featureID, "", tc.notes)
	}
	if tc.facts {
		if asOf != nil {
			ctx.ActiveFacts, _ = s.QueryFactsAsOf(featureID, *asOf)
		} else {
			ctx.ActiveFacts, _ = s.GetActiveFacts(featureID)
		}
	}
	if tc.sessions > 0 {
		ctx.SessionHistory, _ = s.ListSessions(featureID, tc.sessions)
	}
	if tc.links {
		ctx.Links = s.loadFeatureLinks(r, featureID)
	}
	if tc.files {
		ctx.FilesTouched = s.loadFilesTouched(r, featureID)
	}
	return ctx, nil
}

func scanRows[T any](r *sql.DB, query string, args []any, fn func(*sql.Rows) (T, error)) []T {
	rows, err := r.Query(query, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []T
	for rows.Next() {
		if v, err := fn(rows); err == nil {
			out = append(out, v)
		}
	}
	return out
}

func (s *Store) loadPlanInfo(r *sql.DB, featureID string) *PlanInfo {
	pi := &PlanInfo{}
	var planID string
	if r.QueryRow(`SELECT id, title, status FROM plans WHERE feature_id = ? AND status = 'active' ORDER BY created_at DESC LIMIT 1`, featureID).Scan(&planID, &pi.Title, &pi.Status) != nil {
		return nil
	}
	r.QueryRow(`SELECT COUNT(*) FROM plan_steps WHERE plan_id = ?`, planID).Scan(&pi.TotalSteps)
	r.QueryRow(`SELECT COUNT(*) FROM plan_steps WHERE plan_id = ? AND status = 'completed'`, planID).Scan(&pi.CompletedStep)
	return pi
}

func (s *Store) loadRecentCommits(r *sql.DB, featureID string, limit int) []CommitInfo {
	return scanRows(r,
		`SELECT hash, message, author, committed_at FROM commits WHERE feature_id = ? ORDER BY committed_at DESC LIMIT ?`,
		[]any{featureID, limit},
		func(rows *sql.Rows) (CommitInfo, error) {
			var c CommitInfo
			return c, rows.Scan(&c.Hash, &c.Message, &c.Author, &c.CommittedAt)
		})
}

func (s *Store) loadFeatureLinks(r *sql.DB, featureID string) []MemoryLink {
	return scanRows(r,
		`SELECT ml.id, ml.source_id, ml.source_type, ml.target_id, ml.target_type,
		        ml.relationship, ml.strength, ml.created_at
		 FROM memory_links ml
		 WHERE ml.source_id IN (
			SELECT id FROM notes WHERE feature_id = ?
			UNION SELECT id FROM facts WHERE feature_id = ?
			UNION SELECT id FROM commits WHERE feature_id = ?
		 ) ORDER BY ml.strength DESC, ml.created_at DESC LIMIT 50`,
		[]any{featureID, featureID, featureID},
		func(rows *sql.Rows) (MemoryLink, error) {
			var l MemoryLink
			return l, rows.Scan(&l.ID, &l.SourceID, &l.SourceType, &l.TargetID, &l.TargetType,
				&l.Relationship, &l.Strength, &l.CreatedAt)
		})
}

func (s *Store) loadFilesTouched(r *sql.DB, featureID string) []string {
	return scanRows(r,
		`SELECT DISTINCT file_path FROM semantic_changes sc
		 JOIN commits c ON sc.commit_hash = c.hash
		 WHERE c.feature_id = ? ORDER BY file_path`,
		[]any{featureID},
		func(rows *sql.Rows) (string, error) {
			var f string
			return f, rows.Scan(&f)
		})
}
