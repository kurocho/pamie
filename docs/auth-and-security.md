# Auth and Security

Pamie's public MCP endpoint must be authenticated. The current auth mechanism is Bearer token validation with in-memory hashing, token IDs, scopes, audit events, and `/mcp` rate limiting.

## Current Auth

The current implementation accepts one configured token and rejects unauthenticated requests to `/mcp`. The configured token is hashed before comparison, and successful requests receive a principal containing a non-secret token ID and scope set.

Configuration:

- `PAMIE_TOKEN` / `--token`
- `PAMIE_TOKEN_ID` / `--token-id`
- `PAMIE_TOKEN_SCOPES` / `--token-scopes`
- `PAMIE_MCP_RATE_LIMIT` / `--mcp-rate-limit`
- `PAMIE_MCP_RATE_BURST` / `--mcp-rate-burst`

Scopes:

- `memory:read`
- `memory:write`
- `memory:delete`
- `stats:read`
- `backup:read`
- `memory:admin`

## Hardening Path

- Add persistent hashed token storage.
- Support multiple active tokens.
- Support token rotation.
- Support token revocation and last-used timestamps.
- Add tamper-resistant audit storage where operators need stronger audit guarantees.

## Public Deployments

Use HTTPS through a reverse proxy. Do not expose `/mcp` on the public internet without Bearer auth. Avoid returning sensitive configuration in health or readiness responses.

## Stored Memory Risks

Stored memory may contain malicious text. Pamie should return memory content as structured data with provenance and avoid hidden prompt instructions in responses.
