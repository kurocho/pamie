// SPDX-License-Identifier: AGPL-3.0-only

package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/your-org/pamie/internal/audit"
)

func TestBearerAuthenticatorMiddleware(t *testing.T) {
	tests := []struct {
		name       string
		token      string
		header     string
		wantStatus int
	}{
		{name: "not configured", token: "", header: "Bearer secret", wantStatus: http.StatusServiceUnavailable},
		{name: "missing", token: "secret", header: "", wantStatus: http.StatusUnauthorized},
		{name: "malformed scheme", token: "secret", header: "Basic secret", wantStatus: http.StatusUnauthorized},
		{name: "malformed token", token: "secret", header: "Bearer secret extra", wantStatus: http.StatusUnauthorized},
		{name: "invalid", token: "secret", header: "Bearer wrong", wantStatus: http.StatusUnauthorized},
		{name: "valid", token: "secret", header: "Bearer secret", wantStatus: http.StatusNoContent},
		{name: "case insensitive scheme", token: "secret", header: "bearer secret", wantStatus: http.StatusNoContent},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authenticator, err := NewBearerAuthenticator(tt.token)
			if err != nil {
				t.Fatalf("NewBearerAuthenticator() error = %v", err)
			}
			handler := authenticator.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			}))

			req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body = %q", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestNewBearerAuthenticatorRejectsWhitespaceToken(t *testing.T) {
	for _, token := range []string{" secret", "secret ", "sec ret"} {
		t.Run(token, func(t *testing.T) {
			if _, err := NewBearerAuthenticator(token); err == nil {
				t.Fatal("NewBearerAuthenticator() error = nil, want error")
			}
		})
	}
}

func TestBearerAuthenticatorAttachesPrincipalAndAuditsTokenID(t *testing.T) {
	auditor := &captureAudit{}
	scopes, err := ParseScopes("memory:read,stats:read")
	if err != nil {
		t.Fatalf("ParseScopes() error = %v", err)
	}
	authenticator, err := NewBearerAuthenticatorWithOptions("secret", "agent-a", scopes, auditor)
	if err != nil {
		t.Fatalf("NewBearerAuthenticatorWithOptions() error = %v", err)
	}
	handler := authenticator.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := PrincipalFromContext(r.Context())
		if !ok {
			t.Fatal("principal missing from request context")
		}
		if principal.TokenID != "agent-a" || !principal.Scopes.Has(ScopeMemoryRead) || principal.Scopes.Has(ScopeMemoryWrite) {
			t.Fatalf("principal = %+v", principal)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if len(auditor.events) != 1 {
		t.Fatalf("audit events = %+v, want one event", auditor.events)
	}
	event := auditor.events[0]
	if event.Type != "auth" || event.Outcome != "success" || event.TokenID != "agent-a" {
		t.Fatalf("audit event = %+v", event)
	}
	for _, value := range event.Fields {
		if value == "secret" {
			t.Fatalf("audit event leaked token: %+v", event)
		}
	}
}

func TestScopes(t *testing.T) {
	scopes, err := ParseScopes("memory:read stats:read")
	if err != nil {
		t.Fatalf("ParseScopes() error = %v", err)
	}
	if !scopes.Has(ScopeMemoryRead) || !scopes.Has(ScopeStatsRead) {
		t.Fatalf("scopes = %+v, want read and stats", scopes)
	}
	if scopes.Has(ScopeMemoryDelete) {
		t.Fatalf("scopes = %+v, should not allow delete", scopes)
	}
	backup, err := ParseScopes("backup:read")
	if err != nil {
		t.Fatalf("ParseScopes(backup) error = %v", err)
	}
	if !backup.Has(ScopeBackupRead) || backup.Has(ScopeMemoryRead) {
		t.Fatalf("backup scopes = %+v, want backup only", backup)
	}

	admin := ScopeSet{ScopeMemoryAdmin: {}}
	if !admin.Has(ScopeMemoryRead) || !admin.Has(ScopeMemoryWrite) || !admin.Has(ScopeMemoryDelete) || !admin.Has(ScopeBackupRead) || !admin.Has(ScopeStatsRead) {
		t.Fatalf("admin scopes should allow all supported operations")
	}

	if _, err := ParseScopes("memory:unknown"); err == nil {
		t.Fatal("ParseScopes() error = nil, want invalid scope error")
	}
}

type captureAudit struct {
	events []audit.Event
}

func (c *captureAudit) Log(_ context.Context, event audit.Event) {
	c.events = append(c.events, event)
}
