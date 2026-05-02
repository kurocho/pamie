# Testing Strategy

Pamie should use layered tests that match the risk of each package.

## Unit Tests

Use unit tests for configuration validation, auth decisions, request validation, ranking functions, lifecycle rule evaluation, and error mapping. Keep these tests deterministic and free of network or Docker requirements.

## Integration Tests

Use temporary SQLite databases for repository, migration, FTS5, backup, lifecycle, persistence, and search behavior. Tests must not require global state or developer-specific paths.

## Acceptance Tests

Acceptance tests under `internal/acceptance` start the real HTTP handler stack with SQLite-backed memory, MCP tools, resources, Bearer auth, and readiness checks. They exercise `/health`, `/ready`, authenticated `/mcp`, unauthenticated `/mcp`, malformed auth, malformed JSON-RPC, and the full save/get/search/update/pin/delete/recent/stats MCP tool flow.

## Security Tests

Security coverage includes missing tokens, malformed tokens, invalid tokens, scope failures, deletion authorization, rate limits, attempts to inject unsupported fields, and fuzz seeds for auth header parsing.

## Fuzz Tests

Seeded fuzz tests cover Bearer auth header parsing, MCP JSON-RPC request parsing, metadata filter validation, and FTS query sanitization. Normal `go test ./...` runs the seed corpus. Run targeted fuzzing when changing these boundaries:

```sh
go test ./internal/auth -fuzz=FuzzBearerAuthenticatorAuthHeader -fuzztime=30s
go test ./internal/mcp -fuzz=FuzzParseRequest -fuzztime=30s
go test ./internal/db -fuzz=FuzzBuildFTSQuery -fuzztime=30s
go test ./internal/db -fuzz=FuzzValidateMetadataFilter -fuzztime=30s
```

## CI

CI runs formatting checks, `go test ./...`, `go test -race ./...`, `go vet ./...`, a binary build, and Docker image build validation. Coverage reporting and integration matrix jobs can be added when release cadence requires them.

## Docker Smoke

Docker smoke coverage is opt-in and must not be required for normal `go test ./...`. Run it on machines with Docker and curl available:

```sh
make docker-smoke
```

The script builds a local image, starts a temporary container on an ephemeral localhost port, checks `/health`, `/ready`, authenticated `/mcp`, and verifies unauthenticated `/mcp` is rejected.
