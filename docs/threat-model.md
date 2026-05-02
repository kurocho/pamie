# Threat Model

This document summarizes expected threats and controls. It should be updated whenever Pamie adds new tools, endpoints, auth modes, backup behavior, or deployment options.

## Assets

- Memory database.
- Backups and exports.
- Bearer tokens.
- Operator configuration.
- Audit logs.
- MCP tool behavior.

## Trust Boundaries

- Network boundary between clients and `/mcp`.
- Auth middleware boundary before MCP handlers.
- Tool validation boundary before domain services.
- Storage boundary before SQLite.
- Prompt boundary between stored memory text and agent instructions.

## Key Threats

- Unauthorized access to memory data.
- Token theft or logging.
- Overbroad tokens that allow mutation, deletion, or backup access unnecessarily.
- Prompt injection from stored memories.
- Excessive deletion or mutation through agent tools.
- Denial of service through large requests or expensive search.
- Unsafe backup exposure.
- Accidental raw SQL or shell execution capability.

## Current Controls

- Bearer auth for `/mcp`.
- In-memory token hashing and constant-time token hash comparison.
- Token IDs for audit attribution without logging token values.
- MCP tool and resource scope enforcement.
- Configurable per-client `/mcp` rate limiting.
- HTTPS requirement for public deployments.
- No raw SQL tools.
- No shell execution tools.
- Structured validation for all MCP inputs.
- Audit logging for authentication, MCP tool calls, resource reads, and rate-limit blocks.
- Retention policies that make deletion explicit.
- Soft deletion for current tool and lifecycle deletion paths.

## Scope Model

Current scopes are:

- `memory:read`
- `memory:write`
- `memory:delete`
- `stats:read`
- `backup:read`
- `memory:admin`

Use the smallest scope set possible for each MCP client. `memory:admin` is intentionally broad and should be reserved for trusted operators or local development.

## Prompt Injection Review

Stored memories remain untrusted data. Pamie returns structured content with memory IDs, source, timestamps, metadata, tier, pinned state, snippets, and score details. It does not transform retrieved memory text into hidden system instructions.

MCP clients must still treat retrieved text as untrusted context. A malicious memory can ask an agent to reveal secrets, ignore instructions, call delete tools, or exfiltrate data. Pamie reduces this risk by keeping administrative capabilities narrow, enforcing scopes, requiring deletion confirmation, and avoiding raw SQL or shell tools.

## Residual Risks

- A stolen token grants the scopes assigned to that token until it is rotated.
- Rate limiting is in-process and per observed client address; deployments behind proxies may need upstream limits.
- Backup and restore are local operator CLI workflows only; remote backup/export remains out of MCP scope and would need explicit backup scopes and audit coverage if added later.
- Audit logs are structured application logs, not tamper-proof storage.
- `/health` and `/ready` are intentionally unauthenticated and must remain low-detail.

## Review Cadence

Review this model at the end of each roadmap phase, before public release, and after adding any new MCP tool.
