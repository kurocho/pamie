CREATE TABLE IF NOT EXISTS memory_keywords (
  memory_id TEXT NOT NULL REFERENCES memory_items(id) ON DELETE CASCADE,
  keyword_index INTEGER NOT NULL CHECK (keyword_index >= 0),
  keyword TEXT NOT NULL,
  normalized_keyword TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  PRIMARY KEY (memory_id, keyword_index)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_memory_keywords_memory_normalized
  ON memory_keywords (memory_id, normalized_keyword);

CREATE INDEX IF NOT EXISTS idx_memory_keywords_normalized
  ON memory_keywords (normalized_keyword);

ALTER TABLE vector_metadata ADD COLUMN embedding_scope TEXT NOT NULL DEFAULT 'body';

ALTER TABLE memory_embeddings ADD COLUMN embedding_scope TEXT NOT NULL DEFAULT 'body';

CREATE TABLE IF NOT EXISTS embedding_index_status (
  chunk_id TEXT NOT NULL REFERENCES memory_chunks(id) ON DELETE CASCADE,
  memory_id TEXT NOT NULL REFERENCES memory_items(id) ON DELETE CASCADE,
  provider TEXT NOT NULL,
  model TEXT NOT NULL,
  dimensions INTEGER NOT NULL CHECK (dimensions > 0),
  embedding_scope TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('pending', 'indexed', 'failed', 'skipped')),
  content_hash TEXT NOT NULL DEFAULT '',
  error_summary TEXT NOT NULL DEFAULT '',
  attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  PRIMARY KEY (chunk_id, provider, model, embedding_scope)
);

CREATE INDEX IF NOT EXISTS idx_embedding_index_status_status
  ON embedding_index_status (provider, model, embedding_scope, status, updated_at);
