package mcp

import (
	"fmt"
	"os"

	"github.com/arbazkhan971/memorx/internal/memory"
	"github.com/arbazkhan971/memorx/internal/plans"
	"github.com/arbazkhan971/memorx/internal/search"
	"github.com/arbazkhan971/memorx/internal/storage"
	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type DevMemServer struct {
	store            *memory.Store
	searchEngine     *search.Engine
	planManager      *plans.Manager
	db               *storage.DB
	gitRoot          string
	currentSessionID string
}

func NewServer(db *storage.DB, gitRoot string) *DevMemServer {
	return &DevMemServer{
		store:        memory.NewStore(db),
		searchEngine: search.NewEngine(db),
		planManager:  plans.NewManager(db),
		db:           db,
		gitRoot:      gitRoot,
	}
}

func (s *DevMemServer) Start() error {
	srv := server.NewMCPServer("memorx", "1.0.0")
	s.registerTools(srv)
	s.registerResources(srv)

	if feature, err := s.store.GetActiveFeature(); err == nil {
		if sess, err := s.store.CreateSession(feature.ID, "mcp"); err == nil {
			s.currentSessionID = sess.ID
		}

		ctxData, err := s.store.GetContext(feature.ID, "standard", nil)
		if err == nil {
			sessions, _ := s.store.ListSessions(feature.ID, 5)
			ctxData.SessionHistory = sessions
			briefing := formatBriefing(ctxData, feature)
			fmt.Fprintf(os.Stderr, "\n%s\n", briefing)
		}
	}

	fmt.Fprintf(os.Stderr, "memorx: MCP server starting (stdio)\n")
	return server.ServeStdio(srv)
}

func (s *DevMemServer) registerTools(srv *server.MCPServer) {
	srv.AddTools(
		server.ServerTool{
			Tool:    mcplib.NewTool("memorx_briefing", mcplib.WithDescription("Quick briefing: what you were working on, where you left off, and what to do next. Call this at the start of every conversation.")),
			Handler: s.handleBriefing,
		},
		server.ServerTool{
			Tool:    mcplib.NewTool("memorx_status", mcplib.WithDescription("Get project status: active feature, plan progress, session info")),
			Handler: s.handleStatus,
		},
		server.ServerTool{
			Tool: mcplib.NewTool("memorx_list_features",
				mcplib.WithDescription("List development features with their status and activity"),
				mcplib.WithString("status_filter", mcplib.Description("Filter by status: all, active, paused, done"), mcplib.Enum("all", "active", "paused", "done")),
			),
			Handler: s.handleListFeatures,
		},
		server.ServerTool{
			Tool: mcplib.NewTool("memorx_start_feature",
				mcplib.WithDescription("Start or resume a development feature. Creates a new feature or resumes an existing one. Auto-pauses any currently active feature."),
				mcplib.WithString("name", mcplib.Description("Name of the feature to start or resume"), mcplib.Required()),
				mcplib.WithString("description", mcplib.Description("Description of the feature (used when creating new)")),
			),
			Handler: s.handleStartFeature,
		},
		server.ServerTool{
			Tool: mcplib.NewTool("memorx_switch_feature",
				mcplib.WithDescription("Switch to a different feature. Ends the current session and starts a new one under the target feature."),
				mcplib.WithString("name", mcplib.Description("Name of the feature to switch to"), mcplib.Required()),
			),
			Handler: s.handleSwitchFeature,
		},
		server.ServerTool{
			Tool: mcplib.NewTool("memorx_get_context",
				mcplib.WithDescription("Get assembled context for the active feature at a specified tier: compact (summary + last commit), standard (+ notes + facts), detailed (+ session history + links)"),
				mcplib.WithString("tier", mcplib.Description("Context tier: compact, standard, or detailed"), mcplib.Enum("compact", "standard", "detailed")),
				mcplib.WithString("as_of", mcplib.Description("ISO datetime for temporal query (e.g. 2024-01-15T10:30:00Z)")),
			),
			Handler: s.handleGetContext,
		},
		server.ServerTool{
			Tool: mcplib.NewTool("memorx_sync",
				mcplib.WithDescription("Sync git commits into memory. Detects new commits, classifies intent, and matches against plan steps."),
				mcplib.WithString("since", mcplib.Description("ISO datetime to sync commits from (default: last 7 days)")),
			),
			Handler: s.handleSync,
		},
		server.ServerTool{
			Tool: mcplib.NewTool("memorx_remember",
				mcplib.WithDescription("Save a note, decision, or observation. Auto-links to related memories. If content looks like a plan (3+ numbered steps), auto-promotes to a plan."),
				mcplib.WithString("content", mcplib.Description("The content to remember"), mcplib.Required()),
				mcplib.WithString("type", mcplib.Description("Type of note: note, decision, observation, blocker"), mcplib.Enum("note", "decision", "observation", "blocker")),
			),
			Handler: s.handleRemember,
		},
		server.ServerTool{
			Tool: mcplib.NewTool("memorx_search",
				mcplib.WithDescription("Search across memory: notes, commits, facts, and plans. Uses FTS5 + trigram matching."),
				mcplib.WithString("query", mcplib.Description("Search query text"), mcplib.Required()),
				mcplib.WithString("scope", mcplib.Description("Search scope: current_feature or all_features"), mcplib.Enum("current_feature", "all_features")),
				mcplib.WithArray("types", mcplib.Description("Memory types to search: notes, commits, facts, plans"), mcplib.WithStringItems(mcplib.Enum("notes", "commits", "facts", "plans"))),
			),
			Handler: s.handleSearch,
		},
		server.ServerTool{
			Tool: mcplib.NewTool("memorx_history",
				mcplib.WithDescription("Search across all sessions, notes, decisions, and facts chronologically. Find what was discussed or decided at any point in the project's history."),
				mcplib.WithString("query", mcplib.Description("Search term"), mcplib.Required()),
				mcplib.WithNumber("days_back", mcplib.Description("How many days to search back (default 30)")),
				mcplib.WithArray("types", mcplib.Description("Filter by type: decisions, progress, blockers, facts"), mcplib.WithStringItems(mcplib.Enum("decisions", "progress", "blockers", "facts"))),
			),
			Handler: s.handleHistory,
		},
		server.ServerTool{
			Tool: mcplib.NewTool("memorx_save_plan",
				mcplib.WithDescription("Save a development plan with steps. Supersedes any existing active plan, carrying forward completed steps."),
				mcplib.WithString("title", mcplib.Description("Title of the plan"), mcplib.Required()),
				mcplib.WithString("content", mcplib.Description("Full plan content/description")),
				mcplib.WithArray("steps", mcplib.Description("Plan steps as objects with title and optional description"), mcplib.Required()),
			),
			Handler: s.handleSavePlan,
		},
		server.ServerTool{
			Tool: mcplib.NewTool("memorx_import_session",
				mcplib.WithDescription("Import context from the current conversation into memorX. Use this to capture what you know about the project, decisions made, current progress, and plans — especially at the start of using memorX to bootstrap memory from an existing session. The LLM should call this with a structured dump of everything relevant from the current conversation."),
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
			Tool: mcplib.NewTool("memorx_end_session",
				mcplib.WithDescription("End the current session with a summary of what was accomplished. Call this before closing the conversation to capture session context for next time."),
				mcplib.WithString("summary", mcplib.Description("Brief summary of what was done this session"), mcplib.Required()),
			),
			Handler: s.handleEndSession,
		},
		server.ServerTool{
			Tool: mcplib.NewTool("memorx_export",
				mcplib.WithDescription("Export a feature's complete memory as markdown. Useful for sharing context with teammates, backing up, or feeding to another tool."),
				mcplib.WithString("feature_name", mcplib.Description("Feature to export (default: active feature)")),
				mcplib.WithString("format", mcplib.Description("Export format: markdown or json"), mcplib.Enum("markdown", "json")),
			),
			Handler: s.handleExport,
		},
		server.ServerTool{
			Tool: mcplib.NewTool("memorx_health",
				mcplib.WithDescription("Check memory health: conflicts, stale data, orphan notes. Returns a health score and actionable suggestions."),
				mcplib.WithString("feature", mcplib.Description("Check health for a specific feature (default: all)")),
			),
			Handler: s.handleHealth,
		},
		server.ServerTool{
			Tool: mcplib.NewTool("memorx_forget",
				mcplib.WithDescription("Forget/archive stale memories. Use to clean up outdated notes, invalidated facts, or completed features."),
				mcplib.WithString("what", mcplib.Description("What to forget: stale_facts, stale_notes, completed_features, or a specific note/fact ID"), mcplib.Required()),
				mcplib.WithString("feature", mcplib.Description("Scope to a specific feature")),
			),
			Handler: s.handleForget,
		},
		server.ServerTool{
			Tool: mcplib.NewTool("memorx_analytics",
				mcplib.WithDescription("Get development analytics and insights: session counts, commit patterns, blocker frequency, feature health."),
				mcplib.WithString("feature", mcplib.Description("Specific feature name (default: project-wide analytics)")),
			),
			Handler: s.handleAnalytics,
		},
		server.ServerTool{
			Tool: mcplib.NewTool("memorx_track_files",
				mcplib.WithDescription("Record files modified in current session. Pass array of file paths."),
				mcplib.WithArray("files", mcplib.Description("Array of file paths that were modified"), mcplib.Required()),
				mcplib.WithString("action", mcplib.Description("Type of file change: modified, added, or deleted"), mcplib.Enum("modified", "added", "deleted")),
			),
			Handler: s.handleTrackFiles,
		},
		server.ServerTool{
			Tool: mcplib.NewTool("memorx_generate_rules",
				mcplib.WithDescription("Generate an AGENTS.md file from memory. Creates a universal rules file that every AI coding CLI reads."),
				mcplib.WithString("output", mcplib.Description("Output path (default: AGENTS.md at git root)")),
				mcplib.WithBoolean("dry_run", mcplib.Description("Preview without writing file")),
			),
			Handler: s.handleGenerateRules,
		},
		server.ServerTool{
			Tool: mcplib.NewTool("memorx_snapshot",
				mcplib.WithDescription("Save current conversation context before it gets compacted. Call this before /compact or when context is getting large, to preserve details that can be recovered later."),
				mcplib.WithString("content", mcplib.Description("Summary of current conversation state"), mcplib.Required()),
				mcplib.WithString("type", mcplib.Description("Snapshot type: pre_compaction, checkpoint, or milestone"), mcplib.Enum("pre_compaction", "checkpoint", "milestone")),
			),
			Handler: s.handleSnapshot,
		},
		server.ServerTool{
			Tool: mcplib.NewTool("memorx_recover",
				mcplib.WithDescription("Recover specific details that may have been lost to compaction. Searches through saved snapshots for relevant context."),
				mcplib.WithString("query", mcplib.Description("What detail to recover"), mcplib.Required()),
				mcplib.WithNumber("limit", mcplib.Description("Max matches to return (default 3)")),
			),
			Handler: s.handleRecover,
		},
		server.ServerTool{
			Tool: mcplib.NewTool("memorx_manage",
				mcplib.WithDescription("Browse, pin, unpin, or delete memories. Pinned memories always appear in context regardless of tier."),
				mcplib.WithString("action", mcplib.Description("Action: list, pin, unpin, delete"), mcplib.Required(), mcplib.Enum("list", "pin", "unpin", "delete")),
				mcplib.WithString("id", mcplib.Description("Memory ID for pin/unpin/delete")),
				mcplib.WithString("filter", mcplib.Description("Filter: notes, facts, pinned, all"), mcplib.Enum("notes", "facts", "pinned", "all")),
				mcplib.WithNumber("limit", mcplib.Description("Max results for list (default 20)")),
			),
			Handler: s.handleManage,
		},
		server.ServerTool{
			Tool: mcplib.NewTool("memorx_project_map",
				mcplib.WithDescription("Scan and cache the project structure. Shows languages, key files, directory layout. Cached between sessions."),
				mcplib.WithBoolean("rescan", mcplib.Description("Force re-scan even if cached")),
			),
			Handler: s.handleProjectMap,
		},
		server.ServerTool{
			Tool: mcplib.NewTool("memorx_related",
				mcplib.WithDescription("Find all related memories for a topic or file. Combines search + link traversal + file tracking. Returns grouped: related decisions, facts, files, and commits."),
				mcplib.WithString("topic", mcplib.Description("File path, module name, or search term"), mcplib.Required()),
				mcplib.WithNumber("depth", mcplib.Description("Link traversal depth (default 2)")),
			),
			Handler: s.handleRelated,
		},
		server.ServerTool{
			Tool: mcplib.NewTool("memorx_dependencies",
				mcplib.WithDescription("Track which files depend on each other based on commit co-occurrence. Files that often change together are likely dependencies."),
				mcplib.WithString("file", mcplib.Description("File path to check"), mcplib.Required()),
			),
			Handler: s.handleDependencies,
		},
		server.ServerTool{
			Tool: mcplib.NewTool("memorx_diff",
				mcplib.WithDescription("Show what changed in memory between sessions. Compact summary of new facts, invalidated facts, notes, commits, plan progress, links, and files."),
				mcplib.WithString("since", mcplib.Description("ISO date or datetime to diff from (default: last session end time). Examples: 2024-01-15, 2024-01-15T10:30:00Z")),
			),
			Handler: s.handleDiff,
		},
		server.ServerTool{
			Tool: mcplib.NewTool("memorx_onboard",
				mcplib.WithDescription("Generate comprehensive onboarding doc for new developers. Combines project map, decisions, facts, plans, and recent session summaries into one document."),
				mcplib.WithString("feature", mcplib.Description("Specific feature to scope onboarding to (default: all features)")),
			),
			Handler: s.handleOnboard,
		},
		server.ServerTool{
			Tool: mcplib.NewTool("memorx_changelog",
				mcplib.WithDescription("Auto-generate changelog from memory, grouped by feature and time. Shows commits with intent and decisions made."),
				mcplib.WithNumber("days", mcplib.Description("Period in days to cover (default: 7)")),
				mcplib.WithString("format", mcplib.Description("Output format: markdown or slack"), mcplib.Enum("markdown", "slack")),
			),
			Handler: s.handleChangelog,
		},
		server.ServerTool{
			Tool: mcplib.NewTool("memorx_share",
				mcplib.WithDescription("Import memory from a shared file (complement to memorx_export). Reads an exported JSON or markdown file and imports features, notes, facts, and plans."),
				mcplib.WithString("path", mcplib.Description("Path to exported JSON or markdown file"), mcplib.Required()),
			),
			Handler: s.handleShare,
		},
		server.ServerTool{
			Tool: mcplib.NewTool("memorx_review_context",
				mcplib.WithDescription("Enrich a diff/PR with decision history from memory. For each file, searches notes, facts, and commits related to it."),
				mcplib.WithArray("files", mcplib.Description("Array of file paths in the diff"), mcplib.Required()),
			),
			Handler: s.handleReviewContext,
		},
		server.ServerTool{
			Tool: mcplib.NewTool("memorx_review_risk",
				mcplib.WithDescription("Flag risky changes based on memory patterns. For each file: counts changes, checks for blockers, checks for recent refactors. Returns risk score per file."),
				mcplib.WithArray("files", mcplib.Description("Array of file paths to assess"), mcplib.Required()),
			),
			Handler: s.handleReviewRisk,
		},
		server.ServerTool{
			Tool: mcplib.NewTool("memorx_review_checklist",
				mcplib.WithDescription("Auto-generate a review checklist from memory. Uses active decisions, blockers, facts, and pending plan steps."),
				mcplib.WithString("feature", mcplib.Description("Feature name (default: active feature)")),
			),
			Handler: s.handleReviewChecklist,
		},
		server.ServerTool{
			Tool: mcplib.NewTool("memorx_focus_time",
				mcplib.WithDescription("Track time spent per feature from session start/end deltas. Shows hours and session counts per feature."),
				mcplib.WithString("feature", mcplib.Description("Filter to a specific feature")),
				mcplib.WithNumber("days", mcplib.Description("Number of days to look back (default 7)")),
			),
			Handler: s.handleFocusTime,
		},
		server.ServerTool{
			Tool: mcplib.NewTool("memorx_velocity",
				mcplib.WithDescription("Measure plan completion velocity. For features with active plans: calculate steps completed per day and estimate time remaining."),
				mcplib.WithString("feature", mcplib.Description("Filter to a specific feature")),
			),
			Handler: s.handleVelocity,
		},
		server.ServerTool{
			Tool: mcplib.NewTool("memorx_interruptions",
				mcplib.WithDescription("Track context switches between features. Shows switch count and longest uninterrupted focus stretch."),
				mcplib.WithNumber("days", mcplib.Description("Number of days to analyze (default 7)")),
			),
			Handler: s.handleInterruptions,
		},
		server.ServerTool{
			Tool: mcplib.NewTool("memorx_weekly_report",
				mcplib.WithDescription("Auto-generate weekly dev summary. Aggregates features touched, commits by type, decisions, blockers, and session time."),
				mcplib.WithNumber("days", mcplib.Description("Period in days to cover (default 7)")),
			),
			Handler: s.handleWeeklyReport,
		},
	)
}

func (s *DevMemServer) registerResources(srv *server.MCPServer) {
	srv.AddResources(
		server.ServerResource{
			Resource: mcplib.Resource{URI: "memorx://context/active", Name: "Active Feature Context", Description: "Compact context for the currently active feature", MIMEType: "text/plain"},
			Handler:  s.handleResourceActiveContext,
		},
		server.ServerResource{
			Resource: mcplib.Resource{URI: "memorx://changes/recent", Name: "Recent Changes", Description: "Git commits since the last session ended", MIMEType: "text/plain"},
			Handler:  s.handleResourceRecentChanges,
		},
	)
}
