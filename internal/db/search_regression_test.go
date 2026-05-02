// SPDX-License-Identifier: AGPL-3.0-only

package db

import (
	"context"
	"testing"
	"time"
)

func TestMemoryRepositorySearchRankingAndFilterCombinations(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	now := fixedTime()

	memories := []MemoryItem{
		{
			ID:           "mem_acceptance_top",
			Title:        "Top ranked",
			Body:         "release acceptance anchor",
			Source:       "api",
			MetadataJSON: `{"project":"pamie","stage":"acceptance","batch":7,"active":true}`,
			Tier:         TierWorking,
			Importance:   100,
			Pinned:       true,
			CreatedAt:    now.Add(-time.Hour),
			UpdatedAt:    now.Add(-time.Hour),
		},
		{
			ID:           "mem_acceptance_hot",
			Title:        "Hot filtered",
			Body:         "release acceptance anchor",
			Source:       "api",
			MetadataJSON: `{"project":"pamie","stage":"release","batch":7,"active":true}`,
			Tier:         TierHot,
			Importance:   50,
			CreatedAt:    now.Add(-48 * time.Hour),
			UpdatedAt:    now.Add(-48 * time.Hour),
		},
		{
			ID:           "mem_acceptance_cold",
			Title:        "Cold docs",
			Body:         "release acceptance anchor",
			Source:       "docs",
			MetadataJSON: `{"project":"pamie","stage":"release","batch":8,"active":false}`,
			Tier:         TierCold,
			Importance:   0,
			CreatedAt:    now.Add(-60 * 24 * time.Hour),
			UpdatedAt:    now.Add(-60 * 24 * time.Hour),
		},
	}
	for _, item := range memories {
		insertSearchMemory(t, store, item)
	}
	for i := 0; i < 2; i++ {
		if _, err := store.AccessLog().Record(ctx, AccessLogEntry{
			MemoryID:   "mem_acceptance_hot",
			AccessType: "search",
			CreatedAt:  now.Add(-time.Duration(i) * time.Hour),
		}); err != nil {
			t.Fatalf("AccessLog().Record() error = %v", err)
		}
	}

	results, err := store.Memories().Search(ctx, SearchOptions{
		Query: "release acceptance anchor",
		Limit: 3,
		Depth: "deep",
		Now:   now,
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	wantOrder := []string{"mem_acceptance_top", "mem_acceptance_hot", "mem_acceptance_cold"}
	if len(results) != len(wantOrder) {
		t.Fatalf("Search() returned %d result(s), want %d: %+v", len(results), len(wantOrder), results)
	}
	for i, want := range wantOrder {
		if results[i].MemoryID != want {
			t.Fatalf("Search() result %d = %q, want %q; results = %+v", i, results[i].MemoryID, want, results)
		}
	}
	if results[0].Score.Pinned == 0 || results[0].Score.Importance == 0 || results[1].Score.Access == 0 {
		t.Fatalf("ranking signals = top %+v hot %+v, want pinned, importance, and access contributions", results[0].Score, results[1].Score)
	}

	tier := TierHot
	pinned := false
	filtered, err := store.Memories().Search(ctx, SearchOptions{
		Query:         "release",
		Tier:          &tier,
		Pinned:        &pinned,
		Source:        stringPtr("api"),
		Metadata:      map[string]any{"project": "pamie", "stage": "release", "batch": 7, "active": true},
		CreatedAfter:  timePtr(now.Add(-7 * 24 * time.Hour)),
		UpdatedBefore: timePtr(now),
		Limit:         5,
		Depth:         "standard",
		Now:           now,
	})
	if err != nil {
		t.Fatalf("Search() with combined filters error = %v", err)
	}
	if len(filtered) != 1 || filtered[0].MemoryID != "mem_acceptance_hot" {
		t.Fatalf("Search() with combined filters = %+v, want hot API memory", filtered)
	}

	cold := TierCold
	docs, err := store.Memories().Search(ctx, SearchOptions{
		Query:    "acceptance",
		Tier:     &cold,
		Source:   stringPtr("docs"),
		Metadata: map[string]any{"active": false, "batch": 8},
		Limit:    5,
		Now:      now,
	})
	if err != nil {
		t.Fatalf("Search() docs filter error = %v", err)
	}
	if len(docs) != 1 || docs[0].MemoryID != "mem_acceptance_cold" {
		t.Fatalf("Search() docs filter = %+v, want cold docs memory", docs)
	}

	empty, err := store.Memories().Search(ctx, SearchOptions{
		Query:    "release",
		Metadata: map[string]any{"project": "other"},
		Limit:    5,
		Now:      now,
	})
	if err != nil {
		t.Fatalf("Search() unmatched metadata error = %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("Search() unmatched metadata = %+v, want no results", empty)
	}
}

func insertSearchMemory(t *testing.T, store *Store, item MemoryItem) {
	t.Helper()
	ctx := context.Background()
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
