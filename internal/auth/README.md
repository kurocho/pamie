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
- Token generation UX.

## Current Implementation

The current implementation validates one configured Bearer token. Token comparison hashes the provided token and uses constant-time comparison against the configured token hash. If no token is configured, protected endpoints reject requests.

Authenticated requests receive a request-context principal containing a non-secret token ID and scope set. The middleware returns generic unauthorized responses for missing, malformed, and invalid tokens. It emits audit events with token IDs and failure reasons, but not token values.

Implemented scopes:

- `memory:read`
- `memory:write`
- `memory:delete`
- `memory:admin`
- `backup:read`
- `stats:read`

## Boundary

Later implementations should support multiple active tokens, persistent hashed token storage, rotation, revocation, and last-used timestamps.
