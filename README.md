<p align="center">
  <img src="docs/assets/pamie_logo.png" alt="Pamie logo" width="560">
</p>

# Pamie

Self-hosted long-term memory for MCP agents.

Website: <https://pamie.io>

Docker Hub: <https://hub.docker.com/repository/docker/kurocho/pamie/general>

Pamie is a self-hosted, local-first, provider-independent long-term memory server for MCP agents. It runs as a single Go binary exposing an MCP HTTP endpoint protected by Bearer token authentication, with SQLite and FTS5 as durable local storage. Its memory lifecycle is inspired by Elasticsearch ILM and human memory: fresh memories are fast and prominent, old memories remain available, important memories stay easy to retrieve, and access patterns can promote useful memories back into higher tiers.

## Problem

MCP agents can act on tools, but many deployments still lack a durable memory layer that is:

- owned by the operator instead of an AI provider;
- searchable across sessions and agents;
- safe enough to expose as a narrow MCP tool surface;
- durable by default, with explicit retention instead of accidental deletion;
- useful for both fresh context and older archived knowledge.

Pamie is designed to provide that missing long-term memory service.

## Core Principles

- Self-hosted: operators run and control their own memory server.
- Local-first: the SQLite database is the source of truth.
- Provider-independent: no required model vendor, embedding vendor, or hosted database.
- Durable memory: memories do not disappear unless a retention policy permits deletion.
- Tiered retrieval: working, hot, warm, cold, and archive tiers guide ranking and lifecycle.
- MCP-native: agents interact through MCP tools and resources, not private SDK assumptions.
- Safe tool surface: tools expose memory operations, not arbitrary backend access.
- No raw SQL exposure: clients must not execute SQL through Pamie.
- No shell execution exposure: clients must not execute commands through Pamie.

## Planned Architecture

```text
MCP client / agent
      |
      | HTTPS + Bearer token
      v
Pamie Go binary
  +-- HTTP server
  |   +-- /health
  |   +-- /ready
  |   +-- /mcp
  |
  +-- Auth middleware
  +-- MCP tool and resource handlers
  +-- Memory engine
  |   +-- tiering
  |   +-- lifecycle jobs
  |   +-- retention policy checks
  |
  +-- Search engine
  |   +-- SQLite FTS5
  |   +-- metadata filters
  |   +-- recency and tier boosts
  |   +-- optional local vector search
  |
  +-- SQLite database
      +-- memory items
      +-- chunks
      +-- events
      +-- policies
      +-- access log
```

## Install

Build and install the local `pamie` command:

```sh
./scripts/install-local.sh
```

The script builds `./cmd/pamie`, installs it to `~/.local/bin/pamie` by default, and adds that directory to your shell profile if it is missing from `PATH`.

Reload your shell after installation:

```sh
source ~/.zshrc
rehash
pamie --version
```

Choose a different install directory when needed:

```sh
./scripts/install-local.sh --install-dir "$HOME/bin"
```

## Run Locally

Run the installed command:

```sh
PAMIE_TOKEN=dev-token \
PAMIE_TOKEN_ID=dev \
PAMIE_TOKEN_SCOPES=all \
pamie --addr 127.0.0.1:8080 --data-dir ./data
```

Or run directly from source without installing:

```sh
PAMIE_TOKEN=dev-token \
PAMIE_TOKEN_ID=dev \
PAMIE_TOKEN_SCOPES=all \
go run ./cmd/pamie --addr 127.0.0.1:8080 --data-dir ./data
```

Check liveness and readiness:

```sh
curl http://127.0.0.1:8080/health
curl http://127.0.0.1:8080/ready
```

List MCP tools through the protected endpoint:

```sh
curl -i -X POST http://127.0.0.1:8080/mcp \
  -H 'Authorization: Bearer dev-token' \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'
```

If no token is configured, `/mcp` rejects all requests.

Version output:

```sh
go run ./cmd/pamie --version
```

Operator backup and restore:

```sh
pamie backup --db-path data/pamie.db --out backup.db
pamie restore --db-path restored.db --in backup.db --dry-run
```

Portable NDJSON backup and restore:

```sh
pamie backup --format ndjson --db-path data/pamie.db --out backup.ndjson
pamie restore --format ndjson --db-path restored.db --in backup.ndjson --dry-run
```

Optional vector search and embedding backfill:

```sh
pamie \
  --addr 127.0.0.1:8080 \
  --data-dir ./data \
  --vector-search \
  --vector-backend auto \
  --vector-provider ollama \
  --vector-model embeddinggemma \
  --vector-dimensions 384

pamie embeddings backfill --db-path data/pamie.db --provider ollama --model embeddinggemma --dimensions 384 --backend auto --limit 500
```

Vector search is off by default. Keep it disabled with the default settings, or pass `--vector-search=false` to override an environment setting. The `ollama` provider expects a local Ollama server. Use `local-hash` for dependency-free deterministic test embeddings.

## Docker

Published images live at <https://hub.docker.com/repository/docker/kurocho/pamie/general>.

Build the image:

```sh
docker build --build-arg VERSION=dev -t pamie:dev .
```

Run the container with a persistent Docker volume:

```sh
docker volume create pamie-data
export PAMIE_TOKEN="$(openssl rand -hex 32)"
docker run --rm \
  --name pamie \
  -p 127.0.0.1:8080:8080 \
  -v pamie-data:/data \
  -e PAMIE_TOKEN="$PAMIE_TOKEN" \
  -e PAMIE_TOKEN_ID=local \
  -e PAMIE_TOKEN_SCOPES=all \
  pamie:dev
```

Check the running container:

```sh
curl http://127.0.0.1:8080/health
curl http://127.0.0.1:8080/ready
```

Run an operator backup from the image:

```sh
mkdir -p backups
docker run --rm \
  -v pamie-data:/data \
  -v "$PWD/backups:/backup" \
  pamie:dev \
  backup --db-path /data/pamie.db --out /backup/pamie.db
```

## Compose

Run with Docker Compose:

```sh
export PAMIE_TOKEN="$(openssl rand -hex 32)"
docker compose up --build
```

The Compose example binds Pamie to `127.0.0.1:8080` by default. Public deployments must use HTTPS, for example with the included Caddy profile and a real hostname.

Run a backup through Compose:

```sh
mkdir -p backups
docker compose run --rm --no-deps \
  -v "$PWD/backups:/backup" \
  pamie \
  backup --db-path /data/pamie.db --out /backup/pamie.db
```

## Current Status

Docker and release foundation is implemented for the current surface. The Go module starts an HTTP server with structured logging, `/health`, `/ready`, Bearer-protected `/mcp`, configuration from flags and environment, scoped token principals, in-memory token hashing, per-client `/mcp` rate limiting, structured audit events, graceful shutdown, SQLite startup with migrations, WAL mode, foreign keys, initial tables, typed repository methods, MCP JSON-RPC handling, memory tools, first-use MCP instructions, safe read-only resources, deterministic tier lifecycle rules, access-based promotion, retention-policy deletion, lifecycle events, an opt-in scheduled lifecycle worker, FTS5-backed search with safe filters, snippets, depth controls, explainable ranking, optional local vector storage and hybrid ranking with sqlite-vec acceleration, local backup, restore, and embedding backfill operator commands, Docker/Compose assets, Caddy HTTPS guidance, and release artifact automation.

Vector search is disabled by default and supports `local-hash` deterministic embeddings, local Ollama semantic embeddings, SQLite JSON fallback storage, and sqlite-vec acceleration. There is still no multi-token persistent token storage, token rotation, or tamper-proof audit log subsystem yet.

## Roadmap

See [ROADMAP.md](ROADMAP.md) for the implementation phases.

## Security Warning

Do not expose Pamie publicly without Bearer authentication and HTTPS termination. Stored memories are untrusted data and may contain prompt-injection attempts. Pamie must never expose raw SQL or shell execution tools to MCP clients.

## License

Pamie is licensed under the GNU Affero General Public License v3.0 only (`AGPL-3.0-only`). See [LICENSE](LICENSE).
