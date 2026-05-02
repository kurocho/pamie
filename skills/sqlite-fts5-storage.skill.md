# SQLite FTS5 Storage Skill

## Purpose

Design and implement SQLite schema, migrations, WAL mode, FTS5 search storage, and backup behavior.

## When to Use

Use when adding database connections, migrations, repositories, FTS5 tables, backup/export, or storage tests.

## Inputs

- Memory model fields.
- Search requirements.
- Retention and event requirements.
- Backup requirements.

## Step-by-step Procedure

1. Design schema changes as migrations.
2. Add constraints and indexes for expected access patterns.
3. Configure SQLite open options and WAL mode.
4. Keep SQL inside the storage package.
5. Add repository methods for supported operations.
6. Add temporary-database integration tests.
7. Validate backup behavior with WAL enabled.

## Output Format

- Migration files.
- Repository code.
- Storage tests.
- Schema documentation updates.

## Checklist

- [ ] Migration runs on an empty database.
- [ ] Migration is idempotent through the migration runner.
- [ ] WAL mode behavior is documented.
- [ ] FTS5 indexing is tested.
- [ ] Backup or export has a round-trip test.

## Common Mistakes

- Building SQL from unvalidated request strings.
- Forgetting foreign keys or indexes.
- Copying live SQLite files unsafely.
- Letting upper layers depend on table names.

## Security Considerations

Never expose raw SQL through MCP. Treat memory text as untrusted data and avoid logging sensitive content.
