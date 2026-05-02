# Architecture Overview

Pamie is planned as a single Go service with explicit package boundaries. The service exposes HTTP endpoints, validates authentication, handles MCP requests, coordinates memory operations, and persists data in SQLite.

## Main Components

- `cmd/pamie`: process startup, configuration wiring, and version output.
- `internal/config`: flags, environment, validation, and defaults.
- `internal/httpserver`: HTTP routing, middleware, graceful shutdown, health checks.
- `internal/auth`: Bearer token validation, token principals, scopes, and auth audit events.
- `internal/mcp`: MCP transport, tool registration, resource registration, and protocol errors.
- `internal/memory`: memory service, tier rules, lifecycle orchestration, and domain events.
- `internal/db`: SQLite connection, migrations, repositories, transactions, and backups.
- `internal/search`: FTS5 search, ranking, snippets, filters, and vector interface.
- `internal/tools`: MCP tool definitions and validation.
- `internal/resources`: MCP resource definitions.

## Dependency Direction

HTTP and MCP packages should depend on service interfaces, not SQLite details. Storage packages should not depend on MCP. The memory service should coordinate repositories and search interfaces through narrow contracts.

## Operational Model

The default production model is one process and one SQLite database. Operators should be able to back up the database, export memories, and restore into a new Pamie instance without hidden external state.
