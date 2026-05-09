# internal/embedding

Owner of local embedding provider interfaces and built-in local providers.

## Responsibilities

- Define the `Provider` interface used by the memory service.
- Provide deterministic local test embeddings through `local-hash`.
- Provide local semantic embeddings through a locally running Ollama server.
- Validate provider dimensions before vectors are written to storage.
- Provide optional local Ollama autostart process management for `ollama serve`.

## Non-Responsibilities

- Search ranking.
- SQLite vector indexing.
- Hosted embedding APIs.
- MCP request parsing.

## Current Providers

- `local-hash`: dependency-free lexical baseline used for tests and deterministic local operation.
- `ollama`: calls `POST /api/embed` on `PAMIE_VECTOR_OLLAMA_URL` / `--vector-ollama-url`, defaulting to `http://127.0.0.1:11434`.

Ollama autostart is disabled by default. When enabled, Pamie starts `ollama serve` only if the configured URL is unavailable and stops only the process it started.

## Boundary

Provider implementations treat embedding input as untrusted data. The memory service supplies only title/keywords embedding documents; it does not send memory bodies to providers. Providers return numeric vectors only; policy and ranking decisions stay in `internal/memory`, `internal/db`, and `internal/search`.
