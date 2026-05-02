# SQLite Schema

The implemented schema lives in `internal/db/migrations/0001_initial_schema.sql`. This document describes the current shape at a higher level.

## `schema_migrations`

Tracks applied migrations.

Fields:

- `version`
- `name`
- `applied_at`

## `memory_items`

Canonical memory records.

Fields:

- `id`
- `title`
- `body`
- `source`
- `metadata_json`
- `tier`
- `importance`
- `pinned`
- `created_at`
- `updated_at`
- `last_accessed_at`
- `archived_at`
- `deleted_at`

## `memory_chunks`

Searchable chunks derived from memory items.

Fields:

- `id`
- `memory_id`
- `chunk_index`
- `content`
- `created_at`

## `memory_fts`

FTS5 virtual table for chunk content. It stores:

- `content`
- `memory_id`
- `chunk_id`

Triggers on `memory_chunks` keep this table synchronized for insert, update, and delete operations. Phase 5 will add query and ranking behavior on top of this index.

## `memory_events`

Append-only history of memory changes.

Fields:

- `id`
- `memory_id`
- `event_type`
- `event_payload_json`
- `created_at`

## `retention_policies`

Operator-defined lifecycle rules.

Fields:

- `id`
- `name`
- `scope_json`
- `rules_json`
- `enabled`
- `created_at`
- `updated_at`

## `access_log`

Access records for ranking, statistics, and promotion.

Fields:

- `id`
- `memory_id`
- `access_type`
- `token_id`
- `created_at`
