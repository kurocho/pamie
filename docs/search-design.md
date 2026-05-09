# Search Design

Search uses SQLite FTS5 with an optional local vector path and a ranking layer that blends text match quality, vector similarity, and memory lifecycle signals.

## Current Implementation

`context_search` uses SQLite FTS5 keyword search over `memory_chunks`. The chunk content is the full memory body, so exact keyword search can find text anywhere in the stored body. Query text is sanitized into quoted FTS terms instead of exposing raw FTS syntax to MCP clients. When vector search is enabled, the bounded query text is embedded locally and used to add vector candidates after the safe filters are applied.

Saved memory embeddings use the `title_keywords` scope. Pamie embeds only the memory title and explicit agent-provided keywords. Body text, body excerpts, generated summaries, and metadata values are not sent to embedding providers unless the agent also supplies them as keywords.

Results exclude soft-deleted memories by default and support:

- tier filters;
- pinned filters;
- exact source filters;
- metadata equality filters for simple scalar JSON values;
- created and updated time bounds;
- explicit `include_deleted`;
- result limits capped by the memory service;
- search depth controls that widen the internal candidate set before final ranking.

The repository returns one best-ranked chunk per memory item, with a snippet, chunk ID, memory ID, total score, and score breakdown.

## Search Inputs

Supported inputs:

- `query`: required keyword query.
- `tier`: optional `working`, `hot`, `warm`, `cold`, or `archive`.
- `pinned`: optional boolean.
- `metadata`: optional equality filters for string, number, and boolean values.
- `source`: optional exact source value.
- `created_after` and `created_before`: optional RFC3339 bounds.
- `updated_after` and `updated_before`: optional RFC3339 bounds.
- `depth`: optional `shallow`, `standard`, or `deep`.
- `include_deleted`: optional boolean, default false.
- `limit`: maximum returned memories, capped at the service limit.

Metadata filter keys are deliberately narrow: letters, numbers, `_`, and `-`, with a maximum length of 64 characters. Nested JSON path filtering is not exposed yet.

## Ranking Signals

Current ranking considers:

- FTS5 match quality;
- optional vector similarity;
- recency;
- tier;
- pinned status;
- importance;
- recent access frequency.

The current score is additive and explainable:

```text
total = keyword + vector + recency + tier + pinned + importance + access
```

FTS5 `bm25` is used as the keyword signal. Vector similarity uses title/keywords-scope embeddings with cosine similarity and is capped as one score component rather than the whole policy. Recency uses the newest of `updated_at` and `last_accessed_at`. Tier boosts prioritize `working`, then `hot`, `warm`, and `cold`; archive receives no tier boost. Pinned memories, important memories, and recently accessed memories receive additional boosts.

The database first asks FTS5 for a candidate set, optionally asks the configured vector backend for nearest neighbors, then the repository merges candidates by memory ID and re-ranks them in Go. `depth` controls FTS candidate breadth:

- `shallow`: evaluates up to `limit` candidates.
- `standard`: evaluates up to `limit * 3` candidates.
- `deep`: evaluates up to `limit * 8`, capped at 500 candidates.

With the `sqlite-vec` backend, vector candidate lookup uses a local `vec0` virtual table and a `k` value derived from depth. With the `sqlite-json` fallback backend, vector search scans up to ten times the depth-adjusted candidate limit, capped at 2000 filtered embeddings.

## Snippets

Search results should include snippets that help the agent decide whether to retrieve the full memory. Snippets must not be treated as trusted instructions.

The current snippet comes from SQLite FTS5 over the matching chunk when there is a keyword match. Vector-only matches use a plain chunk preview and include `vector_match: true` so clients do not mistake the preview for a semantic body match. Snippets are previews, not summaries and not instructions.

## Vector Disabled Fallback

Vector search is enabled by default with the dependency-free `local-hash` provider. With vector search disabled, `context_search` remains keyword-only and ignores stored embeddings. Existing FTS5 behavior and safe filters remain the baseline path.

Embedding provider failures also degrade to FTS-only behavior. Save and update operations still persist the memory and record indexing status when embedding fails or is skipped. Search falls back to FTS-only if query embedding fails.

## Vector Backends

Supported local backends:

- `auto`: prefer sqlite-vec when available, otherwise sqlite-json.
- `sqlite-vec`: use SQLite-native `vec0` virtual tables for nearest-neighbor lookup.
- `sqlite-json`: store JSON vectors and compute cosine similarity in Go as a portable fallback.

`internal/search` keeps a `VectorSearcher` interface as an extension point for libSQL vector support or another local-only backend.
