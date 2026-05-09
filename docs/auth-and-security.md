# Auth and Security

Pamie's public MCP endpoint must be authenticated. The current auth mechanism is Bearer token validation with persistent hashed tokens, token IDs, scopes, audit events, and `/mcp` rate limiting.

## Current Auth

The current implementation accepts multiple persistent tokens managed by `pamie token` and rejects unauthenticated requests to `/mcp`. Raw generated tokens are shown once, then only salted hashes are stored. Successful requests receive a principal containing a non-secret token ID and scope set, and persistent tokens get a last-used timestamp update.

Token commands:

- `pamie token`: rotate the default token and print the new secret once.
- `pamie token create --id <id> --scopes <scopes>`: create another client token.
- `pamie token list`: list token metadata without secrets.
- `pamie token revoke --id <id>`: disable a token.

Bootstrap environment configuration remains available:

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

## Hashing

Generated tokens use cryptographically secure randomness and are long enough that salted SHA-256 storage is practical for the generated-token path. Do not choose short human tokens for `PAMIE_TOKEN`. Add tamper-resistant audit storage where operators need stronger audit guarantees.

## Public Deployments

Use HTTPS through a reverse proxy. Do not expose `/mcp` on the public internet without Bearer auth. Avoid returning sensitive configuration in health or readiness responses.

## Stored Memory Risks

Stored memory may contain malicious text. Pamie should return memory content as structured data with provenance and avoid hidden prompt instructions in responses.
