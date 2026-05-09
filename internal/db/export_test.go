// SPDX-License-Identifier: AGPL-3.0-only

package db

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBackupSQLiteFromWALEnabledDatabase(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.db")
	backupPath := filepath.Join(dir, "backup.db")

	source, err := Open(ctx, Options{Path: sourcePath})
	if err != nil {
		t.Fatalf("Open(source) error = %v", err)
	}
	defer source.Close()
	seedExportFixture(t, source)

	mode, err := source.JournalMode(ctx)
	if err != nil {
		t.Fatalf("JournalMode() error = %v", err)
	}
	if strings.ToLower(mode) != "wal" {
		t.Fatalf("journal_mode = %q, want wal", mode)
	}

	if err := BackupSQLite(ctx, sourcePath, backupPath); err != nil {
		t.Fatalf("BackupSQLite() error = %v", err)
	}

	backup, err := Open(ctx, Options{Path: backupPath})
	if err != nil {
		t.Fatalf("Open(backup) error = %v", err)
	}
	defer backup.Close()

	item, err := backup.Memories().GetItem(ctx, "mem_export")
	if err != nil {
		t.Fatalf("backup GetItem() error = %v", err)
	}
	if item.Body != "Important durable memory" {
		t.Fatalf("backup body = %q", item.Body)
	}
}

func TestRestoreSQLiteBackupToFreshDatabase(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.db")
	backupPath := filepath.Join(dir, "backup.db")
	restorePath := filepath.Join(dir, "restored.db")

	source, err := Open(ctx, Options{Path: sourcePath})
	if err != nil {
		t.Fatalf("Open(source) error = %v", err)
	}
	seedExportFixture(t, source)
	if err := source.Close(); err != nil {
		t.Fatalf("Close(source) error = %v", err)
	}

	if err := BackupSQLite(ctx, sourcePath, backupPath); err != nil {
		t.Fatalf("BackupSQLite() error = %v", err)
	}
	if err := ValidateSQLiteDatabase(ctx, backupPath); err != nil {
		t.Fatalf("ValidateSQLiteDatabase() error = %v", err)
	}
	if err := RestoreSQLiteBackup(ctx, backupPath, restorePath); err != nil {
		t.Fatalf("RestoreSQLiteBackup() error = %v", err)
	}
	if err := RestoreSQLiteBackup(ctx, backupPath, restorePath); err == nil {
		t.Fatal("RestoreSQLiteBackup() second run error = nil, want destination exists error")
	}

	restored, err := Open(ctx, Options{Path: restorePath})
	if err != nil {
		t.Fatalf("Open(restored) error = %v", err)
	}
	defer restored.Close()

	item, err := restored.Memories().GetItem(ctx, "mem_export")
	if err != nil {
		t.Fatalf("restored GetItem() error = %v", err)
	}
	if item.Body != "Important durable memory" {
		t.Fatalf("restored body = %q", item.Body)
	}
}

func TestExportImportNDJSONRoundTrip(t *testing.T) {
	ctx := context.Background()
	source := openTestStore(t)
	seedExportFixture(t, source)

	var export bytes.Buffer
	summary, err := source.ExportNDJSON(ctx, &export, ExportOptions{
		PamieVersion: "test-version",
		Now:          fixedTime(),
	})
	if err != nil {
		t.Fatalf("ExportNDJSON() error = %v", err)
	}
	if summary.Manifest.Counts != (ExportRecordCounts{
		MemoryItems:       1,
		MemoryChunks:      1,
		MemoryKeywords:    2,
		MemoryEvents:      1,
		RetentionPolicies: 1,
		AccessLogs:        1,
	}) {
		t.Fatalf("manifest counts = %+v", summary.Manifest.Counts)
	}
	if summary.Manifest.Checksums.RecordsSHA256 == "" {
		t.Fatal("manifest checksum is empty")
	}

	target := openTestStore(t)
	dryRun, err := target.ImportNDJSON(ctx, bytes.NewReader(export.Bytes()), ImportOptions{DryRun: true})
	if err != nil {
		t.Fatalf("ImportNDJSON(dry-run) error = %v", err)
	}
	if !dryRun.DryRun {
		t.Fatal("dry run summary DryRun = false")
	}
	stats, err := target.Memories().Stats(ctx)
	if err != nil {
		t.Fatalf("Stats() after dry-run error = %v", err)
	}
	if stats.Total != 0 {
		t.Fatalf("dry-run imported %d memories, want 0", stats.Total)
	}

	imported, err := target.ImportNDJSON(ctx, bytes.NewReader(export.Bytes()), ImportOptions{})
	if err != nil {
		t.Fatalf("ImportNDJSON() error = %v", err)
	}
	if imported.Counts != summary.Manifest.Counts {
		t.Fatalf("import counts = %+v, want %+v", imported.Counts, summary.Manifest.Counts)
	}

	item, err := target.Memories().GetItem(ctx, "mem_export")
	if err != nil {
		t.Fatalf("GetItem() after import error = %v", err)
	}
	if item.Title != "Export fixture" || item.Source != "test" || item.Tier != TierWorking || !item.Pinned || item.Importance != 77 {
		t.Fatalf("imported item = %+v", item)
	}
	if item.LastAccessedAt == nil || item.ArchivedAt == nil {
		t.Fatalf("imported nullable timestamps = last_accessed_at:%v archived_at:%v", item.LastAccessedAt, item.ArchivedAt)
	}

	chunks, err := target.Memories().ListChunks(ctx, "mem_export")
	if err != nil {
		t.Fatalf("ListChunks() after import error = %v", err)
	}
	if len(chunks) != 1 || chunks[0].ID != "chunk_export" || chunks[0].Content != "Important durable memory" {
		t.Fatalf("imported chunks = %+v", chunks)
	}
	keywords, err := target.Memories().ListKeywords(ctx, "mem_export")
	if err != nil {
		t.Fatalf("ListKeywords() after import error = %v", err)
	}
	if len(keywords) != 2 || keywords[0].Keyword != "Pamie" || keywords[1].Keyword != "SQLite FTS5" {
		t.Fatalf("imported keywords = %+v", keywords)
	}

	events, err := target.Memories().ListEvents(ctx, "mem_export")
	if err != nil {
		t.Fatalf("ListEvents() after import error = %v", err)
	}
	if len(events) != 1 || events[0].EventType != "created" || events[0].EventPayloadJSON != `{"source":"test"}` {
		t.Fatalf("imported events = %+v", events)
	}

	var policyCount int
	if err := target.database.QueryRowContext(ctx, "SELECT COUNT(*) FROM retention_policies WHERE id = 'policy_export'").Scan(&policyCount); err != nil {
		t.Fatalf("query retention policies error = %v", err)
	}
	if policyCount != 1 {
		t.Fatalf("policy count = %d, want 1", policyCount)
	}

	var tokenID string
	if err := target.database.QueryRowContext(ctx, "SELECT token_id FROM access_log WHERE id = 1").Scan(&tokenID); err != nil {
		t.Fatalf("query access log error = %v", err)
	}
	if tokenID != "token_1" {
		t.Fatalf("token_id = %q, want token_1", tokenID)
	}
}

func TestImportNDJSONRejectsMalformedRecords(t *testing.T) {
	target := openTestStore(t)
	_, err := target.ImportNDJSON(context.Background(), strings.NewReader("{not-json}\n"), ImportOptions{DryRun: true})
	if err == nil || !strings.Contains(err.Error(), "decode manifest") {
		t.Fatalf("ImportNDJSON() error = %v, want decode manifest error", err)
	}
}

func TestImportNDJSONRejectsChecksumMismatch(t *testing.T) {
	source := openTestStore(t)
	seedExportFixture(t, source)

	var export bytes.Buffer
	if _, err := source.ExportNDJSON(context.Background(), &export, ExportOptions{Now: fixedTime()}); err != nil {
		t.Fatalf("ExportNDJSON() error = %v", err)
	}
	tampered := bytes.Replace(export.Bytes(), []byte("Important durable memory"), []byte("Changed durable memory"), 1)

	target := openTestStore(t)
	_, err := target.ImportNDJSON(context.Background(), bytes.NewReader(tampered), ImportOptions{DryRun: true})
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("ImportNDJSON() error = %v, want checksum mismatch", err)
	}
}

func TestImportNDJSONRejectsDuplicateIDs(t *testing.T) {
	now := fixedTime()
	item := exportMemoryItem{
		ID:           "mem_duplicate",
		Body:         "duplicate body",
		MetadataJSON: "{}",
		Tier:         TierWorking,
		Importance:   10,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	data := exportData{
		Items: []exportMemoryItem{item, item},
	}
	input := encodeTestExport(t, data)

	target := openTestStore(t)
	_, err := target.ImportNDJSON(context.Background(), bytes.NewReader(input), ImportOptions{DryRun: true})
	if err == nil || !strings.Contains(err.Error(), `duplicate memory item id "mem_duplicate"`) {
		t.Fatalf("ImportNDJSON() error = %v, want duplicate memory item id", err)
	}
}

func TestImportNDJSONRejectsDuplicateKeywords(t *testing.T) {
	now := fixedTime()
	item := exportMemoryItem{
		ID:           "mem_keyword_duplicate",
		Body:         "keyword duplicate body",
		MetadataJSON: "{}",
		Tier:         TierWorking,
		Importance:   10,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	data := exportData{
		Items: []exportMemoryItem{item},
		Keywords: []exportMemoryKeyword{
			{MemoryID: item.ID, KeywordIndex: 0, Keyword: "Alpha", NormalizedKeyword: "alpha", CreatedAt: now, UpdatedAt: now},
			{MemoryID: item.ID, KeywordIndex: 1, Keyword: "alpha", NormalizedKeyword: "alpha", CreatedAt: now, UpdatedAt: now},
		},
	}
	input := encodeTestExport(t, data)

	target := openTestStore(t)
	_, err := target.ImportNDJSON(context.Background(), bytes.NewReader(input), ImportOptions{DryRun: true})
	if err == nil || !strings.Contains(err.Error(), `duplicate normalized memory keyword "alpha"`) {
		t.Fatalf("ImportNDJSON() error = %v, want duplicate keyword error", err)
	}
}

func TestImportNDJSONRejectsTargetDuplicateIDs(t *testing.T) {
	source := openTestStore(t)
	seedExportFixture(t, source)

	var export bytes.Buffer
	if _, err := source.ExportNDJSON(context.Background(), &export, ExportOptions{Now: fixedTime()}); err != nil {
		t.Fatalf("ExportNDJSON() error = %v", err)
	}

	target := openTestStore(t)
	if _, err := target.ImportNDJSON(context.Background(), bytes.NewReader(export.Bytes()), ImportOptions{}); err != nil {
		t.Fatalf("first ImportNDJSON() error = %v", err)
	}
	_, err := target.ImportNDJSON(context.Background(), bytes.NewReader(export.Bytes()), ImportOptions{DryRun: true})
	if err == nil || !strings.Contains(err.Error(), "already exists in target database") {
		t.Fatalf("second ImportNDJSON() error = %v, want target duplicate error", err)
	}
}

func seedExportFixture(t *testing.T, store *Store) {
	t.Helper()
	ctx := context.Background()
	now := fixedTime()
	lastAccessed := now.Add(15 * time.Minute)
	archivedAt := now.Add(30 * time.Minute)

	if err := store.Memories().CreateItem(ctx, MemoryItem{
		ID:             "mem_export",
		Title:          "Export fixture",
		Body:           "Important durable memory",
		Source:         "test",
		MetadataJSON:   `{"project":"pamie","phase":9}`,
		Tier:           TierWorking,
		Importance:     77,
		Pinned:         true,
		CreatedAt:      now,
		UpdatedAt:      now.Add(time.Hour),
		LastAccessedAt: &lastAccessed,
		ArchivedAt:     &archivedAt,
	}); err != nil {
		t.Fatalf("CreateItem() error = %v", err)
	}
	if err := store.Memories().AddChunk(ctx, MemoryChunk{
		ID:         "chunk_export",
		MemoryID:   "mem_export",
		ChunkIndex: 0,
		Content:    "Important durable memory",
		CreatedAt:  now,
	}); err != nil {
		t.Fatalf("AddChunk() error = %v", err)
	}
	if err := store.Memories().ReplaceKeywords(ctx, "mem_export", []MemoryKeyword{
		{MemoryID: "mem_export", KeywordIndex: 0, Keyword: "Pamie", NormalizedKeyword: "pamie", CreatedAt: now, UpdatedAt: now},
		{MemoryID: "mem_export", KeywordIndex: 1, Keyword: "SQLite FTS5", NormalizedKeyword: "sqlite fts5", CreatedAt: now, UpdatedAt: now},
	}); err != nil {
		t.Fatalf("ReplaceKeywords() error = %v", err)
	}
	if _, err := store.Memories().RecordEvent(ctx, MemoryEvent{
		MemoryID:         "mem_export",
		EventType:        "created",
		EventPayloadJSON: `{"source":"test"}`,
		CreatedAt:        now,
	}); err != nil {
		t.Fatalf("RecordEvent() error = %v", err)
	}
	if err := store.Policies().Create(ctx, RetentionPolicy{
		ID:        "policy_export",
		Name:      "archive old cold memories",
		ScopeJSON: `{"tier":"cold"}`,
		RulesJSON: `{"archive_after_days":90}`,
		Enabled:   true,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("Policies().Create() error = %v", err)
	}
	if _, err := store.AccessLog().Record(ctx, AccessLogEntry{
		MemoryID:   "mem_export",
		AccessType: "get",
		TokenID:    "token_1",
		CreatedAt:  now,
	}); err != nil {
		t.Fatalf("AccessLog().Record() error = %v", err)
	}
}

func encodeTestExport(t *testing.T, data exportData) []byte {
	t.Helper()
	lines, checksum, err := marshalRecordLines(data)
	if err != nil {
		t.Fatalf("marshalRecordLines() error = %v", err)
	}
	schemaVersion, err := CurrentSchemaVersion()
	if err != nil {
		t.Fatalf("CurrentSchemaVersion() error = %v", err)
	}
	manifest := ExportManifest{
		Format:        exportFormat,
		Version:       exportVersion,
		SchemaVersion: schemaVersion,
		ExportedAt:    fixedTime(),
		PamieVersion:  "test",
		Counts:        data.counts(),
		Checksums:     ExportChecksums{RecordsSHA256: checksum},
	}

	var output bytes.Buffer
	if err := json.NewEncoder(&output).Encode(manifestLine{Type: "manifest", Manifest: manifest}); err != nil {
		t.Fatalf("encode manifest error = %v", err)
	}
	for _, line := range lines {
		output.Write(line)
		output.WriteByte('\n')
	}
	return output.Bytes()
}
