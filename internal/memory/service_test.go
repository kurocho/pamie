// SPDX-License-Identifier: AGPL-3.0-only

package memory

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/your-org/pamie/internal/db"
	"github.com/your-org/pamie/internal/embedding"
)

func TestServiceSaveGetSearchUpdateDelete(t *testing.T) {
	service, closeStore := testService(t)
	defer closeStore()
	ctx := context.Background()

	saved, err := service.Save(ctx, SaveInput{
		Title:      "Decision",
		Body:       "Pamie uses SQLite FTS5 storage",
		Source:     "test",
		Metadata:   map[string]any{"project": "pamie"},
		Importance: 20,
		Pinned:     true,
	})
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if saved.ID == "" || saved.Tier != "working" || !saved.Pinned {
		t.Fatalf("Save() = %+v", saved)
	}

	got, err := service.Get(ctx, saved.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Memory.ID != saved.ID || got.Memory.LastAccessedAt == nil || len(got.Chunks) != 1 {
		t.Fatalf("Get() = %+v", got)
	}

	hits, err := service.Search(ctx, SearchInput{Query: "FTS5", Limit: 5})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(hits) != 1 || hits[0].Memory.ID != saved.ID {
		t.Fatalf("Search() = %+v, want saved memory", hits)
	}
	if hits[0].Snippet == "" || hits[0].Score <= 0 || hits[0].ScoreDetails.Keyword <= 0 {
		t.Fatalf("Search() hit = %+v, want snippet and score details", hits[0])
	}

	tier := "working"
	pinned := true
	createdAfter := time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC)
	filteredHits, err := service.Search(ctx, SearchInput{
		Query:        "FTS5",
		Tier:         &tier,
		Pinned:       &pinned,
		Source:       stringPtr("test"),
		Metadata:     map[string]any{"project": "pamie"},
		CreatedAfter: &createdAfter,
		Depth:        "deep",
		Limit:        5,
	})
	if err != nil {
		t.Fatalf("Search() with filters error = %v", err)
	}
	if len(filteredHits) != 1 || filteredHits[0].Memory.ID != saved.ID {
		t.Fatalf("Search() with filters = %+v, want saved memory", filteredHits)
	}

	newBody := "Pamie exposes basic MCP memory tools"
	updated, err := service.Update(ctx, UpdateInput{ID: saved.ID, Body: &newBody})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.Body != newBody {
		t.Fatalf("Update() body = %q, want %q", updated.Body, newBody)
	}

	hits, err = service.Search(ctx, SearchInput{Query: "MCP", Limit: 5})
	if err != nil {
		t.Fatalf("Search() after update error = %v", err)
	}
	if len(hits) != 1 || hits[0].Memory.ID != saved.ID {
		t.Fatalf("Search() after update = %+v, want updated memory", hits)
	}

	deleted, err := service.Delete(ctx, DeleteInput{ID: saved.ID, Confirm: true})
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if deleted.DeletedAt == nil {
		t.Fatalf("Delete() = %+v, want deleted_at", deleted)
	}
	if _, err := service.Get(ctx, saved.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() after delete error = %v, want ErrNotFound", err)
	}

	hits, err = service.Search(ctx, SearchInput{Query: "MCP", Limit: 5})
	if err != nil {
		t.Fatalf("Search() after delete error = %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("Search() after delete = %+v, want no active hits", hits)
	}
}

func TestServiceStatsAndRecent(t *testing.T) {
	service, closeStore := testService(t)
	defer closeStore()
	ctx := context.Background()

	first, err := service.Save(ctx, SaveInput{Body: "first memory"})
	if err != nil {
		t.Fatalf("Save(first) error = %v", err)
	}
	second, err := service.Save(ctx, SaveInput{Body: "second memory", Tier: "hot"})
	if err != nil {
		t.Fatalf("Save(second) error = %v", err)
	}

	recent, err := service.Recent(ctx, RecentInput{Limit: 10})
	if err != nil {
		t.Fatalf("Recent() error = %v", err)
	}
	if len(recent) != 2 || recent[0].ID != first.ID && recent[0].ID != second.ID {
		t.Fatalf("Recent() = %+v", recent)
	}

	stats, err := service.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats() error = %v", err)
	}
	if stats.Total != 2 || stats.Active != 2 || stats.Working != 1 || stats.Hot != 1 {
		t.Fatalf("Stats() = %+v", stats)
	}
}

func TestServicePersistsSearchableMemoriesAcrossStoreRestart(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "pamie.db")
	clock := func() time.Time {
		return time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	}

	store, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db.Open() error = %v", err)
	}
	service := NewServiceWithClock(store, clock)
	saved, err := service.Save(ctx, SaveInput{
		Title:      "Persistent memory",
		Body:       "persistent acceptance memory survives SQLite restart",
		Source:     "persistence-test",
		Metadata:   map[string]any{"project": "pamie", "phase": "acceptance"},
		Importance: 64,
		Pinned:     true,
	})
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}

	reopened, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db.Open(reopen) error = %v", err)
	}
	defer func() {
		if err := reopened.Close(); err != nil {
			t.Fatalf("reopened Close() error = %v", err)
		}
	}()
	service = NewServiceWithClock(reopened, clock)

	pinned := true
	hits, err := service.Search(ctx, SearchInput{
		Query:    "persistent acceptance",
		Source:   stringPtr("persistence-test"),
		Metadata: map[string]any{"project": "pamie", "phase": "acceptance"},
		Pinned:   &pinned,
		Limit:    5,
		Depth:    "deep",
	})
	if err != nil {
		t.Fatalf("Search() after reopen error = %v", err)
	}
	if len(hits) != 1 || hits[0].MemoryID != saved.ID {
		t.Fatalf("Search() after reopen = %+v, want saved memory %s", hits, saved.ID)
	}

	got, err := service.Get(ctx, saved.ID)
	if err != nil {
		t.Fatalf("Get() after reopen error = %v", err)
	}
	if got.Memory.ID != saved.ID || len(got.Chunks) != 1 {
		t.Fatalf("Get() after reopen = %+v, want persisted memory with chunk", got)
	}
}

func TestServiceConcurrentSaveSearchAndStats(t *testing.T) {
	service, closeStore := testService(t)
	defer closeStore()

	ctx := context.Background()
	const writers = 8
	start := make(chan struct{})
	errs := make(chan error, writers*2)
	var wg sync.WaitGroup

	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			_, err := service.Save(ctx, SaveInput{
				Title:    "Concurrent memory " + fmtIndex(index),
				Body:     "concurrent acceptance memory " + fmtIndex(index),
				Source:   "race-test",
				Metadata: map[string]any{"project": "pamie", "slot": index},
			})
			if err != nil {
				errs <- fmt.Errorf("Save(%d): %w", index, err)
			}
		}(i)
	}
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if _, err := service.Stats(ctx); err != nil {
				errs <- fmt.Errorf("Stats(): %w", err)
			}
			if _, err := service.Search(ctx, SearchInput{Query: "concurrent", Limit: 10}); err != nil {
				errs <- fmt.Errorf("Search(): %w", err)
			}
		}()
	}

	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if t.Failed() {
		return
	}

	stats, err := service.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats() after concurrent operations error = %v", err)
	}
	if stats.Total != writers || stats.Active != writers {
		t.Fatalf("Stats() after concurrent operations = %+v, want %d active memories", stats, writers)
	}
}

func TestServiceEmbeddingStorageUpdateAndBackfill(t *testing.T) {
	provider, err := embedding.NewLocalHashProvider(16)
	if err != nil {
		t.Fatalf("NewLocalHashProvider() error = %v", err)
	}
	service, store, closeStore := testServiceWithOptions(t, Options{
		EmbeddingProvider:   provider,
		VectorSearchEnabled: true,
	})
	defer closeStore()
	ctx := context.Background()
	target := db.EmbeddingTarget{
		Provider:   provider.Name(),
		Model:      provider.Model(),
		Dimensions: provider.Dimensions(),
	}

	saved, err := service.Save(ctx, SaveInput{Body: "alpha vector memory", Metadata: map[string]any{"project": "pamie"}})
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	embeddings, err := store.Memories().ListEmbeddings(ctx, saved.ID, target)
	if err != nil {
		t.Fatalf("ListEmbeddings() error = %v", err)
	}
	if len(embeddings) != 1 || embeddings[0].Dimensions != provider.Dimensions() {
		t.Fatalf("embeddings after Save() = %+v, want one local hash embedding", embeddings)
	}
	firstHash := embeddings[0].ContentHash

	newBody := "beta vector memory"
	if _, err := service.Update(ctx, UpdateInput{ID: saved.ID, Body: &newBody}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	embeddings, err = store.Memories().ListEmbeddings(ctx, saved.ID, target)
	if err != nil {
		t.Fatalf("ListEmbeddings() after update error = %v", err)
	}
	if len(embeddings) != 1 || embeddings[0].ContentHash == firstHash {
		t.Fatalf("embeddings after Update() = %+v, want replaced embedding", embeddings)
	}

	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	if err := store.Memories().CreateItem(ctx, db.MemoryItem{
		ID:         "mem_legacy",
		Body:       "legacy memory before vectors",
		Tier:       db.TierWorking,
		Importance: 10,
		CreatedAt:  now,
		UpdatedAt:  now,
	}); err != nil {
		t.Fatalf("CreateItem(legacy) error = %v", err)
	}
	if err := store.Memories().AddChunk(ctx, db.MemoryChunk{
		ID:         "chunk_legacy",
		MemoryID:   "mem_legacy",
		ChunkIndex: 0,
		Content:    "legacy memory before vectors",
		CreatedAt:  now,
	}); err != nil {
		t.Fatalf("AddChunk(legacy) error = %v", err)
	}

	backfill, err := service.BackfillEmbeddings(ctx, 10)
	if err != nil {
		t.Fatalf("BackfillEmbeddings() error = %v", err)
	}
	if backfill.Scanned != 1 || backfill.Indexed != 1 {
		t.Fatalf("BackfillEmbeddings() = %+v, want one indexed chunk", backfill)
	}
	again, err := service.BackfillEmbeddings(ctx, 10)
	if err != nil {
		t.Fatalf("BackfillEmbeddings() second run error = %v", err)
	}
	if again.Scanned != 0 || again.Indexed != 0 {
		t.Fatalf("BackfillEmbeddings() second run = %+v, want idempotent no-op", again)
	}
	reindexed, err := service.ReindexEmbeddings(ctx, 10)
	if err != nil {
		t.Fatalf("ReindexEmbeddings() error = %v", err)
	}
	if reindexed.Scanned != 2 || reindexed.Indexed != 2 {
		t.Fatalf("ReindexEmbeddings() = %+v, want two reindexed chunks", reindexed)
	}

	hits, err := service.Search(ctx, SearchInput{Query: "beta", Metadata: map[string]any{"project": "pamie"}, Limit: 5})
	if err != nil {
		t.Fatalf("Search() with vector enabled error = %v", err)
	}
	if len(hits) == 0 || hits[0].ScoreDetails.Vector == 0 {
		t.Fatalf("Search() hits = %+v, want vector score component", hits)
	}
}

func TestServiceBulkSaveAndSearch(t *testing.T) {
	provider, err := embedding.NewLocalHashProvider(16)
	if err != nil {
		t.Fatalf("NewLocalHashProvider() error = %v", err)
	}
	service, _, closeStore := testServiceWithOptions(t, Options{
		EmbeddingProvider:   provider,
		VectorSearchEnabled: true,
		VectorBackend:       db.VectorBackendSQLiteVec,
	})
	defer closeStore()

	ctx := context.Background()
	memoryCount := bulkMemoryCount(t)
	expected := make(map[string]string)
	fixtures := []struct {
		query string
		token string
		slot  string
	}{
		{query: "ledger reconciliation finance integration", token: "loadtest-token-0007", slot: "0007"},
		{query: "offline vector ranking semantic retrieval", token: "loadtest-token-0423", slot: "0423"},
		{query: "incident timeline service recovery analysis", token: "loadtest-token-0879", slot: "0879"},
	}

	for i := 0; i < memoryCount; i++ {
		token := "loadtest-token-" + fmtIndex(i)
		body := "Bulk load memory " + fmtIndex(i) + " with " + token + " covering routine project notes and searchable archive text."
		switch i {
		case 7:
			body += " Ledger reconciliation details for the finance integration."
		case 423:
			body += " Offline vector ranking checks for semantic retrieval behavior."
		case 879:
			body += " Incident timeline notes for service recovery analysis."
		}
		saved, err := service.Save(ctx, SaveInput{
			Title:      "Bulk memory " + fmtIndex(i),
			Body:       body,
			Source:     "bulk-test",
			Metadata:   map[string]any{"batch": "bulk-search", "slot": fmtIndex(i)},
			Importance: i % 101,
		})
		if err != nil {
			t.Fatalf("Save(%d) error = %v", i, err)
		}
		expected[token] = saved.ID
	}

	for _, fixture := range fixtures {
		hits, err := service.Search(ctx, SearchInput{
			Query:    fixture.query,
			Source:   stringPtr("bulk-test"),
			Metadata: map[string]any{"batch": "bulk-search", "slot": fixture.slot},
			Limit:    10,
			Depth:    "deep",
		})
		if err != nil {
			t.Fatalf("Search(%q) error = %v", fixture.query, err)
		}
		if len(hits) == 0 {
			t.Fatalf("Search(%q) returned no hits", fixture.query)
		}
		if hits[0].MemoryID != expected[fixture.token] {
			t.Fatalf("Search(%q) top hit = %q, want %q; hits = %+v", fixture.query, hits[0].MemoryID, expected[fixture.token], hits)
		}
		if hits[0].ScoreDetails.Vector <= 0 {
			t.Fatalf("Search(%q) top hit score = %+v, want vector score", fixture.query, hits[0].ScoreDetails)
		}
	}
}

func TestServiceValidation(t *testing.T) {
	service, closeStore := testService(t)
	defer closeStore()

	if _, err := service.Save(context.Background(), SaveInput{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Save() error = %v, want ErrInvalid", err)
	}
	if _, err := service.Delete(context.Background(), DeleteInput{ID: "missing"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Delete() without confirm error = %v, want ErrInvalid", err)
	}
}

func TestServiceUnavailableError(t *testing.T) {
	service := NewService(nil)

	if _, err := service.Stats(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Stats() error = %v, want ErrUnavailable", err)
	}
	if _, err := (*Service)(nil).Stats(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("nil Service Stats() error = %v, want ErrUnavailable", err)
	}
	var store *db.Store
	if _, err := NewService(store).Stats(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("typed nil store Stats() error = %v, want ErrUnavailable", err)
	}
}

func testService(t *testing.T) (*Service, func()) {
	service, _, closeStore := testServiceWithStore(t)
	return service, closeStore
}

func testServiceWithStore(t *testing.T) (*Service, *db.Store, func()) {
	t.Helper()
	return testServiceWithOptions(t, Options{})
}

func testServiceWithOptions(t *testing.T, opts Options) (*Service, *db.Store, func()) {
	t.Helper()
	store, err := db.Open(context.Background(), db.Options{Path: filepath.Join(t.TempDir(), "pamie.db")})
	if err != nil {
		t.Fatalf("db.Open() error = %v", err)
	}
	opts.Clock = func() time.Time {
		return time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	}
	service := NewServiceWithOptions(store, opts)
	return service, store, func() {
		if err := store.Close(); err != nil {
			t.Fatalf("store.Close() error = %v", err)
		}
	}
}

func stringPtr(value string) *string {
	return &value
}

func bulkMemoryCount(t *testing.T) int {
	t.Helper()
	const defaultCount = 1000
	value := getenvForTest("PAMIE_BULK_TEST_MEMORIES")
	if value == "" {
		return defaultCount
	}
	count, err := strconv.Atoi(value)
	if err != nil || count <= 879 {
		t.Fatalf("PAMIE_BULK_TEST_MEMORIES = %q, want integer greater than 879", value)
	}
	return count
}

func fmtIndex(index int) string {
	return fmt.Sprintf("%04d", index)
}

func getenvForTest(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}
