package storage

const schemaV1 = `
CREATE TABLE IF NOT EXISTS features (
    id TEXT PRIMARY KEY, name TEXT NOT NULL UNIQUE, description TEXT DEFAULT '',
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','paused','done')),
    branch TEXT, created_at TEXT NOT NULL DEFAULT (datetime('now')),
    last_active TEXT NOT NULL DEFAULT (datetime('now')));
CREATE INDEX IF NOT EXISTS idx_features_status ON features(status);
CREATE INDEX IF NOT EXISTS idx_features_name ON features(name);
CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY, feature_id TEXT NOT NULL REFERENCES features(id),
    tool TEXT NOT NULL DEFAULT 'unknown', started_at TEXT NOT NULL DEFAULT (datetime('now')),
    ended_at TEXT);
CREATE INDEX IF NOT EXISTS idx_sessions_feature ON sessions(feature_id);
CREATE INDEX IF NOT EXISTS idx_sessions_started ON sessions(started_at);
CREATE TABLE IF NOT EXISTS facts (
    id TEXT PRIMARY KEY, feature_id TEXT NOT NULL REFERENCES features(id),
    session_id TEXT REFERENCES sessions(id), subject TEXT NOT NULL,
    predicate TEXT NOT NULL, object TEXT NOT NULL,
    valid_at TEXT NOT NULL DEFAULT (datetime('now')), invalid_at TEXT,
    recorded_at TEXT NOT NULL DEFAULT (datetime('now')), confidence REAL NOT NULL DEFAULT 1.0);
CREATE INDEX IF NOT EXISTS idx_facts_temporal ON facts(subject, predicate, invalid_at);
CREATE INDEX IF NOT EXISTS idx_facts_feature ON facts(feature_id);
CREATE INDEX IF NOT EXISTS idx_facts_valid ON facts(valid_at);
CREATE INDEX IF NOT EXISTS idx_facts_active ON facts(invalid_at) WHERE invalid_at IS NULL;
CREATE TABLE IF NOT EXISTS notes (
    id TEXT PRIMARY KEY, feature_id TEXT NOT NULL REFERENCES features(id),
    session_id TEXT REFERENCES sessions(id), content TEXT NOT NULL,
    type TEXT NOT NULL DEFAULT 'note' CHECK (type IN ('progress','decision','blocker','next_step','note')),
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')));
CREATE INDEX IF NOT EXISTS idx_notes_feature ON notes(feature_id);
CREATE INDEX IF NOT EXISTS idx_notes_type ON notes(type);
CREATE TABLE IF NOT EXISTS plans (
    id TEXT PRIMARY KEY, feature_id TEXT NOT NULL REFERENCES features(id),
    session_id TEXT REFERENCES sessions(id), title TEXT NOT NULL,
    content TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','completed','abandoned','superseded')),
    source_tool TEXT DEFAULT 'unknown', valid_at TEXT NOT NULL DEFAULT (datetime('now')),
    invalid_at TEXT, created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')));
CREATE INDEX IF NOT EXISTS idx_plans_feature ON plans(feature_id);
CREATE INDEX IF NOT EXISTS idx_plans_status ON plans(status);
CREATE INDEX IF NOT EXISTS idx_plans_active ON plans(invalid_at) WHERE invalid_at IS NULL;
CREATE TABLE IF NOT EXISTS plan_steps (
    id TEXT PRIMARY KEY, plan_id TEXT NOT NULL REFERENCES plans(id) ON DELETE CASCADE,
    step_number INTEGER NOT NULL, title TEXT NOT NULL, description TEXT DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','in_progress','completed','skipped')),
    completed_at TEXT, linked_commits TEXT DEFAULT '[]');
CREATE INDEX IF NOT EXISTS idx_plan_steps_plan ON plan_steps(plan_id);
CREATE INDEX IF NOT EXISTS idx_plan_steps_status ON plan_steps(status);
CREATE TABLE IF NOT EXISTS commits (
    id TEXT PRIMARY KEY, feature_id TEXT NOT NULL REFERENCES features(id),
    session_id TEXT REFERENCES sessions(id), hash TEXT NOT NULL UNIQUE,
    message TEXT NOT NULL, author TEXT NOT NULL, files_changed TEXT DEFAULT '[]',
    intent_type TEXT DEFAULT 'unknown' CHECK (intent_type IN ('feature','bugfix','refactor','test','docs','infra','cleanup','unknown')),
    intent_confidence REAL DEFAULT 0.0, committed_at TEXT NOT NULL,
    synced_at TEXT NOT NULL DEFAULT (datetime('now')));
CREATE INDEX IF NOT EXISTS idx_commits_feature ON commits(feature_id);
CREATE INDEX IF NOT EXISTS idx_commits_hash ON commits(hash);
CREATE INDEX IF NOT EXISTS idx_commits_date ON commits(committed_at);
CREATE TABLE IF NOT EXISTS semantic_changes (
    id TEXT PRIMARY KEY, commit_hash TEXT NOT NULL, file_path TEXT NOT NULL,
    entity_name TEXT NOT NULL,
    entity_kind TEXT NOT NULL CHECK (entity_kind IN ('function','method','struct','class','interface','type','constant','variable','module','package','other')),
    change_type TEXT NOT NULL CHECK (change_type IN ('added','modified','deleted','renamed')),
    old_name TEXT, description TEXT DEFAULT '',
    session_id TEXT REFERENCES sessions(id), created_at TEXT NOT NULL DEFAULT (datetime('now')));
CREATE INDEX IF NOT EXISTS idx_semantic_commit ON semantic_changes(commit_hash);
CREATE INDEX IF NOT EXISTS idx_semantic_entity ON semantic_changes(entity_name);
CREATE TABLE IF NOT EXISTS memory_links (
    id TEXT PRIMARY KEY, source_id TEXT NOT NULL,
    source_type TEXT NOT NULL CHECK (source_type IN ('fact','note','commit','plan','plan_step','semantic_change')),
    target_id TEXT NOT NULL,
    target_type TEXT NOT NULL CHECK (target_type IN ('fact','note','commit','plan','plan_step','semantic_change')),
    relationship TEXT NOT NULL CHECK (relationship IN ('related','contradicts','extends','caused_by','implements','supersedes','blocks','resolves')),
    strength REAL NOT NULL DEFAULT 0.5, created_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(source_id, source_type, target_id, target_type, relationship));
CREATE INDEX IF NOT EXISTS idx_links_source ON memory_links(source_id, source_type);
CREATE INDEX IF NOT EXISTS idx_links_target ON memory_links(target_id, target_type);
CREATE TABLE IF NOT EXISTS summaries (
    id TEXT PRIMARY KEY, scope TEXT NOT NULL, content TEXT NOT NULL,
    generation INTEGER NOT NULL DEFAULT 0, token_count INTEGER NOT NULL DEFAULT 0,
    covers_from TEXT NOT NULL, covers_to TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now')));
CREATE INDEX IF NOT EXISTS idx_summaries_scope ON summaries(scope, generation);
CREATE TABLE IF NOT EXISTS consolidation_state (
    id INTEGER PRIMARY KEY CHECK (id = 1), last_run_at TEXT,
    entropy_score REAL NOT NULL DEFAULT 0.0, unsummarized_count INTEGER NOT NULL DEFAULT 0,
    conflict_count INTEGER NOT NULL DEFAULT 0, next_trigger_at TEXT);
INSERT OR IGNORE INTO consolidation_state (id) VALUES (1);
CREATE TABLE IF NOT EXISTS schema_version (
    version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL DEFAULT (datetime('now')));
`

const ftsSchemaV1 = `
CREATE VIRTUAL TABLE notes_fts USING fts5(content, type, content='notes', content_rowid='rowid', tokenize='porter unicode61');
CREATE VIRTUAL TABLE commits_fts USING fts5(message, content='commits', content_rowid='rowid', tokenize='porter unicode61');
CREATE VIRTUAL TABLE facts_fts USING fts5(subject, predicate, object, content='facts', content_rowid='rowid', tokenize='porter unicode61');
CREATE VIRTUAL TABLE plans_fts USING fts5(title, content, content='plans', content_rowid='rowid', tokenize='porter unicode61');
CREATE VIRTUAL TABLE notes_trigram USING fts5(content, content='notes', content_rowid='rowid', tokenize='trigram');
CREATE VIRTUAL TABLE commits_trigram USING fts5(message, content='commits', content_rowid='rowid', tokenize='trigram');
`

const schemaV3 = `
CREATE TABLE IF NOT EXISTS files_touched (
    id TEXT PRIMARY KEY, feature_id TEXT NOT NULL, session_id TEXT,
    path TEXT NOT NULL, action TEXT DEFAULT 'modified',
    first_seen TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(feature_id, session_id, path));
CREATE INDEX IF NOT EXISTS idx_files_touched_feature ON files_touched(feature_id);
CREATE INDEX IF NOT EXISTS idx_files_touched_session ON files_touched(session_id);
`
