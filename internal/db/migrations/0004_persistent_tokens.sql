CREATE TABLE IF NOT EXISTS auth_tokens (
  id TEXT PRIMARY KEY,
  token_hash TEXT NOT NULL,
  token_salt TEXT NOT NULL,
  scopes TEXT NOT NULL,
  created_at TEXT NOT NULL,
  last_used_at TEXT,
  revoked_at TEXT,
  expires_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_auth_tokens_active
  ON auth_tokens (revoked_at, expires_at);
