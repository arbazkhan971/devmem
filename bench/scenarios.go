package bench

// AllScenarios returns the complete set of benchmark scenarios covering
// all devmem abilities: session continuity, search recall, fact tracking,
// plan awareness, and cross-feature linking.
func AllScenarios() []Scenario {
	return []Scenario{
		// --- Session Continuity ---
		{
			ID:          "sc-001",
			Ability:     "session-continuity",
			Description: "Recall notes from current session via get_context",
			Setup: []Action{
				{Tool: "start_feature", Params: map[string]interface{}{"name": "auth-service", "description": "OAuth2 login flow"}},
				{Tool: "remember", Params: map[string]interface{}{"content": "Decided to use PKCE flow for mobile clients", "type": "decision"}},
				{Tool: "remember", Params: map[string]interface{}{"content": "Token refresh endpoint needs rate limiting", "type": "note"}},
			},
			Query: Query{
				Tool:   "get_context",
				Params: map[string]interface{}{"feature": "auth-service"},
			},
			ExpectedContains: []string{"PKCE", "mobile", "token refresh", "rate limiting"},
		},
		{
			ID:          "sc-002",
			Ability:     "session-continuity",
			Description: "Notes survive across sessions for the same feature",
			Setup: []Action{
				{Tool: "start_feature", Params: map[string]interface{}{"name": "payments", "description": "Stripe integration"}},
				{Tool: "remember", Params: map[string]interface{}{"content": "Using Stripe webhooks for async payment confirmation", "type": "decision"}},
				{Tool: "end_session", Params: map[string]interface{}{}},
				{Tool: "start_session", Params: map[string]interface{}{"feature": "payments", "tool": "benchmark-2"}},
				{Tool: "remember", Params: map[string]interface{}{"content": "Added idempotency keys to prevent duplicate charges", "type": "progress"}},
			},
			Query: Query{
				Tool:   "get_context",
				Params: map[string]interface{}{"feature": "payments"},
			},
			ExpectedContains: []string{"Stripe", "webhooks", "idempotency"},
		},

		// --- Search Recall ---
		{
			ID:          "sr-001",
			Ability:     "search-recall",
			Description: "FTS search finds notes by keyword",
			Setup: []Action{
				{Tool: "start_feature", Params: map[string]interface{}{"name": "caching", "description": "Redis caching layer"}},
				{Tool: "remember", Params: map[string]interface{}{"content": "Cache invalidation uses pub/sub pattern with Redis Streams", "type": "decision"}},
				{Tool: "remember", Params: map[string]interface{}{"content": "TTL set to 15 minutes for user profile data", "type": "note"}},
			},
			Query: Query{
				Tool:   "search",
				Params: map[string]interface{}{"query": "cache invalidation strategy"},
			},
			ExpectedContains: []string{"pub/sub", "Redis"},
		},
		{
			ID:          "sr-002",
			Ability:     "search-recall",
			Description: "Search across features returns results from multiple features",
			Setup: []Action{
				{Tool: "start_feature", Params: map[string]interface{}{"name": "api-gateway", "description": "API gateway routing"}},
				{Tool: "remember", Params: map[string]interface{}{"content": "Rate limiter uses sliding window algorithm", "type": "decision"}},
				{Tool: "end_session", Params: map[string]interface{}{}},
				{Tool: "start_feature", Params: map[string]interface{}{"name": "billing", "description": "Usage-based billing"}},
				{Tool: "remember", Params: map[string]interface{}{"content": "Rate limit data feeds into billing usage counters", "type": "note"}},
			},
			Query: Query{
				Tool:   "search",
				Params: map[string]interface{}{"query": "rate limit"},
			},
			ExpectedContains: []string{"sliding window", "billing"},
		},

		// --- Fact Tracking ---
		{
			ID:          "ft-001",
			Ability:     "fact-tracking",
			Description: "Facts are stored and retrievable",
			Setup: []Action{
				{Tool: "start_feature", Params: map[string]interface{}{"name": "database", "description": "Database schema design"}},
				{Tool: "add_fact", Params: map[string]interface{}{"subject": "users_table", "predicate": "has_column", "object": "email VARCHAR(255) UNIQUE NOT NULL"}},
				{Tool: "add_fact", Params: map[string]interface{}{"subject": "users_table", "predicate": "has_index", "object": "idx_users_email ON email"}},
			},
			Query: Query{
				Tool:   "get_facts",
				Params: map[string]interface{}{"feature": "database"},
			},
			ExpectedContains: []string{"users_table", "email", "VARCHAR", "idx_users_email"},
		},
		{
			ID:          "ft-002",
			Ability:     "fact-tracking",
			Description: "Facts appear in context output",
			Setup: []Action{
				{Tool: "start_feature", Params: map[string]interface{}{"name": "config", "description": "Configuration management"}},
				{Tool: "add_fact", Params: map[string]interface{}{"subject": "deploy_target", "predicate": "is", "object": "Kubernetes 1.28 on GKE"}},
				{Tool: "add_fact", Params: map[string]interface{}{"subject": "min_replicas", "predicate": "equals", "object": "3"}},
			},
			Query: Query{
				Tool:   "get_context",
				Params: map[string]interface{}{"feature": "config"},
			},
			ExpectedContains: []string{"Kubernetes", "GKE", "min_replicas", "3"},
		},

		// --- Plan Awareness ---
		{
			ID:          "pa-001",
			Ability:     "plan-awareness",
			Description: "Plan with steps appears in context",
			Setup: []Action{
				{Tool: "start_feature", Params: map[string]interface{}{"name": "migration", "description": "Database migration from MySQL to Postgres"}},
				{Tool: "save_plan", Params: map[string]interface{}{
					"title":   "MySQL to Postgres Migration",
					"content": "Migrate all tables from MySQL 5.7 to PostgreSQL 15",
					"steps": []interface{}{
						map[string]interface{}{"title": "Schema conversion", "description": "Convert MySQL DDL to Postgres DDL"},
						map[string]interface{}{"title": "Data export", "description": "Export data using pgloader"},
						map[string]interface{}{"title": "Validation", "description": "Run integrity checks"},
					},
				}},
			},
			Query: Query{
				Tool:   "get_context",
				Params: map[string]interface{}{"feature": "migration"},
			},
			ExpectedContains: []string{"MySQL", "Postgres", "Migration"},
		},

		// --- Cross-Feature ---
		{
			ID:          "cf-001",
			Ability:     "cross-feature",
			Description: "list_features shows all created features",
			Setup: []Action{
				{Tool: "start_feature", Params: map[string]interface{}{"name": "notifications", "description": "Push notification service"}},
				{Tool: "end_session", Params: map[string]interface{}{}},
				{Tool: "start_feature", Params: map[string]interface{}{"name": "analytics", "description": "Event tracking pipeline"}},
			},
			Query: Query{
				Tool:   "list_features",
				Params: map[string]interface{}{},
			},
			ExpectedContains:   []string{"notifications", "analytics"},
			ExpectedNotContain: []string{},
		},
	}
}
