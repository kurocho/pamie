# Storage Design

SQLite is the durable source of truth. The storage layer should provide typed repositories and transactions instead of exposing SQL to upper layers.

## Database Mode

The service opens SQLite with the pure Go `modernc.org/sqlite` driver. Startup enables foreign keys, configures a busy timeout, enables WAL mode, and applies migrations before serving HTTP.

The current connection pool is conservative: one open connection and one idle connection. This keeps connection-level PRAGMAs predictable for the early implementation. Concurrency tuning can be revisited after repository behavior and backup handling are more mature.

## Implemented Tables

- `memory_items`: canonical memory data and lifecycle fields.
- `memory_chunks`: normalized searchable chunks.
- `memory_keywords`: first-class semantic retrieval keywords supplied by agents.
- `memory_events`: append-only event history.
- `retention_policies`: operator-defined lifecycle rules.
- `access_log`: reads and accesses used for ranking and promotion.
- `memory_fts`: FTS5 index synchronized from `memory_chunks`.
- `vector_metadata`: local vector backend metadata by provider and model.
- `memory_embeddings`: JSON-encoded chunk embeddings and content hashes.
- `embedding_index_status`: latest title/keywords indexing outcome for each chunk target.

## Migrations

Migrations are embedded SQL files under `internal/db/migrations`. The runner records applied versions in `schema_migrations`, applies missing migrations in order, and fails startup if migration execution fails.

Current migrations:

- `0001_initial_schema.sql`: memory, FTS5, policy, event, and access tables.
- `0002_vector_search_storage.sql`: optional vector metadata and embedding tables.
- `0003_vector_rowids.sql`: stable integer row IDs for sqlite-vec mirrors.
- `0004_persistent_tokens.sql`: hash-only persistent Bearer token metadata.
- `0005_memory_keywords_and_embedding_scope.sql`: memory keywords, embedding scope, and indexing status.

Vector search is optional, but the tables are always migrated so enabling the feature later does not require an out-of-band schema step.

## Vector Storage

Vector storage is local SQLite storage with JSON-encoded float vectors. Each `memory_embeddings` row belongs to one `memory_chunks` row and one provider/model/scope target. The supported scope is `title_keywords`: vectors are generated from the memory title and explicit keywords only. The full body remains stored in `memory_items` and indexed in `memory_chunks`/`memory_fts` for exact search.

`vector_metadata` records the configured provider, model, dimensions, backend name, and distance metric. Supported backend names are `sqlite-json` and `sqlite-vec`; the metric is cosine similarity.

When `sqlite-vec` is selected, the repository creates a deterministic per-target `vec0` virtual table. The canonical embedding row remains in `memory_embeddings`, while the virtual table stores the accelerated vector mirror keyed by `vector_rowid`.

Backfill is repeatable: the repository can list active chunks missing title/keywords embeddings for a provider/model/scope, and the memory service indexes bounded batches. Already indexed chunks are skipped by the primary key on `chunk_id`, provider, and model. Failed indexing status rows are retried; memories with no title or keywords are marked skipped.

## Backups

SQLite backup is implemented through the local `pamie backup` operator command, which uses SQLite-safe backup behavior instead of copying the live database file. Portable NDJSON backup and append-only restore validation are exposed through `pamie backup --format ndjson` and `pamie restore --format ndjson`.

Because WAL mode is enabled, operators must not copy only a live `pamie.db` file without considering WAL state.

SQLite backups include vector tables. NDJSON export preserves canonical memories, chunks, keywords, events, policies, and access logs. Embeddings and embedding status can be safely regenerated with vector backfill after import.

The operator command `pamie embeddings backfill` regenerates missing embedding rows after import or restore. `--reindex` recomputes existing rows and refreshes sqlite-vec mirrors.

## Access Pattern

Upper layers should use repository methods and domain services. No MCP tool should accept SQL or expose table names as public API.

Current repositories validate supported inputs and expose typed methods for memory items, chunks, embeddings, events, retention policies, access logs, and transactions.
