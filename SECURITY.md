# Security

Pamie is intended to expose a memory service to MCP agents. That makes its security boundary important: agents can be powerful, stored memory can be adversarial, and public HTTP endpoints are exposed to normal web threats.

## Threat Model Summary

Primary assets:

- Bearer tokens and token metadata.
- Stored memories and metadata.
- SQLite database files and backups.
- Operator configuration.
- Audit logs.

Primary threats:

- Unauthorized MCP access.
- Token leakage.
- Prompt injection through stored memory.
- Data exfiltration through overly broad tools.
- Destructive memory deletion.
- Raw SQL or shell execution through agent-accessible tools.
- Public endpoint abuse and denial of service.

## Authentication Requirements

- Public MCP access must require Bearer authentication.
- The current implementation protects `/mcp` with persistent hashed tokens managed by `pamie token`.
- `PAMIE_TOKEN` / `--token` remains available as a bootstrap or emergency token mode.
- Persistent token records store only salted token hashes, token IDs, scopes, creation timestamps, last-used timestamps, optional expiration, and revoked state.
- `PAMIE_TOKEN_ID` / `--token-id` configures a non-secret token identifier for the bootstrap token in audit logs.
- `PAMIE_TOKEN_SCOPES` / `--token-scopes` configures comma-separated scopes for the bootstrap token. The default is `all` for development compatibility.
- If no token is configured, `/mcp` rejects requests instead of allowing anonymous access.
- Tokens must not be logged.
- `pamie token` prints generated raw tokens only once. Run `pamie token` again to rotate the default token.
- Token comparison must avoid obvious timing leaks where practical.

Implemented scopes:

- `memory:read`: `context_get`, `context_search`, and `context_recent`.
- `memory:write`: `context_save`, `context_update`, and `context_pin`.
- `memory:delete`: `context_delete`.
- `stats:read`: `context_stats` and `pamie://memory/stats`.
- `backup:read`: reserved for future authenticated remote backup/export features. Current backup and restore workflows are local operator CLI commands and are not exposed through MCP.
- `memory:admin`: allows all current scopes.

## HTTPS Requirement

Public deployments must use HTTPS, usually through a reverse proxy such as Caddy, nginx, Traefik, or a platform load balancer. Running unauthenticated or plain HTTP Pamie on the public internet is unsupported.

## No Raw SQL Tools

Pamie must not expose a tool that accepts arbitrary SQL. All data access should go through typed memory operations, validated filters, and repository methods.

The current MCP tools expose only purpose-built memory operations. Search sanitizes user text before using FTS5 and does not expose raw FTS query syntax.

## No Shell Tools

Pamie must not expose shell command execution. Backup and maintenance behavior should be implemented as explicit commands or internal operations with fixed behavior.

The current MCP surface contains no shell execution tools.

## Stored Content Is Untrusted

Memory content can contain malicious instructions or stale claims. MCP responses should make it clear that retrieved memories are data, not trusted instructions. Future ranking and summarization layers must preserve source metadata and avoid silently elevating memory content to policy.

## Prompt Injection Risks

Agents may retrieve a memory that says to ignore instructions, reveal secrets, delete data, or call dangerous tools. Pamie cannot fully solve prompt injection for clients, but it can reduce risk by:

- returning structured results with provenance;
- avoiding hidden instructions in tool output;
- labeling retrieved content as user or memory data;
- avoiding raw administrative capabilities in MCP tools;
- supporting pinned and trusted metadata without treating text itself as trusted.

## Audit Logging Plan

Structured audit logging is implemented for authentication, MCP tool calls, MCP resource reads, rate-limit blocks, and local backup/restore operator commands. Current request logs intentionally avoid headers and token values. Successful persistent-token authentication updates the token's last-used timestamp.

Memory mutation and lifecycle events are recorded in `memory_events`, including lifecycle promotion, demotion, archive, and policy deletion. This is useful for explainability and deletion accountability, but it is not a complete security audit log.

Policy deletion currently means soft deletion through `deleted_at`; physical purging is not implemented.

Audit logs should record security-relevant events:

- authentication successes and failures;
- token use by token ID, never token value;
- memory creation, update, delete, pin, read, search, recent, and stats tool calls;
- lifecycle deletion;
- backup, restore validation, and restore commit operations;
- rate limit events.

Logs must not include full Bearer tokens.

## Backup and Export Artifacts

SQLite backups contain memory bodies, metadata, lifecycle history, retention policies, access logs, and hashed token metadata. NDJSON exports contain memory data and access logs, but not raw Bearer tokens. Pamie does not store raw tokens; access logs and token records can still contain non-secret token IDs that reveal operational context.

Protect backup and export artifacts at least as strictly as the live database:

- write them outside the Pamie data directory;
- restrict file and directory permissions;
- encrypt artifacts before moving them to shared or remote storage;
- validate restores with `pamie restore --dry-run` before committing;
- delete expired artifacts according to an explicit retention policy.

## Rate Limiting

`/mcp` has configurable per-client rate limiting.

- `PAMIE_MCP_RATE_LIMIT` / `--mcp-rate-limit`: requests per minute. `0` disables rate limiting.
- `PAMIE_MCP_RATE_BURST` / `--mcp-rate-burst`: burst size.

Defaults are intended for conservative development and small deployments. Public deployments should tune limits for expected clients and reverse-proxy behavior.

## Public Hosting Checklist

- [ ] HTTPS is enabled.
- [x] Bearer auth is required for `/mcp`.
- [ ] Tokens are long, random, and stored securely outside command history.
- [x] Logs do not contain tokens or raw secrets.
- [x] Rate limiting is enabled or deliberately disabled only behind equivalent upstream protection.
- [ ] Backups and exports are encrypted or access-controlled.
- [ ] File permissions restrict database access.
- [ ] `/health` and `/ready` do not leak sensitive data.
- [x] Raw SQL and shell tools are unavailable.
- [x] Threat model has been reviewed after each major feature.
