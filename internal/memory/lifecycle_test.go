// SPDX-License-Identifier: AGPL-3.0-only

package memory

import (
	"context"
	"testing"
	"time"

	"github.com/your-org/pamie/internal/db"
)

func TestLifecycleDemotesInactiveMemoryAndRecordsEvent(t *testing.T) {
	service, store, closeStore := testServiceWithStore(t)
	defer closeStore()
	ctx := context.Background()
	now := lifecycleNow()

	createLifecycleMemory(t, store, db.MemoryItem{
		ID:         "mem_working_old",
		Body:       "old working memory",
		Tier:       db.TierWorking,
		Importance: 10,
		CreatedAt:  now.Add(-48 * time.Hour),
		UpdatedAt:  now.Add(-48 * time.Hour),
	})

	report, err := service.RunLifecycle(ctx, LifecycleOptions{})
	if err != nil {
		t.Fatalf("RunLifecycle() error = %v", err)
	}
	if report.Evaluated != 1 || report.Demoted != 1 {
		t.Fatalf("RunLifecycle() = %+v, want one demotion", report)
	}

	item, err := store.Memories().GetItem(ctx, "mem_working_old")
	if err != nil {
		t.Fatalf("GetItem() error = %v", err)
	}
	if item.Tier != db.TierHot {
		t.Fatalf("tier = %q, want hot", item.Tier)
	}
	assertMemoryEvent(t, store, "mem_working_old", "lifecycle_demoted")
}

func TestLifecycleArchivesColdMemory(t *testing.T) {
	service, store, closeStore := testServiceWithStore(t)
	defer closeStore()
	ctx := context.Background()
	now := lifecycleNow()

	createLifecycleMemory(t, store, db.MemoryItem{
		ID:         "mem_cold_old",
		Body:       "old cold memory",
		Tier:       db.TierCold,
		Importance: 10,
		CreatedAt:  now.Add(-100 * 24 * time.Hour),
		UpdatedAt:  now.Add(-100 * 24 * time.Hour),
	})

	report, err := service.RunLifecycle(ctx, LifecycleOptions{})
	if err != nil {
		t.Fatalf("RunLifecycle() error = %v", err)
	}
	if report.Archived != 1 {
		t.Fatalf("RunLifecycle() = %+v, want one archive", report)
	}

	item, err := store.Memories().GetItem(ctx, "mem_cold_old")
	if err != nil {
		t.Fatalf("GetItem() error = %v", err)
	}
	if item.Tier != db.TierArchive || item.ArchivedAt == nil {
		t.Fatalf("item = %+v, want archived memory", item)
	}
	assertMemoryEvent(t, store, "mem_cold_old", "lifecycle_archived")
}

func TestLifecyclePolicyAuthorizedDeletion(t *testing.T) {
	service, store, closeStore := testServiceWithStore(t)
	defer closeStore()
	ctx := context.Background()
	now := lifecycleNow()
	archivedAt := now.Add(-48 * time.Hour)

	if err := store.Policies().Create(ctx, db.RetentionPolicy{
		ID:        "policy_delete_archive",
		Name:      "delete archived quickly",
		ScopeJSON: "{}",
		RulesJSON: `{"delete_archived_after_days":1}`,
		Enabled:   true,
		CreatedAt: now.Add(-time.Hour),
		UpdatedAt: now.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("Create policy error = %v", err)
	}
	createLifecycleMemory(t, store, db.MemoryItem{
		ID:         "mem_archive_old",
		Body:       "old archive memory",
		Tier:       db.TierArchive,
		Importance: 10,
		CreatedAt:  now.Add(-120 * 24 * time.Hour),
		UpdatedAt:  archivedAt,
		ArchivedAt: &archivedAt,
	})

	report, err := service.RunLifecycle(ctx, LifecycleOptions{})
	if err != nil {
		t.Fatalf("RunLifecycle() error = %v", err)
	}
	if report.Deleted != 1 {
		t.Fatalf("RunLifecycle() = %+v, want one policy deletion", report)
	}

	item, err := store.Memories().GetItem(ctx, "mem_archive_old")
	if err != nil {
		t.Fatalf("GetItem() error = %v", err)
	}
	if item.DeletedAt == nil {
		t.Fatalf("item = %+v, want deleted_at", item)
	}
	assertMemoryEvent(t, store, "mem_archive_old", "lifecycle_deleted")
}

func TestLifecycleProtectsPinnedAndImportantMemory(t *testing.T) {
	service, store, closeStore := testServiceWithStore(t)
	defer closeStore()
	ctx := context.Background()
	now := lifecycleNow()

	createLifecycleMemory(t, store, db.MemoryItem{
		ID:         "mem_pinned",
		Body:       "pinned old memory",
		Tier:       db.TierWorking,
		Importance: 10,
		Pinned:     true,
		CreatedAt:  now.Add(-48 * time.Hour),
		UpdatedAt:  now.Add(-48 * time.Hour),
	})
	createLifecycleMemory(t, store, db.MemoryItem{
		ID:         "mem_important",
		Body:       "important old memory",
		Tier:       db.TierHot,
		Importance: 95,
		CreatedAt:  now.Add(-10 * 24 * time.Hour),
		UpdatedAt:  now.Add(-10 * 24 * time.Hour),
	})

	report, err := service.RunLifecycle(ctx, LifecycleOptions{})
	if err != nil {
		t.Fatalf("RunLifecycle() error = %v", err)
	}
	if report.SkippedPinned != 1 || report.SkippedImportant != 1 || report.Demoted != 0 {
		t.Fatalf("RunLifecycle() = %+v, want pinned and important skips", report)
	}

	pinned, err := store.Memories().GetItem(ctx, "mem_pinned")
	if err != nil {
		t.Fatalf("Get pinned error = %v", err)
	}
	if pinned.Tier != db.TierWorking {
		t.Fatalf("pinned tier = %q, want working", pinned.Tier)
	}
	important, err := store.Memories().GetItem(ctx, "mem_important")
	if err != nil {
		t.Fatalf("Get important error = %v", err)
	}
	if important.Tier != db.TierHot {
		t.Fatalf("important tier = %q, want hot", important.Tier)
	}
}

func TestLifecycleAppliesScopedPolicyAcrossMultipleTiers(t *testing.T) {
	service, store, closeStore := testServiceWithStore(t)
	defer closeStore()
	ctx := context.Background()
	now := lifecycleNow()
	archivedAt := now.Add(-48 * time.Hour)

	if err := store.Policies().Create(ctx, db.RetentionPolicy{
		ID:        "policy_release_fast",
		Name:      "release source fast retention",
		ScopeJSON: `{"sources":["release"],"include_pinned":false}`,
		RulesJSON: `{"demote_working_after_days":1,"demote_hot_after_days":2,"demote_warm_after_days":3,"archive_cold_after_days":4,"delete_archived_after_days":1,"protect_importance_at_or_above":100}`,
		Enabled:   true,
		CreatedAt: now.Add(-time.Hour),
		UpdatedAt: now.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("Create policy error = %v", err)
	}

	createLifecycleMemory(t, store, db.MemoryItem{
		ID:         "mem_release_working",
		Body:       "release working memory",
		Source:     "release",
		Tier:       db.TierWorking,
		Importance: 10,
		CreatedAt:  now.Add(-48 * time.Hour),
		UpdatedAt:  now.Add(-48 * time.Hour),
	})
	createLifecycleMemory(t, store, db.MemoryItem{
		ID:         "mem_release_hot",
		Body:       "release hot memory",
		Source:     "release",
		Tier:       db.TierHot,
		Importance: 10,
		CreatedAt:  now.Add(-72 * time.Hour),
		UpdatedAt:  now.Add(-72 * time.Hour),
	})
	createLifecycleMemory(t, store, db.MemoryItem{
		ID:         "mem_release_warm",
		Body:       "release warm memory",
		Source:     "release",
		Tier:       db.TierWarm,
		Importance: 10,
		CreatedAt:  now.Add(-96 * time.Hour),
		UpdatedAt:  now.Add(-96 * time.Hour),
	})
	createLifecycleMemory(t, store, db.MemoryItem{
		ID:         "mem_release_cold",
		Body:       "release cold memory",
		Source:     "release",
		Tier:       db.TierCold,
		Importance: 10,
		CreatedAt:  now.Add(-5 * 24 * time.Hour),
		UpdatedAt:  now.Add(-5 * 24 * time.Hour),
	})
	createLifecycleMemory(t, store, db.MemoryItem{
		ID:         "mem_release_archive",
		Body:       "release archive memory",
		Source:     "release",
		Tier:       db.TierArchive,
		Importance: 10,
		CreatedAt:  now.Add(-10 * 24 * time.Hour),
		UpdatedAt:  archivedAt,
		ArchivedAt: &archivedAt,
	})
	createLifecycleMemory(t, store, db.MemoryItem{
		ID:         "mem_release_pinned",
		Body:       "pinned release memory",
		Source:     "release",
		Tier:       db.TierWorking,
		Importance: 10,
		Pinned:     true,
		CreatedAt:  now.Add(-48 * time.Hour),
		UpdatedAt:  now.Add(-48 * time.Hour),
	})
	createLifecycleMemory(t, store, db.MemoryItem{
		ID:         "mem_release_important",
		Body:       "important release memory",
		Source:     "release",
		Tier:       db.TierHot,
		Importance: 100,
		CreatedAt:  now.Add(-72 * time.Hour),
		UpdatedAt:  now.Add(-72 * time.Hour),
	})

	report, err := service.RunLifecycle(ctx, LifecycleOptions{})
	if err != nil {
		t.Fatalf("RunLifecycle() error = %v", err)
	}
	if report.Evaluated != 7 || report.Demoted != 3 || report.Archived != 1 || report.Deleted != 1 || report.SkippedPinned != 1 || report.SkippedImportant != 1 {
		t.Fatalf("RunLifecycle() = %+v, want scoped tier transitions, retention deletion, and skips", report)
	}

	assertMemoryTier(t, store, "mem_release_working", db.TierHot)
	assertMemoryTier(t, store, "mem_release_hot", db.TierWarm)
	assertMemoryTier(t, store, "mem_release_warm", db.TierCold)
	assertMemoryTier(t, store, "mem_release_cold", db.TierArchive)
	assertMemoryDeleted(t, store, "mem_release_archive")
	assertMemoryTier(t, store, "mem_release_pinned", db.TierWorking)
	assertMemoryTier(t, store, "mem_release_important", db.TierHot)
	assertMemoryEvent(t, store, "mem_release_archive", "lifecycle_deleted")

	for _, change := range report.Changes {
		if change.MemoryID == "mem_release_pinned" || change.MemoryID == "mem_release_important" {
			t.Fatalf("protected memory changed: %+v", change)
		}
		if change.PolicyID != "policy_release_fast" {
			t.Fatalf("change = %+v, want scoped policy id", change)
		}
	}
}

func TestLifecyclePromotesFrequentlyAccessedMemory(t *testing.T) {
	service, store, closeStore := testServiceWithStore(t)
	defer closeStore()
	ctx := context.Background()
	now := lifecycleNow()

	createLifecycleMemory(t, store, db.MemoryItem{
		ID:         "mem_cold_accessed",
		Body:       "old but useful memory",
		Tier:       db.TierCold,
		Importance: 10,
		CreatedAt:  now.Add(-60 * 24 * time.Hour),
		UpdatedAt:  now,
	})
	for _, at := range []time.Time{now.Add(-2 * time.Hour), now.Add(-time.Hour)} {
		if _, err := store.AccessLog().Record(ctx, db.AccessLogEntry{
			MemoryID:   "mem_cold_accessed",
			AccessType: "get",
			CreatedAt:  at,
		}); err != nil {
			t.Fatalf("Record access error = %v", err)
		}
	}

	got, err := service.Get(ctx, "mem_cold_accessed")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Memory.Tier != string(db.TierWarm) {
		t.Fatalf("Get() tier = %q, want warm", got.Memory.Tier)
	}
	assertMemoryEvent(t, store, "mem_cold_accessed", "lifecycle_promoted")
}

func assertMemoryTier(t *testing.T, store *db.Store, memoryID string, want db.Tier) {
	t.Helper()
	item, err := store.Memories().GetItem(context.Background(), memoryID)
	if err != nil {
		t.Fatalf("GetItem(%s) error = %v", memoryID, err)
	}
	if item.Tier != want {
		t.Fatalf("%s tier = %q, want %q", memoryID, item.Tier, want)
	}
}

func assertMemoryDeleted(t *testing.T, store *db.Store, memoryID string) {
	t.Helper()
	item, err := store.Memories().GetItem(context.Background(), memoryID)
	if err != nil {
		t.Fatalf("GetItem(%s) error = %v", memoryID, err)
	}
	if item.DeletedAt == nil {
		t.Fatalf("%s = %+v, want deleted_at", memoryID, item)
	}
}

func createLifecycleMemory(t *testing.T, store *db.Store, item db.MemoryItem) {
	t.Helper()
	if item.Title == "" {
		item.Title = item.ID
	}
	if item.Source == "" {
		item.Source = "test"
	}
	if err := store.Memories().CreateItem(context.Background(), item); err != nil {
		t.Fatalf("CreateItem(%s) error = %v", item.ID, err)
	}
}

func assertMemoryEvent(t *testing.T, store *db.Store, memoryID string, eventType string) {
	t.Helper()
	events, err := store.Memories().ListEvents(context.Background(), memoryID)
	if err != nil {
		t.Fatalf("ListEvents(%s) error = %v", memoryID, err)
	}
	for _, event := range events {
		if event.EventType == eventType {
			return
		}
	}
	t.Fatalf("events for %s = %+v, missing %s", memoryID, events, eventType)
}

func lifecycleNow() time.Time {
	return time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
}
