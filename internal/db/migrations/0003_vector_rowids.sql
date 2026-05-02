ALTER TABLE memory_embeddings ADD COLUMN vector_rowid INTEGER;

CREATE UNIQUE INDEX IF NOT EXISTS idx_memory_embeddings_vector_rowid
  ON memory_embeddings (vector_rowid)
  WHERE vector_rowid IS NOT NULL;
