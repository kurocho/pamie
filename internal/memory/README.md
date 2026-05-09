# internal/memory

Owner of memory domain behavior.

## Responsibilities

- Define memory item domain types.
- Enforce tier semantics.
- Coordinate save, update, delete, pin, and retrieve behavior.
- Coordinate optional title/keywords embedding writes and backfill.
- Apply lifecycle decisions for promotion, demotion, archive, and deletion.
- Record memory events.

## Non-Responsibilities

- HTTP routing.
- MCP transport details.
- SQLite connection management.
- FTS5 or vector SQL query syntax.

## Current Implementation

The memory service depends on a narrow `Store` interface and coordinates typed storage repositories for:

- saving a memory with one initial searchable chunk;
- retrieving a memory and its chunks;
- FTS-backed and optional hybrid search with safe filters, snippets, depth controls, and score details;
- best-effort title/keywords embedding storage on save/update when vector search is enabled;
- repeatable missing/failed embedding backfill;
- updating mutable fields;
- conservative soft deletion;
- pinning and unpinning;
- recent memory listing;
- aggregate stats.
- lifecycle promotion, demotion, archive, policy deletion, pinned protection, and important-memory protection.

Scheduled lifecycle execution lives in `internal/lifecycle`, which calls this package through `RunLifecycle` and does not change the domain rules. Vector search is optional and disabled unless configured at startup. Full memory bodies remain the FTS5 source; embedding providers receive only the memory title and explicit keywords.

## Boundary

Memory service methods should operate on validated domain inputs and should treat stored memory text as untrusted content.

The package may depend on typed storage records and repository interfaces, but it must not depend on SQLite connection management, SQL strings, migrations, MCP transport types, or HTTP request types. New storage behavior should be added behind the service storage interface or in repositories, not by having domain logic build ad hoc queries.
