// SPDX-License-Identifier: AGPL-3.0-only

package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/your-org/pamie/internal/audit"
	"github.com/your-org/pamie/internal/auth"
	"github.com/your-org/pamie/internal/resources"
	"github.com/your-org/pamie/internal/tools"
	"github.com/your-org/pamie/internal/util"
)

const (
	protocolVersion = "2025-11-25"
	maxRequestBytes = 1 << 20
)

type ToolRegistry interface {
	List() []tools.Tool
	Call(context.Context, string, json.RawMessage) (tools.CallResult, error)
}

type scopedToolRegistry interface {
	RequiredScope(string) (auth.Scope, bool)
}

type ResourceRegistry interface {
	List() []resources.Resource
	Read(context.Context, string) ([]resources.Content, error)
}

type scopedResourceRegistry interface {
	RequiredScope(string) (auth.Scope, bool)
}

type Options struct {
	Version   string
	Tools     ToolRegistry
	Resources ResourceRegistry
	Logger    *slog.Logger
	Audit     audit.Logger
}

// Handler serves a minimal JSON-RPC MCP endpoint.
type Handler struct {
	version   string
	tools     ToolRegistry
	resources ResourceRegistry
	logger    *slog.Logger
	audit     audit.Logger
}

func NewHandler(opts Options) *Handler {
	if opts.Version == "" {
		opts.Version = "dev"
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	return &Handler{
		version:   opts.Version,
		tools:     opts.Tools,
		resources: opts.Resources,
		logger:    opts.Logger,
		audit:     opts.Audit,
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeRPCError(w, http.StatusBadRequest, nil, errParse, "invalid JSON-RPC request body")
		return
	}

	req, hasID, err := parseRequest(body)
	if err != nil {
		writeRPCError(w, http.StatusBadRequest, nil, errParse, "invalid JSON-RPC request")
		return
	}
	if req.JSONRPC != "2.0" || req.Method == "" {
		if !hasID {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		writeRPCError(w, http.StatusOK, req.ID, errInvalidRequest, "invalid JSON-RPC request")
		return
	}

	result, rpcErr := h.dispatch(r.Context(), req)
	if !hasID {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	if rpcErr != nil {
		writeRPCError(w, http.StatusOK, req.ID, rpcErr.Code, rpcErr.Message)
		return
	}
	writeRPCResult(w, req.ID, result)
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

func parseRequest(body []byte) (rpcRequest, bool, error) {
	var raw map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&raw); err != nil {
		return rpcRequest{}, false, err
	}
	var extra struct{}
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return rpcRequest{}, false, errors.New("unexpected extra JSON value")
	}

	var req rpcRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return rpcRequest{}, false, err
	}
	_, hasID := raw["id"]
	return req, hasID, nil
}

func (h *Handler) dispatch(ctx context.Context, req rpcRequest) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		return map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities": map[string]any{
				"tools":     map[string]any{"listChanged": false},
				"resources": map[string]any{"listChanged": false},
			},
			"serverInfo": map[string]any{
				"name":    "pamie",
				"version": h.version,
			},
			"instructions": resources.UsageInstructions,
		}, nil
	case "notifications/initialized":
		return map[string]any{}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		if h.tools == nil {
			return nil, rpcErr(errInternal, "tools are not configured")
		}
		return map[string]any{"tools": h.tools.List()}, nil
	case "tools/call":
		return h.callTool(ctx, req.Params)
	case "resources/list":
		if h.resources == nil {
			return nil, rpcErr(errInternal, "resources are not configured")
		}
		return map[string]any{"resources": h.resources.List()}, nil
	case "resources/read":
		return h.readResource(ctx, req.Params)
	case "resources/templates/list":
		return map[string]any{"resourceTemplates": []any{}}, nil
	default:
		return nil, rpcErr(errMethodNotFound, "method not found")
	}
}

func (h *Handler) callTool(ctx context.Context, raw json.RawMessage) (any, *rpcError) {
	if h.tools == nil {
		return nil, rpcErr(errInternal, "tools are not configured")
	}
	var rawParams map[string]json.RawMessage
	if err := json.Unmarshal(raw, &rawParams); err != nil {
		return nil, rpcErr(errInvalidParams, "invalid tools/call params")
	}
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if value, ok := rawParams["name"]; ok {
		if err := json.Unmarshal(value, &params.Name); err != nil {
			return nil, rpcErr(errInvalidParams, "invalid tools/call params")
		}
	}
	if value, ok := rawParams["arguments"]; ok {
		params.Arguments = value
	}
	if params.Name == "" {
		return nil, rpcErr(errInvalidParams, "tool name is required")
	}
	if scoped, ok := h.tools.(scopedToolRegistry); ok {
		if scope, known := scoped.RequiredScope(params.Name); known {
			if err := auth.RequireScope(ctx, scope); err != nil {
				h.logAudit(ctx, audit.Event{
					Type:    "mcp_tool_call",
					Outcome: "forbidden",
					TokenID: tokenID(ctx),
					Action:  string(scope),
					Subject: params.Name,
				})
				return nil, rpcErr(errForbidden, "forbidden")
			}
		}
	}
	result, err := h.tools.Call(ctx, params.Name, params.Arguments)
	if err != nil {
		if errors.Is(err, tools.ErrUnknownTool) {
			return nil, rpcErr(errInvalidParams, "unknown tool")
		}
		h.logAudit(ctx, audit.Event{
			Type:    "mcp_tool_call",
			Outcome: "error",
			TokenID: tokenID(ctx),
			Action:  toolAuditAction(params.Name),
			Subject: params.Name,
		})
		h.logger.Error("tool call failed", "tool", params.Name, "error", err)
		return nil, rpcErr(errInternal, "tool call failed")
	}
	outcome := "success"
	if result.IsError {
		outcome = "tool_error"
	}
	h.logAudit(ctx, audit.Event{
		Type:    "mcp_tool_call",
		Outcome: outcome,
		TokenID: tokenID(ctx),
		Action:  toolAuditAction(params.Name),
		Subject: params.Name,
	})
	return result, nil
}

func (h *Handler) readResource(ctx context.Context, raw json.RawMessage) (any, *rpcError) {
	if h.resources == nil {
		return nil, rpcErr(errInternal, "resources are not configured")
	}
	var params struct {
		URI string `json:"uri"`
	}
	if err := decodeParams(raw, &params); err != nil {
		return nil, rpcErr(errInvalidParams, "invalid resources/read params")
	}
	if params.URI == "" {
		return nil, rpcErr(errInvalidParams, "resource uri is required")
	}
	if scoped, ok := h.resources.(scopedResourceRegistry); ok {
		if scope, known := scoped.RequiredScope(params.URI); known {
			if err := auth.RequireScope(ctx, scope); err != nil {
				h.logAudit(ctx, audit.Event{
					Type:    "mcp_resource_read",
					Outcome: "forbidden",
					TokenID: tokenID(ctx),
					Action:  string(scope),
					Subject: params.URI,
				})
				return nil, rpcErr(errForbidden, "forbidden")
			}
		}
	}
	contents, err := h.resources.Read(ctx, params.URI)
	if err != nil {
		if errors.Is(err, resources.ErrUnknownResource) {
			return nil, rpcErr(errInvalidParams, "unknown resource")
		}
		h.logAudit(ctx, audit.Event{
			Type:    "mcp_resource_read",
			Outcome: "error",
			TokenID: tokenID(ctx),
			Action:  "read",
			Subject: params.URI,
		})
		h.logger.Error("resource read failed", "uri", params.URI, "error", err)
		return nil, rpcErr(errInternal, "resource read failed")
	}
	h.logAudit(ctx, audit.Event{
		Type:    "mcp_resource_read",
		Outcome: "success",
		TokenID: tokenID(ctx),
		Action:  "read",
		Subject: params.URI,
	})
	return map[string]any{"contents": contents}, nil
}

func decodeParams(raw json.RawMessage, dst any) error {
	return util.DecodeJSONObject(raw, dst)
}

const (
	errParse          = -32700
	errInvalidRequest = -32600
	errMethodNotFound = -32601
	errInvalidParams  = -32602
	errInternal       = -32603
	errForbidden      = -32003
)

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func rpcErr(code int, message string) *rpcError {
	return &rpcError{Code: code, Message: message}
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

func writeRPCResult(w http.ResponseWriter, id json.RawMessage, result any) {
	writeRPC(w, http.StatusOK, rpcResponse{JSONRPC: "2.0", ID: id, Result: result})
}

func writeRPCError(w http.ResponseWriter, status int, id json.RawMessage, code int, message string) {
	if len(id) == 0 {
		id = []byte("null")
	}
	writeRPC(w, status, rpcResponse{JSONRPC: "2.0", ID: id, Error: rpcErr(code, message)})
}

func writeRPC(w http.ResponseWriter, status int, response rpcResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(response)
}

func (h *Handler) logAudit(ctx context.Context, event audit.Event) {
	audit.Log(ctx, h.audit, event)
}

func tokenID(ctx context.Context) string {
	principal, ok := auth.PrincipalFromContext(ctx)
	if !ok {
		return ""
	}
	return principal.TokenID
}

func toolAuditAction(name string) string {
	switch name {
	case "context_get", "context_search", "context_recent":
		return "read"
	case "context_save", "context_update", "context_pin":
		return "write"
	case "context_delete":
		return "delete"
	case "context_stats":
		return "stats"
	default:
		return "call"
	}
}
