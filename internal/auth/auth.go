// SPDX-License-Identifier: AGPL-3.0-only

package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/your-org/pamie/internal/audit"
)

var (
	ErrNotConfigured = errors.New("bearer token is not configured")
	ErrMissingToken  = errors.New("missing bearer token")
	ErrMalformedAuth = errors.New("malformed authorization header")
	ErrInvalidToken  = errors.New("invalid bearer token")
	ErrForbidden     = errors.New("required scope is missing")
)

// Scope identifies one authorization capability.
type Scope string

const (
	ScopeAll          Scope = "all"
	ScopeMemoryRead   Scope = "memory:read"
	ScopeMemoryWrite  Scope = "memory:write"
	ScopeMemoryDelete Scope = "memory:delete"
	ScopeMemoryAdmin  Scope = "memory:admin"
	ScopeBackupRead   Scope = "backup:read"
	ScopeStatsRead    Scope = "stats:read"
)

// ScopeSet is a set of authorization scopes.
type ScopeSet map[Scope]struct{}

// ParseScopes parses a comma- or space-separated scope list.
func ParseScopes(value string) (ScopeSet, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, string(ScopeAll)) {
		return AllScopes(), nil
	}

	scopes := ScopeSet{}
	for _, field := range strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\r' || r == '\n'
	}) {
		scope := Scope(strings.TrimSpace(field))
		if scope == "" {
			continue
		}
		if !scope.Valid() {
			return nil, fmt.Errorf("%w: invalid scope %q", ErrForbidden, scope)
		}
		scopes[scope] = struct{}{}
	}
	if len(scopes) == 0 {
		return nil, fmt.Errorf("%w: at least one scope is required", ErrForbidden)
	}
	return scopes, nil
}

// AllScopes returns all currently supported scopes.
func AllScopes() ScopeSet {
	return ScopeSet{
		ScopeMemoryRead:   {},
		ScopeMemoryWrite:  {},
		ScopeMemoryDelete: {},
		ScopeMemoryAdmin:  {},
		ScopeBackupRead:   {},
		ScopeStatsRead:    {},
	}
}

// Valid reports whether s is a known scope.
func (s Scope) Valid() bool {
	switch s {
	case ScopeAll, ScopeMemoryRead, ScopeMemoryWrite, ScopeMemoryDelete, ScopeMemoryAdmin, ScopeBackupRead, ScopeStatsRead:
		return true
	default:
		return false
	}
}

// Has reports whether the set allows required.
func (s ScopeSet) Has(required Scope) bool {
	if required == "" {
		return true
	}
	if _, ok := s[ScopeAll]; ok {
		return true
	}
	if _, ok := s[ScopeMemoryAdmin]; ok {
		return true
	}
	_, ok := s[required]
	return ok
}

// Principal describes the authenticated token without exposing the secret.
type Principal struct {
	TokenID string
	Scopes  ScopeSet
}

type principalContextKey struct{}

// PrincipalFromContext returns the authenticated principal from ctx.
func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalContextKey{}).(Principal)
	return principal, ok
}

// ContextWithPrincipal stores principal in ctx.
func ContextWithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, principal)
}

// RequireScope validates that ctx has an authenticated principal with scope.
func RequireScope(ctx context.Context, scope Scope) error {
	principal, ok := PrincipalFromContext(ctx)
	if !ok {
		return ErrForbidden
	}
	if !principal.Scopes.Has(scope) {
		return ErrForbidden
	}
	return nil
}

// Token configures one Bearer token.
type Token struct {
	ID     string
	Secret string
	Scopes ScopeSet
}

// BearerAuthenticator validates requests against configured Bearer tokens.
type BearerAuthenticator struct {
	configured bool
	tokens     []tokenRecord
	audit      audit.Logger
}

type tokenRecord struct {
	id        string
	tokenHash [sha256.Size]byte
	scopes    ScopeSet
}

// NewBearerAuthenticator creates an authenticator. An empty token is allowed
// and means protected endpoints reject all requests until a token is configured.
func NewBearerAuthenticator(token string) (*BearerAuthenticator, error) {
	return NewBearerAuthenticatorWithOptions(token, "default", AllScopes(), nil)
}

// NewBearerAuthenticatorWithOptions creates an authenticator for one scoped token.
func NewBearerAuthenticatorWithOptions(token string, tokenID string, scopes ScopeSet, auditLogger audit.Logger) (*BearerAuthenticator, error) {
	if token == "" {
		return &BearerAuthenticator{audit: auditLogger}, nil
	}
	if strings.TrimSpace(token) != token || strings.ContainsAny(token, " \t\r\n") {
		return nil, ErrMalformedAuth
	}
	if strings.TrimSpace(tokenID) == "" {
		return nil, errors.New("token id must not be empty")
	}
	if scopes == nil {
		scopes = AllScopes()
	}
	return &BearerAuthenticator{
		configured: true,
		tokens: []tokenRecord{
			{
				id:        tokenID,
				tokenHash: sha256.Sum256([]byte(token)),
				scopes:    scopes,
			},
		},
		audit: auditLogger,
	}, nil
}

// Configured reports whether a token was configured at startup.
func (a *BearerAuthenticator) Configured() bool {
	return a != nil && a.configured
}

// Authenticate validates the HTTP Authorization header.
func (a *BearerAuthenticator) Authenticate(r *http.Request) error {
	_, err := a.AuthenticateRequest(r)
	return err
}

// AuthenticateRequest validates the HTTP Authorization header and returns the principal.
func (a *BearerAuthenticator) AuthenticateRequest(r *http.Request) (Principal, error) {
	if a == nil || !a.configured {
		return Principal{}, ErrNotConfigured
	}
	header := r.Header.Get("Authorization")
	if header == "" {
		return Principal{}, ErrMissingToken
	}

	scheme, token, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" || strings.ContainsAny(token, " \t\r\n") {
		return Principal{}, ErrMalformedAuth
	}

	tokenHash := sha256.Sum256([]byte(token))
	for _, configured := range a.tokens {
		if subtle.ConstantTimeCompare(tokenHash[:], configured.tokenHash[:]) == 1 {
			return Principal{
				TokenID: configured.id,
				Scopes:  configured.scopes,
			}, nil
		}
	}

	return Principal{}, ErrInvalidToken
}

// Middleware protects an HTTP handler with Bearer token authentication.
func (a *BearerAuthenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, err := a.AuthenticateRequest(r)
		if err != nil {
			a.logAudit(r.Context(), audit.Event{
				Type:    "auth",
				Outcome: "failure",
				Action:  "authenticate",
				Fields: map[string]any{
					"path":   r.URL.Path,
					"reason": authFailureReason(err),
				},
			})
			writeAuthError(w, err)
			return
		}
		a.logAudit(r.Context(), audit.Event{
			Type:    "auth",
			Outcome: "success",
			TokenID: principal.TokenID,
			Action:  "authenticate",
			Fields:  map[string]any{"path": r.URL.Path},
		})
		next.ServeHTTP(w, r.WithContext(ContextWithPrincipal(r.Context(), principal)))
	})
}

func (a *BearerAuthenticator) logAudit(ctx context.Context, event audit.Event) {
	if a == nil {
		return
	}
	audit.Log(ctx, a.audit, event)
}

func authFailureReason(err error) string {
	switch {
	case errors.Is(err, ErrNotConfigured):
		return "not_configured"
	case errors.Is(err, ErrMissingToken):
		return "missing_token"
	case errors.Is(err, ErrMalformedAuth):
		return "malformed_header"
	case errors.Is(err, ErrInvalidToken):
		return "invalid_token"
	default:
		return "unknown"
	}
}

func writeAuthError(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	if errors.Is(err, ErrNotConfigured) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"bearer token is not configured"}` + "\n"))
		return
	}

	w.Header().Set("WWW-Authenticate", `Bearer realm="pamie"`)
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":"unauthorized"}` + "\n"))
}
