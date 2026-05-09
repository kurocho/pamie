// SPDX-License-Identifier: AGPL-3.0-only

package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/your-org/pamie/internal/search"
)

// MemoryRepository stores memory items, chunks, and events.
type MemoryRepository struct {
	exec executor
}

// CreateItem inserts a memory item.
func (r *MemoryRepository) CreateItem(ctx context.Context, item MemoryItem) error {
	if err := validateMemoryItem(item); err != nil {
		return err
	}

	_, err := r.exec.ExecContext(ctx, `
INSERT INTO memory_items (
  id, title, body, source, metadata_json, tier, importance, pinned,
  created_at, updated_at, last_accessed_at, archived_at, deleted_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ID,
		item.Title,
		item.Body,
		item.Source,
		defaultJSON(item.MetadataJSON),
		string(item.Tier),
		item.Importance,
		boolInt(item.Pinned),
		formatTime(item.CreatedAt),
		formatTime(item.UpdatedAt),
		nullableTime(item.LastAccessedAt),
		nullableTime(item.ArchivedAt),
		nullableTime(item.DeletedAt),
	)
	if err != nil {
		return fmt.Errorf("create memory item: %w", err)
	}
	return nil
}

// GetItem retrieves a memory item by ID.
func (r *MemoryRepository) GetItem(ctx context.Context, id string) (MemoryItem, error) {
	if err := requireID("memory", id); err != nil {
		return MemoryItem{}, err
	}

	var item MemoryItem
	var tier string
	var pinned int
	var createdAt, updatedAt string
	var lastAccessedAt, archivedAt, deletedAt sql.NullString

	err := r.exec.QueryRowContext(ctx, `
SELECT id, title, body, source, metadata_json, tier, importance, pinned,
       created_at, updated_at, last_accessed_at, archived_at, deleted_at
FROM memory_items
WHERE id = ?`, id).Scan(
		&item.ID,
		&item.Title,
		&item.Body,
		&item.Source,
		&item.MetadataJSON,
		&tier,
		&item.Importance,
		&pinned,
		&createdAt,
		&updatedAt,
		&lastAccessedAt,
		&archivedAt,
		&deletedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return MemoryItem{}, ErrNotFound
	}
	if err != nil {
		return MemoryItem{}, fmt.Errorf("get memory item: %w", err)
	}

	item.Tier = Tier(tier)
	item.Pinned = intBool(pinned)
	if item.CreatedAt, err = parseTime(createdAt); err != nil {
		return MemoryItem{}, fmt.Errorf("parse created_at: %w", err)
	}
	if item.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return MemoryItem{}, fmt.Errorf("parse updated_at: %w", err)
	}
	if item.LastAccessedAt, err = parseNullableTime(lastAccessedAt); err != nil {
		return MemoryItem{}, fmt.Errorf("parse last_accessed_at: %w", err)
	}
	if item.ArchivedAt, err = parseNullableTime(archivedAt); err != nil {
		return MemoryItem{}, fmt.Errorf("parse archived_at: %w", err)
	}
	if item.DeletedAt, err = parseNullableTime(deletedAt); err != nil {
		return MemoryItem{}, fmt.Errorf("parse deleted_at: %w", err)
	}

	return item, nil
}

// UpdateItem applies partial changes to a memory item.
func (r *MemoryRepository) UpdateItem(ctx context.Context, id string, update MemoryUpdate) error {
	if err := requireID("memory", id); err != nil {
		return err
	}
	if update.UpdatedAt.IsZero() {
		return fmt.Errorf("%w: memory updated_at must not be zero", ErrInvalid)
	}

	assignments := []string{"updated_at = ?"}
	args := []any{formatTime(update.UpdatedAt)}

	if update.Title != nil {
		assignments = append(assignments, "title = ?")
		args = append(args, *update.Title)
	}
	if update.Body != nil {
		if *update.Body == "" {
			return fmt.Errorf("%w: memory body must not be empty", ErrInvalid)
		}
		assignments = append(assignments, "body = ?")
		args = append(args, *update.Body)
	}
	if update.Source != nil {
		assignments = append(assignments, "source = ?")
		args = append(args, *update.Source)
	}
	if update.MetadataJSON != nil {
		assignments = append(assignments, "metadata_json = ?")
		args = append(args, defaultJSON(*update.MetadataJSON))
	}
	if update.Tier != nil {
		if !update.Tier.Valid() {
			return fmt.Errorf("%w: invalid memory tier %q", ErrInvalid, *update.Tier)
		}
		assignments = append(assignments, "tier = ?")
		args = append(args, string(*update.Tier))
	}
	if update.Importance != nil {
		if *update.Importance < 0 || *update.Importance > 100 {
			return fmt.Errorf("%w: memory importance must be between 0 and 100", ErrInvalid)
		}
		assignments = append(assignments, "importance = ?")
		args = append(args, *update.Importance)
	}
	if update.Pinned != nil {
		assignments = append(assignments, "pinned = ?")
		args = append(args, boolInt(*update.Pinned))
	}
	if update.LastAccessedAt != nil {
		assignments = append(assignments, "last_accessed_at = ?")
		args = append(args, formatTime(*update.LastAccessedAt))
	}
	if update.ArchivedAt != nil {
		assignments = append(assignments, "archived_at = ?")
		args = append(args, formatTime(*update.ArchivedAt))
	}
	if update.ClearArchivedAt {
		assignments = append(assignments, "archived_at = NULL")
	}
	if update.DeletedAt != nil {
		assignments = append(assignments, "deleted_at = ?")
		args = append(args, formatTime(*update.DeletedAt))
	}

	args = append(args, id)
	result, err := r.exec.ExecContext(ctx, "UPDATE memory_items SET "+strings.Join(assignments, ", ")+" WHERE id = ?", args...)
	if err != nil {
		return fmt.Errorf("update memory item: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update memory item rows affected: %w", err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// AddChunk inserts a searchable memory chunk.
func (r *MemoryRepository) AddChunk(ctx context.Context, chunk MemoryChunk) error {
	if err := validateMemoryChunk(chunk); err != nil {
		return err
	}

	_, err := r.exec.ExecContext(ctx, `
INSERT INTO memory_chunks (id, memory_id, chunk_index, content, created_at)
VALUES (?, ?, ?, ?, ?)`,
		chunk.ID,
		chunk.MemoryID,
		chunk.ChunkIndex,
		chunk.Content,
		formatTime(chunk.CreatedAt),
	)
	if err != nil {
		return fmt.Errorf("add memory chunk: %w", err)
	}
	return nil
}

// ReplaceChunks replaces all chunks for a memory item with the provided chunks.
func (r *MemoryRepository) ReplaceChunks(ctx context.Context, memoryID string, chunks []MemoryChunk) error {
	if err := requireID("memory", memoryID); err != nil {
		return err
	}
	if err := r.deleteSQLiteVecRowsForMemory(ctx, memoryID); err != nil {
		return err
	}
	if _, err := r.exec.ExecContext(ctx, "DELETE FROM memory_chunks WHERE memory_id = ?", memoryID); err != nil {
		return fmt.Errorf("delete memory chunks: %w", err)
	}
	for _, chunk := range chunks {
		if chunk.MemoryID != memoryID {
			return fmt.Errorf("%w: chunk memory_id does not match replacement memory", ErrInvalid)
		}
		if err := r.AddChunk(ctx, chunk); err != nil {
			return err
		}
	}
	return nil
}

// ListChunks returns chunks for one memory item ordered by chunk index.
func (r *MemoryRepository) ListChunks(ctx context.Context, memoryID string) ([]MemoryChunk, error) {
	if err := requireID("memory", memoryID); err != nil {
		return nil, err
	}

	rows, err := r.exec.QueryContext(ctx, `
SELECT id, memory_id, chunk_index, content, created_at
FROM memory_chunks
WHERE memory_id = ?
ORDER BY chunk_index`, memoryID)
	if err != nil {
		return nil, fmt.Errorf("list memory chunks: %w", err)
	}
	defer rows.Close()

	var chunks []MemoryChunk
	for rows.Next() {
		var chunk MemoryChunk
		var createdAt string
		if err := rows.Scan(&chunk.ID, &chunk.MemoryID, &chunk.ChunkIndex, &chunk.Content, &createdAt); err != nil {
			return nil, fmt.Errorf("scan memory chunk: %w", err)
		}
		if chunk.CreatedAt, err = parseTime(createdAt); err != nil {
			return nil, fmt.Errorf("parse chunk created_at: %w", err)
		}
		chunks = append(chunks, chunk)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list memory chunks: %w", err)
	}
	return chunks, nil
}

// ReplaceKeywords replaces the full keyword set for a memory item.
func (r *MemoryRepository) ReplaceKeywords(ctx context.Context, memoryID string, keywords []MemoryKeyword) error {
	if err := requireID("memory", memoryID); err != nil {
		return err
	}
	if _, err := r.exec.ExecContext(ctx, "DELETE FROM memory_keywords WHERE memory_id = ?", memoryID); err != nil {
		return fmt.Errorf("delete memory keywords: %w", err)
	}
	for _, keyword := range keywords {
		keyword.MemoryID = memoryID
		if err := validateMemoryKeyword(keyword); err != nil {
			return err
		}
		_, err := r.exec.ExecContext(ctx, `
INSERT INTO memory_keywords (
  memory_id, keyword_index, keyword, normalized_keyword, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?)`,
			keyword.MemoryID,
			keyword.KeywordIndex,
			keyword.Keyword,
			keyword.NormalizedKeyword,
			formatTime(keyword.CreatedAt),
			formatTime(keyword.UpdatedAt),
		)
		if err != nil {
			return fmt.Errorf("insert memory keyword: %w", err)
		}
	}
	return nil
}

// ListKeywords returns keywords for one memory item in display order.
func (r *MemoryRepository) ListKeywords(ctx context.Context, memoryID string) ([]MemoryKeyword, error) {
	if err := requireID("memory", memoryID); err != nil {
		return nil, err
	}
	rows, err := r.exec.QueryContext(ctx, `
SELECT memory_id, keyword_index, keyword, normalized_keyword, created_at, updated_at
FROM memory_keywords
WHERE memory_id = ?
ORDER BY keyword_index`, memoryID)
	if err != nil {
		return nil, fmt.Errorf("list memory keywords: %w", err)
	}
	defer rows.Close()
	return scanMemoryKeywords(rows)
}

// ListKeywordsForMemories returns keywords keyed by memory ID.
func (r *MemoryRepository) ListKeywordsForMemories(ctx context.Context, memoryIDs []string) (map[string][]MemoryKeyword, error) {
	out := make(map[string][]MemoryKeyword, len(memoryIDs))
	if len(memoryIDs) == 0 {
		return out, nil
	}
	placeholders := make([]string, 0, len(memoryIDs))
	args := make([]any, 0, len(memoryIDs))
	for _, memoryID := range memoryIDs {
		if err := requireID("memory", memoryID); err != nil {
			return nil, err
		}
		placeholders = append(placeholders, "?")
		args = append(args, memoryID)
	}
	rows, err := r.exec.QueryContext(ctx, `
SELECT memory_id, keyword_index, keyword, normalized_keyword, created_at, updated_at
FROM memory_keywords
WHERE memory_id IN (`+strings.Join(placeholders, ",")+`)
ORDER BY memory_id, keyword_index`, args...)
	if err != nil {
		return nil, fmt.Errorf("list memory keywords: %w", err)
	}
	defer rows.Close()
	keywords, err := scanMemoryKeywords(rows)
	if err != nil {
		return nil, err
	}
	for _, keyword := range keywords {
		out[keyword.MemoryID] = append(out[keyword.MemoryID], keyword)
	}
	return out, nil
}

// UpsertVectorMetadata records the local vector index configuration.
func (r *MemoryRepository) UpsertVectorMetadata(ctx context.Context, metadata VectorMetadata) error {
	if err := validateVectorMetadata(metadata); err != nil {
		return err
	}
	if metadata.Backend == VectorBackendSQLiteVec {
		if err := r.ensureSQLiteVecTable(ctx, EmbeddingTarget{
			Provider:   metadata.Provider,
			Model:      metadata.Model,
			Dimensions: metadata.Dimensions,
			Scope:      metadata.EmbeddingScope,
		}); err != nil {
			return err
		}
	}
	_, err := r.exec.ExecContext(ctx, `
INSERT INTO vector_metadata (
  provider, model, dimensions, backend, distance_metric, embedding_scope, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(provider, model) DO UPDATE SET
  dimensions = excluded.dimensions,
  backend = excluded.backend,
  distance_metric = excluded.distance_metric,
  embedding_scope = excluded.embedding_scope,
  updated_at = excluded.updated_at`,
		metadata.Provider,
		metadata.Model,
		metadata.Dimensions,
		metadata.Backend,
		metadata.DistanceMetric,
		metadata.EmbeddingScope,
		formatTime(metadata.CreatedAt),
		formatTime(metadata.UpdatedAt),
	)
	if err != nil {
		return fmt.Errorf("upsert vector metadata: %w", err)
	}
	return nil
}

// UpsertEmbedding stores or replaces the embedding for one memory chunk.
func (r *MemoryRepository) UpsertEmbedding(ctx context.Context, embedding MemoryEmbedding) error {
	if embedding.VectorRowID == 0 {
		embedding.VectorRowID = vectorRowID(embedding.ChunkID, embedding.Provider, embedding.Model, embedding.EmbeddingScope)
	}
	if err := validateMemoryEmbedding(embedding); err != nil {
		return err
	}
	_, err := r.exec.ExecContext(ctx, `
INSERT INTO memory_embeddings (
  chunk_id, memory_id, provider, model, dimensions, embedding_json, content_hash, vector_rowid, embedding_scope, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(chunk_id, provider, model) DO UPDATE SET
  memory_id = excluded.memory_id,
  dimensions = excluded.dimensions,
  embedding_json = excluded.embedding_json,
  content_hash = excluded.content_hash,
  vector_rowid = excluded.vector_rowid,
  embedding_scope = excluded.embedding_scope,
  updated_at = excluded.updated_at`,
		embedding.ChunkID,
		embedding.MemoryID,
		embedding.Provider,
		embedding.Model,
		embedding.Dimensions,
		embedding.EmbeddingJSON,
		embedding.ContentHash,
		embedding.VectorRowID,
		embedding.EmbeddingScope,
		formatTime(embedding.CreatedAt),
		formatTime(embedding.UpdatedAt),
	)
	if err != nil {
		return fmt.Errorf("upsert memory embedding: %w", err)
	}
	backend, err := r.vectorBackend(ctx, EmbeddingTarget{
		Provider:   embedding.Provider,
		Model:      embedding.Model,
		Dimensions: embedding.Dimensions,
		Scope:      embedding.EmbeddingScope,
	})
	if err != nil {
		return err
	}
	if backend == VectorBackendSQLiteVec {
		if err := r.upsertSQLiteVecRow(ctx, embedding); err != nil {
			return err
		}
	}
	return nil
}

// ListEmbeddings returns embeddings for one memory item and target.
func (r *MemoryRepository) ListEmbeddings(ctx context.Context, memoryID string, target EmbeddingTarget) ([]MemoryEmbedding, error) {
	if err := requireID("memory", memoryID); err != nil {
		return nil, err
	}
	if err := validateEmbeddingTarget(target); err != nil {
		return nil, err
	}

	rows, err := r.exec.QueryContext(ctx, `
SELECT chunk_id, memory_id, provider, model, dimensions, embedding_json, content_hash, vector_rowid, embedding_scope, created_at, updated_at
FROM memory_embeddings
WHERE memory_id = ? AND provider = ? AND model = ? AND dimensions = ? AND embedding_scope = ?
ORDER BY chunk_id`, memoryID, target.Provider, target.Model, target.Dimensions, target.Scope)
	if err != nil {
		return nil, fmt.Errorf("list memory embeddings: %w", err)
	}
	defer rows.Close()

	var embeddings []MemoryEmbedding
	for rows.Next() {
		embedding, err := scanMemoryEmbedding(rows)
		if err != nil {
			return nil, err
		}
		embeddings = append(embeddings, embedding)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list memory embeddings: %w", err)
	}
	return embeddings, nil
}

// ListChunksMissingEmbeddings returns active chunks without an embedding for the target.
func (r *MemoryRepository) ListChunksMissingEmbeddings(ctx context.Context, opts EmbeddingBackfillOptions) ([]MemoryChunk, error) {
	candidates, err := r.ListEmbeddingBackfillCandidates(ctx, opts)
	if err != nil {
		return nil, err
	}
	chunks := make([]MemoryChunk, 0, len(candidates))
	for _, candidate := range candidates {
		chunks = append(chunks, candidate.Chunk)
	}
	return chunks, nil
}

// ListEmbeddingBackfillCandidates returns active memories needing scoped embedding work.
func (r *MemoryRepository) ListEmbeddingBackfillCandidates(ctx context.Context, opts EmbeddingBackfillOptions) ([]EmbeddingBackfillCandidate, error) {
	if err := validateEmbeddingTarget(opts.Target); err != nil {
		return nil, err
	}
	if opts.Limit <= 0 {
		return nil, fmt.Errorf("%w: embedding backfill limit must be positive", ErrInvalid)
	}

	rows, err := r.exec.QueryContext(ctx, `
SELECT memory_items.id, memory_items.title, memory_items.body, memory_items.source,
       memory_items.metadata_json, memory_items.tier, memory_items.importance,
       memory_items.pinned, memory_items.created_at, memory_items.updated_at,
       memory_items.last_accessed_at, memory_items.archived_at, memory_items.deleted_at,
       memory_chunks.id, memory_chunks.memory_id, memory_chunks.chunk_index,
       memory_chunks.content, memory_chunks.created_at
FROM memory_chunks
JOIN memory_items ON memory_items.id = memory_chunks.memory_id
LEFT JOIN memory_embeddings
  ON memory_embeddings.chunk_id = memory_chunks.id
 AND memory_embeddings.provider = ?
 AND memory_embeddings.model = ?
 AND memory_embeddings.dimensions = ?
 AND memory_embeddings.embedding_scope = ?
LEFT JOIN embedding_index_status
  ON embedding_index_status.chunk_id = memory_chunks.id
 AND embedding_index_status.provider = ?
 AND embedding_index_status.model = ?
 AND embedding_index_status.embedding_scope = ?
WHERE memory_items.deleted_at IS NULL
  AND (
    ?
    OR memory_embeddings.chunk_id IS NULL
    OR embedding_index_status.status = 'failed'
  )
ORDER BY memory_chunks.created_at, memory_chunks.id
LIMIT ?`,
		opts.Target.Provider,
		opts.Target.Model,
		opts.Target.Dimensions,
		opts.Target.Scope,
		opts.Target.Provider,
		opts.Target.Model,
		opts.Target.Scope,
		boolInt(opts.Force),
		opts.Limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list embedding backfill candidates: %w", err)
	}
	defer rows.Close()

	var candidates []EmbeddingBackfillCandidate
	var memoryIDs []string
	for rows.Next() {
		candidate, err := scanEmbeddingBackfillCandidate(rows)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, candidate)
		memoryIDs = append(memoryIDs, candidate.Item.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list embedding backfill candidates: %w", err)
	}
	keywordsByMemory, err := r.ListKeywordsForMemories(ctx, memoryIDs)
	if err != nil {
		return nil, err
	}
	for index := range candidates {
		candidates[index].Keywords = keywordsByMemory[candidates[index].Item.ID]
	}
	return candidates, nil
}

// UpsertEmbeddingIndexStatus records the latest best-effort indexing result.
func (r *MemoryRepository) UpsertEmbeddingIndexStatus(ctx context.Context, status EmbeddingIndexStatus) error {
	if err := validateEmbeddingIndexStatus(status); err != nil {
		return err
	}
	_, err := r.exec.ExecContext(ctx, `
INSERT INTO embedding_index_status (
  chunk_id, memory_id, provider, model, dimensions, embedding_scope,
  status, content_hash, error_summary, attempts, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(chunk_id, provider, model, embedding_scope) DO UPDATE SET
  memory_id = excluded.memory_id,
  dimensions = excluded.dimensions,
  status = excluded.status,
  content_hash = excluded.content_hash,
  error_summary = excluded.error_summary,
  attempts = embedding_index_status.attempts + excluded.attempts,
  updated_at = excluded.updated_at`,
		status.ChunkID,
		status.MemoryID,
		status.Provider,
		status.Model,
		status.Dimensions,
		status.EmbeddingScope,
		status.Status,
		status.ContentHash,
		status.ErrorSummary,
		status.Attempts,
		formatTime(status.CreatedAt),
		formatTime(status.UpdatedAt),
	)
	if err != nil {
		return fmt.Errorf("upsert embedding index status: %w", err)
	}
	return nil
}

// Search performs FTS-backed or hybrid memory search with safe filters and explainable ranking.
func (r *MemoryRepository) Search(ctx context.Context, opts SearchOptions) ([]SearchResult, error) {
	if opts.Limit <= 0 {
		return nil, fmt.Errorf("%w: search limit must be positive", ErrInvalid)
	}
	if !opts.Depth.Valid() {
		return nil, fmt.Errorf("%w: invalid search depth %q", ErrInvalid, opts.Depth)
	}
	match, err := buildFTSQuery(opts.Query)
	if err != nil {
		return nil, err
	}
	if opts.Now.IsZero() {
		opts.Now = time.Now().UTC()
	}

	memoryWhere, memoryArgs, err := buildMemorySearchFilters(opts)
	if err != nil {
		return nil, err
	}
	accessSince := opts.Now.Add(-30 * 24 * time.Hour)
	candidateLimit := opts.Depth.CandidateLimit(opts.Limit)
	ftsWhere := append([]string{"memory_fts MATCH ?"}, memoryWhere...)
	ftsWhereArgs := append([]any{match}, memoryArgs...)
	args := append([]any{formatTime(accessSince)}, ftsWhereArgs...)
	args = append(args, candidateLimit)

	rows, err := r.exec.QueryContext(ctx, `
SELECT memory_items.id, memory_items.title, memory_items.body, memory_items.source,
       memory_items.metadata_json, memory_items.tier, memory_items.importance,
       memory_items.pinned, memory_items.created_at, memory_items.updated_at,
       memory_items.last_accessed_at, memory_items.archived_at, memory_items.deleted_at,
       memory_fts.memory_id, memory_fts.chunk_id,
       snippet(memory_fts, 0, '[', ']', '...', 20),
       bm25(memory_fts),
       (
         SELECT COUNT(*)
         FROM access_log
         WHERE access_log.memory_id = memory_items.id AND access_log.created_at >= ?
       )
FROM memory_fts
JOIN memory_items ON memory_items.id = memory_fts.memory_id
WHERE `+strings.Join(ftsWhere, " AND ")+`
ORDER BY bm25(memory_fts), memory_items.updated_at DESC
LIMIT ?`, args...)
	if err != nil {
		return nil, fmt.Errorf("search memories: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	byMemoryID := map[string]int{}
	for rows.Next() {
		result, err := scanMemorySearchResult(rows, opts.Now)
		if err != nil {
			return nil, err
		}
		mergeSearchResult(&results, byMemoryID, result, opts.Now)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search memories: %w", err)
	}
	if opts.Vector != nil {
		vectorResults, err := r.searchVectorCandidates(ctx, opts, memoryWhere, memoryArgs, candidateLimit)
		if err != nil {
			return nil, err
		}
		for _, result := range vectorResults {
			mergeSearchResult(&results, byMemoryID, result, opts.Now)
		}
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Score.Total == results[j].Score.Total {
			return results[i].Item.UpdatedAt.After(results[j].Item.UpdatedAt)
		}
		return results[i].Score.Total > results[j].Score.Total
	})
	if len(results) > opts.Limit {
		results = results[:opts.Limit]
	}
	return results, nil
}

func buildMemorySearchFilters(opts SearchOptions) ([]string, []any, error) {
	where := []string{}
	args := []any{}
	if !opts.IncludeDeleted {
		where = append(where, "memory_items.deleted_at IS NULL")
	}
	if opts.Tier != nil {
		if !opts.Tier.Valid() {
			return nil, nil, fmt.Errorf("%w: invalid memory tier %q", ErrInvalid, *opts.Tier)
		}
		where = append(where, "memory_items.tier = ?")
		args = append(args, string(*opts.Tier))
	}
	if opts.Pinned != nil {
		where = append(where, "memory_items.pinned = ?")
		args = append(args, boolInt(*opts.Pinned))
	}
	if opts.Source != nil {
		where = append(where, "memory_items.source = ?")
		args = append(args, *opts.Source)
	}
	if opts.CreatedAfter != nil {
		where = append(where, "memory_items.created_at >= ?")
		args = append(args, formatTime(*opts.CreatedAfter))
	}
	if opts.CreatedBefore != nil {
		where = append(where, "memory_items.created_at <= ?")
		args = append(args, formatTime(*opts.CreatedBefore))
	}
	if opts.UpdatedAfter != nil {
		where = append(where, "memory_items.updated_at >= ?")
		args = append(args, formatTime(*opts.UpdatedAfter))
	}
	if opts.UpdatedBefore != nil {
		where = append(where, "memory_items.updated_at <= ?")
		args = append(args, formatTime(*opts.UpdatedBefore))
	}
	for key, value := range opts.Metadata {
		if err := validateMetadataFilter(key, value); err != nil {
			return nil, nil, err
		}
		where = append(where, "json_extract(memory_items.metadata_json, ?) = ?")
		args = append(args, "$."+key, metadataSQLValue(value))
	}
	return where, args, nil
}

func (r *MemoryRepository) searchVectorCandidates(ctx context.Context, opts SearchOptions, memoryWhere []string, memoryArgs []any, candidateLimit int) ([]SearchResult, error) {
	if opts.Vector == nil {
		return nil, nil
	}
	if err := validateEmbeddingTarget(opts.Vector.Target); err != nil {
		return nil, err
	}
	if len(opts.Vector.QueryEmbedding) != opts.Vector.Target.Dimensions {
		return nil, fmt.Errorf("%w: query embedding dimensions do not match target", ErrInvalid)
	}
	backend, err := r.vectorBackend(ctx, opts.Vector.Target)
	if err != nil {
		return nil, err
	}
	if backend == VectorBackendSQLiteVec {
		return r.searchSQLiteVecCandidates(ctx, opts, memoryWhere, memoryArgs, candidateLimit)
	}
	vectorScanLimit := candidateLimit * 10
	if vectorScanLimit < candidateLimit {
		vectorScanLimit = candidateLimit
	}
	if vectorScanLimit > 2000 {
		vectorScanLimit = 2000
	}

	where := append([]string{}, memoryWhere...)
	where = append(where,
		"memory_embeddings.provider = ?",
		"memory_embeddings.model = ?",
		"memory_embeddings.dimensions = ?",
		"memory_embeddings.embedding_scope = ?",
	)
	args := []any{formatTime(opts.Now.Add(-30 * 24 * time.Hour))}
	args = append(args, memoryArgs...)
	args = append(args,
		opts.Vector.Target.Provider,
		opts.Vector.Target.Model,
		opts.Vector.Target.Dimensions,
		opts.Vector.Target.Scope,
		vectorScanLimit,
	)

	rows, err := r.exec.QueryContext(ctx, `
SELECT memory_items.id, memory_items.title, memory_items.body, memory_items.source,
       memory_items.metadata_json, memory_items.tier, memory_items.importance,
       memory_items.pinned, memory_items.created_at, memory_items.updated_at,
       memory_items.last_accessed_at, memory_items.archived_at, memory_items.deleted_at,
       memory_embeddings.memory_id, memory_embeddings.chunk_id,
       memory_chunks.content,
       memory_embeddings.embedding_json,
       (
         SELECT COUNT(*)
         FROM access_log
         WHERE access_log.memory_id = memory_items.id AND access_log.created_at >= ?
       )
FROM memory_embeddings
JOIN memory_items ON memory_items.id = memory_embeddings.memory_id
JOIN memory_chunks ON memory_chunks.id = memory_embeddings.chunk_id
WHERE `+strings.Join(where, " AND ")+`
ORDER BY memory_items.updated_at DESC
LIMIT ?`, args...)
	if err != nil {
		return nil, fmt.Errorf("search vector memories: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	byMemoryID := map[string]int{}
	for rows.Next() {
		result, err := scanVectorSearchResult(rows, opts.Now, opts.Vector.QueryEmbedding)
		if err != nil {
			return nil, err
		}
		mergeSearchResult(&results, byMemoryID, result, opts.Now)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search vector memories: %w", err)
	}
	sort.SliceStable(results, func(i, j int) bool {
		left, right := -1.0, -1.0
		if results[i].VectorSimilarity != nil {
			left = *results[i].VectorSimilarity
		}
		if results[j].VectorSimilarity != nil {
			right = *results[j].VectorSimilarity
		}
		if left == right {
			return results[i].Item.UpdatedAt.After(results[j].Item.UpdatedAt)
		}
		return left > right
	})
	if len(results) > candidateLimit {
		results = results[:candidateLimit]
	}
	return results, nil
}

func (r *MemoryRepository) searchSQLiteVecCandidates(ctx context.Context, opts SearchOptions, memoryWhere []string, memoryArgs []any, candidateLimit int) ([]SearchResult, error) {
	if opts.Vector == nil {
		return nil, nil
	}
	if err := r.ensureSQLiteVecTable(ctx, opts.Vector.Target); err != nil {
		return nil, err
	}
	vectorScanLimit := candidateLimit * 10
	if vectorScanLimit < candidateLimit {
		vectorScanLimit = candidateLimit
	}
	if vectorScanLimit > 2000 {
		vectorScanLimit = 2000
	}

	where := append([]string{}, memoryWhere...)
	where = append(where,
		"memory_embeddings.provider = ?",
		"memory_embeddings.model = ?",
		"memory_embeddings.dimensions = ?",
		"memory_embeddings.embedding_scope = ?",
		"memory_embeddings.vector_rowid IS NOT NULL",
	)
	queryEmbedding, err := encodeEmbeddingJSON(opts.Vector.QueryEmbedding)
	if err != nil {
		return nil, err
	}
	args := []any{formatTime(opts.Now.Add(-30 * 24 * time.Hour)), queryEmbedding, vectorScanLimit}
	args = append(args, memoryArgs...)
	args = append(args,
		opts.Vector.Target.Provider,
		opts.Vector.Target.Model,
		opts.Vector.Target.Dimensions,
		opts.Vector.Target.Scope,
		vectorScanLimit,
	)

	tableName := quoteIdentifier(sqliteVecTableName(opts.Vector.Target))
	rows, err := r.exec.QueryContext(ctx, `
SELECT memory_items.id, memory_items.title, memory_items.body, memory_items.source,
       memory_items.metadata_json, memory_items.tier, memory_items.importance,
       memory_items.pinned, memory_items.created_at, memory_items.updated_at,
       memory_items.last_accessed_at, memory_items.archived_at, memory_items.deleted_at,
       memory_embeddings.memory_id, memory_embeddings.chunk_id,
       memory_chunks.content,
       vector_index.distance,
       (
         SELECT COUNT(*)
         FROM access_log
         WHERE access_log.memory_id = memory_items.id AND access_log.created_at >= ?
       )
FROM `+tableName+` AS vector_index
JOIN memory_embeddings ON memory_embeddings.vector_rowid = vector_index.rowid
JOIN memory_items ON memory_items.id = memory_embeddings.memory_id
JOIN memory_chunks ON memory_chunks.id = memory_embeddings.chunk_id
WHERE vector_index.embedding MATCH vec_f32(?)
  AND vector_index.k = ?
  AND `+strings.Join(where, " AND ")+`
ORDER BY vector_index.distance
LIMIT ?`, args...)
	if err != nil {
		return nil, fmt.Errorf("search sqlite-vec memories: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	byMemoryID := map[string]int{}
	for rows.Next() {
		result, err := scanSQLiteVecSearchResult(rows, opts.Now)
		if err != nil {
			return nil, err
		}
		mergeSearchResult(&results, byMemoryID, result, opts.Now)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search sqlite-vec memories: %w", err)
	}
	if len(results) > candidateLimit {
		results = results[:candidateLimit]
	}
	return results, nil
}

func mergeSearchResult(results *[]SearchResult, byMemoryID map[string]int, result SearchResult, now time.Time) {
	result.Score = scoreSearchResult(result, now)
	if existing, ok := byMemoryID[result.MemoryID]; ok {
		(*results)[existing] = combineSearchResults((*results)[existing], result, now)
		return
	}
	byMemoryID[result.MemoryID] = len(*results)
	*results = append(*results, result)
}

func combineSearchResults(left, right SearchResult, now time.Time) SearchResult {
	combined := left
	if right.Score.Keyword > left.Score.Keyword {
		combined.ChunkID = right.ChunkID
		combined.Snippet = right.Snippet
		combined.FTSRank = right.FTSRank
		combined.KeywordMatched = right.KeywordMatched
	}
	if !combined.KeywordMatched && right.Snippet != "" {
		combined.ChunkID = right.ChunkID
		combined.Snippet = right.Snippet
	}
	if right.VectorSimilarity != nil && (combined.VectorSimilarity == nil || *right.VectorSimilarity > *combined.VectorSimilarity) {
		combined.VectorSimilarity = right.VectorSimilarity
	}
	if right.AccessCount > combined.AccessCount {
		combined.AccessCount = right.AccessCount
	}
	combined.Score = scoreSearchResult(combined, now)
	return combined
}

func scoreSearchResult(result SearchResult, now time.Time) search.Score {
	return search.ScoreCandidate(search.Candidate{
		FTSRank:          result.FTSRank,
		KeywordMatched:   result.KeywordMatched,
		VectorSimilarity: result.VectorSimilarity,
		Tier:             string(result.Item.Tier),
		Pinned:           result.Item.Pinned,
		Importance:       result.Item.Importance,
		UpdatedAt:        result.Item.UpdatedAt,
		LastAccessedAt:   result.Item.LastAccessedAt,
		AccessCount:      result.AccessCount,
		Now:              now,
	})
}

// Recent returns recently updated memories.
func (r *MemoryRepository) Recent(ctx context.Context, opts RecentOptions) ([]MemoryItem, error) {
	if opts.Limit <= 0 {
		return nil, fmt.Errorf("%w: recent limit must be positive", ErrInvalid)
	}
	where := "deleted_at IS NULL"
	if opts.IncludeDeleted {
		where = "1 = 1"
	}

	rows, err := r.exec.QueryContext(ctx, `
SELECT id, title, body, source, metadata_json, tier, importance, pinned,
       created_at, updated_at, last_accessed_at, archived_at, deleted_at
FROM memory_items
WHERE `+where+`
ORDER BY updated_at DESC
LIMIT ?`, opts.Limit)
	if err != nil {
		return nil, fmt.Errorf("recent memories: %w", err)
	}
	defer rows.Close()

	var items []MemoryItem
	for rows.Next() {
		item, err := scanMemoryItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("recent memories: %w", err)
	}
	return items, nil
}

// ListActive returns active memories for lifecycle evaluation.
func (r *MemoryRepository) ListActive(ctx context.Context, opts LifecycleListOptions) ([]MemoryItem, error) {
	if opts.Limit <= 0 {
		return nil, fmt.Errorf("%w: lifecycle limit must be positive", ErrInvalid)
	}

	rows, err := r.exec.QueryContext(ctx, `
SELECT id, title, body, source, metadata_json, tier, importance, pinned,
       created_at, updated_at, last_accessed_at, archived_at, deleted_at
FROM memory_items
WHERE deleted_at IS NULL
ORDER BY updated_at ASC
LIMIT ?`, opts.Limit)
	if err != nil {
		return nil, fmt.Errorf("list active memories: %w", err)
	}
	defer rows.Close()

	var items []MemoryItem
	for rows.Next() {
		item, err := scanMemoryItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list active memories: %w", err)
	}
	return items, nil
}

// Stats returns aggregate memory counts.
func (r *MemoryRepository) Stats(ctx context.Context) (Stats, error) {
	var stats Stats
	if err := r.exec.QueryRowContext(ctx, `
SELECT
  COUNT(*),
  COALESCE(SUM(CASE WHEN deleted_at IS NULL THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN deleted_at IS NOT NULL THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN pinned = 1 AND deleted_at IS NULL THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN tier = 'working' AND deleted_at IS NULL THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN tier = 'hot' AND deleted_at IS NULL THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN tier = 'warm' AND deleted_at IS NULL THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN tier = 'cold' AND deleted_at IS NULL THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN tier = 'archive' AND deleted_at IS NULL THEN 1 ELSE 0 END), 0)
FROM memory_items`).Scan(
		&stats.Total,
		&stats.Active,
		&stats.Deleted,
		&stats.Pinned,
		&stats.Working,
		&stats.Hot,
		&stats.Warm,
		&stats.Cold,
		&stats.Archive,
	); err != nil {
		return Stats{}, fmt.Errorf("memory stats: %w", err)
	}
	if err := r.exec.QueryRowContext(ctx, "SELECT COUNT(*) FROM access_log").Scan(&stats.AccessEvents); err != nil {
		return Stats{}, fmt.Errorf("access stats: %w", err)
	}
	return stats, nil
}

// RecordEvent appends a memory event and returns its generated ID.
func (r *MemoryRepository) RecordEvent(ctx context.Context, event MemoryEvent) (int64, error) {
	if event.EventType == "" {
		return 0, fmt.Errorf("%w: event type must not be empty", ErrInvalid)
	}
	if event.CreatedAt.IsZero() {
		return 0, fmt.Errorf("%w: event created_at must not be zero", ErrInvalid)
	}

	result, err := r.exec.ExecContext(ctx, `
INSERT INTO memory_events (memory_id, event_type, event_payload_json, created_at)
VALUES (?, ?, ?, ?)`,
		nullString(event.MemoryID),
		event.EventType,
		defaultJSON(event.EventPayloadJSON),
		formatTime(event.CreatedAt),
	)
	if err != nil {
		return 0, fmt.Errorf("record memory event: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("memory event id: %w", err)
	}
	return id, nil
}

// ListEvents returns memory events ordered by creation and ID.
func (r *MemoryRepository) ListEvents(ctx context.Context, memoryID string) ([]MemoryEvent, error) {
	if err := requireID("memory", memoryID); err != nil {
		return nil, err
	}

	rows, err := r.exec.QueryContext(ctx, `
SELECT id, memory_id, event_type, event_payload_json, created_at
FROM memory_events
WHERE memory_id = ?
ORDER BY created_at, id`, memoryID)
	if err != nil {
		return nil, fmt.Errorf("list memory events: %w", err)
	}
	defer rows.Close()

	var events []MemoryEvent
	for rows.Next() {
		var event MemoryEvent
		var createdAt string
		if err := rows.Scan(&event.ID, &event.MemoryID, &event.EventType, &event.EventPayloadJSON, &createdAt); err != nil {
			return nil, fmt.Errorf("scan memory event: %w", err)
		}
		parsed, err := parseTime(createdAt)
		if err != nil {
			return nil, fmt.Errorf("parse event created_at: %w", err)
		}
		event.CreatedAt = parsed
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list memory events: %w", err)
	}
	return events, nil
}

type scanner interface {
	Scan(...any) error
}

func scanMemoryItem(row scanner) (MemoryItem, error) {
	var item MemoryItem
	var tier string
	var pinned int
	var createdAt, updatedAt string
	var lastAccessedAt, archivedAt, deletedAt sql.NullString
	if err := row.Scan(
		&item.ID,
		&item.Title,
		&item.Body,
		&item.Source,
		&item.MetadataJSON,
		&tier,
		&item.Importance,
		&pinned,
		&createdAt,
		&updatedAt,
		&lastAccessedAt,
		&archivedAt,
		&deletedAt,
	); err != nil {
		return MemoryItem{}, fmt.Errorf("scan memory item: %w", err)
	}

	var err error
	item.Tier = Tier(tier)
	item.Pinned = intBool(pinned)
	if item.CreatedAt, err = parseTime(createdAt); err != nil {
		return MemoryItem{}, fmt.Errorf("parse created_at: %w", err)
	}
	if item.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return MemoryItem{}, fmt.Errorf("parse updated_at: %w", err)
	}
	if item.LastAccessedAt, err = parseNullableTime(lastAccessedAt); err != nil {
		return MemoryItem{}, fmt.Errorf("parse last_accessed_at: %w", err)
	}
	if item.ArchivedAt, err = parseNullableTime(archivedAt); err != nil {
		return MemoryItem{}, fmt.Errorf("parse archived_at: %w", err)
	}
	if item.DeletedAt, err = parseNullableTime(deletedAt); err != nil {
		return MemoryItem{}, fmt.Errorf("parse deleted_at: %w", err)
	}
	return item, nil
}

func scanMemoryKeywords(rows *sql.Rows) ([]MemoryKeyword, error) {
	var keywords []MemoryKeyword
	for rows.Next() {
		var keyword MemoryKeyword
		var createdAt, updatedAt string
		if err := rows.Scan(
			&keyword.MemoryID,
			&keyword.KeywordIndex,
			&keyword.Keyword,
			&keyword.NormalizedKeyword,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan memory keyword: %w", err)
		}
		var err error
		if keyword.CreatedAt, err = parseTime(createdAt); err != nil {
			return nil, fmt.Errorf("parse keyword created_at: %w", err)
		}
		if keyword.UpdatedAt, err = parseTime(updatedAt); err != nil {
			return nil, fmt.Errorf("parse keyword updated_at: %w", err)
		}
		keywords = append(keywords, keyword)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list memory keywords: %w", err)
	}
	return keywords, nil
}

func scanEmbeddingBackfillCandidate(row scanner) (EmbeddingBackfillCandidate, error) {
	var item MemoryItem
	var chunk MemoryChunk
	var tier string
	var pinned int
	var itemCreatedAt, itemUpdatedAt, chunkCreatedAt string
	var lastAccessedAt, archivedAt, deletedAt sql.NullString
	if err := row.Scan(
		&item.ID,
		&item.Title,
		&item.Body,
		&item.Source,
		&item.MetadataJSON,
		&tier,
		&item.Importance,
		&pinned,
		&itemCreatedAt,
		&itemUpdatedAt,
		&lastAccessedAt,
		&archivedAt,
		&deletedAt,
		&chunk.ID,
		&chunk.MemoryID,
		&chunk.ChunkIndex,
		&chunk.Content,
		&chunkCreatedAt,
	); err != nil {
		return EmbeddingBackfillCandidate{}, fmt.Errorf("scan embedding backfill candidate: %w", err)
	}
	var err error
	item.Tier = Tier(tier)
	item.Pinned = intBool(pinned)
	if item.CreatedAt, err = parseTime(itemCreatedAt); err != nil {
		return EmbeddingBackfillCandidate{}, fmt.Errorf("parse created_at: %w", err)
	}
	if item.UpdatedAt, err = parseTime(itemUpdatedAt); err != nil {
		return EmbeddingBackfillCandidate{}, fmt.Errorf("parse updated_at: %w", err)
	}
	if item.LastAccessedAt, err = parseNullableTime(lastAccessedAt); err != nil {
		return EmbeddingBackfillCandidate{}, fmt.Errorf("parse last_accessed_at: %w", err)
	}
	if item.ArchivedAt, err = parseNullableTime(archivedAt); err != nil {
		return EmbeddingBackfillCandidate{}, fmt.Errorf("parse archived_at: %w", err)
	}
	if item.DeletedAt, err = parseNullableTime(deletedAt); err != nil {
		return EmbeddingBackfillCandidate{}, fmt.Errorf("parse deleted_at: %w", err)
	}
	if chunk.CreatedAt, err = parseTime(chunkCreatedAt); err != nil {
		return EmbeddingBackfillCandidate{}, fmt.Errorf("parse chunk created_at: %w", err)
	}
	return EmbeddingBackfillCandidate{Item: item, Chunk: chunk}, nil
}

func scanMemorySearchResult(row scanner, now time.Time) (SearchResult, error) {
	var item MemoryItem
	var result SearchResult
	var snippet string
	var tier string
	var pinned int
	var createdAt, updatedAt string
	var lastAccessedAt, archivedAt, deletedAt sql.NullString
	if err := row.Scan(
		&item.ID,
		&item.Title,
		&item.Body,
		&item.Source,
		&item.MetadataJSON,
		&tier,
		&item.Importance,
		&pinned,
		&createdAt,
		&updatedAt,
		&lastAccessedAt,
		&archivedAt,
		&deletedAt,
		&result.MemoryID,
		&result.ChunkID,
		&snippet,
		&result.FTSRank,
		&result.AccessCount,
	); err != nil {
		return SearchResult{}, fmt.Errorf("scan memory search result: %w", err)
	}

	var err error
	item.Tier = Tier(tier)
	item.Pinned = intBool(pinned)
	if item.CreatedAt, err = parseTime(createdAt); err != nil {
		return SearchResult{}, fmt.Errorf("parse created_at: %w", err)
	}
	if item.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return SearchResult{}, fmt.Errorf("parse updated_at: %w", err)
	}
	if item.LastAccessedAt, err = parseNullableTime(lastAccessedAt); err != nil {
		return SearchResult{}, fmt.Errorf("parse last_accessed_at: %w", err)
	}
	if item.ArchivedAt, err = parseNullableTime(archivedAt); err != nil {
		return SearchResult{}, fmt.Errorf("parse archived_at: %w", err)
	}
	if item.DeletedAt, err = parseNullableTime(deletedAt); err != nil {
		return SearchResult{}, fmt.Errorf("parse deleted_at: %w", err)
	}
	result.Item = item
	result.Snippet = snippet
	result.KeywordMatched = true
	result.Score = scoreSearchResult(result, now)
	return result, nil
}

func scanVectorSearchResult(row scanner, now time.Time, queryEmbedding []float64) (SearchResult, error) {
	var item MemoryItem
	var result SearchResult
	var content string
	var embeddingJSON string
	var tier string
	var pinned int
	var createdAt, updatedAt string
	var lastAccessedAt, archivedAt, deletedAt sql.NullString
	if err := row.Scan(
		&item.ID,
		&item.Title,
		&item.Body,
		&item.Source,
		&item.MetadataJSON,
		&tier,
		&item.Importance,
		&pinned,
		&createdAt,
		&updatedAt,
		&lastAccessedAt,
		&archivedAt,
		&deletedAt,
		&result.MemoryID,
		&result.ChunkID,
		&content,
		&embeddingJSON,
		&result.AccessCount,
	); err != nil {
		return SearchResult{}, fmt.Errorf("scan vector search result: %w", err)
	}

	var err error
	item.Tier = Tier(tier)
	item.Pinned = intBool(pinned)
	if item.CreatedAt, err = parseTime(createdAt); err != nil {
		return SearchResult{}, fmt.Errorf("parse created_at: %w", err)
	}
	if item.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return SearchResult{}, fmt.Errorf("parse updated_at: %w", err)
	}
	if item.LastAccessedAt, err = parseNullableTime(lastAccessedAt); err != nil {
		return SearchResult{}, fmt.Errorf("parse last_accessed_at: %w", err)
	}
	if item.ArchivedAt, err = parseNullableTime(archivedAt); err != nil {
		return SearchResult{}, fmt.Errorf("parse archived_at: %w", err)
	}
	if item.DeletedAt, err = parseNullableTime(deletedAt); err != nil {
		return SearchResult{}, fmt.Errorf("parse deleted_at: %w", err)
	}

	embedding, err := decodeEmbeddingJSON(embeddingJSON)
	if err != nil {
		return SearchResult{}, err
	}
	similarity, err := cosineSimilarity(queryEmbedding, embedding)
	if err != nil {
		return SearchResult{}, err
	}
	result.Item = item
	result.Snippet = plainSnippet(content)
	result.VectorSimilarity = &similarity
	result.Score = scoreSearchResult(result, now)
	return result, nil
}

func scanSQLiteVecSearchResult(row scanner, now time.Time) (SearchResult, error) {
	var item MemoryItem
	var result SearchResult
	var content string
	var distance float64
	var tier string
	var pinned int
	var createdAt, updatedAt string
	var lastAccessedAt, archivedAt, deletedAt sql.NullString
	if err := row.Scan(
		&item.ID,
		&item.Title,
		&item.Body,
		&item.Source,
		&item.MetadataJSON,
		&tier,
		&item.Importance,
		&pinned,
		&createdAt,
		&updatedAt,
		&lastAccessedAt,
		&archivedAt,
		&deletedAt,
		&result.MemoryID,
		&result.ChunkID,
		&content,
		&distance,
		&result.AccessCount,
	); err != nil {
		return SearchResult{}, fmt.Errorf("scan sqlite-vec search result: %w", err)
	}

	var err error
	item.Tier = Tier(tier)
	item.Pinned = intBool(pinned)
	if item.CreatedAt, err = parseTime(createdAt); err != nil {
		return SearchResult{}, fmt.Errorf("parse created_at: %w", err)
	}
	if item.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return SearchResult{}, fmt.Errorf("parse updated_at: %w", err)
	}
	if item.LastAccessedAt, err = parseNullableTime(lastAccessedAt); err != nil {
		return SearchResult{}, fmt.Errorf("parse last_accessed_at: %w", err)
	}
	if item.ArchivedAt, err = parseNullableTime(archivedAt); err != nil {
		return SearchResult{}, fmt.Errorf("parse archived_at: %w", err)
	}
	if item.DeletedAt, err = parseNullableTime(deletedAt); err != nil {
		return SearchResult{}, fmt.Errorf("parse deleted_at: %w", err)
	}

	similarity := l2DistanceSimilarity(distance)
	result.Item = item
	result.Snippet = plainSnippet(content)
	result.VectorSimilarity = &similarity
	result.Score = scoreSearchResult(result, now)
	return result, nil
}

func scanMemoryEmbedding(row scanner) (MemoryEmbedding, error) {
	var embedding MemoryEmbedding
	var vectorRowID sql.NullInt64
	var createdAt, updatedAt string
	if err := row.Scan(
		&embedding.ChunkID,
		&embedding.MemoryID,
		&embedding.Provider,
		&embedding.Model,
		&embedding.Dimensions,
		&embedding.EmbeddingJSON,
		&embedding.ContentHash,
		&vectorRowID,
		&embedding.EmbeddingScope,
		&createdAt,
		&updatedAt,
	); err != nil {
		return MemoryEmbedding{}, fmt.Errorf("scan memory embedding: %w", err)
	}
	if vectorRowID.Valid {
		embedding.VectorRowID = vectorRowID.Int64
	}
	var err error
	if embedding.CreatedAt, err = parseTime(createdAt); err != nil {
		return MemoryEmbedding{}, fmt.Errorf("parse embedding created_at: %w", err)
	}
	if embedding.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return MemoryEmbedding{}, fmt.Errorf("parse embedding updated_at: %w", err)
	}
	return embedding, nil
}

func validateMetadataFilter(key string, value any) error {
	if key == "" {
		return fmt.Errorf("%w: metadata filter key must not be empty", ErrInvalid)
	}
	if len(key) > 64 {
		return fmt.Errorf("%w: metadata filter key is too long", ErrInvalid)
	}
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '-':
		default:
			return fmt.Errorf("%w: metadata filter key %q contains unsupported characters", ErrInvalid, key)
		}
	}
	switch value.(type) {
	case string, bool, float64, int, int64:
		return nil
	default:
		return fmt.Errorf("%w: metadata filter %q must be string, number, or boolean", ErrInvalid, key)
	}
}

func metadataSQLValue(value any) any {
	switch typed := value.(type) {
	case bool:
		if typed {
			return 1
		}
		return 0
	default:
		return typed
	}
}

func buildFTSQuery(query string) (string, error) {
	fields := strings.Fields(query)
	if len(fields) == 0 {
		return "", fmt.Errorf("%w: search query must not be empty", ErrInvalid)
	}

	terms := make([]string, 0, len(fields))
	for _, field := range fields {
		term := sanitizeFTSTerm(field)
		if term != "" {
			terms = append(terms, `"`+term+`"`)
		}
	}
	if len(terms) == 0 {
		return "", fmt.Errorf("%w: search query must include searchable text", ErrInvalid)
	}
	return strings.Join(terms, " "), nil
}

func sanitizeFTSTerm(value string) string {
	value = strings.TrimSpace(value)
	var builder strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '_' || r == '-':
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func validateMemoryItem(item MemoryItem) error {
	if err := requireID("memory", item.ID); err != nil {
		return err
	}
	if item.Body == "" {
		return fmt.Errorf("%w: memory body must not be empty", ErrInvalid)
	}
	if !item.Tier.Valid() {
		return fmt.Errorf("%w: invalid memory tier %q", ErrInvalid, item.Tier)
	}
	if item.Importance < 0 || item.Importance > 100 {
		return fmt.Errorf("%w: memory importance must be between 0 and 100", ErrInvalid)
	}
	if item.CreatedAt.IsZero() {
		return fmt.Errorf("%w: memory created_at must not be zero", ErrInvalid)
	}
	if item.UpdatedAt.IsZero() {
		return fmt.Errorf("%w: memory updated_at must not be zero", ErrInvalid)
	}
	return nil
}

func validateMemoryChunk(chunk MemoryChunk) error {
	if err := requireID("chunk", chunk.ID); err != nil {
		return err
	}
	if err := requireID("memory", chunk.MemoryID); err != nil {
		return err
	}
	if chunk.ChunkIndex < 0 {
		return fmt.Errorf("%w: chunk index must not be negative", ErrInvalid)
	}
	if chunk.Content == "" {
		return fmt.Errorf("%w: chunk content must not be empty", ErrInvalid)
	}
	if chunk.CreatedAt.IsZero() {
		return fmt.Errorf("%w: chunk created_at must not be zero", ErrInvalid)
	}
	return nil
}

func validateMemoryKeyword(keyword MemoryKeyword) error {
	if err := requireID("memory", keyword.MemoryID); err != nil {
		return err
	}
	if keyword.KeywordIndex < 0 {
		return fmt.Errorf("%w: keyword index must not be negative", ErrInvalid)
	}
	if strings.TrimSpace(keyword.Keyword) == "" {
		return fmt.Errorf("%w: keyword must not be empty", ErrInvalid)
	}
	if strings.TrimSpace(keyword.NormalizedKeyword) == "" {
		return fmt.Errorf("%w: normalized keyword must not be empty", ErrInvalid)
	}
	if keyword.CreatedAt.IsZero() {
		return fmt.Errorf("%w: keyword created_at must not be zero", ErrInvalid)
	}
	if keyword.UpdatedAt.IsZero() {
		return fmt.Errorf("%w: keyword updated_at must not be zero", ErrInvalid)
	}
	return nil
}

func validateEmbeddingTarget(target EmbeddingTarget) error {
	if strings.TrimSpace(target.Provider) == "" {
		return fmt.Errorf("%w: embedding provider must not be empty", ErrInvalid)
	}
	if strings.TrimSpace(target.Model) == "" {
		return fmt.Errorf("%w: embedding model must not be empty", ErrInvalid)
	}
	if target.Dimensions <= 0 {
		return fmt.Errorf("%w: embedding dimensions must be positive", ErrInvalid)
	}
	if !validEmbeddingScope(target.Scope) {
		return fmt.Errorf("%w: unsupported embedding scope %q", ErrInvalid, target.Scope)
	}
	return nil
}

func validEmbeddingScope(scope string) bool {
	switch scope {
	case EmbeddingScopeBody, EmbeddingScopeTitleKeywords:
		return true
	default:
		return false
	}
}

func validateVectorMetadata(metadata VectorMetadata) error {
	if err := validateEmbeddingTarget(EmbeddingTarget{
		Provider:   metadata.Provider,
		Model:      metadata.Model,
		Dimensions: metadata.Dimensions,
		Scope:      metadata.EmbeddingScope,
	}); err != nil {
		return err
	}
	if strings.TrimSpace(metadata.Backend) == "" {
		return fmt.Errorf("%w: vector backend must not be empty", ErrInvalid)
	}
	switch metadata.Backend {
	case VectorBackendSQLiteJSON, VectorBackendSQLiteVec:
	default:
		return fmt.Errorf("%w: unsupported vector backend %q", ErrInvalid, metadata.Backend)
	}
	if metadata.DistanceMetric != "cosine" {
		return fmt.Errorf("%w: unsupported vector distance metric %q", ErrInvalid, metadata.DistanceMetric)
	}
	if metadata.CreatedAt.IsZero() {
		return fmt.Errorf("%w: vector metadata created_at must not be zero", ErrInvalid)
	}
	if metadata.UpdatedAt.IsZero() {
		return fmt.Errorf("%w: vector metadata updated_at must not be zero", ErrInvalid)
	}
	return nil
}

func validateMemoryEmbedding(embedding MemoryEmbedding) error {
	if err := requireID("chunk", embedding.ChunkID); err != nil {
		return err
	}
	if err := requireID("memory", embedding.MemoryID); err != nil {
		return err
	}
	if err := validateEmbeddingTarget(EmbeddingTarget{
		Provider:   embedding.Provider,
		Model:      embedding.Model,
		Dimensions: embedding.Dimensions,
		Scope:      embedding.EmbeddingScope,
	}); err != nil {
		return err
	}
	if strings.TrimSpace(embedding.EmbeddingJSON) == "" {
		return fmt.Errorf("%w: embedding JSON must not be empty", ErrInvalid)
	}
	values, err := decodeEmbeddingJSON(embedding.EmbeddingJSON)
	if err != nil {
		return err
	}
	if len(values) != embedding.Dimensions {
		return fmt.Errorf("%w: embedding JSON dimensions do not match metadata", ErrInvalid)
	}
	if strings.TrimSpace(embedding.ContentHash) == "" {
		return fmt.Errorf("%w: embedding content hash must not be empty", ErrInvalid)
	}
	if embedding.VectorRowID == 0 {
		return fmt.Errorf("%w: embedding vector rowid must not be zero", ErrInvalid)
	}
	if embedding.CreatedAt.IsZero() {
		return fmt.Errorf("%w: embedding created_at must not be zero", ErrInvalid)
	}
	if embedding.UpdatedAt.IsZero() {
		return fmt.Errorf("%w: embedding updated_at must not be zero", ErrInvalid)
	}
	return nil
}

func validateEmbeddingIndexStatus(status EmbeddingIndexStatus) error {
	if err := requireID("chunk", status.ChunkID); err != nil {
		return err
	}
	if err := requireID("memory", status.MemoryID); err != nil {
		return err
	}
	if err := validateEmbeddingTarget(EmbeddingTarget{
		Provider:   status.Provider,
		Model:      status.Model,
		Dimensions: status.Dimensions,
		Scope:      status.EmbeddingScope,
	}); err != nil {
		return err
	}
	switch status.Status {
	case EmbeddingIndexStatusPending, EmbeddingIndexStatusIndexed, EmbeddingIndexStatusFailed, EmbeddingIndexStatusSkipped:
	default:
		return fmt.Errorf("%w: unsupported embedding index status %q", ErrInvalid, status.Status)
	}
	if status.Attempts < 0 {
		return fmt.Errorf("%w: embedding index attempts must not be negative", ErrInvalid)
	}
	if status.CreatedAt.IsZero() {
		return fmt.Errorf("%w: embedding index status created_at must not be zero", ErrInvalid)
	}
	if status.UpdatedAt.IsZero() {
		return fmt.Errorf("%w: embedding index status updated_at must not be zero", ErrInvalid)
	}
	return nil
}

func (r *MemoryRepository) vectorBackend(ctx context.Context, target EmbeddingTarget) (string, error) {
	if err := validateEmbeddingTarget(target); err != nil {
		return "", err
	}
	var backend string
	err := r.exec.QueryRowContext(ctx, `
SELECT backend
FROM vector_metadata
WHERE provider = ? AND model = ? AND dimensions = ? AND embedding_scope = ?`,
		target.Provider,
		target.Model,
		target.Dimensions,
		target.Scope,
	).Scan(&backend)
	if errors.Is(err, sql.ErrNoRows) {
		return VectorBackendSQLiteJSON, nil
	}
	if err != nil {
		return "", fmt.Errorf("read vector backend: %w", err)
	}
	switch backend {
	case VectorBackendSQLiteJSON, VectorBackendSQLiteVec:
		return backend, nil
	default:
		return "", fmt.Errorf("%w: unsupported vector backend %q", ErrInvalid, backend)
	}
}

func (r *MemoryRepository) ensureSQLiteVecTable(ctx context.Context, target EmbeddingTarget) error {
	if err := validateEmbeddingTarget(target); err != nil {
		return err
	}
	tableName := quoteIdentifier(sqliteVecTableName(target))
	_, err := r.exec.ExecContext(ctx, fmt.Sprintf(
		"CREATE VIRTUAL TABLE IF NOT EXISTS %s USING vec0(embedding float[%d])",
		tableName,
		target.Dimensions,
	))
	if err != nil {
		return fmt.Errorf("ensure sqlite-vec table: %w", err)
	}
	return nil
}

func (r *MemoryRepository) upsertSQLiteVecRow(ctx context.Context, embedding MemoryEmbedding) error {
	target := EmbeddingTarget{
		Provider:   embedding.Provider,
		Model:      embedding.Model,
		Dimensions: embedding.Dimensions,
		Scope:      embedding.EmbeddingScope,
	}
	if err := r.ensureSQLiteVecTable(ctx, target); err != nil {
		return err
	}
	tableName := quoteIdentifier(sqliteVecTableName(target))
	if _, err := r.exec.ExecContext(ctx, "DELETE FROM "+tableName+" WHERE rowid = ?", embedding.VectorRowID); err != nil {
		return fmt.Errorf("delete sqlite-vec row: %w", err)
	}
	if _, err := r.exec.ExecContext(
		ctx,
		"INSERT INTO "+tableName+"(rowid, embedding) VALUES (?, vec_f32(?))",
		embedding.VectorRowID,
		embedding.EmbeddingJSON,
	); err != nil {
		return fmt.Errorf("insert sqlite-vec row: %w", err)
	}
	return nil
}

func (r *MemoryRepository) deleteSQLiteVecRowsForMemory(ctx context.Context, memoryID string) error {
	rows, err := r.exec.QueryContext(ctx, `
SELECT memory_embeddings.provider, memory_embeddings.model, memory_embeddings.dimensions, memory_embeddings.embedding_scope,
       memory_embeddings.vector_rowid, vector_metadata.backend
FROM memory_embeddings
JOIN vector_metadata
  ON vector_metadata.provider = memory_embeddings.provider
 AND vector_metadata.model = memory_embeddings.model
 AND vector_metadata.embedding_scope = memory_embeddings.embedding_scope
WHERE memory_embeddings.memory_id = ?`, memoryID)
	if err != nil {
		return fmt.Errorf("list sqlite-vec rows for memory: %w", err)
	}
	defer rows.Close()

	type row struct {
		target      EmbeddingTarget
		vectorRowID int64
	}
	var pending []row
	for rows.Next() {
		var target EmbeddingTarget
		var vectorRowID sql.NullInt64
		var backend string
		if err := rows.Scan(&target.Provider, &target.Model, &target.Dimensions, &target.Scope, &vectorRowID, &backend); err != nil {
			return fmt.Errorf("scan sqlite-vec row for memory: %w", err)
		}
		if backend == VectorBackendSQLiteVec && vectorRowID.Valid {
			pending = append(pending, row{target: target, vectorRowID: vectorRowID.Int64})
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("list sqlite-vec rows for memory: %w", err)
	}
	for _, item := range pending {
		if err := r.ensureSQLiteVecTable(ctx, item.target); err != nil {
			return err
		}
		tableName := quoteIdentifier(sqliteVecTableName(item.target))
		if _, err := r.exec.ExecContext(ctx, "DELETE FROM "+tableName+" WHERE rowid = ?", item.vectorRowID); err != nil {
			return fmt.Errorf("delete sqlite-vec row for memory: %w", err)
		}
	}
	return nil
}

func decodeEmbeddingJSON(value string) ([]float64, error) {
	var vector []float64
	if err := json.Unmarshal([]byte(value), &vector); err != nil {
		return nil, fmt.Errorf("%w: embedding JSON must be a numeric array", ErrInvalid)
	}
	if len(vector) == 0 {
		return nil, fmt.Errorf("%w: embedding vector must not be empty", ErrInvalid)
	}
	for _, component := range vector {
		if math.IsNaN(component) || math.IsInf(component, 0) {
			return nil, fmt.Errorf("%w: embedding vector contains non-finite component", ErrInvalid)
		}
	}
	return vector, nil
}

func encodeEmbeddingJSON(vector []float64) (string, error) {
	body, err := json.Marshal(vector)
	if err != nil {
		return "", fmt.Errorf("%w: embedding JSON must be encodable", ErrInvalid)
	}
	return string(body), nil
}

func cosineSimilarity(left, right []float64) (float64, error) {
	if len(left) == 0 || len(left) != len(right) {
		return 0, fmt.Errorf("%w: embedding vectors must have matching dimensions", ErrInvalid)
	}
	var dot, leftNorm, rightNorm float64
	for i := range left {
		dot += left[i] * right[i]
		leftNorm += left[i] * left[i]
		rightNorm += right[i] * right[i]
	}
	if leftNorm == 0 || rightNorm == 0 {
		return 0, nil
	}
	return dot / (math.Sqrt(leftNorm) * math.Sqrt(rightNorm)), nil
}

func l2DistanceSimilarity(distance float64) float64 {
	if math.IsNaN(distance) {
		return 0
	}
	if distance <= 0 {
		return 1
	}
	if math.IsInf(distance, 0) {
		return 0
	}
	return 1 / (1 + distance)
}

func vectorRowID(chunkID, provider, model, scope string) int64 {
	sum := sha256.Sum256([]byte(chunkID + "\x00" + provider + "\x00" + model + "\x00" + scope))
	const maxInt64 = uint64(1<<63 - 1)
	value := int64(binary.BigEndian.Uint64(sum[:8]) & maxInt64)
	if value == 0 {
		return 1
	}
	return value
}

func sqliteVecTableName(target EmbeddingTarget) string {
	source := target.Provider + "\x00" + target.Model + "\x00" + strconv.Itoa(target.Dimensions) + "\x00" + target.Scope
	sum := sha256.Sum256([]byte(source))
	return "memory_vec_" + hexLower(sum[:8])
}

func quoteIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

func hexLower(bytes []byte) string {
	const alphabet = "0123456789abcdef"
	out := make([]byte, len(bytes)*2)
	for i, value := range bytes {
		out[i*2] = alphabet[value>>4]
		out[i*2+1] = alphabet[value&0x0f]
	}
	return string(out)
}

func plainSnippet(content string) string {
	const maxRunes = 160
	runes := []rune(strings.TrimSpace(content))
	if len(runes) <= maxRunes {
		return string(runes)
	}
	return string(runes[:maxRunes]) + "..."
}

func defaultJSON(value string) string {
	if value == "" {
		return "{}"
	}
	return value
}

func nullString(value string) sql.NullString {
	if value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: value, Valid: true}
}
