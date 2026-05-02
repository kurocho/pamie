// SPDX-License-Identifier: AGPL-3.0-only

package memory

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/your-org/pamie/internal/db"
	"github.com/your-org/pamie/internal/embedding"
	"github.com/your-org/pamie/internal/search"
)

const (
	defaultSearchLimit = 10
	maxLimit           = 100
)

var (
	ErrUnavailable = errors.New("memory service is not configured")
	ErrInvalid     = errors.New("invalid memory request")
	ErrNotFound    = errors.New("memory not found")
)

// Store is the durable storage behavior required by Service.
type Store interface {
	Memories() *db.MemoryRepository
	WithinTx(context.Context, func(context.Context, *db.Tx) error) error
}

type Options struct {
	Clock               func() time.Time
	EmbeddingProvider   embedding.Provider
	VectorSearchEnabled bool
	VectorBackend       string
}

// Service coordinates memory behavior over durable storage.
type Service struct {
	store         Store
	now           func() time.Time
	embedder      embedding.Provider
	vectorEnabled bool
	vectorBackend string
}

// NewService creates a memory service.
func NewService(store Store) *Service {
	return NewServiceWithClock(store, time.Now)
}

// NewServiceWithClock creates a memory service with an injectable clock.
func NewServiceWithClock(store Store, now func() time.Time) *Service {
	return NewServiceWithOptions(store, Options{Clock: now})
}

// NewServiceWithOptions creates a memory service with optional vector search.
func NewServiceWithOptions(store Store, opts Options) *Service {
	now := opts.Clock
	if now == nil {
		now = time.Now
	}
	return &Service{
		store:         store,
		now:           now,
		embedder:      opts.EmbeddingProvider,
		vectorEnabled: opts.VectorSearchEnabled && opts.EmbeddingProvider != nil,
		vectorBackend: normalizeVectorBackend(opts.VectorBackend),
	}
}

type SaveInput struct {
	Title      string
	Body       string
	Source     string
	Metadata   map[string]any
	Tier       string
	Importance int
	Pinned     bool
}

type UpdateInput struct {
	ID         string
	Title      *string
	Body       *string
	Source     *string
	Metadata   *map[string]any
	Tier       *string
	Importance *int
	Pinned     *bool
}

type DeleteInput struct {
	ID      string
	Confirm bool
}

type PinInput struct {
	ID     string
	Pinned bool
}

type SearchInput struct {
	Query          string
	Tier           *string
	Pinned         *bool
	IncludeDeleted bool
	Limit          int
	Depth          string
	Metadata       map[string]any
	Source         *string
	CreatedAfter   *time.Time
	CreatedBefore  *time.Time
	UpdatedAfter   *time.Time
	UpdatedBefore  *time.Time
}

type RecentInput struct {
	IncludeDeleted bool
	Limit          int
}

type Memory struct {
	ID             string         `json:"id"`
	Title          string         `json:"title"`
	Body           string         `json:"body"`
	Source         string         `json:"source"`
	Metadata       map[string]any `json:"metadata"`
	Tier           string         `json:"tier"`
	Importance     int            `json:"importance"`
	Pinned         bool           `json:"pinned"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	LastAccessedAt *time.Time     `json:"last_accessed_at,omitempty"`
	ArchivedAt     *time.Time     `json:"archived_at,omitempty"`
	DeletedAt      *time.Time     `json:"deleted_at,omitempty"`
}

type Chunk struct {
	ID         string    `json:"id"`
	MemoryID   string    `json:"memory_id"`
	ChunkIndex int       `json:"chunk_index"`
	Content    string    `json:"content"`
	CreatedAt  time.Time `json:"created_at"`
}

type MemoryWithChunks struct {
	Memory Memory  `json:"memory"`
	Chunks []Chunk `json:"chunks"`
}

type SearchHit struct {
	Memory       Memory       `json:"memory"`
	MemoryID     string       `json:"memory_id"`
	ChunkID      string       `json:"chunk_id"`
	Snippet      string       `json:"snippet"`
	Score        float64      `json:"score"`
	ScoreDetails search.Score `json:"score_details"`
}

type Stats = db.Stats

type EmbeddingBackfillResult struct {
	Scanned int `json:"scanned"`
	Indexed int `json:"indexed"`
}

func (s *Service) Save(ctx context.Context, input SaveInput) (Memory, error) {
	store, err := s.requireStore()
	if err != nil {
		return Memory{}, err
	}
	if input.Body == "" {
		return Memory{}, fmt.Errorf("%w: body must not be empty", ErrInvalid)
	}
	tier := parseTier(input.Tier)
	if !tier.Valid() {
		return Memory{}, fmt.Errorf("%w: invalid tier %q", ErrInvalid, input.Tier)
	}
	if input.Importance < 0 || input.Importance > 100 {
		return Memory{}, fmt.Errorf("%w: importance must be between 0 and 100", ErrInvalid)
	}
	metadataJSON, err := marshalMetadata(input.Metadata)
	if err != nil {
		return Memory{}, err
	}

	itemID, err := newID("mem")
	if err != nil {
		return Memory{}, err
	}
	chunkID, err := newID("chunk")
	if err != nil {
		return Memory{}, err
	}

	now := s.now().UTC()
	item := db.MemoryItem{
		ID:           itemID,
		Title:        input.Title,
		Body:         input.Body,
		Source:       input.Source,
		MetadataJSON: metadataJSON,
		Tier:         tier,
		Importance:   input.Importance,
		Pinned:       input.Pinned,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	chunk := db.MemoryChunk{
		ID:         chunkID,
		MemoryID:   item.ID,
		ChunkIndex: 0,
		Content:    item.Body,
		CreatedAt:  now,
	}
	vectorMetadata, chunkEmbedding, err := s.embeddingForChunk(ctx, chunk, now)
	if err != nil {
		return Memory{}, err
	}

	if err := store.WithinTx(ctx, func(ctx context.Context, tx *db.Tx) error {
		if err := tx.Memories().CreateItem(ctx, item); err != nil {
			return mapDBError(err)
		}
		if err := tx.Memories().AddChunk(ctx, chunk); err != nil {
			return mapDBError(err)
		}
		if chunkEmbedding != nil {
			if err := tx.Memories().UpsertVectorMetadata(ctx, vectorMetadata); err != nil {
				return mapDBError(err)
			}
			if err := tx.Memories().UpsertEmbedding(ctx, *chunkEmbedding); err != nil {
				return mapDBError(err)
			}
		}
		_, err := tx.Memories().RecordEvent(ctx, db.MemoryEvent{
			MemoryID:         item.ID,
			EventType:        "created",
			EventPayloadJSON: "{}",
			CreatedAt:        now,
		})
		return mapDBError(err)
	}); err != nil {
		return Memory{}, err
	}

	return toMemory(item), nil
}

func (s *Service) Get(ctx context.Context, id string) (MemoryWithChunks, error) {
	store, err := s.requireStore()
	if err != nil {
		return MemoryWithChunks{}, err
	}
	now := s.now().UTC()
	var item db.MemoryItem
	var chunks []db.MemoryChunk
	if err := store.WithinTx(ctx, func(ctx context.Context, tx *db.Tx) error {
		var err error
		item, err = tx.Memories().GetItem(ctx, id)
		if err != nil {
			return mapDBError(err)
		}
		if item.DeletedAt != nil {
			return ErrNotFound
		}
		item.LastAccessedAt = &now
		if err := tx.Memories().UpdateItem(ctx, id, db.MemoryUpdate{
			LastAccessedAt: &now,
			UpdatedAt:      now,
		}); err != nil {
			return mapDBError(err)
		}
		if _, err := tx.AccessLog().Record(ctx, db.AccessLogEntry{
			MemoryID:   id,
			AccessType: "get",
			CreatedAt:  now,
		}); err != nil {
			return mapDBError(err)
		}
		policies, err := loadLifecyclePolicies(ctx, tx.Policies())
		if err != nil {
			return err
		}
		if change, err := s.promoteByAccess(ctx, tx, item, policyForItem(item, policies), now); err != nil {
			return err
		} else if change != nil {
			item, err = tx.Memories().GetItem(ctx, id)
			if err != nil {
				return mapDBError(err)
			}
			item.LastAccessedAt = &now
		}
		chunks, err = tx.Memories().ListChunks(ctx, id)
		return mapDBError(err)
	}); err != nil {
		return MemoryWithChunks{}, err
	}

	return MemoryWithChunks{
		Memory: toMemory(item),
		Chunks: toChunks(chunks),
	}, nil
}

func (s *Service) Search(ctx context.Context, input SearchInput) ([]SearchHit, error) {
	store, err := s.requireStore()
	if err != nil {
		return nil, err
	}
	limit := normalizeLimit(input.Limit)
	depth := search.Depth(input.Depth)
	if !depth.Valid() {
		return nil, fmt.Errorf("%w: invalid search depth %q", ErrInvalid, input.Depth)
	}
	opts := db.SearchOptions{
		Query:          input.Query,
		IncludeDeleted: input.IncludeDeleted,
		Limit:          limit,
		Depth:          depth,
		Metadata:       input.Metadata,
		Source:         input.Source,
		CreatedAfter:   input.CreatedAfter,
		CreatedBefore:  input.CreatedBefore,
		UpdatedAfter:   input.UpdatedAfter,
		UpdatedBefore:  input.UpdatedBefore,
		Now:            s.now().UTC(),
	}
	if input.Tier != nil {
		tier := db.Tier(*input.Tier)
		opts.Tier = &tier
	}
	opts.Pinned = input.Pinned
	if s.vectorSearchEnabled() {
		queryEmbedding, err := s.embedder.Embed(ctx, input.Query)
		if err != nil {
			return nil, fmt.Errorf("%w: embed search query: %v", ErrInvalid, err)
		}
		opts.Vector = &db.VectorSearchOptions{
			Target:         s.embeddingTarget(),
			QueryEmbedding: queryEmbedding,
		}
	}

	results, err := store.Memories().Search(ctx, opts)
	if err != nil {
		return nil, mapDBError(err)
	}

	hits := make([]SearchHit, 0, len(results))
	for _, result := range results {
		hits = append(hits, SearchHit{
			Memory:       toMemory(result.Item),
			MemoryID:     result.MemoryID,
			ChunkID:      result.ChunkID,
			Snippet:      result.Snippet,
			Score:        result.Score.Total,
			ScoreDetails: result.Score,
		})
	}
	return hits, nil
}

func (s *Service) Update(ctx context.Context, input UpdateInput) (Memory, error) {
	store, err := s.requireStore()
	if err != nil {
		return Memory{}, err
	}
	if input.ID == "" {
		return Memory{}, fmt.Errorf("%w: id must not be empty", ErrInvalid)
	}
	now := s.now().UTC()
	var updated db.MemoryItem
	var replacementChunk *db.MemoryChunk
	var vectorMetadata db.VectorMetadata
	var chunkEmbedding *db.MemoryEmbedding
	if input.Body != nil {
		chunkID, err := newID("chunk")
		if err != nil {
			return Memory{}, err
		}
		chunk := db.MemoryChunk{
			ID:         chunkID,
			MemoryID:   input.ID,
			ChunkIndex: 0,
			Content:    *input.Body,
			CreatedAt:  now,
		}
		replacementChunk = &chunk
		vectorMetadata, chunkEmbedding, err = s.embeddingForChunk(ctx, chunk, now)
		if err != nil {
			return Memory{}, err
		}
	}

	if err := store.WithinTx(ctx, func(ctx context.Context, tx *db.Tx) error {
		current, err := tx.Memories().GetItem(ctx, input.ID)
		if err != nil {
			return mapDBError(err)
		}
		if current.DeletedAt != nil {
			return ErrNotFound
		}

		update := db.MemoryUpdate{UpdatedAt: now}
		if input.Title != nil {
			update.Title = input.Title
		}
		if input.Body != nil {
			update.Body = input.Body
		}
		if input.Source != nil {
			update.Source = input.Source
		}
		if input.Metadata != nil {
			metadataJSON, err := marshalMetadata(*input.Metadata)
			if err != nil {
				return err
			}
			update.MetadataJSON = &metadataJSON
		}
		if input.Tier != nil {
			tier := db.Tier(*input.Tier)
			update.Tier = &tier
		}
		if input.Importance != nil {
			update.Importance = input.Importance
		}
		if input.Pinned != nil {
			update.Pinned = input.Pinned
		}
		if err := tx.Memories().UpdateItem(ctx, input.ID, update); err != nil {
			return mapDBError(err)
		}
		if replacementChunk != nil {
			if err := tx.Memories().ReplaceChunks(ctx, input.ID, []db.MemoryChunk{*replacementChunk}); err != nil {
				return mapDBError(err)
			}
			if chunkEmbedding != nil {
				if err := tx.Memories().UpsertVectorMetadata(ctx, vectorMetadata); err != nil {
					return mapDBError(err)
				}
				if err := tx.Memories().UpsertEmbedding(ctx, *chunkEmbedding); err != nil {
					return mapDBError(err)
				}
			}
		}
		_, err = tx.Memories().RecordEvent(ctx, db.MemoryEvent{
			MemoryID:         input.ID,
			EventType:        "updated",
			EventPayloadJSON: "{}",
			CreatedAt:        now,
		})
		if err != nil {
			return mapDBError(err)
		}
		updated, err = tx.Memories().GetItem(ctx, input.ID)
		return mapDBError(err)
	}); err != nil {
		return Memory{}, err
	}
	return toMemory(updated), nil
}

func (s *Service) Delete(ctx context.Context, input DeleteInput) (Memory, error) {
	store, err := s.requireStore()
	if err != nil {
		return Memory{}, err
	}
	if !input.Confirm {
		return Memory{}, fmt.Errorf("%w: confirm must be true", ErrInvalid)
	}
	now := s.now().UTC()
	var item db.MemoryItem
	if err := store.WithinTx(ctx, func(ctx context.Context, tx *db.Tx) error {
		current, err := tx.Memories().GetItem(ctx, input.ID)
		if err != nil {
			return mapDBError(err)
		}
		if current.DeletedAt != nil {
			return ErrNotFound
		}
		if err := tx.Memories().UpdateItem(ctx, input.ID, db.MemoryUpdate{
			DeletedAt: &now,
			UpdatedAt: now,
		}); err != nil {
			return mapDBError(err)
		}
		_, err = tx.Memories().RecordEvent(ctx, db.MemoryEvent{
			MemoryID:         input.ID,
			EventType:        "deleted",
			EventPayloadJSON: `{"mode":"soft_delete"}`,
			CreatedAt:        now,
		})
		if err != nil {
			return mapDBError(err)
		}
		item, err = tx.Memories().GetItem(ctx, input.ID)
		return mapDBError(err)
	}); err != nil {
		return Memory{}, err
	}
	return toMemory(item), nil
}

func (s *Service) Pin(ctx context.Context, input PinInput) (Memory, error) {
	store, err := s.requireStore()
	if err != nil {
		return Memory{}, err
	}
	now := s.now().UTC()
	var item db.MemoryItem
	if err := store.WithinTx(ctx, func(ctx context.Context, tx *db.Tx) error {
		current, err := tx.Memories().GetItem(ctx, input.ID)
		if err != nil {
			return mapDBError(err)
		}
		if current.DeletedAt != nil {
			return ErrNotFound
		}
		if err := tx.Memories().UpdateItem(ctx, input.ID, db.MemoryUpdate{
			Pinned:    &input.Pinned,
			UpdatedAt: now,
		}); err != nil {
			return mapDBError(err)
		}
		_, err = tx.Memories().RecordEvent(ctx, db.MemoryEvent{
			MemoryID:         input.ID,
			EventType:        "pinned",
			EventPayloadJSON: fmt.Sprintf(`{"pinned":%t}`, input.Pinned),
			CreatedAt:        now,
		})
		if err != nil {
			return mapDBError(err)
		}
		item, err = tx.Memories().GetItem(ctx, input.ID)
		return mapDBError(err)
	}); err != nil {
		return Memory{}, err
	}
	return toMemory(item), nil
}

func (s *Service) Recent(ctx context.Context, input RecentInput) ([]Memory, error) {
	store, err := s.requireStore()
	if err != nil {
		return nil, err
	}
	items, err := store.Memories().Recent(ctx, db.RecentOptions{
		IncludeDeleted: input.IncludeDeleted,
		Limit:          normalizeLimit(input.Limit),
	})
	if err != nil {
		return nil, mapDBError(err)
	}

	memories := make([]Memory, 0, len(items))
	for _, item := range items {
		memories = append(memories, toMemory(item))
	}
	return memories, nil
}

func (s *Service) Stats(ctx context.Context) (Stats, error) {
	store, err := s.requireStore()
	if err != nil {
		return Stats{}, err
	}
	stats, err := store.Memories().Stats(ctx)
	if err != nil {
		return Stats{}, mapDBError(err)
	}
	return stats, nil
}

func (s *Service) BackfillEmbeddings(ctx context.Context, limit int) (EmbeddingBackfillResult, error) {
	return s.backfillEmbeddings(ctx, limit, false)
}

func (s *Service) ReindexEmbeddings(ctx context.Context, limit int) (EmbeddingBackfillResult, error) {
	return s.backfillEmbeddings(ctx, limit, true)
}

func (s *Service) backfillEmbeddings(ctx context.Context, limit int, force bool) (EmbeddingBackfillResult, error) {
	store, err := s.requireStore()
	if err != nil {
		return EmbeddingBackfillResult{}, err
	}
	if !s.vectorSearchEnabled() {
		return EmbeddingBackfillResult{}, ErrUnavailable
	}
	if limit <= 0 {
		limit = maxLimit
	}
	target := s.embeddingTarget()
	chunks, err := store.Memories().ListChunksMissingEmbeddings(ctx, db.EmbeddingBackfillOptions{
		Target: target,
		Limit:  limit,
		Force:  force,
	})
	if err != nil {
		return EmbeddingBackfillResult{}, mapDBError(err)
	}
	result := EmbeddingBackfillResult{Scanned: len(chunks)}
	for _, chunk := range chunks {
		now := s.now().UTC()
		vectorMetadata, chunkEmbedding, err := s.embeddingForChunk(ctx, chunk, now)
		if err != nil {
			return result, err
		}
		if err := store.WithinTx(ctx, func(ctx context.Context, tx *db.Tx) error {
			if err := tx.Memories().UpsertVectorMetadata(ctx, vectorMetadata); err != nil {
				return mapDBError(err)
			}
			return mapDBError(tx.Memories().UpsertEmbedding(ctx, *chunkEmbedding))
		}); err != nil {
			return result, err
		}
		result.Indexed++
	}
	return result, nil
}

func (s *Service) requireStore() (Store, error) {
	if s == nil || s.store == nil {
		return nil, ErrUnavailable
	}
	value := reflect.ValueOf(s.store)
	if value.Kind() == reflect.Pointer && value.IsNil() {
		return nil, ErrUnavailable
	}
	return s.store, nil
}

func (s *Service) vectorSearchEnabled() bool {
	return s != nil && s.vectorEnabled && s.embedder != nil
}

func (s *Service) embeddingTarget() db.EmbeddingTarget {
	if !s.vectorSearchEnabled() {
		return db.EmbeddingTarget{}
	}
	return db.EmbeddingTarget{
		Provider:   s.embedder.Name(),
		Model:      s.embedder.Model(),
		Dimensions: s.embedder.Dimensions(),
	}
}

func (s *Service) embeddingForChunk(ctx context.Context, chunk db.MemoryChunk, now time.Time) (db.VectorMetadata, *db.MemoryEmbedding, error) {
	if !s.vectorSearchEnabled() {
		return db.VectorMetadata{}, nil, nil
	}
	vector, err := s.embedder.Embed(ctx, chunk.Content)
	if err != nil {
		return db.VectorMetadata{}, nil, fmt.Errorf("%w: embed memory chunk: %v", ErrInvalid, err)
	}
	if len(vector) != s.embedder.Dimensions() {
		return db.VectorMetadata{}, nil, fmt.Errorf("%w: embedding provider returned %d dimensions, want %d", ErrInvalid, len(vector), s.embedder.Dimensions())
	}
	embeddingJSON, err := marshalEmbedding(vector)
	if err != nil {
		return db.VectorMetadata{}, nil, err
	}
	target := s.embeddingTarget()
	metadata := db.VectorMetadata{
		Provider:       target.Provider,
		Model:          target.Model,
		Dimensions:     target.Dimensions,
		Backend:        s.vectorBackend,
		DistanceMetric: "cosine",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	embedding := db.MemoryEmbedding{
		ChunkID:       chunk.ID,
		MemoryID:      chunk.MemoryID,
		Provider:      target.Provider,
		Model:         target.Model,
		Dimensions:    target.Dimensions,
		EmbeddingJSON: embeddingJSON,
		ContentHash:   contentHash(chunk.Content),
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	return metadata, &embedding, nil
}

func marshalEmbedding(vector []float64) (string, error) {
	body, err := json.Marshal(vector)
	if err != nil {
		return "", fmt.Errorf("%w: embedding vector must be JSON encodable", ErrInvalid)
	}
	return string(body), nil
}

func contentHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func normalizeVectorBackend(backend string) string {
	if backend == "" {
		return db.VectorBackendSQLiteJSON
	}
	return backend
}

func parseTier(value string) db.Tier {
	if value == "" {
		return db.TierWorking
	}
	return db.Tier(value)
}

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return defaultSearchLimit
	}
	if limit > maxLimit {
		return maxLimit
	}
	return limit
}

func marshalMetadata(metadata map[string]any) (string, error) {
	if metadata == nil {
		return "{}", nil
	}
	body, err := json.Marshal(metadata)
	if err != nil {
		return "", fmt.Errorf("%w: metadata must be JSON encodable", ErrInvalid)
	}
	return string(body), nil
}

func unmarshalMetadata(value string) map[string]any {
	if value == "" {
		return map[string]any{}
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(value), &metadata); err != nil {
		return map[string]any{}
	}
	if metadata == nil {
		return map[string]any{}
	}
	return metadata
}

func toMemory(item db.MemoryItem) Memory {
	return Memory{
		ID:             item.ID,
		Title:          item.Title,
		Body:           item.Body,
		Source:         item.Source,
		Metadata:       unmarshalMetadata(item.MetadataJSON),
		Tier:           string(item.Tier),
		Importance:     item.Importance,
		Pinned:         item.Pinned,
		CreatedAt:      item.CreatedAt,
		UpdatedAt:      item.UpdatedAt,
		LastAccessedAt: item.LastAccessedAt,
		ArchivedAt:     item.ArchivedAt,
		DeletedAt:      item.DeletedAt,
	}
}

func toChunks(chunks []db.MemoryChunk) []Chunk {
	out := make([]Chunk, 0, len(chunks))
	for _, chunk := range chunks {
		out = append(out, Chunk{
			ID:         chunk.ID,
			MemoryID:   chunk.MemoryID,
			ChunkIndex: chunk.ChunkIndex,
			Content:    chunk.Content,
			CreatedAt:  chunk.CreatedAt,
		})
	}
	return out
}

func mapDBError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, db.ErrNotFound):
		return ErrNotFound
	case errors.Is(err, db.ErrInvalid):
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	default:
		return err
	}
}

func newID(prefix string) (string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return prefix + "_" + hex.EncodeToString(random[:]), nil
}
