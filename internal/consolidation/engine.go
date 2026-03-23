package consolidation

import (
	"database/sql"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/arbazkhan971/memorx/internal/storage"
	"github.com/google/uuid"
)

const conflictGroupsSQL = `SELECT COUNT(*) FROM (SELECT subject, predicate FROM facts WHERE invalid_at IS NULL GROUP BY subject, predicate HAVING COUNT(*)>1)`

type ConsolidationState struct {
	LastRunAt, NextTriggerAt         string
	EntropyScore                     float64
	UnsummarizedCount, ConflictCount int
}

type Config struct {
	EntropyThreshold  float64
	IdleTimeout       time.Duration
	MaxUnsummarized   int
	MaxConflicts      int
	DecayHalfLifeDays float64
}

func DefaultConfig() Config {
	return Config{EntropyThreshold: 0.7, IdleTimeout: 5 * time.Minute, MaxUnsummarized: 20, MaxConflicts: 3, DecayHalfLifeDays: 14.0}
}

type Engine struct {
	db      *storage.DB
	cfg     Config
	mu      sync.Mutex
	stopCh  chan struct{}
	done    chan struct{}
	running bool
}

func NewEngine(db *storage.DB, cfg Config) *Engine { return &Engine{db: db, cfg: cfg} }

func (e *Engine) Start() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.running {
		return
	}
	e.stopCh = make(chan struct{})
	e.done = make(chan struct{})
	e.running = true
	go func() {
		defer close(e.done)
		ticker := time.NewTicker(e.cfg.IdleTimeout)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if entropy, err := e.calculateEntropy(); err == nil && entropy >= e.cfg.EntropyThreshold {
					_ = e.RunOnce()
				}
			case <-e.stopCh:
				return
			}
		}
	}()
}

func (e *Engine) Stop() {
	e.mu.Lock()
	if !e.running {
		e.mu.Unlock()
		return
	}
	close(e.stopCh)
	e.running = false
	e.mu.Unlock()
	<-e.done
}

func (e *Engine) RunOnce() error {
	invalidated, err := e.DetectContradictions()
	if err != nil {
		return fmt.Errorf("detect contradictions: %w", err)
	}
	if _, err = e.DiscoverLinks(); err != nil {
		return fmt.Errorf("discover links: %w", err)
	}
	featureIDs, err := e.getFeatureIDs()
	if err != nil {
		return fmt.Errorf("get feature IDs: %w", err)
	}
	for _, fid := range featureIDs {
		e.GenerateSummaries(fid) //nolint:errcheck
	}
	entropy, err := e.calculateEntropy()
	if err != nil {
		return fmt.Errorf("calculate entropy: %w", err)
	}
	unsummarized, err := e.queryCount(
		`SELECT COUNT(*) FROM notes n WHERE NOT EXISTS (
			SELECT 1 FROM summaries s WHERE s.scope='feature:'||n.feature_id AND s.covers_from<=n.created_at AND s.covers_to>=n.created_at)`)
	if err != nil {
		return fmt.Errorf("count unsummarized: %w", err)
	}
	now := time.Now().UTC().Format(time.DateTime)
	if _, err = e.db.Writer().Exec(
		`UPDATE consolidation_state SET last_run_at=?, entropy_score=?, unsummarized_count=?, conflict_count=?, next_trigger_at=? WHERE id=1`,
		now, entropy, unsummarized, invalidated, time.Now().UTC().Add(e.cfg.IdleTimeout).Format(time.DateTime),
	); err != nil {
		return fmt.Errorf("update consolidation state: %w", err)
	}
	return nil
}

func (e *Engine) GetState() (*ConsolidationState, error) {
	state := &ConsolidationState{}
	var lastRunAt, nextTriggerAt sql.NullString
	err := e.db.Reader().QueryRow(
		`SELECT COALESCE(last_run_at,''), entropy_score, unsummarized_count, conflict_count, COALESCE(next_trigger_at,'') FROM consolidation_state WHERE id=1`,
	).Scan(&lastRunAt, &state.EntropyScore, &state.UnsummarizedCount, &state.ConflictCount, &nextTriggerAt)
	if err != nil {
		return nil, fmt.Errorf("get consolidation state: %w", err)
	}
	state.LastRunAt = lastRunAt.String
	state.NextTriggerAt = nextTriggerAt.String
	return state, nil
}

func (e *Engine) calculateEntropy() (float64, error) {
	unsummarized, err := e.queryCount(
		`SELECT COUNT(*) FROM notes n WHERE NOT EXISTS (
			SELECT 1 FROM summaries s WHERE s.scope='feature:'||n.feature_id AND s.covers_from<=n.created_at AND s.covers_to>=n.created_at)`)
	if err != nil {
		return 0, err
	}
	conflicts, err := e.queryCount(conflictGroupsSQL)
	if err != nil {
		return 0, err
	}
	hours, err := e.hoursSinceLastRun()
	if err != nil {
		return 0, err
	}
	cap := func(v, max float64) float64 { return math.Min(v/max, 1.0) }
	return cap(float64(unsummarized), float64(e.cfg.MaxUnsummarized))*0.4 +
		cap(float64(conflicts), float64(e.cfg.MaxConflicts))*0.3 +
		cap(hours, 24.0)*0.3, nil
}

func (e *Engine) queryCount(sql string, args ...any) (int, error) {
	var count int
	if err := e.db.Reader().QueryRow(sql, args...).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (e *Engine) hoursSinceLastRun() (float64, error) {
	var lastRunAt sql.NullString
	if err := e.db.Reader().QueryRow(`SELECT last_run_at FROM consolidation_state WHERE id=1`).Scan(&lastRunAt); err != nil || !lastRunAt.Valid || lastRunAt.String == "" {
		return 24, nil
	}
	lastRun, err := time.Parse(time.DateTime, lastRunAt.String)
	if err != nil {
		return 24, nil
	}
	return time.Since(lastRun).Hours(), nil
}

func (e *Engine) getFeatureIDs() ([]string, error) {
	rows, err := e.db.Reader().Query(`SELECT id FROM features`)
	if err != nil {
		return nil, fmt.Errorf("query features: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan feature id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (e *Engine) ApplyDecay() (int, error) {
	return e.queryCount(
		`SELECT COUNT(*) FROM notes n WHERE n.created_at < datetime('now','-30 days')
		 AND NOT EXISTS (SELECT 1 FROM memory_links ml WHERE ml.source_id=n.id AND ml.source_type='note')`)
}

func (e *Engine) DetectContradictions() (int, error) {
	rows, err := e.db.Reader().Query(
		`SELECT subject, predicate FROM facts WHERE invalid_at IS NULL GROUP BY subject, predicate HAVING COUNT(*)>1`)
	if err != nil {
		return 0, fmt.Errorf("query conflicts: %w", err)
	}
	defer rows.Close()
	type conflictGroup struct{ subject, predicate string }
	var groups []conflictGroup
	for rows.Next() {
		var g conflictGroup
		if err := rows.Scan(&g.subject, &g.predicate); err != nil {
			return 0, fmt.Errorf("scan conflict group: %w", err)
		}
		groups = append(groups, g)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate conflict groups: %w", err)
	}
	totalInvalidated := 0
	now := time.Now().UTC().Format(time.DateTime)
	for _, g := range groups {
		factRows, err := e.db.Reader().Query(
			`SELECT id FROM facts WHERE subject=? AND predicate=? AND invalid_at IS NULL ORDER BY valid_at DESC`, g.subject, g.predicate)
		if err != nil {
			return totalInvalidated, fmt.Errorf("query facts for conflict: %w", err)
		}
		var factIDs []string
		first := true
		for factRows.Next() {
			var id string
			if err := factRows.Scan(&id); err != nil {
				factRows.Close()
				return totalInvalidated, fmt.Errorf("scan fact: %w", err)
			}
			if first {
				first = false
				continue
			}
			factIDs = append(factIDs, id)
		}
		factRows.Close()
		for _, id := range factIDs {
			if _, err := e.db.Writer().Exec(`UPDATE facts SET invalid_at=? WHERE id=?`, now, id); err != nil {
				return totalInvalidated, fmt.Errorf("invalidate fact %s: %w", id, err)
			}
			totalInvalidated++
		}
	}
	return totalInvalidated, nil
}

func (e *Engine) DiscoverLinks() (int, error) {
	rows, err := e.db.Reader().Query(
		`SELECT n.id, n.content FROM notes n LEFT JOIN memory_links ml ON ml.source_id=n.id AND ml.source_type='note' WHERE ml.id IS NULL`)
	if err != nil {
		return 0, fmt.Errorf("query unlinked notes: %w", err)
	}
	defer rows.Close()
	type unlinkedNote struct{ id, content string }
	var notes []unlinkedNote
	for rows.Next() {
		var n unlinkedNote
		if err := rows.Scan(&n.id, &n.content); err != nil {
			return 0, fmt.Errorf("scan unlinked note: %w", err)
		}
		notes = append(notes, n)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate unlinked notes: %w", err)
	}
	totalLinks := 0
	for _, note := range notes {
		query := buildFTSQuery(note.content)
		if query == "" {
			continue
		}
		matchRows, err := e.db.Reader().Query(
			`SELECT n.id, rank FROM notes_fts fts JOIN notes n ON n.rowid=fts.rowid WHERE notes_fts MATCH ? ORDER BY rank LIMIT 10`, query)
		if err != nil {
			continue
		}
		for matchRows.Next() {
			var targetID string
			var rank float64
			if err := matchRows.Scan(&targetID, &rank); err != nil {
				continue
			}
			if targetID == note.id {
				continue
			}
			strength := 0.5
			if rank < -2.0 {
				strength = 0.9
			} else if rank < -1.0 {
				strength = 0.7
			}
			if err := e.createLink(note.id, "note", targetID, "note", "related", strength); err == nil {
				totalLinks++
			}
		}
		matchRows.Close()
	}
	return totalLinks, nil
}

func buildFTSQuery(content string) string {
	if len(content) > 100 {
		content = content[:100]
	}
	words := strings.FieldsFunc(content, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	var terms []string
	seen := make(map[string]bool)
	for _, w := range words {
		w = strings.ToLower(w)
		if len(w) < 3 || seen[w] {
			continue
		}
		seen[w] = true
		terms = append(terms, w)
	}
	if len(terms) == 0 {
		return ""
	}
	if len(terms) > 10 {
		terms = terms[:10]
	}
	return strings.Join(terms, " OR ")
}

func (e *Engine) createLink(sourceID, sourceType, targetID, targetType, relationship string, strength float64) error {
	_, err := e.db.Writer().Exec(
		`INSERT OR IGNORE INTO memory_links (id, source_id, source_type, target_id, target_type, relationship, strength, created_at) VALUES (?,?,?,?,?,?,?,?)`,
		uuid.New().String(), sourceID, sourceType, targetID, targetType, relationship, strength, time.Now().UTC().Format(time.DateTime))
	return err
}
