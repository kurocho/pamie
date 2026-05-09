<p align="center">
  <img src="docs/assets/pamie_logo.png" alt="Pamie logo" width="560">
</p>

# Pamie

Self-hosted long-term memory for MCP agents.

Website: <https://pamie.io>

Docker Hub: <https://hub.docker.com/repository/docker/kurocho/pamie/general>

Pamie is a self-hosted, local-first, provider-independent long-term memory server for MCP agents. It runs as a single Go binary exposing an MCP HTTP endpoint protected by Bearer token authentication, with SQLite and FTS5 as durable local storage. Its memory lifecycle is inspired by Elasticsearch ILM and human memory: fresh memories are fast and prominent, old memories remain available, important memories stay easy to retrieve, and access patterns can promote useful memories back into higher tiers.

## v1.1.0 Highlights

- `pamie start`, `pamie status`, and `pamie stop` manage a local background server on `127.0.0.1:17683`.
- First start creates a persistent hashed Bearer token and prints the raw token once; `pamie token` rotates it later.
- Vector search is enabled by default with local `local-hash` embeddings.
- Ollama semantic embeddings now use short commands: `pamie start --vector-provider ollama` and `pamie embeddings backfill --provider ollama --limit 500`.
- Vector embeddings use only memory titles and explicit `keywords`; full bodies remain in SQLite FTS5 and are not sent to embedding providers.
- Operator commands default to the running daemon database path, so `pamie token list` follows a daemon started with `--db-path`.

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

Install Pamie with Homebrew:

```sh
brew install kurocho/tap/pamie
pamie --version
```

Or tap the repository first:

```sh
brew tap kurocho/tap
brew install pamie
```

Build and install from source for local development:

```sh
./scripts/install-local.sh
```

The source install script builds `./cmd/pamie`, installs it to `~/.local/bin/pamie` by default, and adds that directory to your shell profile if it is missing from `PATH`.

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

Start Pamie in the background:

```sh
pamie start
```

On first start, Pamie creates a local data directory, starts on `127.0.0.1:17683`, and prints a generated Bearer token once. Store that token in your MCP client. If you need a new one later, rotate it:

```sh
pamie token
```

Check or stop the background process:

```sh
pamie status
pamie stop
```

Run in the foreground for development, Docker, or a service manager:

```sh
PAMIE_TOKEN=dev-token \
PAMIE_TOKEN_ID=dev \
PAMIE_TOKEN_SCOPES=all \
go run ./cmd/pamie serve --addr 127.0.0.1:17683 --data-dir ./data
```

Check liveness and readiness:

```sh
curl http://127.0.0.1:17683/health
curl http://127.0.0.1:17683/ready
```

List MCP tools through the protected endpoint:

```sh
curl -i -X POST http://127.0.0.1:17683/mcp \
  -H 'Authorization: Bearer dev-token' \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'
```

If no token is configured, `/mcp` rejects all requests.

Persistent token management:

```sh
pamie token list
pamie token create --id readonly --scopes memory:read,stats:read
pamie token rotate --id readonly
pamie token revoke --id readonly
```

## Connect MCP Clients

Start Pamie and create a token for the MCP client. Use a separate token per client so it can be rotated or revoked independently:

```sh
pamie start
pamie token create --id copilot --scopes memory:read,memory:write,stats:read
```

Copy the `bearer_token` printed by `pamie token create`. Use `memory:delete` only for clients that should be allowed to delete memories.

### GitHub Copilot in VS Code

Create `.vscode/mcp.json` in your workspace:

```json
{
  "inputs": [
    {
      "type": "promptString",
      "id": "pamie-token",
      "description": "Pamie Bearer token",
      "password": true
    }
  ],
  "servers": {
    "pamie": {
      "type": "http",
      "url": "http://127.0.0.1:17683/mcp",
      "headers": {
        "Authorization": "Bearer ${input:pamie-token}"
      }
    }
  }
}
```

Open Copilot Chat, switch to Agent mode, enable the Pamie server in the tools picker, then ask it to use `context_search` or `context_save`.

### Codex CLI or IDE Extension

Codex shares MCP configuration between the CLI and IDE extension:

```sh
export PAMIE_MCP_TOKEN="<token printed by pamie>"
codex mcp add pamie \
  --url http://127.0.0.1:17683/mcp \
  --bearer-token-env-var PAMIE_MCP_TOKEN
codex mcp list
```

Equivalent `~/.codex/config.toml`:

```toml
[mcp_servers.pamie]
url = "http://127.0.0.1:17683/mcp"
bearer_token_env_var = "PAMIE_MCP_TOKEN"
```

### Claude Code

Claude Code can connect to Pamie as an HTTP MCP server:

```sh
export PAMIE_MCP_TOKEN="<token printed by pamie>"
claude mcp add --transport http pamie http://127.0.0.1:17683/mcp \
  --header "Authorization: Bearer $PAMIE_MCP_TOKEN"
claude mcp list
```

For Claude.ai custom connectors, `127.0.0.1` will not work because Claude connects from Anthropic infrastructure. Put Pamie behind HTTPS and keep Bearer authentication enabled before using it as a remote connector.

Version output:

```sh
pamie --version
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

Vector search is enabled by default with dependency-free local hash embeddings. To use Ollama semantic embeddings instead:

```sh
ollama serve
ollama pull embeddinggemma
pamie start --vector-provider ollama
```

If you want Pamie to start a local Ollama process when one is not already running:

```sh
pamie start --vector-provider ollama --vector-ollama-autostart

pamie embeddings backfill --provider ollama --limit 500
```

Pass `--vector-search=false` to disable hybrid vector ranking. The `ollama` provider expects a local Ollama server unless opt-in autostart is enabled with `--vector-ollama-autostart`. `embeddinggemma` is the default 384-dimensional embedding model; `local-hash` is the default dependency-free deterministic provider.

Pamie embeds only memory titles and explicit `keywords` for vector search. Full memory bodies are still stored and indexed by SQLite FTS5 for exact keyword search, but body text is not sent to embedding providers. For long notes, provide keywords with people names, project names, aliases, technologies, decisions, ticket IDs, error messages, dates, and other terms that should retrieve the memory later.

## Docker

Published images live at <https://hub.docker.com/repository/docker/kurocho/pamie/general>.

Run the latest published image with a persistent Docker volume:

```sh
docker volume create pamie-data
export PAMIE_TOKEN="$(openssl rand -hex 32)"
docker run --rm \
  --name pamie \
  -p 127.0.0.1:17683:8080 \
  -v pamie-data:/data \
  -e PAMIE_TOKEN="$PAMIE_TOKEN" \
  -e PAMIE_TOKEN_ID=local \
  -e PAMIE_TOKEN_SCOPES=all \
  kurocho/pamie:latest
```

Build the image:

```sh
docker build --build-arg VERSION=dev -t pamie:dev .
```

Run the locally built image:

```sh
export PAMIE_TOKEN="$(openssl rand -hex 32)"
docker run --rm \
  --name pamie \
  -p 127.0.0.1:17683:8080 \
  -v pamie-data:/data \
  -e PAMIE_TOKEN="$PAMIE_TOKEN" \
  -e PAMIE_TOKEN_ID=local \
  -e PAMIE_TOKEN_SCOPES=all \
  pamie:dev
```

Check the running container:

```sh
curl http://127.0.0.1:17683/health
curl http://127.0.0.1:17683/ready
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

The Compose example binds Pamie to `127.0.0.1:17683` by default. Public deployments must use HTTPS, for example with the included Caddy profile and a real hostname.

Run a backup through Compose:

```sh
mkdir -p backups
docker compose run --rm --no-deps \
  -v "$PWD/backups:/backup" \
  pamie \
  backup --db-path /data/pamie.db --out /backup/pamie.db
```

## Current Status

Docker and release foundation is implemented for the current surface. The Go module starts an HTTP server with structured logging, `/health`, `/ready`, Bearer-protected `/mcp`, configuration from flags and environment, persistent hashed tokens, token rotation and revocation commands, scoped token principals, per-client `/mcp` rate limiting, structured audit events, graceful shutdown, SQLite startup with migrations, WAL mode, foreign keys, initial tables, typed repository methods, MCP JSON-RPC handling, memory tools, first-use MCP instructions, safe read-only resources, deterministic tier lifecycle rules, access-based promotion, retention-policy deletion, lifecycle events, an opt-in scheduled lifecycle worker, FTS5-backed search with safe filters, snippets, depth controls, explainable ranking, optional local vector storage and hybrid ranking with sqlite-vec acceleration, local backup, restore, and embedding backfill operator commands, Docker/Compose assets, Caddy HTTPS guidance, and release artifact automation.

Vector search is enabled by default and supports title/keywords-only `local-hash` deterministic embeddings, local Ollama semantic embeddings, SQLite JSON fallback storage, sqlite-vec acceleration, durable indexing status, and opt-in local Ollama autostart. Tamper-proof audit log storage is still future hardening work.

## Roadmap

See [ROADMAP.md](ROADMAP.md) for the implementation phases.

## Security Warning

Do not expose Pamie publicly without Bearer authentication and HTTPS termination. Stored memories are untrusted data and may contain prompt-injection attempts. Pamie must never expose raw SQL or shell execution tools to MCP clients.

## License

Pamie is licensed under the GNU Affero General Public License v3.0 only (`AGPL-3.0-only`). See [LICENSE](LICENSE).
