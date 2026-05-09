// SPDX-License-Identifier: AGPL-3.0-only

package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/your-org/pamie/internal/auth"
	"github.com/your-org/pamie/internal/memory"
	"github.com/your-org/pamie/internal/util"
)

var ErrUnknownTool = errors.New("unknown tool")

// MemoryService is the memory behavior required by MCP tools.
type MemoryService interface {
	Save(context.Context, memory.SaveInput) (memory.Memory, error)
	Get(context.Context, string) (memory.MemoryWithChunks, error)
	Search(context.Context, memory.SearchInput) ([]memory.SearchHit, error)
	Update(context.Context, memory.UpdateInput) (memory.Memory, error)
	Delete(context.Context, memory.DeleteInput) (memory.Memory, error)
	Pin(context.Context, memory.PinInput) (memory.Memory, error)
	Recent(context.Context, memory.RecentInput) ([]memory.Memory, error)
	Stats(context.Context) (memory.Stats, error)
}

// Registry contains all MCP tool definitions and handlers.
type Registry struct {
	memory MemoryService
}

// NewRegistry creates a tool registry.
func NewRegistry(memoryService MemoryService) *Registry {
	return &Registry{memory: memoryService}
}

type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type Content struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type CallResult struct {
	Content           []Content `json:"content"`
	StructuredContent any       `json:"structuredContent,omitempty"`
	IsError           bool      `json:"isError,omitempty"`
}

func (r *Registry) List() []Tool {
	return []Tool{
		{Name: "context_save", Description: "Store a new durable memory. The full body is indexed by FTS5; vector search embeds only the title and explicit keywords.", InputSchema: schema(
			map[string]any{
				"title":      stringProp("Short optional title."),
				"body":       stringProp("Memory body to store."),
				"keywords":   stringArrayProp("Explicit semantic retrieval keywords for title/keywords vector indexing. Include people names, teams, projects, aliases, technologies, decisions, ticket IDs, dates, error messages, and domain-specific terms that may not appear in the title."),
				"source":     stringProp("Optional source or agent name."),
				"metadata":   objectProp("Optional JSON metadata object."),
				"tier":       enumProp("Initial memory tier.", []string{"working", "hot", "warm", "cold", "archive"}),
				"importance": integerProp("Importance from 0 to 100.", 0, 100),
				"pinned":     boolProp("Whether the memory should be pinned."),
			},
			[]string{"body"},
		)},
		{Name: "context_get", Description: "Retrieve a memory by ID.", InputSchema: schema(
			map[string]any{"id": stringProp("Memory ID.")},
			[]string{"id"},
		)},
		{Name: "context_search", Description: "Search memories with FTS5 keyword search, safe filters, snippets, and ranking details.", InputSchema: schema(
			map[string]any{
				"query":           stringProp("Keyword query."),
				"tier":            enumProp("Optional tier filter.", []string{"working", "hot", "warm", "cold", "archive"}),
				"pinned":          boolProp("Optional pinned filter."),
				"metadata":        objectProp("Optional metadata equality filters."),
				"source":          stringProp("Optional source filter."),
				"created_after":   stringProp("Optional RFC3339 lower creation time bound."),
				"created_before":  stringProp("Optional RFC3339 upper creation time bound."),
				"updated_after":   stringProp("Optional RFC3339 lower update time bound."),
				"updated_before":  stringProp("Optional RFC3339 upper update time bound."),
				"depth":           enumProp("Search depth controls candidate breadth.", []string{"shallow", "standard", "deep"}),
				"include_deleted": boolProp("Include soft-deleted memories."),
				"limit":           integerProp("Maximum results, capped at 100.", 1, 100),
			},
			[]string{"query"},
		)},
		{Name: "context_update", Description: "Update mutable memory fields. Providing keywords replaces the full keyword list used for title/keywords vector indexing.", InputSchema: schema(
			map[string]any{
				"id":         stringProp("Memory ID."),
				"title":      stringProp("New title."),
				"body":       stringProp("New body."),
				"keywords":   stringArrayProp("Replacement semantic retrieval keywords. The full body remains searchable through FTS5; embeddings use only title and keywords."),
				"source":     stringProp("New source."),
				"metadata":   objectProp("Replacement metadata object."),
				"tier":       enumProp("New memory tier.", []string{"working", "hot", "warm", "cold", "archive"}),
				"importance": integerProp("Importance from 0 to 100.", 0, 100),
				"pinned":     boolProp("Pinned state."),
			},
			[]string{"id"},
		)},
		{Name: "context_delete", Description: "Soft-delete a memory. Requires confirm=true.", InputSchema: schema(
			map[string]any{
				"id":      stringProp("Memory ID."),
				"confirm": boolProp("Must be true to soft-delete the memory."),
			},
			[]string{"id", "confirm"},
		)},
		{Name: "context_pin", Description: "Pin or unpin a memory.", InputSchema: schema(
			map[string]any{
				"id":     stringProp("Memory ID."),
				"pinned": boolProp("Pinned state. Defaults to true when omitted."),
			},
			[]string{"id"},
		)},
		{Name: "context_recent", Description: "List recently updated memories.", InputSchema: schema(
			map[string]any{
				"include_deleted": boolProp("Include soft-deleted memories."),
				"limit":           integerProp("Maximum results, capped at 100.", 1, 100),
			},
			nil,
		)},
		{Name: "context_stats", Description: "Return aggregate memory statistics.", InputSchema: schema(map[string]any{}, nil)},
	}
}

// RequiredScope returns the scope required to call name.
func (r *Registry) RequiredScope(name string) (auth.Scope, bool) {
	switch name {
	case "context_get", "context_search", "context_recent":
		return auth.ScopeMemoryRead, true
	case "context_save", "context_update", "context_pin":
		return auth.ScopeMemoryWrite, true
	case "context_delete":
		return auth.ScopeMemoryDelete, true
	case "context_stats":
		return auth.ScopeStatsRead, true
	default:
		return "", false
	}
}

func (r *Registry) Call(ctx context.Context, name string, arguments json.RawMessage) (CallResult, error) {
	switch name {
	case "context_save":
		return r.contextSave(ctx, arguments), nil
	case "context_get":
		return r.contextGet(ctx, arguments), nil
	case "context_search":
		return r.contextSearch(ctx, arguments), nil
	case "context_update":
		return r.contextUpdate(ctx, arguments), nil
	case "context_delete":
		return r.contextDelete(ctx, arguments), nil
	case "context_pin":
		return r.contextPin(ctx, arguments), nil
	case "context_recent":
		return r.contextRecent(ctx, arguments), nil
	case "context_stats":
		return r.contextStats(ctx, arguments), nil
	default:
		return CallResult{}, fmt.Errorf("%w: %s", ErrUnknownTool, name)
	}
}

func (r *Registry) contextSave(ctx context.Context, raw json.RawMessage) CallResult {
	var args saveArgs
	if err := decode(raw, &args); err != nil {
		return toolError("invalid arguments", err)
	}
	importance := 0
	if args.Importance != nil {
		importance = *args.Importance
	}
	memoryItem, err := r.memory.Save(ctx, memory.SaveInput{
		Title:      args.Title,
		Body:       args.Body,
		Keywords:   args.Keywords,
		Source:     args.Source,
		Metadata:   args.Metadata,
		Tier:       args.Tier,
		Importance: importance,
		Pinned:     args.Pinned,
	})
	if err != nil {
		return serviceError(err)
	}
	return structuredResult("memory saved: "+memoryItem.ID, map[string]any{"memory": memoryItem})
}

func (r *Registry) contextGet(ctx context.Context, raw json.RawMessage) CallResult {
	var args idArgs
	if err := decode(raw, &args); err != nil {
		return toolError("invalid arguments", err)
	}
	result, err := r.memory.Get(ctx, args.ID)
	if err != nil {
		return serviceError(err)
	}
	return structuredResult("memory retrieved: "+result.Memory.ID, result)
}

func (r *Registry) contextSearch(ctx context.Context, raw json.RawMessage) CallResult {
	var args searchArgs
	if err := decode(raw, &args); err != nil {
		return toolError("invalid arguments", err)
	}
	createdAfter, err := parseOptionalTime(args.CreatedAfter)
	if err != nil {
		return toolError("invalid arguments", err)
	}
	createdBefore, err := parseOptionalTime(args.CreatedBefore)
	if err != nil {
		return toolError("invalid arguments", err)
	}
	updatedAfter, err := parseOptionalTime(args.UpdatedAfter)
	if err != nil {
		return toolError("invalid arguments", err)
	}
	updatedBefore, err := parseOptionalTime(args.UpdatedBefore)
	if err != nil {
		return toolError("invalid arguments", err)
	}
	hits, err := r.memory.Search(ctx, memory.SearchInput{
		Query:          args.Query,
		Tier:           args.Tier,
		Pinned:         args.Pinned,
		IncludeDeleted: args.IncludeDeleted,
		Limit:          args.Limit,
		Depth:          args.Depth,
		Metadata:       args.Metadata,
		Source:         args.Source,
		CreatedAfter:   createdAfter,
		CreatedBefore:  createdBefore,
		UpdatedAfter:   updatedAfter,
		UpdatedBefore:  updatedBefore,
	})
	if err != nil {
		return serviceError(err)
	}
	return structuredResult(fmt.Sprintf("search returned %d result(s)", len(hits)), map[string]any{"results": hits})
}

func (r *Registry) contextUpdate(ctx context.Context, raw json.RawMessage) CallResult {
	var args updateArgs
	if err := decode(raw, &args); err != nil {
		return toolError("invalid arguments", err)
	}
	if !args.hasUpdate() {
		return toolError("invalid arguments", errors.New("at least one mutable field is required"))
	}
	memoryItem, err := r.memory.Update(ctx, memory.UpdateInput{
		ID:         args.ID,
		Title:      args.Title,
		Body:       args.Body,
		Keywords:   args.Keywords,
		Source:     args.Source,
		Metadata:   args.Metadata,
		Tier:       args.Tier,
		Importance: args.Importance,
		Pinned:     args.Pinned,
	})
	if err != nil {
		return serviceError(err)
	}
	return structuredResult("memory updated: "+memoryItem.ID, map[string]any{"memory": memoryItem})
}

func (r *Registry) contextDelete(ctx context.Context, raw json.RawMessage) CallResult {
	var args deleteArgs
	if err := decode(raw, &args); err != nil {
		return toolError("invalid arguments", err)
	}
	memoryItem, err := r.memory.Delete(ctx, memory.DeleteInput{ID: args.ID, Confirm: args.Confirm})
	if err != nil {
		return serviceError(err)
	}
	return structuredResult("memory soft-deleted: "+memoryItem.ID, map[string]any{"memory": memoryItem})
}

func (r *Registry) contextPin(ctx context.Context, raw json.RawMessage) CallResult {
	var args pinArgs
	if err := decode(raw, &args); err != nil {
		return toolError("invalid arguments", err)
	}
	pinned := true
	if args.Pinned != nil {
		pinned = *args.Pinned
	}
	memoryItem, err := r.memory.Pin(ctx, memory.PinInput{ID: args.ID, Pinned: pinned})
	if err != nil {
		return serviceError(err)
	}
	return structuredResult("memory pin state updated: "+memoryItem.ID, map[string]any{"memory": memoryItem})
}

func (r *Registry) contextRecent(ctx context.Context, raw json.RawMessage) CallResult {
	var args recentArgs
	if err := decode(raw, &args); err != nil {
		return toolError("invalid arguments", err)
	}
	memories, err := r.memory.Recent(ctx, memory.RecentInput{IncludeDeleted: args.IncludeDeleted, Limit: args.Limit})
	if err != nil {
		return serviceError(err)
	}
	return structuredResult(fmt.Sprintf("recent returned %d memory item(s)", len(memories)), map[string]any{"memories": memories})
}

func (r *Registry) contextStats(ctx context.Context, raw json.RawMessage) CallResult {
	var args struct{}
	if err := decode(raw, &args); err != nil {
		return toolError("invalid arguments", err)
	}
	stats, err := r.memory.Stats(ctx)
	if err != nil {
		return serviceError(err)
	}
	return structuredResult("memory stats retrieved", map[string]any{"stats": stats})
}

type saveArgs struct {
	Title      string         `json:"title"`
	Body       string         `json:"body"`
	Keywords   []string       `json:"keywords"`
	Source     string         `json:"source"`
	Metadata   map[string]any `json:"metadata"`
	Tier       string         `json:"tier"`
	Importance *int           `json:"importance"`
	Pinned     bool           `json:"pinned"`
}

type idArgs struct {
	ID string `json:"id"`
}

type searchArgs struct {
	Query          string         `json:"query"`
	Tier           *string        `json:"tier"`
	Pinned         *bool          `json:"pinned"`
	Metadata       map[string]any `json:"metadata"`
	Source         *string        `json:"source"`
	CreatedAfter   *string        `json:"created_after"`
	CreatedBefore  *string        `json:"created_before"`
	UpdatedAfter   *string        `json:"updated_after"`
	UpdatedBefore  *string        `json:"updated_before"`
	Depth          string         `json:"depth"`
	IncludeDeleted bool           `json:"include_deleted"`
	Limit          int            `json:"limit"`
}

type updateArgs struct {
	ID         string          `json:"id"`
	Title      *string         `json:"title"`
	Body       *string         `json:"body"`
	Keywords   *[]string       `json:"keywords"`
	Source     *string         `json:"source"`
	Metadata   *map[string]any `json:"metadata"`
	Tier       *string         `json:"tier"`
	Importance *int            `json:"importance"`
	Pinned     *bool           `json:"pinned"`
}

func (a updateArgs) hasUpdate() bool {
	return a.Title != nil || a.Body != nil || a.Keywords != nil || a.Source != nil || a.Metadata != nil || a.Tier != nil || a.Importance != nil || a.Pinned != nil
}

type deleteArgs struct {
	ID      string `json:"id"`
	Confirm bool   `json:"confirm"`
}

type pinArgs struct {
	ID     string `json:"id"`
	Pinned *bool  `json:"pinned"`
}

type recentArgs struct {
	IncludeDeleted bool `json:"include_deleted"`
	Limit          int  `json:"limit"`
}

func decode(raw json.RawMessage, dst any) error {
	return util.DecodeJSONObject(raw, dst)
}

func serviceError(err error) CallResult {
	switch {
	case errors.Is(err, memory.ErrInvalid):
		return toolError("invalid arguments", err)
	case errors.Is(err, memory.ErrNotFound):
		return toolError("memory not found", err)
	default:
		return toolError("tool execution failed", errors.New("internal error"))
	}
}

func toolError(message string, err error) CallResult {
	text := message
	if err != nil {
		text = message + ": " + err.Error()
	}
	return CallResult{
		IsError: true,
		Content: []Content{{
			Type: "text",
			Text: text,
		}},
	}
}

func structuredResult(text string, structured any) CallResult {
	return CallResult{
		Content: []Content{{
			Type: "text",
			Text: text,
		}},
		StructuredContent: structured,
	}
}

func schema(properties map[string]any, required []string) map[string]any {
	if required == nil {
		required = []string{}
	}
	return map[string]any{
		"type":       "object",
		"properties": properties,
		"required":   required,
	}
}

func stringProp(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func boolProp(description string) map[string]any {
	return map[string]any{"type": "boolean", "description": description}
}

func objectProp(description string) map[string]any {
	return map[string]any{"type": "object", "description": description, "additionalProperties": true}
}

func stringArrayProp(description string) map[string]any {
	return map[string]any{"type": "array", "description": description, "items": map[string]any{"type": "string"}}
}

func integerProp(description string, min int, max int) map[string]any {
	return map[string]any{"type": "integer", "description": description, "minimum": min, "maximum": max}
}

func enumProp(description string, values []string) map[string]any {
	return map[string]any{"type": "string", "description": description, "enum": values}
}

func parseOptionalTime(value *string) (*time.Time, error) {
	if value == nil || *value == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, *value)
	if err != nil {
		return nil, fmt.Errorf("time %q must be RFC3339", *value)
	}
	parsed = parsed.UTC()
	return &parsed, nil
}
