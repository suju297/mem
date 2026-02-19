CREATE TABLE IF NOT EXISTS repos (
    repo_id TEXT PRIMARY KEY,
    git_root TEXT NOT NULL,
    origin_hash TEXT,
    last_head TEXT,
    last_branch TEXT,
    created_at TEXT NOT NULL,
    last_seen_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS state_current (
    repo_id TEXT NOT NULL,
    workspace TEXT NOT NULL,
    state_json TEXT NOT NULL,
    state_tokens INTEGER,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (repo_id, workspace)
);

CREATE TABLE IF NOT EXISTS state_history (
    state_id TEXT PRIMARY KEY,
    repo_id TEXT NOT NULL,
    workspace TEXT NOT NULL,
    state_json TEXT NOT NULL,
    state_tokens INTEGER,
    created_at TEXT NOT NULL,
    reason TEXT
);

CREATE TABLE IF NOT EXISTS threads (
    thread_id TEXT NOT NULL,
    repo_id TEXT NOT NULL,
    workspace TEXT NOT NULL DEFAULT 'default',
    title TEXT,
    tags_json TEXT,
    created_at TEXT NOT NULL,
    PRIMARY KEY (thread_id, repo_id, workspace)
);

CREATE TABLE IF NOT EXISTS memories (
    id TEXT PRIMARY KEY,
    repo_id TEXT NOT NULL,
    workspace TEXT NOT NULL DEFAULT 'default',
    thread_id TEXT,
    title TEXT NOT NULL,
    summary TEXT NOT NULL,
    summary_tokens INTEGER,
    tags_json TEXT,
    tags_text TEXT,
    entities_json TEXT,
    entities_text TEXT,
    created_at TEXT NOT NULL,
    anchor_commit TEXT,
    superseded_by TEXT,
    deleted_at TEXT
);

CREATE TABLE IF NOT EXISTS artifacts (
    artifact_id TEXT PRIMARY KEY,
    repo_id TEXT NOT NULL,
    workspace TEXT NOT NULL DEFAULT 'default',
    kind TEXT,
    source TEXT,
    content_hash TEXT,
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS chunks (
    chunk_id TEXT PRIMARY KEY,
    repo_id TEXT NOT NULL,
    workspace TEXT NOT NULL DEFAULT 'default',
    artifact_id TEXT,
    thread_id TEXT,
    locator TEXT,
    text TEXT,
    text_hash TEXT,
    text_tokens INTEGER,
    tags_json TEXT,
    tags_text TEXT,
    chunk_type TEXT DEFAULT 'line',
    symbol_name TEXT,
    symbol_kind TEXT,
    created_at TEXT NOT NULL,
    deleted_at TEXT
);

CREATE TABLE IF NOT EXISTS embeddings (
    repo_id TEXT NOT NULL,
    workspace TEXT NOT NULL DEFAULT 'default',
    kind TEXT NOT NULL,
    item_id TEXT NOT NULL,
    model TEXT NOT NULL,
    content_hash TEXT NOT NULL,
    vector_json TEXT NOT NULL,
    vector_dim INTEGER NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (repo_id, workspace, kind, item_id, model)
);

CREATE TABLE IF NOT EXISTS embedding_queue (
    queue_id INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_id TEXT NOT NULL,
    workspace TEXT NOT NULL DEFAULT 'default',
    kind TEXT NOT NULL,
    item_id TEXT NOT NULL,
    model TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS links (
    from_id TEXT NOT NULL,
    rel TEXT NOT NULL,
    to_id TEXT NOT NULL,
    weight REAL,
    created_at TEXT NOT NULL,
    PRIMARY KEY (from_id, rel, to_id)
);

CREATE TABLE IF NOT EXISTS meta (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_memories_repo_created ON memories (repo_id, workspace, created_at);
CREATE INDEX IF NOT EXISTS idx_memories_thread ON memories (repo_id, workspace, thread_id);
CREATE INDEX IF NOT EXISTS idx_chunks_repo_created ON chunks (repo_id, workspace, created_at);
CREATE INDEX IF NOT EXISTS idx_chunks_thread ON chunks (repo_id, workspace, thread_id);
CREATE INDEX IF NOT EXISTS idx_embeddings_kind_model ON embeddings (repo_id, workspace, kind, model);
CREATE UNIQUE INDEX IF NOT EXISTS idx_embedding_queue_unique ON embedding_queue (repo_id, workspace, kind, item_id, model);
CREATE INDEX IF NOT EXISTS idx_links_from ON links (from_id);
CREATE INDEX IF NOT EXISTS idx_links_to ON links (to_id);

CREATE VIRTUAL TABLE IF NOT EXISTS memories_fts USING fts5 (
    title,
    summary,
    tags,
    entities,
    repo_id UNINDEXED,
    workspace UNINDEXED,
    mem_id UNINDEXED,
    tokenize = 'porter unicode61'
);

CREATE VIRTUAL TABLE IF NOT EXISTS chunks_fts USING fts5 (
    locator,
    text,
    tags,
    repo_id UNINDEXED,
    workspace UNINDEXED,
    chunk_id UNINDEXED,
    thread_id UNINDEXED,
    tokenize = 'porter unicode61'
);
