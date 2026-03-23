# memorX

SOTA developer memory system. Single Go binary MCP server that gives any coding CLI persistent, project-scoped memory across sessions, tools, and features.

**320 tests | 17 tools | 70/70 benchmark (99.6%) | <1ms latency | Zero dependencies**

## The Problem

Every AI coding CLI (Claude Code, Codex, Cursor, Windsurf, Gemini CLI) suffers from amnesia. Close a session, lose all context. Switch tools, start from scratch. You waste 5-10 minutes per session re-explaining your project, decisions, and progress.

memorX fixes this. One binary, works everywhere, remembers everything.

## What It Does

- **Session continuity** — picks up where you left off, in any MCP-compatible tool
- **Feature tracking** — organize work by feature ("auth-v2", "billing-fix")
- **Git integration** — auto-syncs commits with intent classification (feature/bugfix/refactor)
- **Plan persistence** — plans survive across sessions, auto-track progress from commits
- **Bi-temporal facts** — tracks what's true now AND what was true before (contradiction resolution)
- **Memory linking** — A-MEM/Zettelkasten-style connections between related memories
- **Background consolidation** — detects contradictions, decays stale memories, generates summaries
- **3-layer search** — FTS5 + trigram + fuzzy across all memory types
- **Auto-briefing** — "welcome back" context on every session start
- **Session summaries** — captures what happened for next time
- **Development analytics** — session counts, commit patterns, blocker frequency
- **Memory health** — health score (0-100) with actionable suggestions
- **Smart forgetting** — clean up stale facts, notes, completed features
- **AGENTS.md generation** — auto-generate universal rules file from memory

## Install

```bash
go install github.com/arbazkhan971/memorx/cmd/devmem@latest
```

Or build from source:
```bash
git clone https://github.com/arbazkhan971/memorx.git
cd memorx
go build -o bin/memorx ./cmd/devmem
```

## Setup

### Claude Code
```bash
claude mcp add -s user --transport stdio memorx -- memorx
```

### Cursor
Add to `.cursor/mcp.json`:
```json
{
    "mcpServers": {
        "memorx": { "command": "memorx", "transport": "stdio" }
    }
}
```

### Windsurf / Codex / Other MCP Clients
Add `memorx` as a stdio MCP server in your tool's MCP configuration.

### Recommended: Add to CLAUDE.md
```markdown
## Memory
This project uses memorX. At the start of every session:
1. Call memorx_briefing to see where we left off
2. When making decisions, call memorx_remember with type="decision"
3. Before ending, call memorx_end_session with a summary
```

## Tools (17)

### Core
| Tool | What it does |
|------|-------------|
| `memorx_status` | Project overview, active feature, plan progress |
| `memorx_briefing` | Quick "welcome back" — what you were working on, where you left off |
| `memorx_list_features` | All features with status and commit breakdown |
| `memorx_start_feature` | Create or resume a feature |
| `memorx_switch_feature` | Switch to a different feature |
| `memorx_get_context` | Full context at 3 tiers: compact (~200 tokens) / standard (~500) / detailed (~1500) |
| `memorx_sync` | Pull git commits, classify intent, auto-match to plan steps |
| `memorx_remember` | Save a note, decision, blocker, or next step. Auto-links to related memories |
| `memorx_search` | 3-layer search (FTS5 + trigram + fuzzy) across all memory types |
| `memorx_save_plan` | Store a plan with trackable steps. Supersedes old plans, carries completed steps |

### Session Management
| Tool | What it does |
|------|-------------|
| `memorx_end_session` | End session with a summary — next session reads it automatically |
| `memorx_import_session` | Bootstrap memory from current conversation (decisions, facts, plans) |
| `memorx_export` | Export feature memory as markdown or JSON |

### Intelligence
| Tool | What it does |
|------|-------------|
| `memorx_analytics` | Dev patterns: session counts, commit intent breakdown, blockers, time spent |
| `memorx_health` | Memory health score (0-100) with suggestions (conflicts, stale data, orphans) |
| `memorx_forget` | Smart cleanup: stale facts, stale notes, completed features, or specific IDs |
| `memorx_generate_rules` | Auto-generate AGENTS.md from memory — universal rules file for all CLIs |

## How It Works

```
You open Claude Code on Monday:
  memorX: "Welcome back! Active feature: auth-v2 (branch: feature/auth-v2)
           Plan: Auth Migration (4/7 steps done)
           Last session: Friday via claude-code
           Recent: Token refresh working, need to test expiry edge cases"

You work, make commits, make decisions.
  memorx_sync → captures commits, classifies intent, matches plan steps
  memorx_remember → stores decisions with auto-linking
  memorx_end_session → "Completed token refresh tests. Next: update routes"

Tuesday, you open Cursor on the same project:
  memorX: "Welcome back! Last session summary: Completed token refresh tests.
           Next: update routes. Plan: 5/7 steps done."

  Full context in 1 tool call. Zero re-explaining.

Wednesday, you open Codex CLI:
  Same memory. Same context. Same progress.
```

## Parallel Work (Multiple CLIs Simultaneously)

memorX supports concurrent access via SQLite WAL mode:

```
Terminal 1: Claude Code          Terminal 2: Cursor
┌────────────────────┐          ┌────────────────────┐
│ feature: auth-v2   │          │ feature: billing    │
│ "Token refresh     │          │ "Webhook handler    │
│  working"          │          │  done"              │
└────────┬───────────┘          └────────┬───────────┘
         └──────────┬────────────────────┘
                    ▼
            .memory/memory.db
            (WAL mode: concurrent reads, serialized writes)
```

Both tools read/write to the same database. Different features don't interfere. Search across all features with `scope=all_features`.

## Importing Existing Sessions

Already been working without memorX? Bootstrap from your current conversation:

```
You: "Import everything we've discussed into memorX"

Claude calls memorx_import_session with:
  feature_name: "auth-v2"
  decisions: ["Chose better-auth for compliance", "Using opaque tokens"]
  progress_notes: ["Middleware extracted", "Token refresh implemented"]
  facts: [{ subject: "auth", predicate: "uses", object: "better-auth" }]
  plan_title: "Auth Migration"
  plan_steps: [
    { title: "Extract middleware", status: "completed" },
    { title: "Token refresh", status: "completed" },
    { title: "Update routes", status: "pending" }
  ]

Result: 8 items imported, 3 links created. Future sessions have full context.
```

## Architecture

```
MCP Client (Claude Code / Cursor / Codex / Windsurf)
    │ stdio
    ▼
memorX (single Go binary, 13MB)
    ├── MCP Layer (17 tools + 2 resources)
    ├── Session Manager (features, sessions, briefings)
    ├── Git Engine (commits, intent classification, sync)
    ├── Search Engine (FTS5 + trigram + BM25 scoring)
    ├── Plan Engine (CRUD, auto-detect, commit-to-step matching)
    ├── Memory Core (bi-temporal facts, notes, A-MEM links)
    ├── Consolidation Engine (contradictions, decay, summarization)
    ├── Analytics (dev patterns, health scoring)
    └── SQLite (WAL mode, .memory/memory.db)
```

**Key design choices:**
- **Single binary** — no Docker, no daemon, no external dependencies
- **SQLite + WAL** — concurrent access from multiple tools, sub-millisecond writes
- **FTS5** — full-text search with BM25 ranking, trigram fallback, no embedding model needed
- **Bi-temporal** — every fact has valid_at/invalid_at, enables "what was true last week?" queries
- **stdio transport** — MCP client spawns the binary, communicates via stdin/stdout

## Benchmark

memorX ships with a 70-scenario benchmark across 7 developer memory abilities:

```bash
make benchmark
```

```
Ability                  Score    Accuracy   Scenarios
─────────────────────    ─────    ────────   ─────────
Decision Recall          100.0%   100.0%     10/10
Knowledge Updates        100.0%   100.0%     10/10
Plan Tracking            100.0%   100.0%     10/10
Abstention               100.0%   100.0%      7/7
Session Continuity        99.3%   100.0%     15/15
Temporal Reasoning        99.0%   100.0%     10/10
Cross-Feature             98.8%   100.0%      8/8

Overall: 99.6% score | 100% accuracy | <1ms latency
```

## Token Savings

memorX reduces wasted tokens by eliminating context re-establishment:

```
Without memorX: 5,000-10,000 tokens per session re-explaining context
With memorX:    200-500 tokens via memorx_briefing + memorx_get_context

Estimated savings: ~$265/month for heavy users (3+ sessions/day)
```

## What Makes This SOTA

No other MCP memory tool combines all of these:

| Feature | memorX | Mem0 | Zep | Supermemory | Letta | KeepGoing |
|---------|--------|------|-----|-------------|-------|-----------|
| Session/feature tracking | Yes | No | No | No | No | Partial |
| Git commit integration | Yes | No | No | No | No | Partial |
| Plan persistence | Yes | No | No | No | No | No |
| Bi-temporal facts | Yes | No | Yes | Partial | No | No |
| Memory linking | Yes | Yes | Yes | Yes | No | No |
| Auto-briefing on connect | Yes | No | No | No | No | Yes |
| Session summaries | Yes | No | No | No | No | No |
| Dev analytics | Yes | No | No | No | No | No |
| Memory health scoring | Yes | No | No | No | No | No |
| Smart forgetting | Yes | No | No | No | No | No |
| AGENTS.md generation | Yes | No | No | No | No | No |
| Single binary, zero deps | Yes | No | No | No | No | No |
| 100% local, no cloud | Yes | No | No | No | Partial | Yes |
| Built-in benchmark | Yes | No | No | Yes | Yes | No |

## Cloud Roadmap

memorX is local-first but designed for future cloud sync:
- Bi-temporal facts enable conflict-free merging
- Append-only memory links can be union-merged
- Session attribution tracks which machine/tool created each memory
- Chunk-based sync model (Engram-inspired) for zero-conflict replication

Planned: team memory, multi-machine sync, cross-project intelligence, dashboard.

## License

MIT
