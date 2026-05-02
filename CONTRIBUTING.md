# Contributing

Pamie is currently in scaffold stage. Contributions should keep the implementation small, documented, and security-conscious.

## Coding Standards

- Prefer simple Go packages with clear ownership.
- Keep exported APIs small and documented when they become public across packages.
- Return errors with enough context for operators and tests.
- Keep side effects behind interfaces where tests need control.
- Do not introduce heavy dependencies without documenting the reason.
- Treat stored memory content as untrusted data.

## Go Formatting

Run:

```sh
make fmt
```

All Go code must be formatted with `gofmt`.

## Testing Expectations

Run:

```sh
make fmt
make test
go test -race ./...
make vet
make build
```

Storage, lifecycle, search, MCP, and HTTP acceptance behavior should include deterministic tests. Integration tests should use temporary directories and temporary SQLite databases.

Useful focused commands:

```sh
go test ./internal/acceptance
go test ./internal/memory -run 'Persistence|Lifecycle|Concurrent'
go test ./internal/db -run 'Search'
go test ./internal/auth -run FuzzBearerAuthenticatorAuthHeader
go test ./internal/mcp -run FuzzParseRequest
```

Run opt-in Docker smoke coverage only when Docker is available:

```sh
make docker-smoke
```

## Pull Request Checklist

- [ ] The change is scoped to one phase or one clear behavior.
- [ ] Tests cover the changed behavior.
- [ ] `go test ./...` passes.
- [ ] `go vet ./...` passes.
- [ ] The binary builds.
- [ ] Documentation changed when architecture, tools, security, or operations changed.
- [ ] No raw SQL or shell execution tool surface was added.

## Licensing

By contributing to Pamie, you agree that your contribution is licensed under the GNU Affero General Public License v3.0 only (`AGPL-3.0-only`), the same license as the project.

## Documentation Requirements

Update the relevant docs when behavior changes:

- `ARCHITECTURE.md` for package, runtime, and system design changes.
- `SECURITY.md` for auth, public exposure, or data handling changes.
- `TASKS.md` when implementation work starts or completes.
- `DECISIONS.md` when a meaningful architectural decision is made.
- `docs/` pages for operator or protocol guidance.
