# Memory Model

Pamie stores durable memories as structured records with searchable text, metadata, lifecycle state, and event history.

## Memory Item

A memory item is the canonical record. Planned fields include:

- stable ID;
- title or summary;
- body;
- source;
- metadata map;
- tier;
- importance;
- pinned flag;
- timestamps for creation, update, access, and lifecycle changes.

## Chunks

Long memories may be split into chunks for FTS5 and optional vector search. Chunks should preserve a link to the source item and enough position information to return useful snippets.

## Events

Memory changes should append events. Event history is useful for audits, lifecycle debugging, and future explainability.

Implemented event types include:

- `created`
- `updated`
- `deleted`
- `pinned`
- `lifecycle_promoted`
- `lifecycle_demoted`
- `lifecycle_archived`
- `lifecycle_deleted`

## Access

Reads should update access metadata in a controlled way. Access frequency and recency are planned signals for ranking and promotion.

The current implementation records `access_log` rows on `context_get` and promotes older memories after repeated recent access.

## Trust Boundary

Memory text is data, not policy. Agents should receive structured memory results with provenance so they can decide how to use retrieved content.
