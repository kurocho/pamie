// SPDX-License-Identifier: AGPL-3.0-only

package db

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/your-org/pamie/internal/search"
)

var (
	ErrInvalid  = errors.New("invalid storage record")
	ErrNotFound = errors.New("record not found")
)

const (
	VectorBackendAuto       = "auto"
	VectorBackendSQLiteJSON = "sqlite-json"
	VectorBackendSQLiteVec  = "sqlite-vec"
)

// Tier identifies the lifecycle tier for a memory item.
type Tier string

const (
	TierWorking Tier = "working"
	TierHot     Tier = "hot"
	TierWarm    Tier = "warm"
	TierCold    Tier = "cold"
	TierArchive Tier = "archive"
)

// Valid reports whether t is a known memory tier.
func (t Tier) Valid() bool {
	switch t {
	case TierWorking, TierHot, TierWarm, TierCold, TierArchive:
		return true
	default:
		return false
	}
}

// MemoryItem is the canonical durable memory row.
type MemoryItem struct {
	ID             string
	Title          string
	Body           string
	Source         string
	MetadataJSON   string
	Tier           Tier
	Importance     int
	Pinned         bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
	LastAccessedAt *time.Time
	ArchivedAt     *time.Time
	DeletedAt      *time.Time
}

// MemoryChunk is a searchable chunk derived from a memory item.
type MemoryChunk struct {
	ID         string
	MemoryID   string
	ChunkIndex int
	Content    string
	CreatedAt  time.Time
}

// VectorMetadata describes one local vector index configuration.
type VectorMetadata struct {
	Provider       string
	Model          string
	Dimensions     int
	Backend        string
	DistanceMetric string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// MemoryEmbedding stores a vector embedding for one memory chunk.
type MemoryEmbedding struct {
	ChunkID       string
	MemoryID      string
	Provider      string
	Model         string
	Dimensions    int
	EmbeddingJSON string
	ContentHash   string
	VectorRowID   int64
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// EmbeddingTarget identifies the vector index used for storage or search.
type EmbeddingTarget struct {
	Provider   string
	Model      string
	Dimensions int
}

// EmbeddingBackfillOptions selects chunks missing embeddings for one target.
type EmbeddingBackfillOptions struct {
	Target EmbeddingTarget
	Limit  int
	Force  bool
}

// MemoryEvent records a durable memory mutation or lifecycle event.
type MemoryEvent struct {
	ID               int64
	MemoryID         string
	EventType        string
	EventPayloadJSON string
	CreatedAt        time.Time
}

// RetentionPolicy is an operator-defined lifecycle policy.
type RetentionPolicy struct {
	ID        string
	Name      string
	ScopeJSON string
	RulesJSON string
	Enabled   bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// AccessLogEntry records a memory access for future ranking and promotion.
type AccessLogEntry struct {
	ID         int64
	MemoryID   string
	AccessType string
	TokenID    string
	CreatedAt  time.Time
}

// MemoryUpdate contains partial changes to a memory item.
type MemoryUpdate struct {
	Title           *string
	Body            *string
	Source          *string
	MetadataJSON    *string
	Tier            *Tier
	Importance      *int
	Pinned          *bool
	LastAccessedAt  *time.Time
	ArchivedAt      *time.Time
	DeletedAt       *time.Time
	ClearArchivedAt bool
	UpdatedAt       time.Time
}

// LifecycleListOptions describes supported lifecycle candidate filters.
type LifecycleListOptions struct {
	Limit int
}

// SearchOptions describes the supported safe search filters.
type SearchOptions struct {
	Query          string
	Tier           *Tier
	Pinned         *bool
	IncludeDeleted bool
	Limit          int
	Depth          search.Depth
	Metadata       map[string]any
	Source         *string
	CreatedAfter   *time.Time
	CreatedBefore  *time.Time
	UpdatedAfter   *time.Time
	UpdatedBefore  *time.Time
	Now            time.Time
	Vector         *VectorSearchOptions
}

// VectorSearchOptions enables optional local vector candidates for search.
type VectorSearchOptions struct {
	Target         EmbeddingTarget
	QueryEmbedding []float64
}

// SearchResult is an FTS-backed or hybrid-ranked memory search result.
type SearchResult struct {
	Item             MemoryItem
	MemoryID         string
	ChunkID          string
	Snippet          string
	FTSRank          float64
	KeywordMatched   bool
	AccessCount      int
	VectorSimilarity *float64
	Score            search.Score
}

// RecentOptions describes supported recent-memory filters.
type RecentOptions struct {
	IncludeDeleted bool
	Limit          int
}

// Stats contains aggregate memory counts.
type Stats struct {
	Total        int `json:"total"`
	Active       int `json:"active"`
	Deleted      int `json:"deleted"`
	Pinned       int `json:"pinned"`
	Working      int `json:"working"`
	Hot          int `json:"hot"`
	Warm         int `json:"warm"`
	Cold         int `json:"cold"`
	Archive      int `json:"archive"`
	AccessEvents int `json:"access_events"`
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, err
	}
	return parsed.UTC(), nil
}

func nullableTime(value *time.Time) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: formatTime(*value), Valid: true}
}

func parseNullableTime(value sql.NullString) (*time.Time, error) {
	if !value.Valid {
		return nil, nil
	}
	parsed, err := parseTime(value.String)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func intBool(value int) bool {
	return value != 0
}

func requireID(kind, id string) error {
	if id == "" {
		return fmt.Errorf("%w: %s id must not be empty", ErrInvalid, kind)
	}
	return nil
}
