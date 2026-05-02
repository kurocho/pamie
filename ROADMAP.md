# Roadmap

## Phase 0 - Repository Foundation

- Repository layout.
- Documentation.
- Agent definitions.
- Reusable skills.
- GitHub CI.
- Minimal compiling Go binary.

## Phase 1 - Core Server

- Configuration loading.
- Structured logging.
- HTTP server.
- Health endpoint.
- Readiness endpoint.
- Bearer authentication middleware.
- Graceful shutdown.

## Phase 2 - SQLite Storage

- Migrations.
- Schema.
- WAL mode.
- `memory_items`.
- `memory_chunks`.
- `memory_events`.
- `retention_policies`.
- `access_log`.

## Phase 3 - MCP Integration

- MCP HTTP endpoint.
- Tool skeletons.
- Resource skeletons.
- Request validation.
- Error mapping.

## Phase 4 - Memory Tools

- `context_save`.
- `context_get`.
- `context_search`.
- `context_update`.
- `context_delete`.
- `context_pin`.
- `context_recent`.
- `context_stats`.

## Phase 5 - Search

- FTS5 indexing.
- Ranking.
- Snippets.
- Metadata filters.
- Search depth.

## Phase 6 - Lifecycle

- Retention policies.
- Promotion.
- Demotion.
- Archive tier.
- Deletion by policy.

## Phase 7 - Backup and Export

- SQLite backup.
- NDJSON export.
- Restore/import plan.
- Operator documentation.

## Phase 8 - Security Hardening

- Token hashing.
- Scopes.
- Rate limiting.
- Audit logs.
- Threat model validation.

## Phase 9 - Vector Search

- `VectorSearcher` interface.
- sqlite-vec accelerated local backend.
- SQLite JSON fallback backend.
- Local embeddings through `local-hash` and Ollama.
- Hybrid ranking.
- Operator embedding backfill.
