# Backup and Restore

Pamie stores durable memory in SQLite, so backup and restore behavior must preserve database consistency and operator control.

Pamie provides two explicit operator commands: `backup` and `restore`. These commands are local process operations, not MCP tools.

## Backup Goals

- Create consistent backups while the service may be running.
- Preserve memory items, chunks, events, policies, and access logs.
- Support scheduled operator backups.
- Avoid exposing tokens in exported artifacts.

## SQLite Backup

Use `pamie backup` to create a consistent SQLite database backup:

```sh
pamie backup \
  --db-path /var/lib/pamie/pamie.db \
  --out /var/backups/pamie/pamie-$(date -u +%Y%m%dT%H%M%SZ).db
```

The destination file must not already exist. Pamie creates parent directories as needed and runs an integrity check before reporting success.

The command uses SQLite `VACUUM INTO`, so it is safe with WAL mode. Do not copy only `pamie.db` while Pamie is running; the `-wal` and `-shm` files may contain required state.

Validate a SQLite backup before restore:

```sh
pamie restore \
  --db-path /tmp/pamie-restore-test/pamie.db \
  --in /var/backups/pamie/pamie-20260501T120000Z.db \
  --dry-run
```

For a full restore from a SQLite backup:

1. Stop Pamie.
2. Move the existing database file and any `pamie.db-wal` / `pamie.db-shm` files aside.
3. Run `pamie restore --in <backup.db> --db-path <target.db> --confirm`.
4. Start Pamie.
5. Check `/ready`.
6. Call `context_stats` with a token that has `stats:read`.

The restore command refuses to overwrite an existing target path. Do not restore over an active database.

## NDJSON Backup

Use `pamie backup --format ndjson` for a portable memory export:

```sh
pamie backup \
  --db-path /var/lib/pamie/pamie.db \
  --format ndjson \
  --out /var/backups/pamie/pamie-$(date -u +%Y%m%dT%H%M%SZ).ndjson
```

The first line is a manifest with:

- export format and version;
- SQLite schema version;
- export time;
- Pamie version;
- per-record counts;
- SHA-256 checksum for all record lines.

Records include memory items, memory chunks, memory events, retention policies, and access logs. Memory IDs, timestamps, tiers, pinned state, importance, metadata, source, and event history are preserved.

Use `--out -` to write NDJSON to stdout. Audit logs go to stderr so stdout remains machine-readable.

## NDJSON Restore Validation

Use `pamie restore --dry-run` before committing a restore from NDJSON:

```sh
pamie restore \
  --db-path /tmp/pamie-restore-test/pamie.db \
  --format ndjson \
  --in /var/backups/pamie/pamie-20260501T120000Z.ndjson \
  --dry-run
```

To commit an NDJSON restore:

```sh
pamie restore \
  --db-path /var/lib/pamie/pamie.db \
  --format ndjson \
  --in /var/backups/pamie/pamie-20260501T120000Z.ndjson \
  --confirm
```

NDJSON restore is append-only. Duplicate IDs in the export file or target database are rejected; Pamie does not overwrite existing rows. For full restore, restore into an empty database or replace the database with a SQLite backup while the service is stopped.

Validation rejects malformed NDJSON, unsupported format versions, incompatible schema versions, checksum mismatches, count mismatches, invalid JSON object fields, missing referenced memory IDs, and duplicate IDs.

## Compose Examples

Run an online SQLite backup from the Pamie image with an additional backup bind mount. The host backup directory must be writable by container user `65532`.

```sh
mkdir -p backups
docker compose run --rm --no-deps \
  -v "$PWD/backups:/backup" \
  pamie \
  backup --db-path /data/pamie.db --out /backup/pamie-$(date -u +%Y%m%dT%H%M%SZ).db
```

Create an NDJSON backup the same way:

```sh
docker compose run --rm --no-deps \
  -v "$PWD/backups:/backup" \
  pamie \
  backup --db-path /data/pamie.db --format ndjson --out /backup/pamie-$(date -u +%Y%m%dT%H%M%SZ).ndjson
```

## Scheduled Backups

Use a host scheduler to call the explicit operator command:

- `systemd` timer on a VPS or Linux host;
- NAS scheduled task;
- Kubernetes CronJob that runs `pamie backup` against the mounted data volume.

Example cron entry:

```cron
15 * * * * /usr/local/bin/pamie backup --db-path /var/lib/pamie/pamie.db --out /var/backups/pamie/pamie-$(date -u +\%Y\%m\%dT\%H\%M\%SZ).db
```

Store backups outside the Pamie data volume. Encrypt backups or write them only to access-controlled storage.

## Artifact Sensitivity

SQLite backups and NDJSON exports contain memory bodies, metadata, lifecycle history, policies, and access logs. They do not contain raw Bearer tokens because Pamie does not store raw tokens, but access logs can contain token IDs.

Treat artifacts as sensitive data. Restrict file permissions, avoid shared shell history for paths that reveal sensitive context, and rotate or delete old artifacts according to your retention policy.

## Filesystem Permissions

- The container runs as user `65532:65532`.
- The data directory must be writable by that user in container deployments.
- Restrict host data directory permissions to the service account.
- Do not store Bearer tokens inside the SQLite data directory.
- Protect backup directories at least as strictly as the live data directory.

## Testing

Pamie includes round-trip tests that create fixture data, export or back up, restore into a new database, and compare representative records. Tests also cover WAL backup, malformed NDJSON, duplicate IDs, and checksum validation.
