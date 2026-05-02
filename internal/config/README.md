# internal/config

Owner of process configuration.

## Responsibilities

- Load settings from flags, environment, and optional config files.
- Validate required values before server startup.
- Define defaults for listen address, data directory, database path, logging, and auth mode.
- Keep configuration structs stable and easy to test.

## Non-Responsibilities

- Opening the database.
- Starting the HTTP server.
- Reading secrets from arbitrary external secret managers.

## Current Implementation

Configuration is loaded once during startup from environment variables and command-line flags. Environment values set defaults and explicit flags take precedence.

Current environment variables:

- `PAMIE_ADDR`
- `PAMIE_TOKEN`
- `PAMIE_TOKEN_ID`
- `PAMIE_TOKEN_SCOPES`
- `PAMIE_DATA_DIR`
- `PAMIE_DB_PATH`
- `PAMIE_LOG_LEVEL`
- `PAMIE_READ_HEADER_TIMEOUT`
- `PAMIE_SHUTDOWN_TIMEOUT`
- `PAMIE_MCP_RATE_LIMIT`
- `PAMIE_MCP_RATE_BURST`
- `PAMIE_LIFECYCLE_WORKER_ENABLED`
- `PAMIE_LIFECYCLE_INTERVAL`
- `PAMIE_LIFECYCLE_BATCH_SIZE`
- `PAMIE_LIFECYCLE_RUN_ON_START`
- `PAMIE_LIFECYCLE_STARTUP_DELAY`
- `PAMIE_VECTOR_SEARCH_ENABLED`
- `PAMIE_VECTOR_BACKEND`
- `PAMIE_VECTOR_PROVIDER`
- `PAMIE_VECTOR_MODEL`
- `PAMIE_VECTOR_DIMENSIONS`
- `PAMIE_VECTOR_OLLAMA_URL`
- `PAMIE_VECTOR_OLLAMA_KEEP_ALIVE`

Current flags:

- `--version`
- `--addr`
- `--token`
- `--token-id`
- `--token-scopes`
- `--data-dir`
- `--db-path`
- `--log-level`
- `--read-header-timeout`
- `--shutdown-timeout`
- `--mcp-rate-limit`
- `--mcp-rate-burst`
- `--lifecycle-worker`
- `--lifecycle-interval`
- `--lifecycle-batch-size`
- `--lifecycle-run-on-start`
- `--lifecycle-startup-delay`
- `--vector-search`
- `--vector-backend`
- `--vector-provider`
- `--vector-model`
- `--vector-dimensions`
- `--vector-ollama-url`
- `--vector-ollama-keep-alive`

`PAMIE_TOKEN_SCOPES` / `--token-scopes` accepts `all` or a comma-separated list such as `memory:read,memory:write,stats:read`. `PAMIE_MCP_RATE_LIMIT=0` disables the in-process `/mcp` rate limiter.

The lifecycle worker is disabled by default. `PAMIE_LIFECYCLE_WORKER_ENABLED=true` / `--lifecycle-worker=true` enables scheduled lifecycle evaluation. The default interval is `1h`, default batch size is `500`, run-on-start is false, and startup delay is `0s`.

Vector search is disabled by default. `PAMIE_VECTOR_SEARCH_ENABLED=true` / `--vector-search=true` enables local embedding storage and hybrid ranking. `PAMIE_VECTOR_BACKEND` / `--vector-backend` accepts `auto`, `sqlite-json`, or `sqlite-vec`. Built-in providers are `local-hash` and `ollama`; default dimensions are `384`.

## Boundary

Configuration should be parsed once during startup and passed into constructors. Runtime packages should not read environment variables directly.
