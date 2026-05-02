# Tiering and Retention

Pamie's tiering model is inspired by index lifecycle management and human recall. Recent memories should be prominent, but older memories should remain available unless deletion is explicitly allowed.

## Tiers

- `working`: immediate context and newly saved memories.
- `hot`: recent memories that are still likely to matter.
- `warm`: useful older memories.
- `cold`: low-access memories retained for search.
- `archive`: retained memories with low default ranking.

## Current Default Rules

The lifecycle service currently applies deterministic built-in defaults:

- `working` demotes to `hot` after 1 day without activity.
- `hot` demotes to `warm` after 7 days without activity.
- `warm` demotes to `cold` after 30 days without activity.
- `cold` moves to `archive` after 90 days without activity.
- `archive` is not deleted by default.
- Memories promote one tier after 3 accesses within 7 days.
- Pinned memories are protected from lifecycle demotion, archive, and deletion by default.
- Memories with importance `90` or higher are protected from lifecycle demotion, archive, and deletion by default.

Activity is currently based on `last_accessed_at`, `updated_at`, and `created_at`.

## Scheduled Worker

Lifecycle evaluation can run automatically inside the `pamie` server process. The worker is disabled by default and must be enabled with `PAMIE_LIFECYCLE_WORKER_ENABLED=true` or `--lifecycle-worker=true`.

Worker controls:

- `PAMIE_LIFECYCLE_INTERVAL` / `--lifecycle-interval`: interval between scheduled lifecycle runs. Default: `1h`.
- `PAMIE_LIFECYCLE_BATCH_SIZE` / `--lifecycle-batch-size`: maximum active memories evaluated per run. Default: `500`.
- `PAMIE_LIFECYCLE_RUN_ON_START` / `--lifecycle-run-on-start`: run once immediately after the worker starts. Default: `false`.
- `PAMIE_LIFECYCLE_STARTUP_DELAY` / `--lifecycle-startup-delay`: delay before the first run when run-on-start is false. Default: `0s`.

The worker starts asynchronously and does not block HTTP startup. It shares the process shutdown context, waits for an active lifecycle run to observe cancellation, and prevents overlapping lifecycle runs. Failed runs are logged and audited without crashing the process.

Operational impact is limited to the existing `RunLifecycle` behavior: one-tier demotion per run, access-based promotion, archive transitions, and explicit-policy soft deletion. Enabling the worker makes these decisions periodic; it does not add distributed locking, physical purging, or new retention semantics.

## Promotion

A memory can move to a more active tier when it is accessed repeatedly, pinned, marked important, or referenced by a new memory. Promotion rules should be deterministic and recorded as events.

The current implementation promotes on access. For example, a `cold` memory with enough recent accesses promotes to `warm`; an `archive` memory promotes to `cold` and clears `archived_at`.

## Demotion

A memory can move to a less active tier when it ages without access. Demotion should consider pinned status, importance, retention policy, and recent retrieval activity.

The current implementation demotes one tier per lifecycle run and records `lifecycle_demoted`.

## Deletion

Deletion should happen only when an explicit retention policy permits it or a user asks for deletion through an authorized tool. Pinned memories should not be deleted by default lifecycle rules.

The current implementation uses soft deletion. Lifecycle policy deletion sets `deleted_at` and records `lifecycle_deleted`; it does not physically purge rows.

## Policy Design

Policies should be understandable to operators. A policy should describe scope, tier thresholds, archive behavior, deletion rules, and exceptions.

Retention policies are stored in `retention_policies` as JSON scope and JSON rules. Empty scope applies to all memories. Supported scope fields:

- `tiers`: list of tier names.
- `sources`: list of source names.
- `include_pinned`: when false, the policy does not match pinned memories.

Supported rule fields:

- `demote_working_after_days`
- `demote_hot_after_days`
- `demote_warm_after_days`
- `archive_cold_after_days`
- `delete_archived_after_days`
- `promote_access_count`
- `promote_access_window_days`
- `protect_pinned`
- `protect_importance_at_or_above`

Rules omitted or set to zero inherit the built-in default, except `delete_archived_after_days`; deletion remains disabled unless the policy sets a positive value.
