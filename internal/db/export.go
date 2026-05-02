// SPDX-License-Identifier: AGPL-3.0-only

package db

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	exportFormat  = "pamie.ndjson"
	exportVersion = 1
)

var errDryRunRollback = errors.New("dry-run rollback")

// ExportOptions configures NDJSON export.
type ExportOptions struct {
	PamieVersion string
	Now          time.Time
}

// ImportOptions configures NDJSON import.
type ImportOptions struct {
	DryRun bool
}

// ExportSummary describes a completed NDJSON export.
type ExportSummary struct {
	Manifest ExportManifest
}

// ImportSummary describes a validated or completed NDJSON import.
type ImportSummary struct {
	DryRun bool
	Counts ExportRecordCounts
}

// ExportManifest is the first line of every Pamie NDJSON export.
type ExportManifest struct {
	Format        string             `json:"format"`
	Version       int                `json:"version"`
	SchemaVersion int                `json:"schema_version"`
	ExportedAt    time.Time          `json:"exported_at"`
	PamieVersion  string             `json:"pamie_version"`
	Counts        ExportRecordCounts `json:"record_counts"`
	Checksums     ExportChecksums    `json:"checksums"`
}

// ExportRecordCounts records per-table row counts in the manifest.
type ExportRecordCounts struct {
	MemoryItems       int `json:"memory_items"`
	MemoryChunks      int `json:"memory_chunks"`
	MemoryEvents      int `json:"memory_events"`
	RetentionPolicies int `json:"retention_policies"`
	AccessLogs        int `json:"access_logs"`
}

// ExportChecksums records integrity data for the exported record stream.
type ExportChecksums struct {
	RecordsSHA256 string `json:"records_sha256"`
}

type exportData struct {
	Items    []exportMemoryItem
	Chunks   []exportMemoryChunk
	Events   []exportMemoryEvent
	Policies []exportRetentionPolicy
	Access   []exportAccessLogEntry
}

type manifestLine struct {
	Type     string         `json:"type"`
	Manifest ExportManifest `json:"manifest"`
}

type exportRecordLine struct {
	Type            string                 `json:"type"`
	MemoryItem      *exportMemoryItem      `json:"memory_item,omitempty"`
	MemoryChunk     *exportMemoryChunk     `json:"memory_chunk,omitempty"`
	MemoryEvent     *exportMemoryEvent     `json:"memory_event,omitempty"`
	RetentionPolicy *exportRetentionPolicy `json:"retention_policy,omitempty"`
	AccessLog       *exportAccessLogEntry  `json:"access_log,omitempty"`
}

type exportMemoryItem struct {
	ID             string     `json:"id"`
	Title          string     `json:"title"`
	Body           string     `json:"body"`
	Source         string     `json:"source"`
	MetadataJSON   string     `json:"metadata_json"`
	Tier           Tier       `json:"tier"`
	Importance     int        `json:"importance"`
	Pinned         bool       `json:"pinned"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	LastAccessedAt *time.Time `json:"last_accessed_at,omitempty"`
	ArchivedAt     *time.Time `json:"archived_at,omitempty"`
	DeletedAt      *time.Time `json:"deleted_at,omitempty"`
}

type exportMemoryChunk struct {
	ID         string    `json:"id"`
	MemoryID   string    `json:"memory_id"`
	ChunkIndex int       `json:"chunk_index"`
	Content    string    `json:"content"`
	CreatedAt  time.Time `json:"created_at"`
}

type exportMemoryEvent struct {
	ID               int64     `json:"id"`
	MemoryID         *string   `json:"memory_id,omitempty"`
	EventType        string    `json:"event_type"`
	EventPayloadJSON string    `json:"event_payload_json"`
	CreatedAt        time.Time `json:"created_at"`
}

type exportRetentionPolicy struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	ScopeJSON string    `json:"scope_json"`
	RulesJSON string    `json:"rules_json"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type exportAccessLogEntry struct {
	ID         int64     `json:"id"`
	MemoryID   string    `json:"memory_id"`
	AccessType string    `json:"access_type"`
	TokenID    *string   `json:"token_id,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// ExportNDJSON writes a complete, versioned NDJSON export of durable memory data.
func (s *Store) ExportNDJSON(ctx context.Context, writer io.Writer, opts ExportOptions) (ExportSummary, error) {
	if s == nil || s.database == nil {
		return ExportSummary{}, errors.New("database is not open")
	}
	if writer == nil {
		return ExportSummary{}, errors.New("export writer must not be nil")
	}
	data, err := s.readExportData(ctx)
	if err != nil {
		return ExportSummary{}, err
	}
	lines, recordsChecksum, err := marshalRecordLines(data)
	if err != nil {
		return ExportSummary{}, err
	}
	schemaVersion, err := CurrentSchemaVersion()
	if err != nil {
		return ExportSummary{}, err
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	pamieVersion := strings.TrimSpace(opts.PamieVersion)
	if pamieVersion == "" {
		pamieVersion = "unknown"
	}
	manifest := ExportManifest{
		Format:        exportFormat,
		Version:       exportVersion,
		SchemaVersion: schemaVersion,
		ExportedAt:    now.UTC(),
		PamieVersion:  pamieVersion,
		Counts:        data.counts(),
		Checksums: ExportChecksums{
			RecordsSHA256: recordsChecksum,
		},
	}

	buffered := bufio.NewWriter(writer)
	manifestLine, err := json.Marshal(manifestLine{Type: "manifest", Manifest: manifest})
	if err != nil {
		return ExportSummary{}, fmt.Errorf("encode export manifest: %w", err)
	}
	if _, err := buffered.Write(append(manifestLine, '\n')); err != nil {
		return ExportSummary{}, fmt.Errorf("write export manifest: %w", err)
	}
	for _, line := range lines {
		if _, err := buffered.Write(append(line, '\n')); err != nil {
			return ExportSummary{}, fmt.Errorf("write export record: %w", err)
		}
	}
	if err := buffered.Flush(); err != nil {
		return ExportSummary{}, fmt.Errorf("flush export: %w", err)
	}
	return ExportSummary{Manifest: manifest}, nil
}

// ImportNDJSON validates and optionally imports a Pamie NDJSON export.
//
// Duplicate IDs in the target database are always rejected. Imports are append-only
// and never overwrite existing rows.
func (s *Store) ImportNDJSON(ctx context.Context, reader io.Reader, opts ImportOptions) (ImportSummary, error) {
	if s == nil || s.database == nil {
		return ImportSummary{}, errors.New("database is not open")
	}
	if reader == nil {
		return ImportSummary{}, errors.New("import reader must not be nil")
	}
	manifest, data, err := parseNDJSONExport(reader)
	if err != nil {
		return ImportSummary{}, err
	}
	if err := validateImportData(manifest, data); err != nil {
		return ImportSummary{}, err
	}
	if err := s.ensureNoImportConflicts(ctx, data); err != nil {
		return ImportSummary{}, err
	}

	err = s.WithinTx(ctx, func(ctx context.Context, tx *Tx) error {
		if err := applyImport(ctx, tx, data); err != nil {
			return err
		}
		if opts.DryRun {
			return errDryRunRollback
		}
		return nil
	})
	if errors.Is(err, errDryRunRollback) {
		return ImportSummary{DryRun: true, Counts: data.counts()}, nil
	}
	if err != nil {
		return ImportSummary{}, err
	}
	return ImportSummary{DryRun: false, Counts: data.counts()}, nil
}

// CurrentSchemaVersion returns the highest embedded migration version.
func CurrentSchemaVersion() (int, error) {
	migrations, err := loadMigrations()
	if err != nil {
		return 0, err
	}
	if len(migrations) == 0 {
		return 0, errors.New("no migrations are embedded")
	}
	return migrations[len(migrations)-1].Version, nil
}

func (s *Store) readExportData(ctx context.Context) (exportData, error) {
	var data exportData
	var err error
	if data.Items, err = s.listExportMemoryItems(ctx); err != nil {
		return exportData{}, err
	}
	if data.Chunks, err = s.listExportMemoryChunks(ctx); err != nil {
		return exportData{}, err
	}
	if data.Events, err = s.listExportMemoryEvents(ctx); err != nil {
		return exportData{}, err
	}
	if data.Policies, err = s.listExportRetentionPolicies(ctx); err != nil {
		return exportData{}, err
	}
	if data.Access, err = s.listExportAccessLogs(ctx); err != nil {
		return exportData{}, err
	}
	return data, nil
}

func (s *Store) listExportMemoryItems(ctx context.Context) ([]exportMemoryItem, error) {
	rows, err := s.database.QueryContext(ctx, `
SELECT id, title, body, source, metadata_json, tier, importance, pinned,
       created_at, updated_at, last_accessed_at, archived_at, deleted_at
FROM memory_items
ORDER BY created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("export memory items: %w", err)
	}
	defer rows.Close()

	var items []exportMemoryItem
	for rows.Next() {
		var item MemoryItem
		if item, err = scanMemoryItem(rows); err != nil {
			return nil, err
		}
		items = append(items, exportMemoryItem{
			ID:             item.ID,
			Title:          item.Title,
			Body:           item.Body,
			Source:         item.Source,
			MetadataJSON:   item.MetadataJSON,
			Tier:           item.Tier,
			Importance:     item.Importance,
			Pinned:         item.Pinned,
			CreatedAt:      item.CreatedAt,
			UpdatedAt:      item.UpdatedAt,
			LastAccessedAt: item.LastAccessedAt,
			ArchivedAt:     item.ArchivedAt,
			DeletedAt:      item.DeletedAt,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("export memory items: %w", err)
	}
	return items, nil
}

func (s *Store) listExportMemoryChunks(ctx context.Context) ([]exportMemoryChunk, error) {
	rows, err := s.database.QueryContext(ctx, `
SELECT id, memory_id, chunk_index, content, created_at
FROM memory_chunks
ORDER BY memory_id, chunk_index, id`)
	if err != nil {
		return nil, fmt.Errorf("export memory chunks: %w", err)
	}
	defer rows.Close()

	var chunks []exportMemoryChunk
	for rows.Next() {
		var chunk exportMemoryChunk
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
		return nil, fmt.Errorf("export memory chunks: %w", err)
	}
	return chunks, nil
}

func (s *Store) listExportMemoryEvents(ctx context.Context) ([]exportMemoryEvent, error) {
	rows, err := s.database.QueryContext(ctx, `
SELECT id, memory_id, event_type, event_payload_json, created_at
FROM memory_events
ORDER BY created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("export memory events: %w", err)
	}
	defer rows.Close()

	var events []exportMemoryEvent
	for rows.Next() {
		var event exportMemoryEvent
		var memoryID sql.NullString
		var createdAt string
		if err := rows.Scan(&event.ID, &memoryID, &event.EventType, &event.EventPayloadJSON, &createdAt); err != nil {
			return nil, fmt.Errorf("scan memory event: %w", err)
		}
		if memoryID.Valid {
			value := memoryID.String
			event.MemoryID = &value
		}
		if event.CreatedAt, err = parseTime(createdAt); err != nil {
			return nil, fmt.Errorf("parse event created_at: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("export memory events: %w", err)
	}
	return events, nil
}

func (s *Store) listExportRetentionPolicies(ctx context.Context) ([]exportRetentionPolicy, error) {
	rows, err := s.database.QueryContext(ctx, `
SELECT id, name, scope_json, rules_json, enabled, created_at, updated_at
FROM retention_policies
ORDER BY created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("export retention policies: %w", err)
	}
	defer rows.Close()

	var policies []exportRetentionPolicy
	for rows.Next() {
		policy, err := scanRetentionPolicy(rows)
		if err != nil {
			return nil, err
		}
		policies = append(policies, exportRetentionPolicy{
			ID:        policy.ID,
			Name:      policy.Name,
			ScopeJSON: policy.ScopeJSON,
			RulesJSON: policy.RulesJSON,
			Enabled:   policy.Enabled,
			CreatedAt: policy.CreatedAt,
			UpdatedAt: policy.UpdatedAt,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("export retention policies: %w", err)
	}
	return policies, nil
}

func (s *Store) listExportAccessLogs(ctx context.Context) ([]exportAccessLogEntry, error) {
	rows, err := s.database.QueryContext(ctx, `
SELECT id, memory_id, access_type, token_id, created_at
FROM access_log
ORDER BY created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("export access logs: %w", err)
	}
	defer rows.Close()

	var entries []exportAccessLogEntry
	for rows.Next() {
		var entry exportAccessLogEntry
		var tokenID sql.NullString
		var createdAt string
		if err := rows.Scan(&entry.ID, &entry.MemoryID, &entry.AccessType, &tokenID, &createdAt); err != nil {
			return nil, fmt.Errorf("scan access log: %w", err)
		}
		if tokenID.Valid {
			value := tokenID.String
			entry.TokenID = &value
		}
		if entry.CreatedAt, err = parseTime(createdAt); err != nil {
			return nil, fmt.Errorf("parse access created_at: %w", err)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("export access logs: %w", err)
	}
	return entries, nil
}

func marshalRecordLines(data exportData) ([][]byte, string, error) {
	lines := make([][]byte, 0, data.totalRecords())
	appendRecord := func(record exportRecordLine) error {
		line, err := json.Marshal(record)
		if err != nil {
			return err
		}
		lines = append(lines, line)
		return nil
	}

	for _, item := range data.Items {
		item := item
		if err := appendRecord(exportRecordLine{Type: "memory_item", MemoryItem: &item}); err != nil {
			return nil, "", fmt.Errorf("encode memory item: %w", err)
		}
	}
	for _, chunk := range data.Chunks {
		chunk := chunk
		if err := appendRecord(exportRecordLine{Type: "memory_chunk", MemoryChunk: &chunk}); err != nil {
			return nil, "", fmt.Errorf("encode memory chunk: %w", err)
		}
	}
	for _, event := range data.Events {
		event := event
		if err := appendRecord(exportRecordLine{Type: "memory_event", MemoryEvent: &event}); err != nil {
			return nil, "", fmt.Errorf("encode memory event: %w", err)
		}
	}
	for _, policy := range data.Policies {
		policy := policy
		if err := appendRecord(exportRecordLine{Type: "retention_policy", RetentionPolicy: &policy}); err != nil {
			return nil, "", fmt.Errorf("encode retention policy: %w", err)
		}
	}
	for _, entry := range data.Access {
		entry := entry
		if err := appendRecord(exportRecordLine{Type: "access_log", AccessLog: &entry}); err != nil {
			return nil, "", fmt.Errorf("encode access log: %w", err)
		}
	}
	return lines, checksumRecordLines(lines), nil
}

func checksumRecordLines(lines [][]byte) string {
	hasher := sha256.New()
	for _, line := range lines {
		_, _ = hasher.Write(line)
		_, _ = hasher.Write([]byte{'\n'})
	}
	return hex.EncodeToString(hasher.Sum(nil))
}

func parseNDJSONExport(reader io.Reader) (ExportManifest, exportData, error) {
	buffered := bufio.NewReader(reader)
	line, lineNo, err := readNDJSONLine(buffered, 0)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return ExportManifest{}, exportData{}, errors.New("import file is empty")
		}
		return ExportManifest{}, exportData{}, err
	}
	var manifestEnvelope manifestLine
	if err := decodeStrict(line, &manifestEnvelope); err != nil {
		return ExportManifest{}, exportData{}, fmt.Errorf("line %d: decode manifest: %w", lineNo, err)
	}
	if manifestEnvelope.Type != "manifest" {
		return ExportManifest{}, exportData{}, fmt.Errorf("line %d: first record must be manifest", lineNo)
	}

	var data exportData
	var recordLines [][]byte
	for {
		line, lineNo, err = readNDJSONLine(buffered, lineNo)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return ExportManifest{}, exportData{}, err
		}
		recordLines = append(recordLines, append([]byte(nil), line...))
		var record exportRecordLine
		if err := decodeStrict(line, &record); err != nil {
			return ExportManifest{}, exportData{}, fmt.Errorf("line %d: decode record: %w", lineNo, err)
		}
		if err := appendParsedRecord(&data, record); err != nil {
			return ExportManifest{}, exportData{}, fmt.Errorf("line %d: %w", lineNo, err)
		}
	}
	actualChecksum := checksumRecordLines(recordLines)
	if manifestEnvelope.Manifest.Checksums.RecordsSHA256 != actualChecksum {
		return ExportManifest{}, exportData{}, fmt.Errorf("record checksum mismatch: manifest has %s, calculated %s", manifestEnvelope.Manifest.Checksums.RecordsSHA256, actualChecksum)
	}
	return manifestEnvelope.Manifest, data, nil
}

func readNDJSONLine(reader *bufio.Reader, previousLineNo int) ([]byte, int, error) {
	line, err := reader.ReadBytes('\n')
	if errors.Is(err, io.EOF) && len(line) == 0 {
		return nil, previousLineNo, io.EOF
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, previousLineNo, fmt.Errorf("read line %d: %w", previousLineNo+1, err)
	}
	line = bytes.TrimSuffix(line, []byte{'\n'})
	line = bytes.TrimSuffix(line, []byte{'\r'})
	lineNo := previousLineNo + 1
	if len(bytes.TrimSpace(line)) == 0 {
		return nil, lineNo, fmt.Errorf("line %d: empty NDJSON records are not allowed", lineNo)
	}
	return line, lineNo, nil
}

func decodeStrict(line []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(line))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("record must contain exactly one JSON object")
	}
	return nil
}

func appendParsedRecord(data *exportData, record exportRecordLine) error {
	if countPayloads(record) != 1 {
		return fmt.Errorf("record type %q must include exactly one payload", record.Type)
	}
	switch record.Type {
	case "memory_item":
		if record.MemoryItem == nil {
			return errors.New("memory_item record missing memory_item payload")
		}
		data.Items = append(data.Items, *record.MemoryItem)
	case "memory_chunk":
		if record.MemoryChunk == nil {
			return errors.New("memory_chunk record missing memory_chunk payload")
		}
		data.Chunks = append(data.Chunks, *record.MemoryChunk)
	case "memory_event":
		if record.MemoryEvent == nil {
			return errors.New("memory_event record missing memory_event payload")
		}
		data.Events = append(data.Events, *record.MemoryEvent)
	case "retention_policy":
		if record.RetentionPolicy == nil {
			return errors.New("retention_policy record missing retention_policy payload")
		}
		data.Policies = append(data.Policies, *record.RetentionPolicy)
	case "access_log":
		if record.AccessLog == nil {
			return errors.New("access_log record missing access_log payload")
		}
		data.Access = append(data.Access, *record.AccessLog)
	default:
		return fmt.Errorf("unsupported record type %q", record.Type)
	}
	return nil
}

func countPayloads(record exportRecordLine) int {
	count := 0
	if record.MemoryItem != nil {
		count++
	}
	if record.MemoryChunk != nil {
		count++
	}
	if record.MemoryEvent != nil {
		count++
	}
	if record.RetentionPolicy != nil {
		count++
	}
	if record.AccessLog != nil {
		count++
	}
	return count
}

func validateImportData(manifest ExportManifest, data exportData) error {
	if err := validateManifest(manifest, data.counts()); err != nil {
		return err
	}
	memoryIDs := map[string]struct{}{}
	chunkIDs := map[string]struct{}{}
	chunkIndexes := map[string]struct{}{}
	eventIDs := map[int64]struct{}{}
	policyIDs := map[string]struct{}{}
	accessIDs := map[int64]struct{}{}

	for _, item := range data.Items {
		dbItem := MemoryItem{
			ID:             item.ID,
			Title:          item.Title,
			Body:           item.Body,
			Source:         item.Source,
			MetadataJSON:   item.MetadataJSON,
			Tier:           item.Tier,
			Importance:     item.Importance,
			Pinned:         item.Pinned,
			CreatedAt:      item.CreatedAt,
			UpdatedAt:      item.UpdatedAt,
			LastAccessedAt: item.LastAccessedAt,
			ArchivedAt:     item.ArchivedAt,
			DeletedAt:      item.DeletedAt,
		}
		if err := validateMemoryItem(dbItem); err != nil {
			return err
		}
		if err := validateJSONObject("memory metadata_json", item.MetadataJSON); err != nil {
			return err
		}
		if _, exists := memoryIDs[item.ID]; exists {
			return fmt.Errorf("duplicate memory item id %q", item.ID)
		}
		memoryIDs[item.ID] = struct{}{}
	}
	for _, chunk := range data.Chunks {
		dbChunk := MemoryChunk{
			ID:         chunk.ID,
			MemoryID:   chunk.MemoryID,
			ChunkIndex: chunk.ChunkIndex,
			Content:    chunk.Content,
			CreatedAt:  chunk.CreatedAt,
		}
		if err := validateMemoryChunk(dbChunk); err != nil {
			return err
		}
		if _, exists := memoryIDs[chunk.MemoryID]; !exists {
			return fmt.Errorf("memory chunk %q references missing memory %q", chunk.ID, chunk.MemoryID)
		}
		if _, exists := chunkIDs[chunk.ID]; exists {
			return fmt.Errorf("duplicate memory chunk id %q", chunk.ID)
		}
		chunkIDs[chunk.ID] = struct{}{}
		indexKey := chunk.MemoryID + "\x00" + fmt.Sprint(chunk.ChunkIndex)
		if _, exists := chunkIndexes[indexKey]; exists {
			return fmt.Errorf("duplicate memory chunk index %d for memory %q", chunk.ChunkIndex, chunk.MemoryID)
		}
		chunkIndexes[indexKey] = struct{}{}
	}
	for _, event := range data.Events {
		if event.ID <= 0 {
			return fmt.Errorf("%w: memory event id must be positive", ErrInvalid)
		}
		if event.EventType == "" {
			return fmt.Errorf("%w: event type must not be empty", ErrInvalid)
		}
		if event.CreatedAt.IsZero() {
			return fmt.Errorf("%w: event created_at must not be zero", ErrInvalid)
		}
		if err := validateJSONObject("memory event payload", event.EventPayloadJSON); err != nil {
			return err
		}
		if event.MemoryID != nil {
			if *event.MemoryID == "" {
				return fmt.Errorf("%w: memory event memory_id must not be empty when present", ErrInvalid)
			}
			if _, exists := memoryIDs[*event.MemoryID]; !exists {
				return fmt.Errorf("memory event %d references missing memory %q", event.ID, *event.MemoryID)
			}
		}
		if _, exists := eventIDs[event.ID]; exists {
			return fmt.Errorf("duplicate memory event id %d", event.ID)
		}
		eventIDs[event.ID] = struct{}{}
	}
	for _, policy := range data.Policies {
		dbPolicy := RetentionPolicy{
			ID:        policy.ID,
			Name:      policy.Name,
			ScopeJSON: policy.ScopeJSON,
			RulesJSON: policy.RulesJSON,
			Enabled:   policy.Enabled,
			CreatedAt: policy.CreatedAt,
			UpdatedAt: policy.UpdatedAt,
		}
		if err := validateRetentionPolicy(dbPolicy); err != nil {
			return err
		}
		if err := validateJSONObject("retention policy scope_json", policy.ScopeJSON); err != nil {
			return err
		}
		if err := validateJSONObject("retention policy rules_json", policy.RulesJSON); err != nil {
			return err
		}
		if _, exists := policyIDs[policy.ID]; exists {
			return fmt.Errorf("duplicate retention policy id %q", policy.ID)
		}
		policyIDs[policy.ID] = struct{}{}
	}
	for _, entry := range data.Access {
		if entry.ID <= 0 {
			return fmt.Errorf("%w: access log id must be positive", ErrInvalid)
		}
		dbEntry := AccessLogEntry{
			MemoryID:   entry.MemoryID,
			AccessType: entry.AccessType,
			CreatedAt:  entry.CreatedAt,
		}
		if entry.TokenID != nil {
			dbEntry.TokenID = *entry.TokenID
		}
		if err := validateAccessLogEntry(dbEntry); err != nil {
			return err
		}
		if _, exists := memoryIDs[entry.MemoryID]; !exists {
			return fmt.Errorf("access log %d references missing memory %q", entry.ID, entry.MemoryID)
		}
		if _, exists := accessIDs[entry.ID]; exists {
			return fmt.Errorf("duplicate access log id %d", entry.ID)
		}
		accessIDs[entry.ID] = struct{}{}
	}
	return nil
}

func validateManifest(manifest ExportManifest, counts ExportRecordCounts) error {
	if manifest.Format != exportFormat {
		return fmt.Errorf("unsupported export format %q", manifest.Format)
	}
	if manifest.Version != exportVersion {
		return fmt.Errorf("unsupported export version %d", manifest.Version)
	}
	schemaVersion, err := CurrentSchemaVersion()
	if err != nil {
		return err
	}
	if manifest.SchemaVersion != schemaVersion {
		return fmt.Errorf("unsupported schema version %d; current schema version is %d", manifest.SchemaVersion, schemaVersion)
	}
	if manifest.ExportedAt.IsZero() {
		return errors.New("manifest exported_at must not be zero")
	}
	if strings.TrimSpace(manifest.PamieVersion) == "" {
		return errors.New("manifest pamie_version must not be empty")
	}
	if manifest.Checksums.RecordsSHA256 == "" {
		return errors.New("manifest records_sha256 checksum must not be empty")
	}
	if len(manifest.Checksums.RecordsSHA256) != sha256.Size*2 {
		return fmt.Errorf("manifest records_sha256 checksum has invalid length %d", len(manifest.Checksums.RecordsSHA256))
	}
	if _, err := hex.DecodeString(manifest.Checksums.RecordsSHA256); err != nil {
		return fmt.Errorf("manifest records_sha256 checksum is not hex: %w", err)
	}
	if manifest.Counts != counts {
		return fmt.Errorf("manifest record counts %+v do not match records %+v", manifest.Counts, counts)
	}
	return nil
}

func validateJSONObject(kind, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%w: %s must not be empty", ErrInvalid, kind)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(value), &decoded); err != nil {
		return fmt.Errorf("%w: %s must be a JSON object: %v", ErrInvalid, kind, err)
	}
	if decoded == nil {
		return fmt.Errorf("%w: %s must be a JSON object", ErrInvalid, kind)
	}
	return nil
}

func (s *Store) ensureNoImportConflicts(ctx context.Context, data exportData) error {
	for _, item := range data.Items {
		if err := s.ensureNoRowID(ctx, "memory_items", "id", item.ID); err != nil {
			return err
		}
	}
	for _, chunk := range data.Chunks {
		if err := s.ensureNoRowID(ctx, "memory_chunks", "id", chunk.ID); err != nil {
			return err
		}
	}
	for _, event := range data.Events {
		if err := s.ensureNoRowID(ctx, "memory_events", "id", event.ID); err != nil {
			return err
		}
	}
	for _, policy := range data.Policies {
		if err := s.ensureNoRowID(ctx, "retention_policies", "id", policy.ID); err != nil {
			return err
		}
	}
	for _, entry := range data.Access {
		if err := s.ensureNoRowID(ctx, "access_log", "id", entry.ID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ensureNoRowID(ctx context.Context, table, column string, id any) error {
	var exists int
	query := fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM %s WHERE %s = ?)", table, column)
	if err := s.database.QueryRowContext(ctx, query, id).Scan(&exists); err != nil {
		return fmt.Errorf("check duplicate %s.%s: %w", table, column, err)
	}
	if exists != 0 {
		return fmt.Errorf("duplicate %s.%s %v already exists in target database", table, column, id)
	}
	return nil
}

func applyImport(ctx context.Context, tx *Tx, data exportData) error {
	for _, item := range data.Items {
		if err := tx.Memories().CreateItem(ctx, MemoryItem{
			ID:             item.ID,
			Title:          item.Title,
			Body:           item.Body,
			Source:         item.Source,
			MetadataJSON:   item.MetadataJSON,
			Tier:           item.Tier,
			Importance:     item.Importance,
			Pinned:         item.Pinned,
			CreatedAt:      item.CreatedAt,
			UpdatedAt:      item.UpdatedAt,
			LastAccessedAt: item.LastAccessedAt,
			ArchivedAt:     item.ArchivedAt,
			DeletedAt:      item.DeletedAt,
		}); err != nil {
			return err
		}
	}
	for _, chunk := range data.Chunks {
		if err := tx.Memories().AddChunk(ctx, MemoryChunk{
			ID:         chunk.ID,
			MemoryID:   chunk.MemoryID,
			ChunkIndex: chunk.ChunkIndex,
			Content:    chunk.Content,
			CreatedAt:  chunk.CreatedAt,
		}); err != nil {
			return err
		}
	}
	for _, event := range data.Events {
		if err := insertMemoryEvent(ctx, tx.tx, event); err != nil {
			return err
		}
	}
	for _, policy := range data.Policies {
		if err := tx.Policies().Create(ctx, RetentionPolicy{
			ID:        policy.ID,
			Name:      policy.Name,
			ScopeJSON: policy.ScopeJSON,
			RulesJSON: policy.RulesJSON,
			Enabled:   policy.Enabled,
			CreatedAt: policy.CreatedAt,
			UpdatedAt: policy.UpdatedAt,
		}); err != nil {
			return err
		}
	}
	for _, entry := range data.Access {
		if err := insertAccessLog(ctx, tx.tx, entry); err != nil {
			return err
		}
	}
	return nil
}

func insertMemoryEvent(ctx context.Context, exec executor, event exportMemoryEvent) error {
	var memoryID sql.NullString
	if event.MemoryID != nil {
		memoryID = sql.NullString{String: *event.MemoryID, Valid: true}
	}
	_, err := exec.ExecContext(ctx, `
INSERT INTO memory_events (id, memory_id, event_type, event_payload_json, created_at)
VALUES (?, ?, ?, ?, ?)`,
		event.ID,
		memoryID,
		event.EventType,
		defaultJSON(event.EventPayloadJSON),
		formatTime(event.CreatedAt),
	)
	if err != nil {
		return fmt.Errorf("import memory event: %w", err)
	}
	return nil
}

func insertAccessLog(ctx context.Context, exec executor, entry exportAccessLogEntry) error {
	var tokenID sql.NullString
	if entry.TokenID != nil {
		tokenID = sql.NullString{String: *entry.TokenID, Valid: true}
	}
	_, err := exec.ExecContext(ctx, `
INSERT INTO access_log (id, memory_id, access_type, token_id, created_at)
VALUES (?, ?, ?, ?, ?)`,
		entry.ID,
		entry.MemoryID,
		entry.AccessType,
		tokenID,
		formatTime(entry.CreatedAt),
	)
	if err != nil {
		return fmt.Errorf("import access log: %w", err)
	}
	return nil
}

func (data exportData) counts() ExportRecordCounts {
	return ExportRecordCounts{
		MemoryItems:       len(data.Items),
		MemoryChunks:      len(data.Chunks),
		MemoryEvents:      len(data.Events),
		RetentionPolicies: len(data.Policies),
		AccessLogs:        len(data.Access),
	}
}

func (data exportData) totalRecords() int {
	return len(data.Items) + len(data.Chunks) + len(data.Events) + len(data.Policies) + len(data.Access)
}
