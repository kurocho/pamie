# internal/audit

Owner of structured security audit event logging.

## Responsibilities

- Define audit event shape.
- Record authentication, authorization, tool, resource, and rate-limit events.
- Keep token values and Authorization headers out of logs.
- Provide small test-friendly interfaces.

## Non-Responsibilities

- Tamper-proof log storage.
- Database event history.
- Token authentication.

## Current Implementation

Audit events are emitted through `slog` with stable fields such as `audit_type`, `outcome`, `token_id`, `action`, and `subject`.

Token IDs are non-secret attribution labels. Raw Bearer tokens must never be added to audit fields.
