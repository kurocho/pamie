// SPDX-License-Identifier: AGPL-3.0-only

package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/your-org/pamie/internal/audit"
	"github.com/your-org/pamie/internal/auth"
	"github.com/your-org/pamie/internal/db"
	"github.com/your-org/pamie/internal/httpserver"
	"github.com/your-org/pamie/internal/memory"
	"github.com/your-org/pamie/internal/resources"
	"github.com/your-org/pamie/internal/tools"
)

func TestEndpointRequiresAuth(t *testing.T) {
	handler, closeStore := testHTTPHandler(t)
	defer closeStore()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestToolListAndBasicSaveGetSearchFlow(t *testing.T) {
	handler, closeStore := testHTTPHandler(t)
	defer closeStore()

	list := rpc(t, handler, "tools/list", nil)
	toolsValue := list["tools"].([]any)
	if len(toolsValue) != 8 {
		t.Fatalf("tools/list returned %d tools, want 8", len(toolsValue))
	}

	save := rpc(t, handler, "tools/call", map[string]any{
		"name": "context_save",
		"arguments": map[string]any{
			"title":      "Storage note",
			"body":       "SQLite backs Pamie memory",
			"source":     "test",
			"metadata":   map[string]any{"project": "pamie", "priority": 3},
			"importance": 50,
			"pinned":     true,
		},
	})
	memoryID := save["structuredContent"].(map[string]any)["memory"].(map[string]any)["id"].(string)
	if memoryID == "" {
		t.Fatal("context_save returned empty memory id")
	}

	get := rpc(t, handler, "tools/call", map[string]any{
		"name":      "context_get",
		"arguments": map[string]any{"id": memoryID},
	})
	gotID := get["structuredContent"].(map[string]any)["memory"].(map[string]any)["id"].(string)
	if gotID != memoryID {
		t.Fatalf("context_get id = %q, want %q", gotID, memoryID)
	}

	search := rpc(t, handler, "tools/call", map[string]any{
		"name": "context_search",
		"arguments": map[string]any{
			"query":         "SQLite",
			"limit":         5,
			"depth":         "deep",
			"source":        "test",
			"metadata":      map[string]any{"project": "pamie", "priority": 3},
			"created_after": "2026-04-30T00:00:00Z",
			"pinned":        true,
		},
	})
	results := search["structuredContent"].(map[string]any)["results"].([]any)
	if len(results) != 1 {
		t.Fatalf("context_search returned %d result(s), want 1", len(results))
	}
	result := results[0].(map[string]any)
	if result["snippet"] == "" || result["score_details"] == nil {
		t.Fatalf("context_search result = %+v, want snippet and score details", result)
	}
}

func TestInitializeIncludesUsageInstructions(t *testing.T) {
	handler, closeStore := testHTTPHandler(t)
	defer closeStore()

	result := rpc(t, handler, "initialize", map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "test", "version": "test"},
	})

	instructions, ok := result["instructions"].(string)
	if !ok || instructions == "" {
		t.Fatalf("initialize instructions = %#v, want non-empty string", result["instructions"])
	}
	if !bytes.Contains([]byte(instructions), []byte("pamie://guide")) {
		t.Fatalf("initialize instructions = %q, want guide resource mention", instructions)
	}
}

func TestInvalidToolArgumentsReturnToolError(t *testing.T) {
	handler, closeStore := testHTTPHandler(t)
	defer closeStore()

	result := rpc(t, handler, "tools/call", map[string]any{
		"name":      "context_save",
		"arguments": map[string]any{"unknown": true},
	})
	if isError, _ := result["isError"].(bool); !isError {
		t.Fatalf("context_save invalid args result = %+v, want isError=true", result)
	}
}

func TestProtocolErrorMapping(t *testing.T) {
	handler, closeStore := testHTTPHandler(t)
	defer closeStore()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"unknown"}`))
	req.Header.Set("Authorization", "Bearer secret")
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var response map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	errBody := response["error"].(map[string]any)
	if code := int(errBody["code"].(float64)); code != errMethodNotFound {
		t.Fatalf("error code = %d, want %d", code, errMethodNotFound)
	}
}

func TestResourcesListAndRead(t *testing.T) {
	handler, closeStore := testHTTPHandler(t)
	defer closeStore()

	list := rpc(t, handler, "resources/list", nil)
	if len(list["resources"].([]any)) != 3 {
		t.Fatalf("resources/list = %+v", list)
	}

	guide := rpc(t, handler, "resources/read", map[string]any{"uri": resources.GuideURI})
	guideContents := guide["contents"].([]any)
	if len(guideContents) != 1 {
		t.Fatalf("guide contents = %+v", guideContents)
	}
	guideText := guideContents[0].(map[string]any)["text"].(string)
	if !bytes.Contains([]byte(guideText), []byte("Recommended Agent Workflow")) {
		t.Fatalf("guide text = %q, want workflow guidance", guideText)
	}

	read := rpc(t, handler, "resources/read", map[string]any{"uri": resources.StatsURI})
	contents := read["contents"].([]any)
	if len(contents) != 1 {
		t.Fatalf("resources/read contents = %+v", contents)
	}
}

func TestToolScopeEnforcement(t *testing.T) {
	handler, closeStore := testHTTPHandlerWithScopes(t, auth.ScopeSet{auth.ScopeMemoryRead: {}})
	defer closeStore()

	writeResponse := rpcResponseFor(t, handler, "tools/call", map[string]any{
		"name":      "context_save",
		"arguments": map[string]any{"body": "write should be denied"},
	})
	errBody := writeResponse["error"].(map[string]any)
	if code := int(errBody["code"].(float64)); code != errForbidden {
		t.Fatalf("error code = %d, want %d", code, errForbidden)
	}

	deleteResponse := rpcResponseFor(t, handler, "tools/call", map[string]any{
		"name":      "context_delete",
		"arguments": map[string]any{"id": "mem_missing", "confirm": true},
	})
	errBody = deleteResponse["error"].(map[string]any)
	if code := int(errBody["code"].(float64)); code != errForbidden {
		t.Fatalf("delete error code = %d, want %d", code, errForbidden)
	}
}

func TestStatsScopeAllowsStatsOnly(t *testing.T) {
	handler, closeStore := testHTTPHandlerWithScopes(t, auth.ScopeSet{auth.ScopeStatsRead: {}})
	defer closeStore()

	stats := rpc(t, handler, "tools/call", map[string]any{
		"name":      "context_stats",
		"arguments": map[string]any{},
	})
	if stats["structuredContent"] == nil {
		t.Fatalf("context_stats result = %+v, want structured content", stats)
	}

	response := rpcResponseFor(t, handler, "tools/call", map[string]any{
		"name":      "context_recent",
		"arguments": map[string]any{},
	})
	errBody := response["error"].(map[string]any)
	if code := int(errBody["code"].(float64)); code != errForbidden {
		t.Fatalf("context_recent error code = %d, want %d", code, errForbidden)
	}
}

func TestAuditEventsForToolCalls(t *testing.T) {
	auditor := &captureAudit{}
	handler, closeStore := testHTTPHandlerWithOptions(t, auth.AllScopes(), auditor)
	defer closeStore()

	_ = rpc(t, handler, "tools/call", map[string]any{
		"name":      "context_save",
		"arguments": map[string]any{"body": "audited memory"},
	})

	var found bool
	for _, event := range auditor.events {
		if event.Type == "mcp_tool_call" && event.Outcome == "success" && event.TokenID == "test-token" && event.Subject == "context_save" && event.Action == "write" {
			found = true
		}
		for _, value := range event.Fields {
			if value == "secret" {
				t.Fatalf("audit event leaked token: %+v", event)
			}
		}
	}
	if !found {
		t.Fatalf("audit events = %+v, want successful context_save event", auditor.events)
	}
}

func testHTTPHandler(t *testing.T) (http.Handler, func()) {
	return testHTTPHandlerWithOptions(t, auth.AllScopes(), nil)
}

func testHTTPHandlerWithScopes(t *testing.T, scopes auth.ScopeSet) (http.Handler, func()) {
	return testHTTPHandlerWithOptions(t, scopes, nil)
}

func testHTTPHandlerWithOptions(t *testing.T, scopes auth.ScopeSet, auditor audit.Logger) (http.Handler, func()) {
	t.Helper()
	store, err := db.Open(context.Background(), db.Options{Path: filepath.Join(t.TempDir(), "pamie.db")})
	if err != nil {
		t.Fatalf("db.Open() error = %v", err)
	}
	memoryService := memory.NewServiceWithClock(store, func() time.Time {
		return time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	})
	authenticator, err := auth.NewBearerAuthenticatorWithOptions("secret", "test-token", scopes, auditor)
	if err != nil {
		t.Fatalf("NewBearerAuthenticatorWithOptions() error = %v", err)
	}
	mcpHandler := NewHandler(Options{
		Version:   "test",
		Tools:     tools.NewRegistry(memoryService),
		Resources: resources.NewRegistry(memoryService),
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		Audit:     auditor,
	})
	return httpserver.NewHandler(httpserver.HandlerOptions{
			Authenticator: authenticator,
			MCPHandler:    mcpHandler,
			Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		}), func() {
			if err := store.Close(); err != nil {
				t.Fatalf("store.Close() error = %v", err)
			}
		}
}

func rpc(t *testing.T, handler http.Handler, method string, params any) map[string]any {
	t.Helper()
	response := rpcResponseFor(t, handler, method, params)
	if response["error"] != nil {
		t.Fatalf("%s returned error: %+v", method, response["error"])
	}
	return response["result"].(map[string]any)
}

func rpcResponseFor(t *testing.T, handler http.Handler, method string, params any) map[string]any {
	t.Helper()
	request := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
	}
	if params != nil {
		request["params"] = params
	}
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%s status = %d, want %d; body = %q", method, rec.Code, http.StatusOK, rec.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response["error"] != nil {
		return response
	}
	if response["result"] == nil {
		t.Fatalf("%s returned no result or error: %+v", method, response)
	}
	return response
}

type captureAudit struct {
	events []audit.Event
}

func (c *captureAudit) Log(_ context.Context, event audit.Event) {
	c.events = append(c.events, event)
}
