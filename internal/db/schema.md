# SQLite Schema

The implemented schema lives in `internal/db/migrations/`. This document describes the current shape at a higher level.

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

Searchable chunks derived from memory items. Pamie currently stores one full-body chunk per memory so FTS5 can search the complete body.

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

## `memory_keywords`

First-class semantic retrieval keywords supplied by agents. These values are used with the memory title for the `title_keywords` vector embedding scope.

Fields:

- `memory_id`
- `keyword_index`
- `keyword`
- `normalized_keyword`
- `created_at`
- `updated_at`

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

## `auth_tokens`

Persistent Bearer token metadata. Raw token values are never stored.

Fields:

- `id`
- `token_hash`
- `token_salt`
- `scopes`
- `created_at`
- `last_used_at`
- `revoked_at`
- `expires_at`

## `vector_metadata`

Optional local vector backend metadata by provider and model.

Fields:

- `provider`
- `model`
- `dimensions`
- `backend`
- `distance_metric`
- `embedding_scope`
- `created_at`
- `updated_at`

## `memory_embeddings`

Optional local vector rows anchored to memory chunks. New rows use `embedding_scope = title_keywords` and their `content_hash` is computed from the deterministic title/keywords embedding document, not from the body.

Fields:

- `chunk_id`
- `memory_id`
- `provider`
- `model`
- `dimensions`
- `embedding_json`
- `content_hash`
- `created_at`
- `updated_at`
- `vector_rowid`
- `embedding_scope`

## `embedding_index_status`

Durable status for best-effort embedding indexing.

Fields:

- `chunk_id`
- `memory_id`
- `provider`
- `model`
- `dimensions`
- `embedding_scope`
- `status`
- `content_hash`
- `error_summary`
- `attempts`
- `created_at`
- `updated_at`
