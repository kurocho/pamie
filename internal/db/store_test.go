// SPDX-License-Identifier: AGPL-3.0-only

package db

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestOpenAppliesPragmasAndMigrations(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	mode, err := store.JournalMode(ctx)
	if err != nil {
		t.Fatalf("JournalMode() error = %v", err)
	}
	if strings.ToLower(mode) != "wal" {
		t.Fatalf("journal_mode = %q, want wal", mode)
	}

	foreignKeys, err := store.ForeignKeysEnabled(ctx)
	if err != nil {
		t.Fatalf("ForeignKeysEnabled() error = %v", err)
	}
	if !foreignKeys {
		t.Fatal("foreign keys disabled")
	}

	vecVersion, err := store.SQLiteVecVersion(ctx)
	if err != nil {
		t.Fatalf("SQLiteVecVersion() error = %v", err)
	}
	if vecVersion == "" {
		t.Fatal("SQLiteVecVersion() = empty")
	}
	backend, err := store.ResolveVectorBackend(ctx, VectorBackendAuto)
	if err != nil {
		t.Fatalf("ResolveVectorBackend(auto) error = %v", err)
	}
	if backend != VectorBackendSQLiteVec {
		t.Fatalf("ResolveVectorBackend(auto) = %q, want sqlite-vec", backend)
	}

	versions, err := store.AppliedMigrationVersions(ctx)
	if err != nil {
		t.Fatalf("AppliedMigrationVersions() error = %v", err)
	}
	if !reflect.DeepEqual(versions, []int{1, 2, 3}) {
		t.Fatalf("versions = %v, want [1 2 3]", versions)
	}

	if err := store.ApplyMigrations(ctx); err != nil {
		t.Fatalf("ApplyMigrations() second run error = %v", err)
	}
	versions, err = store.AppliedMigrationVersions(ctx)
	if err != nil {
		t.Fatalf("AppliedMigrationVersions() after rerun error = %v", err)
	}
	if !reflect.DeepEqual(versions, []int{1, 2, 3}) {
		t.Fatalf("versions after rerun = %v, want [1 2 3]", versions)
	}
}

func TestMemoryRepositoryInsertGetAndChunks(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	now := fixedTime()

	item := MemoryItem{
		ID:           "mem_1",
		Title:        "Project note",
		Body:         "Alpha durable memory",
		Source:       "test",
		MetadataJSON: `{"project":"pamie"}`,
		Tier:         TierWorking,
		Importance:   42,
		Pinned:       true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := store.Memories().CreateItem(ctx, item); err != nil {
		t.Fatalf("CreateItem() error = %v", err)
	}

	got, err := store.Memories().GetItem(ctx, item.ID)
	if err != nil {
		t.Fatalf("GetItem() error = %v", err)
	}
	if got.ID != item.ID || got.Title != item.Title || got.Body != item.Body || got.Tier != item.Tier || !got.Pinned {
		t.Fatalf("GetItem() = %+v, want representative fields from %+v", got, item)
	}

	chunk := MemoryChunk{
		ID:         "chunk_1",
		MemoryID:   item.ID,
		ChunkIndex: 0,
		Content:    "Alpha searchable chunk",
		CreatedAt:  now,
	}
	if err := store.Memories().AddChunk(ctx, chunk); err != nil {
		t.Fatalf("AddChunk() error = %v", err)
	}

	chunks, err := store.Memories().ListChunks(ctx, item.ID)
	if err != nil {
		t.Fatalf("ListChunks() error = %v", err)
	}
	if len(chunks) != 1 || chunks[0].ID != chunk.ID || chunks[0].Content != chunk.Content {
		t.Fatalf("ListChunks() = %+v, want one inserted chunk", chunks)
	}

	var matchedMemoryID string
	if err := store.database.QueryRowContext(ctx, "SELECT memory_id FROM memory_fts WHERE memory_fts MATCH 'Alpha'").Scan(&matchedMemoryID); err != nil {
		t.Fatalf("query memory_fts error = %v", err)
	}
	if matchedMemoryID != item.ID {
		t.Fatalf("FTS memory_id = %q, want %q", matchedMemoryID, item.ID)
	}
}

func TestMemoryRepositoryEmbeddingsAndBackfillCandidates(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	now := fixedTime()
	target := EmbeddingTarget{Provider: "test", Model: "unit-v1", Dimensions: 3}

	if err := store.Memories().CreateItem(ctx, MemoryItem{
		ID:         "mem_embed",
		Body:       "embed this memory",
		Tier:       TierWorking,
		Importance: 10,
		CreatedAt:  now,
		UpdatedAt:  now,
	}); err != nil {
		t.Fatalf("CreateItem() error = %v", err)
	}
	chunk := MemoryChunk{
		ID:         "chunk_embed",
		MemoryID:   "mem_embed",
		ChunkIndex: 0,
		Content:    "embed this memory",
		CreatedAt:  now,
	}
	if err := store.Memories().AddChunk(ctx, chunk); err != nil {
		t.Fatalf("AddChunk() error = %v", err)
	}

	missing, err := store.Memories().ListChunksMissingEmbeddings(ctx, EmbeddingBackfillOptions{Target: target, Limit: 10})
	if err != nil {
		t.Fatalf("ListChunksMissingEmbeddings() error = %v", err)
	}
	if len(missing) != 1 || missing[0].ID != chunk.ID {
		t.Fatalf("missing chunks = %+v, want chunk_embed", missing)
	}

	if err := store.Memories().UpsertVectorMetadata(ctx, VectorMetadata{
		Provider:       target.Provider,
		Model:          target.Model,
		Dimensions:     target.Dimensions,
		Backend:        "sqlite-json",
		DistanceMetric: "cosine",
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("UpsertVectorMetadata() error = %v", err)
	}
	if err := store.Memories().UpsertEmbedding(ctx, MemoryEmbedding{
		ChunkID:       chunk.ID,
		MemoryID:      chunk.MemoryID,
		Provider:      target.Provider,
		Model:         target.Model,
		Dimensions:    target.Dimensions,
		EmbeddingJSON: vectorJSON(t, []float64{1, 0, 0}),
		ContentHash:   "hash-1",
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("UpsertEmbedding() error = %v", err)
	}
	if err := store.Memories().UpsertEmbedding(ctx, MemoryEmbedding{
		ChunkID:       chunk.ID,
		MemoryID:      chunk.MemoryID,
		Provider:      target.Provider,
		Model:         target.Model,
		Dimensions:    target.Dimensions,
		EmbeddingJSON: vectorJSON(t, []float64{0, 1, 0}),
		ContentHash:   "hash-2",
		CreatedAt:     now,
		UpdatedAt:     now.Add(time.Minute),
	}); err != nil {
		t.Fatalf("UpsertEmbedding() second run error = %v", err)
	}

	embeddings, err := store.Memories().ListEmbeddings(ctx, "mem_embed", target)
	if err != nil {
		t.Fatalf("ListEmbeddings() error = %v", err)
	}
	if len(embeddings) != 1 || embeddings[0].ContentHash != "hash-2" {
		t.Fatalf("embeddings = %+v, want one updated row", embeddings)
	}
	missing, err = store.Memories().ListChunksMissingEmbeddings(ctx, EmbeddingBackfillOptions{Target: target, Limit: 10})
	if err != nil {
		t.Fatalf("ListChunksMissingEmbeddings() after upsert error = %v", err)
	}
	if len(missing) != 0 {
		t.Fatalf("missing chunks after upsert = %+v, want none", missing)
	}
}

func TestMemoryRepositorySearchFiltersAndRanking(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	now := fixedTime()
	deletedAt := now.Add(-time.Hour)

	items := []MemoryItem{
		{
			ID:           "mem_boosted",
			Title:        "Boosted memory",
			Body:         "Pamie recall anchor",
			Source:       "agent-a",
			MetadataJSON: `{"project":"pamie","kind":"decision","priority":3,"flag":true}`,
			Tier:         TierWorking,
			Importance:   90,
			Pinned:       true,
			CreatedAt:    now.Add(-2 * time.Hour),
			UpdatedAt:    now.Add(-time.Hour),
		},
		{
			ID:           "mem_cold",
			Title:        "Cold memory",
			Body:         "Pamie recall anchor",
			Source:       "agent-b",
			MetadataJSON: `{"project":"pamie","kind":"note"}`,
			Tier:         TierCold,
			Importance:   10,
			CreatedAt:    now.Add(-120 * 24 * time.Hour),
			UpdatedAt:    now.Add(-100 * 24 * time.Hour),
		},
		{
			ID:           "mem_deleted",
			Title:        "Deleted memory",
			Body:         "Pamie recall anchor",
			Source:       "agent-a",
			MetadataJSON: `{"project":"pamie","kind":"decision"}`,
			Tier:         TierWorking,
			Importance:   100,
			Pinned:       true,
			CreatedAt:    now.Add(-time.Hour),
			UpdatedAt:    now.Add(-30 * time.Minute),
			DeletedAt:    &deletedAt,
		},
	}
	for _, item := range items {
		if err := store.Memories().CreateItem(ctx, item); err != nil {
			t.Fatalf("CreateItem(%s) error = %v", item.ID, err)
		}
		if err := store.Memories().AddChunk(ctx, MemoryChunk{
			ID:         "chunk_" + item.ID,
			MemoryID:   item.ID,
			ChunkIndex: 0,
			Content:    item.Body,
			CreatedAt:  item.CreatedAt,
		}); err != nil {
			t.Fatalf("AddChunk(%s) error = %v", item.ID, err)
		}
	}
	for i := 0; i < 3; i++ {
		if _, err := store.AccessLog().Record(ctx, AccessLogEntry{
			MemoryID:   "mem_boosted",
			AccessType: "search",
			CreatedAt:  now.Add(-time.Duration(i) * time.Hour),
		}); err != nil {
			t.Fatalf("AccessLog().Record() error = %v", err)
		}
	}

	results, err := store.Memories().Search(ctx, SearchOptions{
		Query: "recall anchor",
		Limit: 10,
		Now:   now,
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("Search() returned %d result(s), want 2 active results: %+v", len(results), results)
	}
	if results[0].MemoryID != "mem_boosted" {
		t.Fatalf("top result = %q, want boosted memory", results[0].MemoryID)
	}
	if results[0].Snippet == "" || !strings.Contains(results[0].Snippet, "[recall]") {
		t.Fatalf("snippet = %q, want highlighted recall term", results[0].Snippet)
	}
	if results[0].Score.Pinned == 0 || results[0].Score.Importance == 0 || results[0].Score.Access == 0 || results[0].Score.Tier == 0 {
		t.Fatalf("score details = %+v, want populated ranking signals", results[0].Score)
	}

	tier := TierWorking
	pinned := true
	filtered, err := store.Memories().Search(ctx, SearchOptions{
		Query:         "recall",
		Tier:          &tier,
		Pinned:        &pinned,
		Source:        stringPtr("agent-a"),
		Metadata:      map[string]any{"project": "pamie", "kind": "decision", "priority": 3, "flag": true},
		CreatedAfter:  timePtr(now.Add(-24 * time.Hour)),
		UpdatedBefore: timePtr(now),
		Limit:         10,
		Now:           now,
	})
	if err != nil {
		t.Fatalf("Search() with filters error = %v", err)
	}
	if len(filtered) != 1 || filtered[0].MemoryID != "mem_boosted" {
		t.Fatalf("filtered Search() = %+v, want boosted memory only", filtered)
	}

	deletedResults, err := store.Memories().Search(ctx, SearchOptions{
		Query:          "recall",
		IncludeDeleted: true,
		Limit:          10,
		Now:            now,
	})
	if err != nil {
		t.Fatalf("Search(include deleted) error = %v", err)
	}
	if len(deletedResults) != 3 {
		t.Fatalf("Search(include deleted) returned %d result(s), want 3", len(deletedResults))
	}
}

func TestMemoryRepositoryHybridVectorSearch(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	now := fixedTime()
	target := EmbeddingTarget{Provider: "test", Model: "hybrid-v1", Dimensions: 2}
	if err := store.Memories().UpsertVectorMetadata(ctx, VectorMetadata{
		Provider:       target.Provider,
		Model:          target.Model,
		Dimensions:     target.Dimensions,
		Backend:        "sqlite-json",
		DistanceMetric: "cosine",
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("UpsertVectorMetadata() error = %v", err)
	}

	insertMemoryWithVector(t, store, MemoryItem{
		ID:           "mem_keyword_only",
		Body:         "alpha keyword match",
		MetadataJSON: `{"project":"pamie"}`,
		Tier:         TierCold,
		Importance:   0,
		CreatedAt:    now.Add(-120 * 24 * time.Hour),
		UpdatedAt:    now.Add(-120 * 24 * time.Hour),
	}, MemoryChunk{ID: "chunk_keyword", MemoryID: "mem_keyword_only", ChunkIndex: 0, Content: "alpha keyword match", CreatedAt: now}, target, []float64{1, 0})
	insertMemoryWithVector(t, store, MemoryItem{
		ID:           "mem_vector",
		Body:         "semantic neighbor",
		MetadataJSON: `{"project":"pamie"}`,
		Tier:         TierWorking,
		Importance:   100,
		Pinned:       true,
		CreatedAt:    now.Add(-time.Hour),
		UpdatedAt:    now.Add(-time.Hour),
	}, MemoryChunk{ID: "chunk_vector", MemoryID: "mem_vector", ChunkIndex: 0, Content: "semantic neighbor", CreatedAt: now}, target, []float64{0, 1})
	insertMemoryWithVector(t, store, MemoryItem{
		ID:           "mem_other_project",
		Body:         "another semantic neighbor",
		MetadataJSON: `{"project":"other"}`,
		Tier:         TierWorking,
		Importance:   100,
		Pinned:       true,
		CreatedAt:    now.Add(-time.Hour),
		UpdatedAt:    now.Add(-time.Hour),
	}, MemoryChunk{ID: "chunk_other", MemoryID: "mem_other_project", ChunkIndex: 0, Content: "another semantic neighbor", CreatedAt: now}, target, []float64{0, 1})

	disabledResults, err := store.Memories().Search(ctx, SearchOptions{
		Query: "alpha",
		Limit: 10,
		Now:   now,
	})
	if err != nil {
		t.Fatalf("Search() vector disabled error = %v", err)
	}
	if len(disabledResults) != 1 || disabledResults[0].MemoryID != "mem_keyword_only" {
		t.Fatalf("Search() vector disabled = %+v, want keyword-only result", disabledResults)
	}

	results, err := store.Memories().Search(ctx, SearchOptions{
		Query:    "alpha",
		Metadata: map[string]any{"project": "pamie"},
		Limit:    10,
		Now:      now,
		Vector: &VectorSearchOptions{
			Target:         target,
			QueryEmbedding: []float64{0, 1},
		},
	})
	if err != nil {
		t.Fatalf("Search() hybrid error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("Search() hybrid returned %d result(s), want 2: %+v", len(results), results)
	}
	if results[0].MemoryID != "mem_vector" {
		t.Fatalf("top hybrid result = %q, want mem_vector: %+v", results[0].MemoryID, results)
	}
	if results[0].Score.Vector == 0 || results[0].Score.Keyword != 0 {
		t.Fatalf("top score = %+v, want vector-only score component", results[0].Score)
	}
	for _, result := range results {
		if result.MemoryID == "mem_other_project" {
			t.Fatalf("metadata filter allowed other project result: %+v", results)
		}
	}
}

func TestMemoryRepositorySQLiteVecSearch(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	now := fixedTime()
	target := EmbeddingTarget{Provider: "test", Model: "sqlite-vec-v1", Dimensions: 2}
	if err := store.Memories().UpsertVectorMetadata(ctx, VectorMetadata{
		Provider:       target.Provider,
		Model:          target.Model,
		Dimensions:     target.Dimensions,
		Backend:        VectorBackendSQLiteVec,
		DistanceMetric: "cosine",
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("UpsertVectorMetadata(sqlite-vec) error = %v", err)
	}
	insertMemoryWithVector(t, store, MemoryItem{
		ID:           "mem_vec_near",
		Body:         "near vector",
		MetadataJSON: `{"project":"pamie"}`,
		Tier:         TierWorking,
		Importance:   100,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, MemoryChunk{ID: "chunk_vec_near", MemoryID: "mem_vec_near", ChunkIndex: 0, Content: "near vector", CreatedAt: now}, target, []float64{1, 0})
	insertMemoryWithVector(t, store, MemoryItem{
		ID:           "mem_vec_far",
		Body:         "far vector",
		MetadataJSON: `{"project":"pamie"}`,
		Tier:         TierWorking,
		Importance:   10,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, MemoryChunk{ID: "chunk_vec_far", MemoryID: "mem_vec_far", ChunkIndex: 0, Content: "far vector", CreatedAt: now}, target, []float64{0, 1})

	results, err := store.Memories().Search(ctx, SearchOptions{
		Query:    "missing-keyword",
		Metadata: map[string]any{"project": "pamie"},
		Limit:    10,
		Now:      now,
		Vector: &VectorSearchOptions{
			Target:         target,
			QueryEmbedding: []float64{1, 0},
		},
	})
	if err != nil {
		t.Fatalf("Search() sqlite-vec error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("Search() sqlite-vec returned %d result(s), want 2: %+v", len(results), results)
	}
	if results[0].MemoryID != "mem_vec_near" {
		t.Fatalf("top sqlite-vec result = %q, want mem_vec_near: %+v", results[0].MemoryID, results)
	}
	if results[0].Score.Vector == 0 {
		t.Fatalf("sqlite-vec score = %+v, want vector component", results[0].Score)
	}
}

func TestMemoryRepositorySearchRejectsUnsafeFilters(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	_, err := store.Memories().Search(ctx, SearchOptions{
		Query:    "recall",
		Metadata: map[string]any{"project.name": "pamie"},
		Limit:    10,
		Now:      fixedTime(),
	})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("Search() error = %v, want ErrInvalid", err)
	}
}

func TestRepositoriesValidateAndReturnNotFound(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	if _, err := store.Memories().GetItem(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetItem() error = %v, want ErrNotFound", err)
	}

	err := store.Memories().CreateItem(ctx, MemoryItem{
		ID:         "bad",
		Body:       "body",
		Tier:       Tier("unknown"),
		Importance: 1,
		CreatedAt:  fixedTime(),
		UpdatedAt:  fixedTime(),
	})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("CreateItem() error = %v, want ErrInvalid", err)
	}

	err = store.Memories().AddChunk(ctx, MemoryChunk{
		ID:         "chunk",
		MemoryID:   "",
		ChunkIndex: 0,
		Content:    "content",
		CreatedAt:  fixedTime(),
	})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("AddChunk() error = %v, want ErrInvalid", err)
	}
}

func TestPolicyAccessLogAndEvents(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	now := fixedTime()

	if err := store.Memories().CreateItem(ctx, MemoryItem{
		ID:         "mem_1",
		Body:       "Policy target",
		Tier:       TierHot,
		Importance: 10,
		CreatedAt:  now,
		UpdatedAt:  now,
	}); err != nil {
		t.Fatalf("CreateItem() error = %v", err)
	}

	eventID, err := store.Memories().RecordEvent(ctx, MemoryEvent{
		MemoryID:         "mem_1",
		EventType:        "created",
		EventPayloadJSON: `{"source":"test"}`,
		CreatedAt:        now,
	})
	if err != nil {
		t.Fatalf("RecordEvent() error = %v", err)
	}
	if eventID == 0 {
		t.Fatal("RecordEvent() id = 0")
	}

	if err := store.Policies().Create(ctx, RetentionPolicy{
		ID:        "policy_1",
		Name:      "default",
		ScopeJSON: `{"tier":"archive"}`,
		RulesJSON: `{"delete_after_days":365}`,
		Enabled:   true,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("Policies().Create() error = %v", err)
	}

	accessID, err := store.AccessLog().Record(ctx, AccessLogEntry{
		MemoryID:   "mem_1",
		AccessType: "get",
		TokenID:    "token_1",
		CreatedAt:  now,
	})
	if err != nil {
		t.Fatalf("AccessLog().Record() error = %v", err)
	}
	if accessID == 0 {
		t.Fatal("AccessLog().Record() id = 0")
	}
}

func TestWithinTxRollback(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	now := fixedTime()
	wantErr := errors.New("rollback")

	err := store.WithinTx(ctx, func(ctx context.Context, tx *Tx) error {
		if err := tx.Memories().CreateItem(ctx, MemoryItem{
			ID:         "mem_rollback",
			Body:       "rollback",
			Tier:       TierWorking,
			Importance: 1,
			CreatedAt:  now,
			UpdatedAt:  now,
		}); err != nil {
			return err
		}
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("WithinTx() error = %v, want rollback error", err)
	}

	if _, err := store.Memories().GetItem(ctx, "mem_rollback"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetItem() after rollback error = %v, want ErrNotFound", err)
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(context.Background(), Options{
		Path: filepath.Join(t.TempDir(), "pamie.db"),
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})
	return store
}

func fixedTime() time.Time {
	return time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
}

func stringPtr(value string) *string {
	return &value
}

func timePtr(value time.Time) *time.Time {
	return &value
}

func vectorJSON(t *testing.T, vector []float64) string {
	t.Helper()
	body, err := json.Marshal(vector)
	if err != nil {
		t.Fatalf("json.Marshal(vector) error = %v", err)
	}
	return string(body)
}

func insertMemoryWithVector(t *testing.T, store *Store, item MemoryItem, chunk MemoryChunk, target EmbeddingTarget, vector []float64) {
	t.Helper()
	ctx := context.Background()
	if item.MetadataJSON == "" {
		item.MetadataJSON = "{}"
	}
	if err := store.Memories().CreateItem(ctx, item); err != nil {
		t.Fatalf("CreateItem(%s) error = %v", item.ID, err)
	}
	if err := store.Memories().AddChunk(ctx, chunk); err != nil {
		t.Fatalf("AddChunk(%s) error = %v", chunk.ID, err)
	}
	if err := store.Memories().UpsertEmbedding(ctx, MemoryEmbedding{
		ChunkID:       chunk.ID,
		MemoryID:      chunk.MemoryID,
		Provider:      target.Provider,
		Model:         target.Model,
		Dimensions:    target.Dimensions,
		EmbeddingJSON: vectorJSON(t, vector),
		ContentHash:   "hash-" + chunk.ID,
		CreatedAt:     item.CreatedAt,
		UpdatedAt:     item.UpdatedAt,
	}); err != nil {
		t.Fatalf("UpsertEmbedding(%s) error = %v", chunk.ID, err)
	}
}
