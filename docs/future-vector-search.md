# Vector Search

## Design Goals

- Keep vector search optional.
- Avoid mandatory hosted providers.
- Support local embeddings.
- Preserve metadata and retention controls.
- Blend vector similarity with keyword, recency, tier, pinned, importance, and access signals.
- Keep keyword-only search working when vector search is disabled.

## Backend Decision

Pamie supports two local vector backends:

- `sqlite-vec`: preferred acceleration path. It uses `modernc.org/sqlite/vec` and per-provider `vec0` virtual tables.
- `sqlite-json`: fallback path. It stores JSON vectors in `memory_embeddings` and computes cosine similarity in Go over a bounded filtered candidate set.

`PAMIE_VECTOR_BACKEND=auto` resolves to `sqlite-vec` when the extension is registered and falls back to `sqlite-json` otherwise. libSQL vector support remains a future option for deployments that already choose libSQL, but Pamie does not require it.

The current backend has predictable tradeoffs:

- Disk: one JSON vector per chunk plus provider/model metadata.
- CPU: `sqlite-vec` handles nearest-neighbor search inside SQLite; `sqlite-json` scans a bounded filtered candidate set in Go.
- Quality: `local-hash` is deterministic and local but lexical; `ollama` can use a real local semantic embedding model.

## Runtime Configuration

Vector search is disabled by default. Enable it with:

```sh
PAMIE_VECTOR_SEARCH_ENABLED=true
PAMIE_VECTOR_BACKEND=auto
PAMIE_VECTOR_PROVIDER=ollama
PAMIE_VECTOR_MODEL=embeddinggemma
PAMIE_VECTOR_DIMENSIONS=384
PAMIE_VECTOR_OLLAMA_URL=http://127.0.0.1:11434
```

Equivalent flags are:

```sh
--vector-search=true --vector-backend auto --vector-provider ollama --vector-model embeddinggemma --vector-dimensions 384
```

Built-in providers:

- `local-hash`: dependency-free deterministic baseline and test provider.
- `ollama`: local semantic provider that calls a locally running Ollama `/api/embed` endpoint.

## Storage

Migration `0002_vector_search_storage.sql` adds:

- `vector_metadata`: provider, model, dimensions, backend, and distance metric.
- `memory_embeddings`: one embedding per chunk and provider/model target, including dimensions, JSON vector, content hash, and timestamps.

Migration `0003_vector_rowids.sql` adds stable integer row IDs used to mirror embedding rows into sqlite-vec `vec0` virtual tables.

Embeddings are tied to `memory_chunks` and cascade on chunk deletion, so memory body updates replace stale chunk embeddings.

## Interfaces

Embedding generation is owned by `internal/embedding`:

```go
type Provider interface {
    Name() string
    Model() string
    Dimensions() int
    Embed(ctx context.Context, text string) ([]float64, error)
}
```

`internal/search.VectorSearcher` remains a narrow extension point for future vector backends beyond sqlite-vec and sqlite-json.

## Indexing and Backfill

When vector search is enabled, `context_save` embeds the new memory chunk inside the same storage transaction as the memory write. `context_update` replaces chunks and embeddings when the body changes.

Backfill is resumable through `memory.Service.BackfillEmbeddings`: it lists active chunks missing an embedding for the configured provider/model, indexes a bounded batch, and can be run repeatedly. Completed chunks are skipped by the unique `chunk_id`, provider, and model embedding row.

Operators can run backfill from the CLI:

```sh
pamie embeddings backfill --limit 500
```

For Ollama-backed semantic embeddings, run Ollama locally and pull the default embedding model first:

```sh
ollama serve
ollama pull embeddinggemma
pamie start --vector-search --vector-provider ollama --vector-model embeddinggemma --vector-dimensions 384
pamie embeddings backfill --provider ollama --model embeddinggemma --dimensions 384 --backend auto --limit 500
```

Use `--reindex` after changing provider, model, dimensions, or backend:

```sh
pamie embeddings backfill --provider ollama --model embeddinggemma --dimensions 384 --backend sqlite-vec --reindex
```

## Hybrid Ranking

Search still requires safe keyword query text and always applies metadata, source, tier, pinned, timestamp, and deletion filters. When vector search is enabled, the repository also evaluates filtered vector candidates and merges them with FTS candidates by memory ID.

The explainable score is now:

```text
total = keyword + vector + recency + tier + pinned + importance + access
```

Vector score is capped below the keyword signal so semantic similarity can promote useful memories without overriding explicit operator controls by itself.
