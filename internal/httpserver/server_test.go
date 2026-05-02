// SPDX-License-Identifier: AGPL-3.0-only

package httpserver

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/your-org/pamie/internal/auth"
)

func TestHealthHandler(t *testing.T) {
	handler := testHandler(t, "secret", nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Fatalf("body = %q, want ok status", rec.Body.String())
	}
}

func TestReadyHandler(t *testing.T) {
	handler := testHandler(t, "secret", nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), `"status":"ready"`) {
		t.Fatalf("body = %q, want ready status", rec.Body.String())
	}
}

func TestReadyHandlerFailure(t *testing.T) {
	handler := testHandler(t, "secret", []ReadinessCheck{
		{
			Name: "dependency",
			Check: func(context.Context) error {
				return errors.New("not available")
			},
		},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if !strings.Contains(rec.Body.String(), `"status":"not_ready"`) {
		t.Fatalf("body = %q, want not_ready status", rec.Body.String())
	}
}

func TestMCPIsAuthenticatedAndNotImplemented(t *testing.T) {
	handler := testHandler(t, "secret", nil)

	unauthorized := httptest.NewRecorder()
	unauthorizedReq := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	handler.ServeHTTP(unauthorized, unauthorizedReq)
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d, want %d", unauthorized.Code, http.StatusUnauthorized)
	}

	implemented := httptest.NewRecorder()
	implementedReq := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	implementedReq.Header.Set("Authorization", "Bearer secret")
	handler.ServeHTTP(implemented, implementedReq)
	if implemented.Code != http.StatusNotImplemented {
		t.Fatalf("authorized status = %d, want %d", implemented.Code, http.StatusNotImplemented)
	}
	if !strings.Contains(implemented.Body.String(), "not implemented") {
		t.Fatalf("body = %q, want not implemented message", implemented.Body.String())
	}
}

func TestMCPRateLimit(t *testing.T) {
	authenticator, err := auth.NewBearerAuthenticator("secret")
	if err != nil {
		t.Fatalf("NewBearerAuthenticator() error = %v", err)
	}
	handler := NewHandler(HandlerOptions{
		Authenticator: authenticator,
		Logger:        discardLogger(),
		MCPRateLimit: RateLimitOptions{
			RequestsPerMinute: 60,
			Burst:             1,
		},
	})

	first := httptest.NewRecorder()
	firstReq := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	firstReq.RemoteAddr = "192.0.2.10:1234"
	firstReq.Header.Set("Authorization", "Bearer secret")
	handler.ServeHTTP(first, firstReq)
	if first.Code != http.StatusNotImplemented {
		t.Fatalf("first status = %d, want %d", first.Code, http.StatusNotImplemented)
	}

	second := httptest.NewRecorder()
	secondReq := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	secondReq.RemoteAddr = "192.0.2.10:5678"
	secondReq.Header.Set("Authorization", "Bearer secret")
	handler.ServeHTTP(second, secondReq)
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want %d", second.Code, http.StatusTooManyRequests)
	}
}

func TestServeGracefulShutdown(t *testing.T) {
	authenticator, err := auth.NewBearerAuthenticator("secret")
	if err != nil {
		t.Fatalf("NewBearerAuthenticator() error = %v", err)
	}
	server, err := New(Options{
		Addr:              "127.0.0.1:0",
		Authenticator:     authenticator,
		Logger:            discardLogger(),
		ReadHeaderTimeout: time.Second,
		ShutdownTimeout:   time.Second,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Serve(ctx, listener)
	}()

	client := http.Client{Timeout: time.Second}
	resp, err := client.Get("http://" + listener.Addr().String() + "/health")
	if err != nil {
		cancel()
		t.Fatalf("GET /health error = %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		cancel()
		t.Fatalf("GET /health status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Serve() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve() did not stop after context cancellation")
	}
}

func testHandler(t *testing.T, token string, checks []ReadinessCheck) http.Handler {
	t.Helper()
	authenticator, err := auth.NewBearerAuthenticator(token)
	if err != nil {
		t.Fatalf("NewBearerAuthenticator() error = %v", err)
	}
	return NewHandler(HandlerOptions{
		Authenticator:   authenticator,
		Logger:          discardLogger(),
		ReadinessChecks: checks,
	})
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
