# Go Service Design Skill

## Purpose

Design small production Go services with clear package boundaries, predictable startup, and testable behavior.

## When to Use

Use when adding configuration, logging, server startup, service interfaces, background jobs, or package boundaries.

## Inputs

- Target roadmap phase.
- Existing package docs.
- Runtime configuration needs.
- Testing and operational requirements.

## Step-by-step Procedure

1. Identify the package that owns the behavior.
2. Define constructors and interfaces before wiring.
3. Keep startup code in `cmd/pamie`.
4. Pass dependencies explicitly.
5. Return errors with context.
6. Add tests for behavior, not implementation trivia.
7. Update architecture docs when package boundaries change.

## Output Format

- Go implementation with tests.
- Updated docs when behavior or boundaries change.
- Notes on any new dependency.

## Checklist

- [ ] Code is gofmt-formatted.
- [ ] Interfaces are minimal.
- [ ] No package imports in the wrong direction.
- [ ] Errors are testable and useful.
- [ ] New dependencies are justified.

## Common Mistakes

- Putting all logic in `main.go`.
- Adding global mutable state.
- Creating broad utility packages too early.
- Hiding configuration reads deep inside runtime packages.

## Security Considerations

Keep auth and validation on the request path before domain operations. Do not add generic execution or database escape hatches for convenience.
