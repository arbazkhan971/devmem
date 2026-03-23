package mcp

import (
	"fmt"
	"os"

	"github.com/arbaz/devmem/internal/memory"
	"github.com/arbaz/devmem/internal/plans"
	"github.com/arbaz/devmem/internal/search"
	"github.com/arbaz/devmem/internal/storage"
	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// DevMemServer wraps the MCP server with devmem-specific state.
type DevMemServer struct {
	store            *memory.Store
	searchEngine     *search.Engine
	planManager      *plans.Manager
	db               *storage.DB
	gitRoot          string
	currentSessionID string
}

// NewServer creates a new DevMemServer backed by the given database and git root.
func NewServer(db *storage.DB, gitRoot string) *DevMemServer {
	return &DevMemServer{
		store:        memory.NewStore(db),
		searchEngine: search.NewEngine(db),
		planManager:  plans.NewManager(db),
		db:           db,
		gitRoot:      gitRoot,
	}
}

// Start initializes the MCP server, registers tools and resources,
// creates a session under the active feature (if any), and starts
// serving via stdio transport.
func (s *DevMemServer) Start() error {
	srv := server.NewMCPServer("devmem", "1.0.0")
	s.registerTools(srv)
	s.registerResources(srv)

	if feature, err := s.store.GetActiveFeature(); err == nil {
		if sess, err := s.store.CreateSession(feature.ID, "mcp"); err == nil {
			s.currentSessionID = sess.ID
		}

		// Auto-inject context briefing to stderr so the LLM sees it on connect
		ctxData, err := s.store.GetContext(feature.ID, "standard", nil)
		if err == nil {
			// Load session history for the briefing
			sessions, _ := s.store.ListSessions(feature.ID, 5)
			ctxData.SessionHistory = sessions
			briefing := formatBriefing(ctxData, feature)
			fmt.Fprintf(os.Stderr, "\n%s\n", briefing)
		}
	}

	fmt.Fprintf(os.Stderr, "devmem: MCP server starting (stdio)\n")
	return server.ServeStdio(srv)
}

// registerTools registers all tool handlers on the MCP server.
func (s *DevMemServer) registerTools(srv *server.MCPServer) {
	srv.AddTools(
		server.ServerTool{
			Tool:    mcplib.NewTool("devmem_briefing", mcplib.WithDescription("Quick briefing: what you were working on, where you left off, and what to do next. Call this at the start of every conversation.")),
			Handler: s.handleBriefing,
		},
		server.ServerTool{
			Tool:    mcplib.NewTool("devmem_status", mcplib.WithDescription("Get project status: active feature, plan progress, session info")),
			Handler: s.handleStatus,
		},
		server.ServerTool{
			Tool: mcplib.NewTool("devmem_list_features",
				mcplib.WithDescription("List development features with their status and activity"),
				mcplib.WithString("status_filter", mcplib.Description("Filter by status: all, active, paused, done"), mcplib.Enum("all", "active", "paused", "done")),
			),
			Handler: s.handleListFeatures,
		},
		server.ServerTool{
			Tool: mcplib.NewTool("devmem_start_feature",
				mcplib.WithDescription("Start or resume a development feature. Creates a new feature or resumes an existing one. Auto-pauses any currently active feature."),
				mcplib.WithString("name", mcplib.Description("Name of the feature to start or resume"), mcplib.Required()),
				mcplib.WithString("description", mcplib.Description("Description of the feature (used when creating new)")),
			),
			Handler: s.handleStartFeature,
		},
		server.ServerTool{
			Tool: mcplib.NewTool("devmem_switch_feature",
				mcplib.WithDescription("Switch to a different feature. Ends the current session and starts a new one under the target feature."),
				mcplib.WithString("name", mcplib.Description("Name of the feature to switch to"), mcplib.Required()),
			),
			Handler: s.handleSwitchFeature,
		},
		server.ServerTool{
			Tool: mcplib.NewTool("devmem_get_context",
				mcplib.WithDescription("Get assembled context for the active feature at a specified tier: compact (summary + last commit), standard (+ notes + facts), detailed (+ session history + links)"),
				mcplib.WithString("tier", mcplib.Description("Context tier: compact, standard, or detailed"), mcplib.Enum("compact", "standard", "detailed")),
				mcplib.WithString("as_of", mcplib.Description("ISO datetime for temporal query (e.g. 2024-01-15T10:30:00Z)")),
			),
			Handler: s.handleGetContext,
		},
		server.ServerTool{
			Tool: mcplib.NewTool("devmem_sync",
				mcplib.WithDescription("Sync git commits into memory. Detects new commits, classifies intent, and matches against plan steps."),
				mcplib.WithString("since", mcplib.Description("ISO datetime to sync commits from (default: last 7 days)")),
			),
			Handler: s.handleSync,
		},
		server.ServerTool{
			Tool: mcplib.NewTool("devmem_remember",
				mcplib.WithDescription("Save a note, decision, or observation. Auto-links to related memories. If content looks like a plan (3+ numbered steps), auto-promotes to a plan."),
				mcplib.WithString("content", mcplib.Description("The content to remember"), mcplib.Required()),
				mcplib.WithString("type", mcplib.Description("Type of note: note, decision, observation, blocker"), mcplib.Enum("note", "decision", "observation", "blocker")),
			),
			Handler: s.handleRemember,
		},
		server.ServerTool{
			Tool: mcplib.NewTool("devmem_search",
				mcplib.WithDescription("Search across memory: notes, commits, facts, and plans. Uses FTS5 + trigram matching."),
				mcplib.WithString("query", mcplib.Description("Search query text"), mcplib.Required()),
				mcplib.WithString("scope", mcplib.Description("Search scope: current_feature or all_features"), mcplib.Enum("current_feature", "all_features")),
				mcplib.WithArray("types", mcplib.Description("Memory types to search: notes, commits, facts, plans"), mcplib.WithStringItems(mcplib.Enum("notes", "commits", "facts", "plans"))),
			),
			Handler: s.handleSearch,
		},
		server.ServerTool{
			Tool: mcplib.NewTool("devmem_save_plan",
				mcplib.WithDescription("Save a development plan with steps. Supersedes any existing active plan, carrying forward completed steps."),
				mcplib.WithString("title", mcplib.Description("Title of the plan"), mcplib.Required()),
				mcplib.WithString("content", mcplib.Description("Full plan content/description")),
				mcplib.WithArray("steps", mcplib.Description("Plan steps as objects with title and optional description"), mcplib.Required()),
			),
			Handler: s.handleSavePlan,
		},
		server.ServerTool{
			Tool: mcplib.NewTool("devmem_import_session",
				mcplib.WithDescription("Import context from the current conversation into devmem. Use this to capture what you know about the project, decisions made, current progress, and plans — especially at the start of using devmem to bootstrap memory from an existing session. The LLM should call this with a structured dump of everything relevant from the current conversation."),
				mcplib.WithString("feature_name", mcplib.Description("Feature name to import into (creates if doesn't exist)"), mcplib.Required()),
				mcplib.WithString("description", mcplib.Description("Feature description")),
				mcplib.WithArray("decisions", mcplib.Description("Key decisions made (array of strings)")),
				mcplib.WithArray("progress_notes", mcplib.Description("Progress updates (array of strings)")),
				mcplib.WithArray("blockers", mcplib.Description("Current blockers (array of strings)")),
				mcplib.WithArray("next_steps", mcplib.Description("Planned next steps (array of strings)")),
				mcplib.WithArray("facts", mcplib.Description("Key facts as objects with subject, predicate, object (e.g. {subject:'auth', predicate:'uses', object:'better-auth'})")),
				mcplib.WithArray("plan_steps", mcplib.Description("If there's an active plan, its steps as objects with title and status (pending/completed)")),
				mcplib.WithString("plan_title", mcplib.Description("Title of the active plan if one exists")),
			),
			Handler: s.handleImportSession,
		},
		server.ServerTool{
			Tool: mcplib.NewTool("devmem_end_session",
				mcplib.WithDescription("End the current session with a summary of what was accomplished. Call this before closing the conversation to capture session context for next time."),
				mcplib.WithString("summary", mcplib.Description("Brief summary of what was done this session"), mcplib.Required()),
			),
			Handler: s.handleEndSession,
		},
		server.ServerTool{
			Tool: mcplib.NewTool("devmem_export",
				mcplib.WithDescription("Export a feature's complete memory as markdown. Useful for sharing context with teammates, backing up, or feeding to another tool."),
				mcplib.WithString("feature_name", mcplib.Description("Feature to export (default: active feature)")),
				mcplib.WithString("format", mcplib.Description("Export format: markdown or json"), mcplib.Enum("markdown", "json")),
			),
			Handler: s.handleExport,
		},
		server.ServerTool{
			Tool: mcplib.NewTool("devmem_health",
				mcplib.WithDescription("Check memory health: conflicts, stale data, orphan notes. Returns a health score and actionable suggestions."),
				mcplib.WithString("feature", mcplib.Description("Check health for a specific feature (default: all)")),
			),
			Handler: s.handleHealth,
		},
		server.ServerTool{
			Tool: mcplib.NewTool("devmem_forget",
				mcplib.WithDescription("Forget/archive stale memories. Use to clean up outdated notes, invalidated facts, or completed features."),
				mcplib.WithString("what", mcplib.Description("What to forget: stale_facts, stale_notes, completed_features, or a specific note/fact ID"), mcplib.Required()),
				mcplib.WithString("feature", mcplib.Description("Scope to a specific feature")),
			),
			Handler: s.handleForget,
		},
		server.ServerTool{
			Tool: mcplib.NewTool("devmem_analytics",
				mcplib.WithDescription("Get development analytics and insights: session counts, commit patterns, blocker frequency, feature health."),
				mcplib.WithString("feature", mcplib.Description("Specific feature name (default: project-wide analytics)")),
			),
			Handler: s.handleAnalytics,
		},
		server.ServerTool{
			Tool: mcplib.NewTool("devmem_generate_rules",
				mcplib.WithDescription("Generate an AGENTS.md file from memory. Creates a universal rules file that every AI coding CLI reads."),
				mcplib.WithString("output", mcplib.Description("Output path (default: AGENTS.md at git root)")),
				mcplib.WithBoolean("dry_run", mcplib.Description("Preview without writing file")),
			),
			Handler: s.handleGenerateRules,
		},
	)
}

// registerResources registers MCP resources.
func (s *DevMemServer) registerResources(srv *server.MCPServer) {
	srv.AddResources(
		server.ServerResource{
			Resource: mcplib.Resource{URI: "devmem://context/active", Name: "Active Feature Context", Description: "Compact context for the currently active feature", MIMEType: "text/plain"},
			Handler:  s.handleResourceActiveContext,
		},
		server.ServerResource{
			Resource: mcplib.Resource{URI: "devmem://changes/recent", Name: "Recent Changes", Description: "Git commits since the last session ended", MIMEType: "text/plain"},
			Handler:  s.handleResourceRecentChanges,
		},
	)
}
