# internal/db

Owner of SQLite access, migrations, transactions, and backup primitives.

## Responsibilities

- Open SQLite with safe defaults.
- Enable WAL mode when appropriate.
- Run schema migrations.
- Provide typed repositories for memory, chunks, embeddings, events, policies, and access logs.
- Expose transaction helpers.
- Implement SQLite-safe backup and restore primitives.

## Non-Responsibilities

- MCP protocol handling.
- Search ranking policy outside SQL query support.
- Lifecycle decisions.
- Raw SQL access for clients.

## Current Implementation

- Uses `modernc.org/sqlite`.
- Opens a configured local database path.
- Creates the parent data directory for normal filesystem paths.
- Enables foreign keys.
- Enables WAL mode.
- Sets a busy timeout.
- Applies embedded SQL migrations.
- Provides typed repositories for memory items, chunks, embeddings, events, retention policies, and access logs.
- Provides FTS5-backed and optional vector-assisted memory search with safe filters, snippets, and candidate ranking inputs.
- Provides a transaction helper that exposes transaction-bound repositories.
- Satisfies the memory service storage interface without exposing SQLite connection management to the domain service.

## Boundary

No upper layer should build ad hoc SQL for public request fields. Repository methods should validate supported query shapes.
