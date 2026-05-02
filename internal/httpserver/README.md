# internal/httpserver

Owner of HTTP routing, middleware wiring, and graceful shutdown.

## Responsibilities

- Build the HTTP router.
- Expose `/health`, `/ready`, and `/mcp`.
- Apply request size limits, timeouts, auth middleware, rate limiting, and logging.
- Coordinate graceful shutdown.
- Keep health responses minimal and non-sensitive.

## Non-Responsibilities

- MCP protocol semantics.
- Token storage details.
- SQLite schema.
- Memory lifecycle policy.

## Current Implementation

Implemented routes:

- `GET /health`
- `GET /ready`
- `POST /mcp`

`/mcp` is authenticated, scope-checked at the MCP layer, and can be protected by a configurable per-client rate limiter.

The server supports graceful shutdown through context cancellation, which `cmd/pamie` wires to SIGINT and SIGTERM.

## Boundary

Handlers should delegate to interfaces. HTTP code should not directly query SQLite or implement memory business rules.
