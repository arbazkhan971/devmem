# devmem

A SOTA developer memory system. Local MCP server in Go that gives any coding CLI persistent, project-scoped session/feature memory.

## What it does

- **Feature tracking** — organize work by feature ("auth-v2", "billing-fix")
- **Session continuity** — picks up where you left off, across any MCP-compatible tool
- **Git integration** — auto-syncs commits with intent classification
- **Bi-temporal facts** — tracks what's true now AND what was true before
- **Plan persistence** — plans survive across sessions, auto-track progress from commits
- **Memory linking** — A-MEM style connections between related memories
- **Background consolidation** — detects contradictions, decays stale memories, generates summaries
- **3-layer search** — FTS5 + trigram + fuzzy across all memory types

## Install

```bash
go install github.com/arbaz/devmem/cmd/devmem@latest
```

## Setup

### Claude Code
```bash
claude mcp add -s user --transport stdio devmem -- devmem
```

### Cursor
Add to `.cursor/mcp.json`:
```json
{
    "mcpServers": {
        "devmem": { "command": "devmem", "transport": "stdio" }
    }
}
```

### Windsurf / Other MCP Clients
Add `devmem` as a stdio MCP server in your tool's MCP configuration.

## Usage

devmem auto-detects your project from the git root. No configuration needed.

### Tools

| Tool | What it does |
|------|-------------|
| `devmem_status` | Project overview, active feature, plan progress |
| `devmem_list_features` | All features with status and commit breakdown |
| `devmem_start_feature` | Create or resume a feature |
| `devmem_switch_feature` | Switch to a different feature |
| `devmem_get_context` | Where you left off (compact/standard/detailed) |
| `devmem_sync` | Pull git commits, classify intent, match to plan steps |
| `devmem_remember` | Save a note, decision, blocker, or next step |
| `devmem_search` | Search across all memory types |
| `devmem_save_plan` | Store a plan with trackable steps |
| `devmem_import_session` | Import context from current conversation into memory |
| `devmem_export` | Export a feature's memory as markdown or JSON |

### Importing Existing Sessions

Already been working on a project without devmem? No problem. Just tell your AI assistant:

> "Import everything from this session into devmem"

The AI will call `devmem_import_session` with all the decisions, progress, blockers, facts, and plans from your current conversation. This works in **any MCP-compatible tool** — Claude Code, Cursor, Codex, Windsurf.

**What gets imported:**
- Decisions ("we chose better-auth over next-auth")
- Progress notes ("webhook handler done, need tests")
- Blockers ("token refresh breaks on Safari")
- Next steps ("write integration tests for billing")
- Facts ("auth uses better-auth", "DB is PostgreSQL")
- Plans with step-level status

**Example — bootstrap from an existing Claude Code session:**

```
You: "I've been working on auth-v2 migration. Import what we've
     discussed into devmem so future sessions have this context."

Claude: calls devmem_import_session with:
  feature_name: "auth-v2"
  description: "Migrating from custom JWT to better-auth"
  decisions: [
    "Chose better-auth over next-auth for compliance",
    "Using opaque tokens instead of JWT"
  ]
  progress_notes: [
    "Middleware extracted from monolith",
    "Token refresh rotation implemented"
  ]
  facts: [
    { subject: "auth", predicate: "uses", object: "better-auth v2" },
    { subject: "tokens", predicate: "format", object: "opaque" }
  ]
  plan_steps: [
    { title: "Extract auth middleware", status: "completed" },
    { title: "Setup better-auth", status: "completed" },
    { title: "Add refresh rotation", status: "completed" },
    { title: "Update protected routes", status: "pending" },
    { title: "Write integration tests", status: "pending" }
  ]
  plan_title: "Auth V2 Migration Plan"

Result: 12 items imported, 5 links created
        Memory bootstrapped — future sessions have full context.
```

**Exporting for sharing or backup:**

```
You: "Export the auth-v2 feature memory as markdown"
Claude: calls devmem_export → returns full markdown with decisions,
        facts, commits, plan progress, session history
```

### Example Flow

```
You: "What am I working on?"
-> devmem_status shows 3 features, auth-v2 is active

You: "Let's continue billing-fix"
-> devmem_switch_feature loads full context from last session

You: (work, make commits)
-> devmem_sync pulls commits, matches to plan steps

You: "Remember: webhook handler done, need tests"
-> devmem_remember stores note, auto-links to related memories

Next day, different CLI tool:
-> devmem_get_context shows exactly where you left off
```

## How it works

- Single Go binary, zero external dependencies
- SQLite database at `<project>/.memory/memory.db`
- `current.json` human-readable snapshot always up to date
- WAL mode for concurrent access (multiple tools simultaneously)
- FTS5 for full-text search with BM25 ranking

## Architecture

```
MCP Client (Claude Code / Cursor / Codex)
    | stdio
    v
devmem (Go binary)
    |-- Session Manager (features, sessions)
    |-- Git Engine (commits, intent, sync)
    |-- Search Engine (FTS5 + trigram + scoring)
    |-- Plan Engine (CRUD, auto-detect, commit matching)
    |-- Memory Core (bi-temporal facts, notes, links)
    |-- Consolidation Engine (background goroutine)
    +-- SQLite (WAL mode, .memory/memory.db)
```

## License

MIT
