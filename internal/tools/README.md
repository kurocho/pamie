# internal/tools

Owner of MCP tool definitions and tool-level validation.

## Responsibilities

- Define tool names, descriptions, input schemas, and output shapes.
- Validate user-provided tool inputs before calling services.
- Keep tool behavior narrow and documented.
- Add tests for each tool contract.

## Non-Responsibilities

- MCP wire transport.
- SQLite access.
- Search implementation.
- Token storage.

## Implemented Tools

- `context_save`
- `context_get`
- `context_search`
- `context_update`
- `context_delete`
- `context_pin`
- `context_recent`
- `context_stats`

Deletion is currently conservative soft deletion. Search uses FTS5 keyword search and optional local vector ranking through the memory service with safe filters, snippets, depth controls, and explainable score details. Lifecycle behavior supports deterministic promotion, demotion, archive, policy-controlled deletion, and the opt-in scheduled background runner.

Tool scopes:

- `context_get`, `context_search`, `context_recent`: `memory:read`.
- `context_save`, `context_update`, `context_pin`: `memory:write`.
- `context_delete`: `memory:delete`.
- `context_stats`: `stats:read`.

Tool argument decoding uses the shared strict JSON object helper from `internal/util`, so unknown fields and trailing JSON values are rejected consistently with MCP params.

## Boundary

Tools depend on a memory service interface and memory domain DTOs. They should not import `internal/db`, build storage filters directly, or expose SQLite, filesystem, shell, or raw administrative operations as tools.
