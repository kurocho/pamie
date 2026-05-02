# Repository Bootstrap Skill

## Purpose

Create and maintain the initial Pamie repository structure without prematurely implementing product features.

## When to Use

Use when creating new scaffolding, adding missing top-level files, or realigning the repository with the documented layout.

## Inputs

- Desired repository tree.
- Project name, binary name, and module path.
- Required docs and CI expectations.
- Implementation boundaries.

## Step-by-step Procedure

1. Confirm the repository root and existing files.
2. Create only the requested directories and scaffold files.
3. Add minimal compiling Go code.
4. Add Makefile and CI commands that match local commands.
5. Add documentation that explains future phases and current non-implementation.
6. Run formatting, tests, vet, and build.
7. Summarize what exists and what is intentionally absent.

## Output Format

- Repository files committed or ready for review.
- Short summary of created structure.
- Verification command results.

## Checklist

- [ ] `go test ./...` passes.
- [ ] `go vet ./...` passes.
- [ ] `go build ./cmd/pamie` passes.
- [ ] Docs state current status.
- [ ] No real database, MCP, auth, or search implementation was added during bootstrap.

## Common Mistakes

- Adding dependencies before they are needed.
- Implementing a partial server that is not tested.
- Leaving empty placeholder docs.
- Forgetting CI or Makefile parity.

## Security Considerations

Bootstrap must preserve the documented security boundary: no raw SQL tools, no shell tools, no unauthenticated public MCP endpoint.
