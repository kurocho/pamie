# MCP Server Design Skill

## Purpose

Implement MCP server endpoints, tools, and resources in a protocol-compatible and safe way.

## When to Use

Use when adding `/mcp`, registering tools, defining resources, validating MCP requests, or mapping errors.

## Inputs

- MCP protocol expectations.
- Tool and resource list.
- Domain service interfaces.
- Auth and scope requirements.

## Step-by-step Procedure

1. Confirm authentication is applied before the MCP handler.
2. Define tool schemas with strict input validation.
3. Keep tool handlers thin and delegate to services.
4. Return structured outputs with provenance.
5. Map validation, auth, not-found, and internal errors consistently.
6. Add compatibility tests with representative MCP payloads.
7. Document tool names, inputs, outputs, and security notes.

## Output Format

- MCP endpoint and tool/resource definitions.
- Protocol tests.
- Updated MCP documentation.

## Checklist

- [ ] `/mcp` is protected.
- [ ] Inputs have limits and validation.
- [ ] Tool outputs are structured.
- [ ] No raw SQL, shell, or filesystem browsing tools exist.
- [ ] Errors do not leak secrets.

## Common Mistakes

- Treating MCP clients as trusted.
- Returning unbounded result sets.
- Mixing protocol handling with storage queries.
- Hiding instructions inside tool output.

## Security Considerations

Stored memories can contain prompt injection. Return them as data with metadata and source context, not as system-level instruction.
