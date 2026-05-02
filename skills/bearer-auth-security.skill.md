# Bearer Auth Security Skill

## Purpose

Implement Bearer token auth, scopes, hashing, and public endpoint hardening.

## When to Use

Use when protecting endpoints, validating tokens, designing scopes, logging auth events, or reviewing public deployment readiness.

## Inputs

- Endpoint list.
- Token configuration.
- Scope model.
- Audit log requirements.
- Deployment assumptions.

## Step-by-step Procedure

1. Identify public and private endpoints.
2. Require Bearer auth for `/mcp`.
3. Reject missing, malformed, and invalid tokens consistently.
4. Avoid logging token values.
5. Add token IDs before audit logging token usage.
6. Add scopes for sensitive tools.
7. Test auth success, failure, and edge cases.

## Output Format

- Auth middleware or service.
- Scope checks.
- Auth tests.
- Security documentation updates.

## Checklist

- [ ] Tokens are not logged.
- [ ] Public MCP requests require auth.
- [ ] Scope failures are tested.
- [ ] HTTPS requirement is documented.
- [ ] Rate limiting plan exists.

## Common Mistakes

- Accepting tokens from query strings.
- Returning different detailed errors for invalid tokens.
- Logging request headers.
- Treating localhost assumptions as public deployment safety.

## Security Considerations

Use long random tokens. Hash tokens at rest when token persistence exists. Prefer token IDs in logs and audit events.
