# Deployment

Pamie should run as a small service with a persistent data directory and an HTTPS reverse proxy when exposed outside localhost.

## Local Development

Current development server:

```sh
PAMIE_TOKEN=dev-token \
PAMIE_TOKEN_ID=dev \
PAMIE_TOKEN_SCOPES=all \
go run ./cmd/pamie --addr 127.0.0.1:8080 --data-dir ./data
```

Health and readiness:

```sh
curl http://127.0.0.1:8080/health
curl http://127.0.0.1:8080/ready
```

## Production Shape

- `pamie` binary.
- Persistent `/data` directory.
- SQLite database under the data directory.
- Reverse proxy for HTTPS.
- Environment or file-based token configuration.
- Scoped Bearer tokens with non-secret token IDs.
- Per-client `/mcp` rate limiting in Pamie and optionally at the reverse proxy.
- Regular backups through explicit operator commands.

## Docker Image

The project includes a multi-stage `Dockerfile`.

- Build stage: Go toolchain.
- Runtime stage: `scratch`.
- Runtime user: numeric non-root user `65532:65532`.
- Data directory: `/data`.
- Listen address: `0.0.0.0:8080` inside the container.
- Secrets: supplied only at runtime through environment variables or platform secrets.

Build locally:

```sh
docker build --build-arg VERSION=dev -t kurocho/pamie:dev .
```

Run locally with a persistent named volume and localhost-only port binding:

```sh
docker volume create pamie-data
docker run --rm \
  --name pamie \
  -p 127.0.0.1:8080:8080 \
  -v pamie-data:/data \
  -e PAMIE_TOKEN="$PAMIE_TOKEN" \
  -e PAMIE_TOKEN_ID=local \
  -e PAMIE_TOKEN_SCOPES=all \
  kurocho/pamie:dev
```

Check health from the host:

```sh
curl http://127.0.0.1:8080/health
```

The image does not include a shell, `curl`, `sqlite3`, or backup tools.
It does include Pamie's own `backup` and `restore` operator subcommands.

## Compose

`compose.yaml` provides a local-first deployment. By default it binds Pamie to `127.0.0.1:8080` only.

```sh
export PAMIE_TOKEN="$(openssl rand -hex 32)"
export PAMIE_TOKEN_ID=primary
export PAMIE_TOKEN_SCOPES=memory:read,memory:write,memory:delete,stats:read
docker compose up --build
```

For public HTTPS exposure, enable the Caddy profile and set a real hostname:

```sh
export PAMIE_HOSTNAME=pamie.example.com
docker compose --profile https up -d --build
```

The Caddy example in `deploy/Caddyfile.example` terminates HTTPS, proxies to Pamie, limits request bodies, sets defensive headers, and hides `/health` and `/ready` from the public hostname. `PAMIE_HOSTNAME` is optional for local Compose parsing but must be set to a real DNS name before enabling the HTTPS profile.

## Security Configuration

Minimum public-facing configuration:

```sh
PAMIE_ADDR=127.0.0.1:8080
PAMIE_TOKEN='<long random token>'
PAMIE_TOKEN_ID='primary-agent'
PAMIE_TOKEN_SCOPES='memory:read,memory:write,memory:delete,stats:read'
PAMIE_MCP_RATE_LIMIT=120
PAMIE_MCP_RATE_BURST=30
```

Use `memory:admin` only for trusted operator tokens. Use narrower scopes for routine agents, for example `memory:read,stats:read` for read-only retrieval clients.

Do not put tokens in command history for production systems. Prefer a service manager environment file with restrictive permissions or a platform secret store. Pamie logs token IDs for audit attribution but does not log raw Bearer tokens.

Public deployments must terminate HTTPS before `/mcp`. Keep `/health` and `/ready` available only where operationally needed, or restrict them at the reverse proxy.

## Deployment Examples

### Local Workstation

Use `go run` or Compose with the default localhost binding. Use `PAMIE_TOKEN_SCOPES=all` only for local development.

### NAS

Run the container with a named Docker volume or a dedicated dataset mounted at `/data`. Restrict filesystem permissions to the container runtime user where the NAS allows it. Keep the service bound to the private LAN or behind the NAS HTTPS reverse proxy.

### VPS

Run Pamie behind Caddy, nginx, Traefik, or a platform load balancer. Expose only HTTPS publicly. Use a long random token, scoped tokens, host firewall rules, and scheduled backups to storage outside the VPS.

### Homelab

Bind Pamie to a private address or VPN-only interface. If exposing through a tunnel or reverse proxy, keep Bearer auth enabled and rate limiting active. Avoid sharing the same token between every agent; use scoped tokens once persistent multi-token support exists.

## Scheduled Backups

Prefer Pamie's SQLite-safe backup command over filesystem copies:

```sh
pamie backup \
  --db-path /var/lib/pamie/pamie.db \
  --out /var/backups/pamie/pamie-$(date -u +%Y%m%dT%H%M%SZ).db
```

The command is safe while the service is running with WAL enabled and refuses to overwrite an existing destination file.

For systemd timers, wrap the timestamp expansion in a shell and escape `%` in the date format:

```ini
[Service]
Type=oneshot
ExecStart=/bin/sh -c '/usr/local/bin/pamie backup --db-path /var/lib/pamie/pamie.db --out /var/backups/pamie/pamie-$(date -u +%%Y%%m%%dT%%H%%M%%SZ).db'
```

For Compose, mount a host backup directory that is writable by container user `65532` when running the operator command:

```sh
mkdir -p backups
docker compose run --rm --no-deps \
  -v "$PWD/backups:/backup" \
  pamie \
  backup --db-path /data/pamie.db --out /backup/pamie-$(date -u +%Y%m%dT%H%M%SZ).db
```

Use `pamie backup --format ndjson --out <file>.ndjson` when you need a portable, manifest-checked export for validation or migration. Use `pamie restore --dry-run` against a temporary database before any committed restore.

## Release Builds

Version is injected with Go linker flags:

```sh
go build -trimpath -ldflags "-s -w -X main.version=v0.1.0" -o ./bin/pamie ./cmd/pamie
```

Local release snapshot:

```sh
make release-snapshot VERSION=v0.1.0
make checksums
```

Tagged GitHub releases use `.github/workflows/release.yml`. The workflow builds Linux and macOS binaries for `amd64` and `arm64`, writes `checksums.txt`, publishes release artifacts to GitHub, and publishes multi-architecture Docker images to Docker Hub.

Published Docker Hub tags:

- `kurocho/pamie:<git tag>`, for example `kurocho/pamie:v0.1.0`.
- `kurocho/pamie:latest`.

Required GitHub secret:

- `DOCKERHUB_PAT`: Docker Hub personal access token with write access to `kurocho/pamie`.

The Docker Hub username is fixed in the workflow as `kurocho`.
