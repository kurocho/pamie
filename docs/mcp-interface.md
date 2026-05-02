# MCP Interface

Pamie exposes memory capabilities through an MCP HTTP endpoint. The endpoint is narrow, typed, validated, and protected by Bearer authentication.

## Endpoint

- `POST /mcp`: MCP transport endpoint.

The current implementation supports a JSON-RPC 2.0 MCP subset:

- `initialize`
- `ping`
- `tools/list`
- `tools/call`
- `resources/list`
- `resources/read`
- `resources/templates/list`

Notifications without an `id` are accepted with an empty `202 Accepted` response.

The `initialize` result includes server instructions that briefly explain how an agent should use Pamie and points clients to the full `pamie://guide` resource.

## Tools

- `context_save`: store a new memory.
- `context_get`: retrieve a memory by ID.
- `context_search`: search memory by text and filters.
- `context_update`: update memory fields.
- `context_delete`: delete or mark a memory for deletion subject to policy.
- `context_pin`: pin or unpin important memory.
- `context_recent`: retrieve recent memories.
- `context_stats`: return aggregate memory statistics.

Tool calls return MCP content plus `structuredContent`. Retrieved memory output includes IDs, source, metadata, tier, pinned state, and timestamps. Stored memory text remains data and should not be treated as trusted instructions.

## Authorization

`/mcp` requires Bearer authentication. After authentication, tool and resource access is checked against token scopes:

- `memory:read`: `context_get`, `context_search`, and `context_recent`.
- `memory:write`: `context_save`, `context_update`, and `context_pin`.
- `memory:delete`: `context_delete`.
- `stats:read`: `context_stats` and `pamie://memory/stats`.
- `memory:admin`: allows all current operations.

Scope failures return a JSON-RPC error instead of calling the tool.

### `context_search`

`context_search` accepts:

- `query`: required keyword query.
- `tier`: optional memory tier.
- `pinned`: optional boolean.
- `metadata`: optional equality filters for simple scalar metadata values.
- `source`: optional exact source filter.
- `created_after` and `created_before`: optional RFC3339 creation bounds.
- `updated_after` and `updated_before`: optional RFC3339 update bounds.
- `depth`: optional `shallow`, `standard`, or `deep` candidate breadth.
- `include_deleted`: optional boolean.
- `limit`: optional maximum result count.

Results include `memory`, `memory_id`, `chunk_id`, `snippet`, `score`, and `score_details`. `score_details` exposes keyword, recency, tier, pinned, importance, and access components so agents can inspect why a result ranked highly.

## Resources

- `pamie://status`: read-only service status.
- `pamie://guide`: read-only Markdown guidance for using Pamie tools safely and effectively.
- `pamie://memory/stats`: read-only aggregate memory counts.

## Validation Rules

Tool inputs validate IDs, limits, filters, known fields, and deletion intent. `context_delete` requires `confirm: true` and performs a soft delete. Lifecycle policy deletion is separate and only applies when explicit retention policy rules allow it.

Search does not expose raw SQL or raw FTS syntax. Query text is converted into safe FTS terms, and metadata filters are limited to simple keys and scalar values.

Protocol errors are mapped to JSON-RPC error codes. Tool validation and not-found conditions are returned as MCP tool results with `isError: true`. Internal errors are not exposed with raw database details.

## Example Tool Call

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "context_save",
    "arguments": {
      "title": "Project decision",
      "body": "Pamie uses SQLite as the local source of truth.",
      "source": "operator"
    }
  }
}
```
