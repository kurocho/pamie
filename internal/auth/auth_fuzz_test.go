// SPDX-License-Identifier: AGPL-3.0-only

package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func FuzzBearerAuthenticatorAuthHeader(f *testing.F) {
	for _, seed := range []string{
		"",
		"Bearer secret",
		"bearer secret",
		"Basic secret",
		"Bearer",
		"Bearer secret extra",
		"Bearer wrong",
		"Bearer sec ret",
		"Bearer \tsecret",
	} {
		f.Add(seed)
	}

	authenticator, err := NewBearerAuthenticator("secret")
	if err != nil {
		f.Fatalf("NewBearerAuthenticator() error = %v", err)
	}
	f.Fuzz(func(t *testing.T, header string) {
		req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
		if header != "" {
			req.Header.Set("Authorization", header)
		}
		principal, err := authenticator.AuthenticateRequest(req)
		if err != nil {
			return
		}
		if !validFuzzBearerHeader(header) {
			t.Fatalf("AuthenticateRequest(%q) succeeded for invalid header", header)
		}
		if principal.TokenID != "default" || !principal.Scopes.Has(ScopeMemoryRead) {
			t.Fatalf("principal = %+v, want default all-scopes principal", principal)
		}
	})
}

func validFuzzBearerHeader(header string) bool {
	scheme, token, ok := strings.Cut(header, " ")
	return ok && strings.EqualFold(scheme, "Bearer") && token == "secret"
}
