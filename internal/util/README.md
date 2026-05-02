# internal/util

Shared internal helpers may live here only when they are genuinely reusable and small.

## Guidelines

- Prefer package-local helpers first.
- Move code here only after at least two packages need it.
- Avoid broad, vague utility APIs.
- Keep helpers deterministic and easy to test.

## Candidate Helpers

- Time helpers used by lifecycle tests.
- Redaction helpers for logs.
- Small validation helpers shared by tools and services.

## Current Implementation

- `DecodeJSONObject` decodes optional JSON object inputs, rejects unknown fields, and rejects trailing JSON values. It is used by MCP params and tool arguments to keep validation behavior aligned.

## Non-Goals

This package should not become a dumping ground for business logic, storage logic, or protocol code.
