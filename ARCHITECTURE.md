# Architecture

Pamie is planned as a single, self-contained Go service. The first production target is a small binary named `pamie` that can run on a laptop, workstation, NAS, homelab server, or small VPS.

## Runtime Shape

- One Go binary owns all application logic.
- One HTTP listener exposes operational endpoints and the MCP endpoint shell.
- One local SQLite database is the source of truth.
- A reverse proxy terminates HTTPS for public deployments.
- Optional background lifecycle jobs run inside the process on a controlled schedule.

## HTTP Endpoints

- `GET /health`: implemented liveness check. It does not require authentication or database access.
- `GET /ready`: implemented readiness check. It verifies the SQLite connection.
- `POST /mcp`: implemented authenticated MCP JSON-RPC endpoint.

## Authentication

Bearer authentication is implemented as HTTP middleware before `/mcp`. The current phase accepts persistent hashed tokens from SQLite and an optional bootstrap token from `PAMIE_TOKEN` or `--token`. If no token is configured, `/mcp` rejects requests with `503 Service Unavailable` instead of becoming open.

Persistent token commands create, rotate, list, and revoke token metadata while showing raw generated tokens only once. Authenticated requests carry a token ID and scope set in request context, MCP tools/resources enforce scopes, `/mcp` has configurable per-client rate limiting, and audit events record auth, MCP calls, resource reads, and rate-limit blocks without token values.

Future production hardening should add tamper-resistant audit storage.

## Configuration and Logging

Startup configuration is parsed in `internal/config` from environment variables and flags, then passed into runtime packages. Runtime packages should not read environment variables directly.

Structured JSON logging uses Go's standard `log/slog` package. Request logging records method, path, status, and duration without logging headers or Bearer tokens.

Lifecycle worker configuration is opt-in through `PAMIE_LIFECYCLE_WORKER_ENABLED` / `--lifecycle-worker`. Operators can set the interval, batch size, run-on-start behavior, and startup delay without changing retention policy semantics.

Vector search is enabled by default with the dependency-free `local-hash` provider and can be disabled through `PAMIE_VECTOR_SEARCH_ENABLED=false` / `--vector-search=false`. Built-in providers are `local-hash` and `ollama`; backends are `auto`, `sqlite-json`, and `sqlite-vec`.

## Package Boundaries

`cmd/pamie` is the composition root. It wires configuration, logging, authentication, storage, memory services, MCP registries, and HTTP serving.

Runtime package direction is intentionally narrow:

- `internal/httpserver` owns HTTP routing and middleware only.
- `internal/mcp` owns JSON-RPC/MCP protocol behavior and depends on tool/resource interfaces.
- `internal/tools` and `internal/resources` define MCP-facing contracts and depend on memory service interfaces.
- `internal/memory` owns domain behavior and depends on a storage interface plus typed storage records, not raw SQL or SQLite connection details.
- `internal/embedding` owns local embedding provider interfaces and built-in providers.
- `internal/lifecycle` owns scheduled lifecycle worker timing, structured lifecycle-run logs, and worker audit events.
- `internal/db` owns SQLite setup, migrations, transactions, query construction, and repository validation.
- `internal/search` owns ranking policy and vector-search extension interfaces.

Shared helpers belong in `internal/util` only when at least two packages need the same small behavior. Current shared JSON decoding is used by MCP params and tool arguments so both layers reject unknown fields and trailing values consistently.

## Storage

SQLite is the durable source of truth. The storage layer owns connection setup, WAL mode, foreign key enforcement, schema migrations, typed repositories, transaction boundaries, SQLite backup, and NDJSON import/export primitives.

The current implementation uses the pure Go `modernc.org/sqlite` driver to avoid CGO in default builds. The sqlite-vec extension is registered through `modernc.org/sqlite/vec`. The startup path opens the configured database, applies PRAGMAs, runs migrations, and fails startup if storage cannot be initialized.

Implemented core tables:

- `memory_items`: canonical memory records and metadata.
- `memory_chunks`: searchable text chunks derived from memory records.
- `memory_keywords`: explicit semantic retrieval keywords supplied by agents.
- `memory_events`: append-only lifecycle and mutation history.
- `retention_policies`: explicit rules for demotion, archive, and deletion.
- `access_log`: retrieval and access events used for ranking and promotion.
- `memory_fts`: FTS5 index maintained by chunk triggers for keyword search.
- `vector_metadata`: optional local vector backend metadata.
- `memory_embeddings`: optional chunk embeddings for hybrid ranking.
- `embedding_index_status`: durable title/keywords embedding indexing outcomes.

## Search

MVP search uses SQLite FTS5 over `memory_chunks`. Query text is sanitized into safe FTS terms, then repository search applies typed filters and reranks candidates in Go.

Implemented search behavior combines:

- keyword match quality;
- snippets and highlights;
- metadata filters;
- recency boosts;
- tier boosts;
- pinned and importance boosts;
- access-frequency signals.

`context_search` supports tier, pinned, source, metadata equality, creation/update time bounds, deletion inclusion, limits, and `shallow`/`standard`/`deep` candidate depth. Results include snippets and explainable score details.

Vector search is enabled by default. The memory service embeds only saved memory titles and explicit keywords with a local provider, stores `title_keywords` vectors in SQLite, and search blends vector similarity with the existing keyword and lifecycle signals. Full bodies are never sent to embedding providers; they remain durably stored and fully indexed by FTS5. The `sqlite-vec` backend uses local `vec0` virtual tables for accelerated nearest-neighbor lookup; `sqlite-json` remains the portable fallback. The `ollama` provider gives operators a local semantic embedding path when a local Ollama server is running, with optional local-only autostart.

## Memory Tiers

Pamie uses five planned tiers:

- `working`: newest or session-adjacent context that should be fast and prominent.
- `hot`: recent useful memories that remain high priority.
- `warm`: older memories with ongoing value.
- `cold`: rarely accessed memories that should remain searchable.
- `archive`: retained memories that are available but not usually prominent.

Pinned memories and explicitly important memories should resist demotion and deletion unless a policy clearly allows it. Frequently accessed old memories can be promoted into more active tiers.

## Lifecycle Job

The lifecycle service applies retention and tiering policies through `RunLifecycle`. The optional background worker calls that service on a configured interval and uses the same cancellation context as the HTTP server.

The worker is disabled by default. When enabled, it:

- starts asynchronously so HTTP startup is not blocked;
- optionally runs immediately on startup or after a configured startup delay;
- evaluates at most the configured batch size per run;
- prevents overlapping lifecycle runs and skips ticks while a run is already active;
- logs start, completion, failure, cancellation, and skipped-overlap events with structured fields;
- emits audit events with lifecycle-run outcome and report counters;
- stops through the shared shutdown context and waits for any active lifecycle run to observe cancellation.

Current behavior:

- demotes inactive memories one tier at a time;
- promotes memories after recent repeated access;
- protects pinned memories by default;
- protects memories with importance `90` or higher by default;
- archives cold memories after the archive threshold;
- soft-deletes archived memories only when an enabled retention policy sets `delete_archived_after_days`;
- records every lifecycle change in `memory_events`.

## MCP Tools and Resources

MCP handlers should expose purpose-built memory operations rather than backend primitives. Planned tools include:

- `context_save`
- `context_get`
- `context_search`
- `context_update`
- `context_delete`
- `context_pin`
- `context_recent`
- `context_stats`

These tools are implemented with storage-backed behavior. Search is FTS5-backed with optional local vector assistance, safe filters, and explainable ranking. Deletion is a conservative soft delete, while lifecycle policy deletion remains explicit and policy-controlled.

Implemented read-only resources:

- `pamie://status`
- `pamie://memory/stats`

Resources expose safe operational and memory views without leaking sensitive configuration or raw database access.

## Backup and Export

The backup/export subsystem should support SQLite-safe backups, NDJSON export, and restore/import workflows. Backups must preserve memory IDs, timestamps, tiers, metadata, and lifecycle events.
