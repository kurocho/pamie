// SPDX-License-Identifier: AGPL-3.0-only

package acceptance

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

	"github.com/your-org/pamie/internal/auth"
	"github.com/your-org/pamie/internal/db"
	"github.com/your-org/pamie/internal/httpserver"
	"github.com/your-org/pamie/internal/mcp"
	"github.com/your-org/pamie/internal/memory"
	"github.com/your-org/pamie/internal/resources"
	"github.com/your-org/pamie/internal/tools"
)

const acceptanceToken = "acceptance-secret"

func TestHTTPAcceptanceEndpointsAndAuth(t *testing.T) {
	server := newAcceptanceServer(t)
	client := server.Client()

	health := getJSON(t, client, server.URL+"/health", http.StatusOK)
	if health["service"] != "pamie" || health["status"] != "ok" {
		t.Fatalf("/health = %+v, want pamie ok", health)
	}

	ready := getJSON(t, client, server.URL+"/ready", http.StatusOK)
	if ready["status"] != "ready" {
		t.Fatalf("/ready = %+v, want ready", ready)
	}

	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	for _, tt := range []struct {
		name   string
		header string
		want   int
	}{
		{name: "missing auth", want: http.StatusUnauthorized},
		{name: "malformed bearer header", header: "Bearer " + acceptanceToken + " extra", want: http.StatusUnauthorized},
		{name: "wrong auth scheme", header: "Basic " + acceptanceToken, want: http.StatusUnauthorized},
	} {
		t.Run(tt.name, func(t *testing.T) {
			status, responseBody := postMCP(t, client, server.URL, tt.header, body)
			if status != tt.want {
				t.Fatalf("POST /mcp status = %d, want %d; body = %q", status, tt.want, responseBody)
			}
		})
	}

	status, responseBody := postMCP(t, client, server.URL, "Bearer "+acceptanceToken, []byte(`{`))
	if status != http.StatusBadRequest {
		t.Fatalf("authenticated malformed JSON status = %d, want %d; body = %q", status, http.StatusBadRequest, responseBody)
	}

	result := rpc(t, client, server.URL, "tools/list", nil)
	toolsValue := asSlice(t, result["tools"])
	if len(toolsValue) != 8 {
		t.Fatalf("tools/list returned %d tools, want 8", len(toolsValue))
	}
}

func TestMCPAcceptanceMemoryToolFlow(t *testing.T) {
	server := newAcceptanceServer(t)
	client := server.Client()

	save := callTool(t, client, server.URL, "context_save", map[string]any{
		"title":      "Acceptance memory",
		"body":       "alpha acceptance memory covers MCP tool flows",
		"source":     "acceptance",
		"metadata":   map[string]any{"project": "pamie", "stage": "acceptance", "priority": 10},
		"tier":       "working",
		"importance": 42,
		"pinned":     true,
	})
	saved := asMap(t, structured(t, save)["memory"])
	memoryID, ok := saved["id"].(string)
	if !ok || memoryID == "" {
		t.Fatalf("context_save memory = %+v, want id", saved)
	}

	get := callTool(t, client, server.URL, "context_get", map[string]any{"id": memoryID})
	got := structured(t, get)
	if asMap(t, got["memory"])["id"] != memoryID {
		t.Fatalf("context_get = %+v, want memory %s", got, memoryID)
	}
	if chunks := asSlice(t, got["chunks"]); len(chunks) != 1 {
		t.Fatalf("context_get chunks = %+v, want one chunk", chunks)
	}

	search := callTool(t, client, server.URL, "context_search", map[string]any{
		"query":    "alpha acceptance",
		"source":   "acceptance",
		"metadata": map[string]any{"project": "pamie", "stage": "acceptance", "priority": 10},
		"pinned":   true,
		"limit":    5,
		"depth":    "deep",
	})
	results := asSlice(t, structured(t, search)["results"])
	if len(results) != 1 || asMap(t, asMap(t, results[0])["memory"])["id"] != memoryID {
		t.Fatalf("context_search results = %+v, want saved memory", results)
	}

	update := callTool(t, client, server.URL, "context_update", map[string]any{
		"id":         memoryID,
		"body":       "beta acceptance memory verifies update replacement",
		"metadata":   map[string]any{"project": "pamie", "stage": "updated"},
		"tier":       "hot",
		"importance": 77,
	})
	updated := asMap(t, structured(t, update)["memory"])
	if updated["body"] != "beta acceptance memory verifies update replacement" || updated["tier"] != "hot" {
		t.Fatalf("context_update memory = %+v, want updated body and hot tier", updated)
	}

	unpin := callTool(t, client, server.URL, "context_pin", map[string]any{"id": memoryID, "pinned": false})
	if asMap(t, structured(t, unpin)["memory"])["pinned"] != false {
		t.Fatalf("context_pin false = %+v", unpin)
	}
	pin := callTool(t, client, server.URL, "context_pin", map[string]any{"id": memoryID})
	if asMap(t, structured(t, pin)["memory"])["pinned"] != true {
		t.Fatalf("context_pin default = %+v", pin)
	}

	recent := callTool(t, client, server.URL, "context_recent", map[string]any{"limit": 5})
	recentMemories := asSlice(t, structured(t, recent)["memories"])
	if len(recentMemories) != 1 || asMap(t, recentMemories[0])["id"] != memoryID {
		t.Fatalf("context_recent memories = %+v, want saved memory", recentMemories)
	}

	stats := asMap(t, structured(t, callTool(t, client, server.URL, "context_stats", map[string]any{}))["stats"])
	if stats["total"] != float64(1) || stats["active"] != float64(1) || stats["pinned"] != float64(1) {
		t.Fatalf("context_stats = %+v, want one active pinned memory", stats)
	}

	deleted := callTool(t, client, server.URL, "context_delete", map[string]any{"id": memoryID, "confirm": true})
	if asMap(t, structured(t, deleted)["memory"])["deleted_at"] == nil {
		t.Fatalf("context_delete = %+v, want deleted_at", deleted)
	}

	activeSearch := callTool(t, client, server.URL, "context_search", map[string]any{
		"query": "beta acceptance",
		"limit": 5,
	})
	if results := asSlice(t, structured(t, activeSearch)["results"]); len(results) != 0 {
		t.Fatalf("active context_search after delete = %+v, want no results", results)
	}

	deletedSearch := callTool(t, client, server.URL, "context_search", map[string]any{
		"query":           "beta acceptance",
		"include_deleted": true,
		"limit":           5,
	})
	if results := asSlice(t, structured(t, deletedSearch)["results"]); len(results) != 1 {
		t.Fatalf("deleted context_search = %+v, want one deleted result", results)
	}

	stats = asMap(t, structured(t, callTool(t, client, server.URL, "context_stats", map[string]any{}))["stats"])
	if stats["total"] != float64(1) || stats["active"] != float64(0) || stats["deleted"] != float64(1) {
		t.Fatalf("context_stats after delete = %+v, want one deleted memory", stats)
	}
}

func newAcceptanceServer(t *testing.T) *httptest.Server {
	t.Helper()
	store, err := db.Open(context.Background(), db.Options{Path: filepath.Join(t.TempDir(), "pamie.db")})
	if err != nil {
		t.Fatalf("db.Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("store.Close() error = %v", err)
		}
	})

	service := memory.NewServiceWithClock(store, func() time.Time {
		return time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)
	})
	authenticator, err := auth.NewBearerAuthenticatorWithOptions(acceptanceToken, "acceptance-token", auth.AllScopes(), nil)
	if err != nil {
		t.Fatalf("NewBearerAuthenticatorWithOptions() error = %v", err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mcpHandler := mcp.NewHandler(mcp.Options{
		Version:   "acceptance",
		Tools:     tools.NewRegistry(service),
		Resources: resources.NewRegistry(service),
		Logger:    logger,
	})
	server := httptest.NewServer(httpserver.NewHandler(httpserver.HandlerOptions{
		Authenticator: authenticator,
		MCPHandler:    mcpHandler,
		Logger:        logger,
		ReadinessChecks: []httpserver.ReadinessCheck{
			{Name: "sqlite", Check: store.Ping},
		},
	}))
	t.Cleanup(server.Close)
	return server
}

func getJSON(t *testing.T, client *http.Client, url string, wantStatus int) map[string]any {
	t.Helper()
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET %s error = %v", url, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if resp.StatusCode != wantStatus {
		t.Fatalf("GET %s status = %d, want %d; body = %q", url, resp.StatusCode, wantStatus, body)
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode JSON response: %v; body = %q", err, body)
	}
	return decoded
}

func callTool(t *testing.T, client *http.Client, baseURL string, name string, arguments map[string]any) map[string]any {
	t.Helper()
	return rpc(t, client, baseURL, "tools/call", map[string]any{
		"name":      name,
		"arguments": arguments,
	})
}

func rpc(t *testing.T, client *http.Client, baseURL string, method string, params any) map[string]any {
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
	status, responseBody := postMCP(t, client, baseURL, "Bearer "+acceptanceToken, body)
	if status != http.StatusOK {
		t.Fatalf("%s status = %d, want %d; body = %q", method, status, http.StatusOK, responseBody)
	}
	var response map[string]any
	if err := json.Unmarshal(responseBody, &response); err != nil {
		t.Fatalf("decode RPC response: %v; body = %q", err, responseBody)
	}
	if response["error"] != nil {
		t.Fatalf("%s returned error: %+v", method, response["error"])
	}
	return asMap(t, response["result"])
}

func postMCP(t *testing.T, client *http.Client, baseURL string, authorization string, body []byte) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, baseURL+"/mcp", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST /mcp error = %v", err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read /mcp response body: %v", err)
	}
	return resp.StatusCode, responseBody
}

func structured(t *testing.T, result map[string]any) map[string]any {
	t.Helper()
	return asMap(t, result["structuredContent"])
}

func asMap(t *testing.T, value any) map[string]any {
	t.Helper()
	typed, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("value = %#v, want object", value)
	}
	return typed
}

func asSlice(t *testing.T, value any) []any {
	t.Helper()
	typed, ok := value.([]any)
	if !ok {
		t.Fatalf("value = %#v, want array", value)
	}
	return typed
}
