# Testing Go Services Skill

## Purpose

Test Go services and storage-backed behavior with deterministic, maintainable coverage.

## When to Use

Use when adding package tests, integration tests, acceptance tests, fixtures, or CI changes.

## Inputs

- Acceptance criteria.
- Package interfaces.
- Expected errors.
- Storage and network boundaries.

## Step-by-step Procedure

1. Test pure domain behavior first.
2. Use table-driven tests for validation and rules.
3. Use temporary directories and databases for integration tests.
4. Avoid real network calls in default tests.
5. Add fixtures only when they improve readability.
6. Run `go test ./...`, `go vet ./...`, and build.
7. Update CI when new required checks are added.

## Output Format

- Test files.
- Fixtures.
- CI updates if needed.
- Summary of covered behavior and remaining gaps.

## Checklist

- [ ] Tests are deterministic.
- [ ] Tests do not need developer-specific paths.
- [ ] Temporary data is isolated.
- [ ] Security edge cases are covered.
- [ ] CI runs the relevant checks.

## Common Mistakes

- Testing only happy paths.
- Depending on test order.
- Using real user data.
- Hiding flaky tests behind sleeps.

## Security Considerations

Never use real tokens or real memory data in tests. Include tests for auth failures, deletion authorization, and untrusted content handling.
