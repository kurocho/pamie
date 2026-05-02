CREATE TABLE IF NOT EXISTS schema_migrations (
  version INTEGER PRIMARY KEY,
  name TEXT NOT NULL,
  applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE IF NOT EXISTS memory_items (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL DEFAULT '',
  body TEXT NOT NULL,
  source TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  tier TEXT NOT NULL CHECK (tier IN ('working', 'hot', 'warm', 'cold', 'archive')),
  importance INTEGER NOT NULL DEFAULT 0 CHECK (importance >= 0 AND importance <= 100),
  pinned INTEGER NOT NULL DEFAULT 0 CHECK (pinned IN (0, 1)),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  last_accessed_at TEXT,
  archived_at TEXT,
  deleted_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_memory_items_tier_updated
  ON memory_items (tier, updated_at);

CREATE INDEX IF NOT EXISTS idx_memory_items_pinned
  ON memory_items (pinned, updated_at);

CREATE TABLE IF NOT EXISTS memory_chunks (
  id TEXT PRIMARY KEY,
  memory_id TEXT NOT NULL REFERENCES memory_items(id) ON DELETE CASCADE,
  chunk_index INTEGER NOT NULL CHECK (chunk_index >= 0),
  content TEXT NOT NULL,
  created_at TEXT NOT NULL,
  UNIQUE (memory_id, chunk_index)
);

CREATE INDEX IF NOT EXISTS idx_memory_chunks_memory_id
  ON memory_chunks (memory_id, chunk_index);

CREATE VIRTUAL TABLE IF NOT EXISTS memory_fts USING fts5(
  content,
  memory_id UNINDEXED,
  chunk_id UNINDEXED
);

CREATE TRIGGER IF NOT EXISTS memory_chunks_ai AFTER INSERT ON memory_chunks BEGIN
  INSERT INTO memory_fts(rowid, content, memory_id, chunk_id)
  VALUES (new.rowid, new.content, new.memory_id, new.id);
END;

CREATE TRIGGER IF NOT EXISTS memory_chunks_ad AFTER DELETE ON memory_chunks BEGIN
  DELETE FROM memory_fts WHERE rowid = old.rowid;
END;

CREATE TRIGGER IF NOT EXISTS memory_chunks_au AFTER UPDATE ON memory_chunks BEGIN
  DELETE FROM memory_fts WHERE rowid = old.rowid;
  INSERT INTO memory_fts(rowid, content, memory_id, chunk_id)
  VALUES (new.rowid, new.content, new.memory_id, new.id);
END;

CREATE TABLE IF NOT EXISTS memory_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  memory_id TEXT REFERENCES memory_items(id) ON DELETE SET NULL,
  event_type TEXT NOT NULL,
  event_payload_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_memory_events_memory_id
  ON memory_events (memory_id, created_at);

CREATE TABLE IF NOT EXISTS retention_policies (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  scope_json TEXT NOT NULL DEFAULT '{}',
  rules_json TEXT NOT NULL DEFAULT '{}',
  enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_retention_policies_enabled
  ON retention_policies (enabled, updated_at);

CREATE TABLE IF NOT EXISTS access_log (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  memory_id TEXT NOT NULL REFERENCES memory_items(id) ON DELETE CASCADE,
  access_type TEXT NOT NULL,
  token_id TEXT,
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_access_log_memory_created
  ON access_log (memory_id, created_at);
