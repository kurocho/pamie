# internal/auth

Owner of authentication and authorization decisions.

## Responsibilities

- Validate Bearer tokens for protected HTTP endpoints.
- Hide token comparison and token lookup details from handlers.
- Provide token principals, IDs, and scopes such as `memory:read`, `memory:write`, and `memory:admin`.
- Avoid logging raw tokens.
- Return stable auth errors that HTTP and MCP layers can map safely.

## Non-Responsibilities

- HTTP routing.
- MCP tool behavior.
- Memory storage.
- SQLite persistence details for token metadata.

## Current Implementation

The current implementation validates either a bootstrap Bearer token from process configuration or persistent tokens from a dynamic token source. Persistent tokens store only salted hashes, token IDs, scopes, creation timestamps, last-used timestamps, optional expiration, and revoked state. If no token is configured, protected endpoints reject requests.

Authenticated requests receive a request-context principal containing a non-secret token ID and scope set. The middleware returns generic unauthorized responses for missing, malformed, and invalid tokens. It emits audit events with token IDs and failure reasons, but not token values.

Implemented scopes:

- `memory:read`
- `memory:write`
- `memory:delete`
- `memory:admin`
- `backup:read`
- `stats:read`

## Boundary

This package owns token generation, hashing, verification, principal construction, and scope checks. Storage packages own durable persistence.
