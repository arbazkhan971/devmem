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
	mcpServer := server.NewMCPServer("devmem", "1.0.0")

	// Register all 9 tools
	s.registerTools(mcpServer)

	// Register 2 resources
	s.registerResources(mcpServer)

	// Create a session under the active feature if one exists
	if feature, err := s.store.GetActiveFeature(); err == nil {
		sess, err := s.store.CreateSession(feature.ID, "mcp")
		if err == nil {
			s.currentSessionID = sess.ID
		}
	}

	fmt.Fprintf(os.Stderr, "devmem: MCP server starting (stdio)\n")

	return server.ServeStdio(mcpServer)
}

// registerTools registers all 9 tool handlers on the MCP server.
func (s *DevMemServer) registerTools(mcpServer *server.MCPServer) {
	mcpServer.AddTool(
		mcplib.NewTool("devmem_status",
			mcplib.WithDescription("Get project status: active feature, plan progress, session info"),
		),
		s.handleStatus,
	)

	mcpServer.AddTool(
		mcplib.NewTool("devmem_list_features",
			mcplib.WithDescription("List development features with their status and activity"),
			mcplib.WithString("status_filter",
				mcplib.Description("Filter by status: all, active, paused, done"),
				mcplib.Enum("all", "active", "paused", "done"),
			),
		),
		s.handleListFeatures,
	)

	mcpServer.AddTool(
		mcplib.NewTool("devmem_start_feature",
			mcplib.WithDescription("Start or resume a development feature. Creates a new feature or resumes an existing one. Auto-pauses any currently active feature."),
			mcplib.WithString("name",
				mcplib.Description("Name of the feature to start or resume"),
				mcplib.Required(),
			),
			mcplib.WithString("description",
				mcplib.Description("Description of the feature (used when creating new)"),
			),
		),
		s.handleStartFeature,
	)

	mcpServer.AddTool(
		mcplib.NewTool("devmem_switch_feature",
			mcplib.WithDescription("Switch to a different feature. Ends the current session and starts a new one under the target feature."),
			mcplib.WithString("name",
				mcplib.Description("Name of the feature to switch to"),
				mcplib.Required(),
			),
		),
		s.handleSwitchFeature,
	)

	mcpServer.AddTool(
		mcplib.NewTool("devmem_get_context",
			mcplib.WithDescription("Get assembled context for the active feature at a specified tier: compact (summary + last commit), standard (+ notes + facts), detailed (+ session history + links)"),
			mcplib.WithString("tier",
				mcplib.Description("Context tier: compact, standard, or detailed"),
				mcplib.Enum("compact", "standard", "detailed"),
			),
			mcplib.WithString("as_of",
				mcplib.Description("ISO datetime for temporal query (e.g. 2024-01-15T10:30:00Z)"),
			),
		),
		s.handleGetContext,
	)

	mcpServer.AddTool(
		mcplib.NewTool("devmem_sync",
			mcplib.WithDescription("Sync git commits into memory. Detects new commits, classifies intent, and matches against plan steps."),
			mcplib.WithString("since",
				mcplib.Description("ISO datetime to sync commits from (default: last 7 days)"),
			),
		),
		s.handleSync,
	)

	mcpServer.AddTool(
		mcplib.NewTool("devmem_remember",
			mcplib.WithDescription("Save a note, decision, or observation. Auto-links to related memories. If content looks like a plan (3+ numbered steps), auto-promotes to a plan."),
			mcplib.WithString("content",
				mcplib.Description("The content to remember"),
				mcplib.Required(),
			),
			mcplib.WithString("type",
				mcplib.Description("Type of note: note, decision, observation, blocker"),
				mcplib.Enum("note", "decision", "observation", "blocker"),
			),
		),
		s.handleRemember,
	)

	mcpServer.AddTool(
		mcplib.NewTool("devmem_search",
			mcplib.WithDescription("Search across memory: notes, commits, facts, and plans. Uses FTS5 + trigram matching."),
			mcplib.WithString("query",
				mcplib.Description("Search query text"),
				mcplib.Required(),
			),
			mcplib.WithString("scope",
				mcplib.Description("Search scope: current_feature or all_features"),
				mcplib.Enum("current_feature", "all_features"),
			),
			mcplib.WithArray("types",
				mcplib.Description("Memory types to search: notes, commits, facts, plans"),
				mcplib.WithStringItems(
					mcplib.Enum("notes", "commits", "facts", "plans"),
				),
			),
		),
		s.handleSearch,
	)

	mcpServer.AddTool(
		mcplib.NewTool("devmem_save_plan",
			mcplib.WithDescription("Save a development plan with steps. Supersedes any existing active plan, carrying forward completed steps."),
			mcplib.WithString("title",
				mcplib.Description("Title of the plan"),
				mcplib.Required(),
			),
			mcplib.WithString("content",
				mcplib.Description("Full plan content/description"),
			),
			mcplib.WithArray("steps",
				mcplib.Description("Plan steps as objects with title and optional description"),
				mcplib.Required(),
			),
		),
		s.handleSavePlan,
	)

	mcpServer.AddTool(
		mcplib.NewTool("devmem_import_session",
			mcplib.WithDescription("Import context from the current conversation into devmem. Use this to capture what you know about the project, decisions made, current progress, and plans — especially at the start of using devmem to bootstrap memory from an existing session. The LLM should call this with a structured dump of everything relevant from the current conversation."),
			mcplib.WithString("feature_name",
				mcplib.Description("Feature name to import into (creates if doesn't exist)"),
				mcplib.Required(),
			),
			mcplib.WithString("description",
				mcplib.Description("Feature description"),
			),
			mcplib.WithArray("decisions",
				mcplib.Description("Key decisions made (array of strings)"),
			),
			mcplib.WithArray("progress_notes",
				mcplib.Description("Progress updates (array of strings)"),
			),
			mcplib.WithArray("blockers",
				mcplib.Description("Current blockers (array of strings)"),
			),
			mcplib.WithArray("next_steps",
				mcplib.Description("Planned next steps (array of strings)"),
			),
			mcplib.WithArray("facts",
				mcplib.Description("Key facts as objects with subject, predicate, object (e.g. {subject:'auth', predicate:'uses', object:'better-auth'})"),
			),
			mcplib.WithArray("plan_steps",
				mcplib.Description("If there's an active plan, its steps as objects with title and status (pending/completed)"),
			),
			mcplib.WithString("plan_title",
				mcplib.Description("Title of the active plan if one exists"),
			),
		),
		s.handleImportSession,
	)

	mcpServer.AddTool(
		mcplib.NewTool("devmem_export",
			mcplib.WithDescription("Export a feature's complete memory as markdown. Useful for sharing context with teammates, backing up, or feeding to another tool."),
			mcplib.WithString("feature_name",
				mcplib.Description("Feature to export (default: active feature)"),
			),
			mcplib.WithString("format",
				mcplib.Description("Export format: markdown or json"),
				mcplib.Enum("markdown", "json"),
			),
		),
		s.handleExport,
	)
}

// registerResources registers the 2 MCP resources.
func (s *DevMemServer) registerResources(mcpServer *server.MCPServer) {
	mcpServer.AddResource(
		mcplib.Resource{
			URI:         "devmem://context/active",
			Name:        "Active Feature Context",
			Description: "Compact context for the currently active feature",
			MIMEType:    "text/plain",
		},
		s.handleResourceActiveContext,
	)

	mcpServer.AddResource(
		mcplib.Resource{
			URI:         "devmem://changes/recent",
			Name:        "Recent Changes",
			Description: "Git commits since the last session ended",
			MIMEType:    "text/plain",
		},
		s.handleResourceRecentChanges,
	)
}
