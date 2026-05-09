# internal/resources

Owner of MCP resource definitions.

## Responsibilities

- Define read-only MCP resources.
- Expose safe service status and memory summaries.
- Avoid secrets and raw database internals.
- Keep resource output stable and documented.

## Non-Responsibilities

- Mutating memory state.
- Authentication implementation.
- Backup/export execution.

## Implemented Resources

- `pamie://status`
- `pamie://guide`
- `pamie://memory/stats`

`pamie://guide` provides read-only Markdown onboarding guidance for agents and clients, including the title/keywords-only vector indexing policy and keyword examples for long notes. `pamie://memory/stats` requires `stats:read`. `pamie://status` and `pamie://guide` are available to authenticated clients without an additional scope.

## Boundary

Resources should be read-only. Mutations belong in tools with explicit validation and authorization.
