package bench

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/arbazkhan971/memorx/internal/memory"
	"github.com/arbazkhan971/memorx/internal/plans"
	"github.com/arbazkhan971/memorx/internal/search"
	"github.com/arbazkhan971/memorx/internal/storage"
)

type Scenario struct {
	ID, Ability, Description string
	Setup                    []Action
	Query                    Query
	ExpectedContains         []string `json:"expected_contains"`
	ExpectedNotContain       []string `json:"expected_not_contain"`
	ExpectedFacts            []Fact   `json:"expected_facts,omitempty"`
}

type Action struct {
	Tool   string                 `json:"tool"`
	Params map[string]interface{} `json:"params"`
}

type Query struct {
	Tool   string                 `json:"tool"`
	Params map[string]interface{} `json:"params"`
}

type Fact struct{ Subject, Predicate, Object string }

type Result struct {
	ScenarioID, Ability string
	Passed              bool
	Score               float64
	LatencyMs           int64
	ResponseText        string
	MatchedTerms        []string `json:"matched_terms"`
	MissedTerms         []string `json:"missed_terms"`
	FalsePositives      []string `json:"false_positives"`
	Error               string   `json:"error,omitempty"`
}

type Evaluator struct {
	store      *memory.Store
	search     *search.Engine
	plans      *plans.Manager
	db         *storage.DB
	gitRoot    string
	sessionID  string
	featureIDs map[string]string
}

func NewEvaluator(dbPath, gitRoot string) (*Evaluator, error) {
	db, err := storage.NewDB(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if err := storage.Migrate(db); err != nil {
		return nil, fmt.Errorf("migrate db: %w", err)
	}
	return &Evaluator{
		store: memory.NewStore(db), search: search.NewEngine(db),
		plans: plans.NewManager(db), db: db, gitRoot: gitRoot,
		featureIDs: make(map[string]string),
	}, nil
}

func (e *Evaluator) Close() error { return e.db.Close() }

func (e *Evaluator) RunAll(scenarios []Scenario) []Result {
	results := make([]Result, 0, len(scenarios))
	for _, s := range scenarios {
		results = append(results, e.RunScenario(s))
		_ = e.Reset()
	}
	return results
}

func (e *Evaluator) RunScenario(s Scenario) Result {
	r := Result{ScenarioID: s.ID, Ability: s.Ability}
	for i, action := range s.Setup {
		if err := e.execAction(action); err != nil {
			r.Error = fmt.Sprintf("setup action %d (%s): %v", i, action.Tool, err)
			return r
		}
	}
	start := time.Now()
	response, err := e.execQuery(s.Query)
	r.LatencyMs = time.Since(start).Milliseconds()
	if err != nil {
		r.Error = fmt.Sprintf("query (%s): %v", s.Query.Tool, err)
		if len(s.ExpectedContains) == 0 && len(s.ExpectedNotContain) == 0 {
			r.Score, r.Passed, r.ResponseText = 1.0, true, r.Error
		}
		return r
	}
	r.ResponseText = response
	responseLower := strings.ToLower(response)
	for _, term := range s.ExpectedContains {
		if strings.Contains(responseLower, strings.ToLower(term)) {
			r.MatchedTerms = append(r.MatchedTerms, term)
		} else {
			r.MissedTerms = append(r.MissedTerms, term)
		}
	}
	for _, term := range s.ExpectedNotContain {
		if strings.Contains(responseLower, strings.ToLower(term)) {
			r.FalsePositives = append(r.FalsePositives, term)
		}
	}
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

func (e *Evaluator) Reset() error {
	w := e.db.Writer()
	for _, t := range []string{
		"notes_fts", "notes_trigram", "commits_fts", "commits_trigram",
		"facts_fts", "plans_fts", "memory_links", "semantic_changes",
		"summaries", "plan_steps", "plans", "notes", "facts", "commits",
		"sessions", "features", "consolidation_state", "schema_version",
	} {
		if _, err := w.Exec("DROP TABLE IF EXISTS " + t); err != nil {
			return fmt.Errorf("drop table %s: %w", t, err)
		}
	}
	if err := storage.Migrate(e.db); err != nil {
		return fmt.Errorf("re-migrate: %w", err)
	}
	e.featureIDs = make(map[string]string)
	e.sessionID = ""
	return nil
}

func paramStr(p map[string]interface{}, key string) string {
	if s, ok := p[key].(string); ok {
		return s
	}
	if v, ok := p[key]; ok {
		return fmt.Sprintf("%v", v)
	}
	return ""
}

func paramStrSlice(p map[string]interface{}, key string) []string {
	switch val := p[key].(type) {
	case []interface{}:
		out := make([]string, 0, len(val))
		for _, item := range val {
			out = append(out, fmt.Sprintf("%v", item))
		}
		return out
	case []string:
		return val
	}
	return nil
}

func (e *Evaluator) execAction(a Action) error {
	actions := map[string]func(Action) error{
		"start_feature": func(a Action) error {
			f, err := e.store.StartFeature(paramStr(a.Params, "name"), paramStr(a.Params, "description"))
			if err != nil {
				return err
			}
			e.featureIDs[f.Name] = f.ID
			sess, err := e.store.CreateSession(f.ID, "benchmark")
			if err != nil {
				return fmt.Errorf("auto-create session: %w", err)
			}
			e.sessionID = sess.ID
			return nil
		},
		"remember": func(a Action) error {
			t := paramStr(a.Params, "type")
			if t == "" {
				t = "note"
			}
			_, err := e.store.CreateNote(e.resolveFeatureID(a.Params), e.sessionID, paramStr(a.Params, "content"), t)
			return err
		},
		"add_fact": func(a Action) error {
			_, err := e.store.CreateFact(e.resolveFeatureID(a.Params), e.sessionID,
				paramStr(a.Params, "subject"), paramStr(a.Params, "predicate"), paramStr(a.Params, "object"))
			return err
		},
		"save_plan": func(a Action) error {
			var steps []plans.StepInput
			if sl, ok := a.Params["steps"].([]interface{}); ok {
				for _, item := range sl {
					if m, ok := item.(map[string]interface{}); ok {
						steps = append(steps, plans.StepInput{Title: paramStr(m, "title"), Description: paramStr(m, "description")})
					}
				}
			}
			_, err := e.plans.CreatePlan(e.resolveFeatureID(a.Params), e.sessionID,
				paramStr(a.Params, "title"), paramStr(a.Params, "content"), "benchmark", steps)
			return err
		},
		"sync": func(Action) error { return nil },
		"end_session": func(Action) error {
			if e.sessionID == "" {
				return nil
			}
			err := e.store.EndSession(e.sessionID)
			e.sessionID = ""
			return err
		},
		"start_session": func(a Action) error {
			t := paramStr(a.Params, "tool")
			if t == "" {
				t = "benchmark"
			}
			sess, err := e.store.CreateSession(e.resolveFeatureID(a.Params), t)
			if err != nil {
				return err
			}
			e.sessionID = sess.ID
			return nil
		},
	}
	if fn, ok := actions[a.Tool]; ok {
		return fn(a)
	}
	return fmt.Errorf("unknown action tool: %q", a.Tool)
}

func (e *Evaluator) execQuery(q Query) (string, error) {
	queries := map[string]func(Query) (string, error){
		"get_context": func(q Query) (string, error) {
			tier := paramStr(q.Params, "tier")
			if tier == "" {
				tier = "standard"
			}
			ctx, err := e.store.GetContext(e.resolveFeatureID(q.Params), tier, nil)
			if err != nil {
				return "", err
			}
			return formatContext(ctx), nil
		},
		"search": func(q Query) (string, error) {
			scope := paramStr(q.Params, "scope")
			if scope == "" {
				scope = "all_features"
			}
			results, err := e.search.Search(paramStr(q.Params, "query"), scope,
				paramStrSlice(q.Params, "types"), e.resolveFeatureID(q.Params), 20)
			if err != nil {
				return "", err
			}
			return formatSearchResults(results), nil
		},
		"get_facts": func(q Query) (string, error) {
			facts, err := e.store.GetActiveFacts(e.resolveFeatureID(q.Params))
			if err != nil {
				return "", err
			}
			return formatFacts(facts), nil
		},
		"list_features": func(Query) (string, error) {
			features, err := e.store.ListFeatures("all")
			if err != nil {
				return "", err
			}
			return formatFeatures(features), nil
		},
	}
	if fn, ok := queries[q.Tool]; ok {
		return fn(q)
	}
	return "", fmt.Errorf("unknown query tool: %q", q.Tool)
}

func (e *Evaluator) resolveFeatureID(p map[string]interface{}) string {
	if id := paramStr(p, "feature_id"); id != "" {
		return id
	}
	if name := paramStr(p, "feature"); name != "" {
		if id, ok := e.featureIDs[name]; ok {
			return id
		}
	}
	for _, id := range e.featureIDs {
		return id
	}
	return ""
}

func formatContext(ctx *memory.Context) string {
	var b strings.Builder
	if f := ctx.Feature; f != nil {
		fmt.Fprintf(&b, "%s [%s]", f.Name, f.Status)
		if f.Description != "" {
			fmt.Fprintf(&b, " %s", f.Description)
		}
		b.WriteByte('\n')
	}
	if ctx.Summary != "" {
		fmt.Fprintf(&b, "Summary: %s\n", ctx.Summary)
	}
	if p := ctx.Plan; p != nil {
		fmt.Fprintf(&b, "Plan: %s %d/%d\n", p.Title, p.CompletedStep, p.TotalSteps)
	}
	appendSection := func(label string, items int, fn func()) {
		if items > 0 {
			b.WriteString(label)
			fn()
			b.WriteByte('\n')
		}
	}
	appendSection("Facts:", len(ctx.ActiveFacts), func() {
		for _, f := range ctx.ActiveFacts {
			fmt.Fprintf(&b, " %s %s %s;", f.Subject, f.Predicate, f.Object)
		}
	})
	appendSection("Notes:", len(ctx.RecentNotes), func() {
		for _, n := range ctx.RecentNotes {
			fmt.Fprintf(&b, " [%s] %s;", n.Type, n.Content)
		}
	})
	appendSection("Commits:", len(ctx.RecentCommits), func() {
		for _, c := range ctx.RecentCommits {
			h := c.Hash
			if len(h) > 7 {
				h = h[:7]
			}
			fmt.Fprintf(&b, " %s %s;", h, c.Message)
		}
	})
	appendSection("Sessions:", len(ctx.SessionHistory), func() {
		for _, s := range ctx.SessionHistory {
			fmt.Fprintf(&b, " %s %s;", s.Tool, s.StartedAt)
		}
	})
	return b.String()
}

func formatSearchResults(results []search.SearchResult) string {
	if len(results) == 0 {
		return "No results found."
	}
	var b strings.Builder
	for _, r := range results {
		fmt.Fprintf(&b, "[%s] %s (relevance=%.2f feature=%s)\n", r.Type, r.Content, r.Relevance, r.FeatureName)
	}
	return b.String()
}

func formatFacts(facts []memory.Fact) string {
	if len(facts) == 0 {
		return "No active facts."
	}
	var b strings.Builder
	for _, f := range facts {
		fmt.Fprintf(&b, "%s %s %s\n", f.Subject, f.Predicate, f.Object)
	}
	return b.String()
}

func formatFeatures(features []memory.Feature) string {
	if len(features) == 0 {
		return "No features found."
	}
	var b strings.Builder
	for _, f := range features {
		fmt.Fprintf(&b, "%s [%s] %s\n", f.Name, f.Status, f.Description)
	}
	return b.String()
}

func NewTempEvaluator() (*Evaluator, error) {
	tmpFile, err := os.CreateTemp("", "memorx-bench-*.db")
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
