CREATE TABLE IF NOT EXISTS vector_metadata (
  provider TEXT NOT NULL,
  model TEXT NOT NULL,
  dimensions INTEGER NOT NULL CHECK (dimensions > 0),
  backend TEXT NOT NULL DEFAULT 'sqlite-json',
  distance_metric TEXT NOT NULL DEFAULT 'cosine',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  PRIMARY KEY (provider, model)
);

CREATE TABLE IF NOT EXISTS memory_embeddings (
  chunk_id TEXT NOT NULL REFERENCES memory_chunks(id) ON DELETE CASCADE,
  memory_id TEXT NOT NULL REFERENCES memory_items(id) ON DELETE CASCADE,
  provider TEXT NOT NULL,
  model TEXT NOT NULL,
  dimensions INTEGER NOT NULL CHECK (dimensions > 0),
  embedding_json TEXT NOT NULL,
  content_hash TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  PRIMARY KEY (chunk_id, provider, model),
  FOREIGN KEY (provider, model) REFERENCES vector_metadata(provider, model) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_memory_embeddings_memory_provider_model
  ON memory_embeddings (memory_id, provider, model);

CREATE INDEX IF NOT EXISTS idx_memory_embeddings_provider_model_updated
  ON memory_embeddings (provider, model, updated_at);
