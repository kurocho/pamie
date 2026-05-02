# Tasks

This checklist tracks implementation work. Keep it updated when a phase is started, split, or completed.

## Phase 0 - Repository Foundation

- [x] Create repository layout.
- [x] Add minimal Go module.
- [x] Add `cmd/pamie/main.go`.
- [x] Add Makefile.
- [x] Add CI workflow.
- [x] Add top-level docs.
- [x] Add package planning docs under `internal/`.
- [x] Add agent definitions.
- [x] Add reusable skills.
- [x] Add future implementation prompts.

## Phase 1 - Core Server

- [x] Define configuration struct and defaults.
- [x] Load configuration from flags and environment.
- [x] Add structured logging.
- [x] Implement HTTP server construction.
- [x] Implement `/health`.
- [x] Implement `/ready`.
- [x] Add Bearer auth middleware.
- [x] Add graceful shutdown on SIGINT and SIGTERM.
- [x] Add server unit tests.
- [x] Document local run flow.

## Phase 2 - SQLite Storage

- [x] Choose SQLite driver.
- [x] Add database open options.
- [x] Enable WAL mode.
- [x] Add migration runner.
- [x] Create `memory_items` schema.
- [x] Create `memory_chunks` schema.
- [x] Create `memory_events` schema.
- [x] Create `retention_policies` schema.
- [x] Create `access_log` schema.
- [x] Add repository interfaces.
- [x] Add integration tests with temporary databases.
- [x] Document backup-safe connection settings.

## Phase 3 - MCP Integration

- [x] Choose MCP Go library or define protocol boundary.
- [x] Implement `/mcp` endpoint shell.
- [x] Add tool registration.
- [x] Add resource registration.
- [x] Add request validation.
- [x] Map application errors to MCP errors.
- [x] Add compatibility tests with sample MCP requests.

## Phase 4 - Memory Tools

- [x] Implement `context_save`.
- [x] Implement `context_get`.
- [x] Implement `context_search`.
- [x] Implement `context_update`.
- [x] Implement `context_delete`.
- [x] Implement `context_pin`.
- [x] Implement `context_recent`.
- [x] Implement `context_stats`.
- [x] Add tool-level validation tests.
- [x] Add audit events for mutating tools.

## Phase 5 - Search

- [x] Create FTS5 virtual table.
- [x] Implement indexing for memory chunks.
- [x] Implement keyword search.
- [x] Implement metadata filters.
- [x] Implement snippet generation.
- [x] Add recency boost.
- [x] Add tier boost.
- [x] Add pinned and importance boost.
- [x] Add search depth controls.
- [x] Add ranking regression tests.

## Phase 6 - Lifecycle

- [x] Define tier transition rules.
- [x] Implement retention policy model.
- [x] Implement lifecycle evaluation service for demotion.
- [x] Add scheduled background lifecycle runner.
- [x] Implement promotion on access.
- [x] Implement archive behavior.
- [x] Implement deletion only by explicit policy.
- [x] Protect pinned memories by default.
- [x] Record lifecycle events.
- [x] Add deterministic lifecycle tests.

## Phase 7 - Backup and Export

- [x] Implement SQLite backup command or endpoint.
- [x] Implement NDJSON export.
- [x] Implement restore/import validation.
- [x] Document filesystem permissions.
- [x] Document scheduled backups.
- [x] Add restore test using exported fixture.

## Phase 8 - Security Hardening

- [x] Hash stored tokens.
- [x] Add token IDs.
- [x] Add scopes.
- [x] Add rate limiting.
- [x] Add audit logs.
- [x] Review prompt injection handling.
- [x] Review public deployment checklist.
- [x] Run threat model validation.

## Phase 9 - Vector Search

- [x] Define `VectorSearcher` interface.
- [x] Define embedding storage model.
- [x] Evaluate sqlite-vec for future local acceleration.
- [x] Evaluate libSQL vector search as a non-default future option.
- [x] Add local embedding provider abstraction.
- [x] Add deterministic local hash embedding provider.
- [x] Implement optional hybrid ranking.
- [x] Add migration and backfill plan.
- [x] Add vector-disabled fallback tests.
- [x] Add local semantic embedding provider through Ollama.
- [x] Add sqlite-vec acceleration behind the same storage/ranking contract.
- [x] Add operator embedding backfill command.
- [ ] Add model-specific operator guidance and benchmark fixtures.

## Phase 10 - Code Quality and Package Boundaries

- [x] Review package dependency direction across runtime packages.
- [x] Extract a narrow storage interface for the memory service.
- [x] Normalize strict JSON argument/params decoding across MCP and tools.
- [x] Add a stable memory service unavailable error.
- [x] Review context propagation in lifecycle loops.
- [x] Add package comments for internal ownership boundaries.
- [x] Update architecture and boundary documentation.
- [x] Add focused tests for changed validation and error behavior.

## Phase 11 - Testing and Acceptance

- [x] Add HTTP acceptance tests for `/health`, `/ready`, authenticated `/mcp`, unauthenticated `/mcp`, malformed auth, and malformed JSON-RPC.
- [x] Add MCP acceptance coverage for save/get/search/update/pin/delete/recent/stats.
- [x] Add SQLite persistence coverage across store restart.
- [x] Add lifecycle integration coverage across multiple tiers and scoped retention policy behavior.
- [x] Add search regression coverage for ranking order and combined filters.
- [x] Add fuzz seed tests for auth headers, MCP request parsing, metadata filters, and FTS query sanitization.
- [x] Add race-test-compatible concurrent memory service coverage.
- [x] Add opt-in Docker smoke script for image, health, readiness, and MCP auth validation.
- [x] Add CI race-test step.

## Release and Deployment

- [x] Add Dockerfile.
- [x] Add Compose example.
- [x] Add Caddy HTTPS reverse proxy example.
- [x] Add version injection with linker flags.
- [x] Add release artifact workflow.
- [x] Add checksum generation.
- [x] Add Docker build validation in CI.
- [x] Add Docker Hub image publishing on tagged releases.
- [x] Document local, NAS, VPS, and homelab deployments.
- [x] Document current backup and restore operations.
