# internal/mcp

Owner of MCP protocol integration.

## Responsibilities

- Implement the MCP HTTP transport endpoint.
- Register tools and resources.
- Validate protocol-level requests.
- Map service errors to MCP-compatible errors.
- Keep compatibility behavior documented and tested.

## Non-Responsibilities

- Raw memory ranking logic.
- SQLite migrations.
- Token hashing.
- HTTP listener lifecycle.

## Current Implementation

The package implements a minimal JSON-RPC 2.0 MCP HTTP endpoint with:

- `initialize`
- `ping`
- `tools/list`
- `tools/call`
- `resources/list`
- `resources/read`
- `resources/templates/list`

The `initialize` response includes concise usage instructions so first-time clients can discover how Pamie should be used before calling tools. The longer guide is exposed as the `pamie://guide` resource.

The handler is intentionally small and depends on tool and resource registry interfaces.

JSON-RPC params are decoded with the shared strict JSON object helper from `internal/util`, matching tool argument validation for unknown fields and trailing JSON values.

## Boundary

MCP should expose purpose-built memory operations only. It must not expose raw SQL, shell execution, local file browsing, or arbitrary administrative primitives.
