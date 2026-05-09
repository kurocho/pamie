# Memory Model

Pamie stores durable memories as structured records with searchable text, metadata, lifecycle state, and event history.

## Memory Item

A memory item is the canonical record. Planned fields include:

- stable ID;
- title or summary;
- body;
- explicit keywords for semantic retrieval;
- source;
- metadata map;
- tier;
- importance;
- pinned flag;
- timestamps for creation, update, access, and lifecycle changes.

## Chunks

Pamie currently stores one full-body chunk per memory for FTS5. The body chunk is the exact keyword search source. Vector embeddings do not use body chunks as input; they use the memory title and explicit keywords only.

## Keywords

Keywords are first-class durable retrieval data supplied by the agent. They should include people names, team names, project names, organizations, aliases, abbreviations, technologies, decisions, ticket IDs, dates, error messages, customer or vendor names, and domain-specific terms that should retrieve the memory later.

Poor or missing keywords reduce semantic vector recall but do not reduce exact body search, because the full body remains indexed by FTS5.

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
