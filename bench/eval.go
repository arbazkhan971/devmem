package bench

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/arbaz/devmem/internal/memory"
	"github.com/arbaz/devmem/internal/plans"
	"github.com/arbaz/devmem/internal/search"
	"github.com/arbaz/devmem/internal/storage"
)

// Scenario defines a single test case for the benchmark evaluation.
type Scenario struct {
	ID                 string   `json:"id"`
	Ability            string   `json:"ability"`
	Description        string   `json:"description"`
	Setup              []Action `json:"setup"`
	Query              Query    `json:"query"`
	ExpectedContains   []string `json:"expected_contains"`
	ExpectedNotContain []string `json:"expected_not_contain"`
	ExpectedFacts      []Fact   `json:"expected_facts,omitempty"`
}

// Action represents a setup step that populates the devmem instance before querying.
type Action struct {
	Tool   string                 `json:"tool"`
	Params map[string]interface{} `json:"params"`
}

// Query represents the operation to execute and evaluate against expectations.
type Query struct {
	Tool   string                 `json:"tool"`
	Params map[string]interface{} `json:"params"`
}

// Fact is an expected subject-predicate-object triple for fact verification.
type Fact struct {
	Subject   string `json:"subject"`
	Predicate string `json:"predicate"`
	Object    string `json:"object"`
}

// Result holds the outcome of one scenario evaluation.
type Result struct {
	ScenarioID     string   `json:"scenario_id"`
	Ability        string   `json:"ability"`
	Passed         bool     `json:"passed"`
	Score          float64  `json:"score"`
	LatencyMs      int64    `json:"latency_ms"`
	ResponseText   string   `json:"response_text"`
	MatchedTerms   []string `json:"matched_terms"`
	MissedTerms    []string `json:"missed_terms"`
	FalsePositives []string `json:"false_positives"`
	Error          string   `json:"error,omitempty"`
}

// Evaluator runs scenarios against a devmem instance.
type Evaluator struct {
	store     *memory.Store
	search    *search.Engine
	plans     *plans.Manager
	db        *storage.DB
	gitRoot   string
	sessionID string

	// featureIDs tracks feature IDs created during setup, keyed by name.
	featureIDs map[string]string
}

// NewEvaluator creates a new Evaluator backed by a temporary SQLite database.
// The database is fully migrated and ready for use.
func NewEvaluator(dbPath, gitRoot string) (*Evaluator, error) {
	db, err := storage.NewDB(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if err := storage.Migrate(db); err != nil {
		return nil, fmt.Errorf("migrate db: %w", err)
	}

	return &Evaluator{
		store:      memory.NewStore(db),
		search:     search.NewEngine(db),
		plans:      plans.NewManager(db),
		db:         db,
		gitRoot:    gitRoot,
		featureIDs: make(map[string]string),
	}, nil
}

// Close releases the database resources.
func (e *Evaluator) Close() error {
	return e.db.Close()
}

// RunAll runs every scenario, resetting the database between each one.
// Returns one Result per scenario.
func (e *Evaluator) RunAll(scenarios []Scenario) []Result {
	results := make([]Result, 0, len(scenarios))
	for _, s := range scenarios {
		r := e.RunScenario(s)
		results = append(results, r)
		_ = e.Reset()
	}
	return results
}

// RunScenario executes a single scenario: setup, query, and evaluation.
func (e *Evaluator) RunScenario(s Scenario) Result {
	r := Result{
		ScenarioID: s.ID,
		Ability:    s.Ability,
	}

	// Execute setup actions.
	for i, action := range s.Setup {
		if err := e.execAction(action); err != nil {
			r.Error = fmt.Sprintf("setup action %d (%s): %v", i, action.Tool, err)
			return r
		}
	}

	// Execute the query and measure latency.
	start := time.Now()
	response, err := e.execQuery(s.Query)
	r.LatencyMs = time.Since(start).Milliseconds()

	if err != nil {
		r.Error = fmt.Sprintf("query (%s): %v", s.Query.Tool, err)
		return r
	}
	r.ResponseText = response

	// Evaluate: check expected_contains (case-insensitive).
	responseLower := strings.ToLower(response)
	for _, term := range s.ExpectedContains {
		if strings.Contains(responseLower, strings.ToLower(term)) {
			r.MatchedTerms = append(r.MatchedTerms, term)
		} else {
			r.MissedTerms = append(r.MissedTerms, term)
		}
	}

	// Evaluate: check expected_not_contain (case-insensitive).
	for _, term := range s.ExpectedNotContain {
		if strings.Contains(responseLower, strings.ToLower(term)) {
			r.FalsePositives = append(r.FalsePositives, term)
		}
	}

	// Calculate score.
	if len(s.ExpectedContains) > 0 {
		r.Score = float64(len(r.MatchedTerms)) / float64(len(s.ExpectedContains))
	} else {
		r.Score = 1.0
	}
	r.Score -= float64(len(r.FalsePositives)) * 0.1
	if r.Score < 0 {
		r.Score = 0
	}

	r.Passed = r.Score >= 0.8
	return r
}

// Reset drops and recreates all tables for a clean slate.
func (e *Evaluator) Reset() error {
	w := e.db.Writer()

	// Drop all tables (order matters for foreign keys).
	tables := []string{
		"notes_fts", "notes_trigram",
		"commits_fts", "commits_trigram",
		"facts_fts",
		"plans_fts",
		"memory_links",
		"semantic_changes",
		"summaries",
		"plan_steps",
		"plans",
		"notes",
		"facts",
		"commits",
		"sessions",
		"features",
		"consolidation_state",
		"schema_version",
	}
	for _, t := range tables {
		if _, err := w.Exec("DROP TABLE IF EXISTS " + t); err != nil {
			return fmt.Errorf("drop table %s: %w", t, err)
		}
	}

	// Re-migrate.
	if err := storage.Migrate(e.db); err != nil {
		return fmt.Errorf("re-migrate: %w", err)
	}

	// Reset internal state.
	e.featureIDs = make(map[string]string)
	e.sessionID = ""
	return nil
}

// paramStr extracts a string parameter, returning "" if absent.
func paramStr(params map[string]interface{}, key string) string {
	v, ok := params[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return fmt.Sprintf("%v", v)
	}
	return s
}

// paramStrSlice extracts a string-slice parameter from an interface slice.
func paramStrSlice(params map[string]interface{}, key string) []string {
	v, ok := params[key]
	if !ok {
		return nil
	}
	switch val := v.(type) {
	case []interface{}:
		out := make([]string, 0, len(val))
		for _, item := range val {
			out = append(out, fmt.Sprintf("%v", item))
		}
		return out
	case []string:
		return val
	default:
		return nil
	}
}

// execAction dispatches a setup action to the appropriate store/plans method.
func (e *Evaluator) execAction(a Action) error {
	switch a.Tool {
	case "start_feature":
		name := paramStr(a.Params, "name")
		desc := paramStr(a.Params, "description")
		f, err := e.store.StartFeature(name, desc)
		if err != nil {
			return err
		}
		e.featureIDs[name] = f.ID
		// Auto-create a session for the feature.
		sess, err := e.store.CreateSession(f.ID, "benchmark")
		if err != nil {
			return fmt.Errorf("auto-create session: %w", err)
		}
		e.sessionID = sess.ID
		return nil

	case "remember":
		featureID := e.resolveFeatureID(a.Params)
		content := paramStr(a.Params, "content")
		noteType := paramStr(a.Params, "type")
		if noteType == "" {
			noteType = "note"
		}
		_, err := e.store.CreateNote(featureID, e.sessionID, content, noteType)
		return err

	case "add_fact":
		featureID := e.resolveFeatureID(a.Params)
		subject := paramStr(a.Params, "subject")
		predicate := paramStr(a.Params, "predicate")
		object := paramStr(a.Params, "object")
		_, err := e.store.CreateFact(featureID, e.sessionID, subject, predicate, object)
		return err

	case "save_plan":
		featureID := e.resolveFeatureID(a.Params)
		title := paramStr(a.Params, "title")
		content := paramStr(a.Params, "content")
		var steps []plans.StepInput
		if raw, ok := a.Params["steps"]; ok {
			if sl, ok := raw.([]interface{}); ok {
				for _, item := range sl {
					if m, ok := item.(map[string]interface{}); ok {
						steps = append(steps, plans.StepInput{
							Title:       paramStr(m, "title"),
							Description: paramStr(m, "description"),
						})
					}
				}
			}
		}
		_, err := e.plans.CreatePlan(featureID, e.sessionID, title, content, "benchmark", steps)
		return err

	case "sync":
		// Skip in benchmark — no git repo available.
		return nil

	case "end_session":
		if e.sessionID == "" {
			return nil
		}
		err := e.store.EndSession(e.sessionID)
		e.sessionID = ""
		return err

	case "start_session":
		featureID := e.resolveFeatureID(a.Params)
		tool := paramStr(a.Params, "tool")
		if tool == "" {
			tool = "benchmark"
		}
		sess, err := e.store.CreateSession(featureID, tool)
		if err != nil {
			return err
		}
		e.sessionID = sess.ID
		return nil

	default:
		return fmt.Errorf("unknown action tool: %q", a.Tool)
	}
}

// execQuery dispatches a query tool and returns the formatted response string.
func (e *Evaluator) execQuery(q Query) (string, error) {
	switch q.Tool {
	case "get_context":
		featureID := e.resolveFeatureID(q.Params)
		tier := paramStr(q.Params, "tier")
		if tier == "" {
			tier = "standard"
		}
		ctx, err := e.store.GetContext(featureID, tier, nil)
		if err != nil {
			return "", err
		}
		return formatContext(ctx), nil

	case "search":
		query := paramStr(q.Params, "query")
		scope := paramStr(q.Params, "scope")
		if scope == "" {
			scope = "all_features"
		}
		types := paramStrSlice(q.Params, "types")
		featureID := e.resolveFeatureID(q.Params)
		results, err := e.search.Search(query, scope, types, featureID, 20)
		if err != nil {
			return "", err
		}
		return formatSearchResults(results), nil

	case "get_facts":
		featureID := e.resolveFeatureID(q.Params)
		facts, err := e.store.GetActiveFacts(featureID)
		if err != nil {
			return "", err
		}
		return formatFacts(facts), nil

	case "list_features":
		features, err := e.store.ListFeatures("all")
		if err != nil {
			return "", err
		}
		return formatFeatures(features), nil

	default:
		return "", fmt.Errorf("unknown query tool: %q", q.Tool)
	}
}

// resolveFeatureID looks up the feature ID from params, trying "feature_id"
// first, then falling back to looking up "feature" by name. If nothing is
// found, returns the first known feature ID.
func (e *Evaluator) resolveFeatureID(params map[string]interface{}) string {
	// Explicit feature_id.
	if id := paramStr(params, "feature_id"); id != "" {
		return id
	}
	// Lookup by name.
	if name := paramStr(params, "feature"); name != "" {
		if id, ok := e.featureIDs[name]; ok {
			return id
		}
	}
	// Fall back to first tracked feature.
	for _, id := range e.featureIDs {
		return id
	}
	return ""
}

// --- Formatting helpers ---

func formatContext(ctx *memory.Context) string {
	var b strings.Builder
	if ctx.Feature != nil {
		fmt.Fprintf(&b, "Feature: %s (%s)\n", ctx.Feature.Name, ctx.Feature.Status)
		if ctx.Feature.Description != "" {
			fmt.Fprintf(&b, "Description: %s\n", ctx.Feature.Description)
		}
	}
	if ctx.Summary != "" {
		fmt.Fprintf(&b, "Summary: %s\n", ctx.Summary)
	}
	if ctx.Plan != nil {
		fmt.Fprintf(&b, "Plan: %s [%s] (%d/%d steps)\n",
			ctx.Plan.Title, ctx.Plan.Status, ctx.Plan.CompletedStep, ctx.Plan.TotalSteps)
	}
	if len(ctx.ActiveFacts) > 0 {
		b.WriteString("Facts:\n")
		for _, f := range ctx.ActiveFacts {
			fmt.Fprintf(&b, "  - %s %s %s\n", f.Subject, f.Predicate, f.Object)
		}
	}
	if len(ctx.RecentNotes) > 0 {
		b.WriteString("Notes:\n")
		for _, n := range ctx.RecentNotes {
			fmt.Fprintf(&b, "  - [%s] %s\n", n.Type, n.Content)
		}
	}
	if len(ctx.RecentCommits) > 0 {
		b.WriteString("Commits:\n")
		for _, c := range ctx.RecentCommits {
			fmt.Fprintf(&b, "  - %s %s\n", c.Hash[:minLen(len(c.Hash), 7)], c.Message)
		}
	}
	if len(ctx.SessionHistory) > 0 {
		b.WriteString("Sessions:\n")
		for _, s := range ctx.SessionHistory {
			fmt.Fprintf(&b, "  - %s (%s)\n", s.Tool, s.StartedAt)
		}
	}
	return b.String()
}

func formatSearchResults(results []search.SearchResult) string {
	var b strings.Builder
	for _, r := range results {
		fmt.Fprintf(&b, "[%s] %s (relevance=%.2f feature=%s)\n", r.Type, r.Content, r.Relevance, r.FeatureName)
	}
	if b.Len() == 0 {
		return "No results found."
	}
	return b.String()
}

func formatFacts(facts []memory.Fact) string {
	var b strings.Builder
	for _, f := range facts {
		fmt.Fprintf(&b, "%s %s %s\n", f.Subject, f.Predicate, f.Object)
	}
	if b.Len() == 0 {
		return "No active facts."
	}
	return b.String()
}

func formatFeatures(features []memory.Feature) string {
	var b strings.Builder
	for _, f := range features {
		fmt.Fprintf(&b, "%s [%s] %s\n", f.Name, f.Status, f.Description)
	}
	if b.Len() == 0 {
		return "No features found."
	}
	return b.String()
}

func minLen(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// NewTempEvaluator creates an Evaluator with a temporary database file
// that is automatically cleaned up when Close is called.
func NewTempEvaluator() (*Evaluator, error) {
	tmpFile, err := os.CreateTemp("", "devmem-bench-*.db")
	if err != nil {
		return nil, fmt.Errorf("create temp db: %w", err)
	}
	dbPath := tmpFile.Name()
	tmpFile.Close()

	eval, err := NewEvaluator(dbPath, "")
	if err != nil {
		os.Remove(dbPath)
		return nil, err
	}
	return eval, nil
}
