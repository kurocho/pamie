// SPDX-License-Identifier: AGPL-3.0-only

package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/your-org/pamie/internal/auth"
)

// ReadinessCheck is a dependency check used by /ready.
type ReadinessCheck struct {
	Name  string
	Check func(context.Context) error
}

// Options configures the HTTP server.
type Options struct {
	Addr              string
	Authenticator     *auth.BearerAuthenticator
	MCPHandler        http.Handler
	Logger            *slog.Logger
	ReadHeaderTimeout time.Duration
	ShutdownTimeout   time.Duration
	MCPRateLimit      RateLimitOptions
	ReadinessChecks   []ReadinessCheck
}

// Server owns the HTTP listener lifecycle.
type Server struct {
	httpServer      *http.Server
	logger          *slog.Logger
	shutdownTimeout time.Duration
}

// New builds a configured HTTP server without starting it.
func New(opts Options) (*Server, error) {
	if opts.Addr == "" {
		return nil, errors.New("addr must not be empty")
	}
	if opts.Authenticator == nil {
		return nil, errors.New("authenticator must not be nil")
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.ReadHeaderTimeout <= 0 {
		return nil, errors.New("read header timeout must be positive")
	}
	if opts.ShutdownTimeout <= 0 {
		return nil, errors.New("shutdown timeout must be positive")
	}
	if err := opts.MCPRateLimit.validate(); err != nil {
		return nil, err
	}

	handler := NewHandler(HandlerOptions{
		Authenticator:   opts.Authenticator,
		MCPHandler:      opts.MCPHandler,
		Logger:          opts.Logger,
		MCPRateLimit:    opts.MCPRateLimit,
		ReadinessChecks: opts.ReadinessChecks,
	})

	return &Server{
		httpServer: &http.Server{
			Addr:              opts.Addr,
			Handler:           handler,
			ReadHeaderTimeout: opts.ReadHeaderTimeout,
		},
		logger:          opts.Logger,
		shutdownTimeout: opts.ShutdownTimeout,
	}, nil
}

// ListenAndServe starts the server and shuts it down when ctx is canceled.
func (s *Server) ListenAndServe(ctx context.Context) error {
	listener, err := net.Listen("tcp", s.httpServer.Addr)
	if err != nil {
		return err
	}
	return s.Serve(ctx, listener)
}

// Serve serves HTTP on listener and shuts down gracefully when ctx is canceled.
func (s *Server) Serve(ctx context.Context, listener net.Listener) error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.httpServer.Serve(listener)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
		defer cancel()
		if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
			return err
		}
		err := <-errCh
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

// HandlerOptions configures the HTTP handler tree.
type HandlerOptions struct {
	Authenticator   *auth.BearerAuthenticator
	MCPHandler      http.Handler
	Logger          *slog.Logger
	MCPRateLimit    RateLimitOptions
	ReadinessChecks []ReadinessCheck
}

// NewHandler builds the HTTP handler tree.
func NewHandler(opts HandlerOptions) http.Handler {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.Authenticator == nil {
		opts.Authenticator, _ = auth.NewBearerAuthenticator("")
	}
	if opts.MCPHandler == nil {
		opts.MCPHandler = http.HandlerFunc(mcpNotImplementedHandler)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", healthHandler)
	mux.HandleFunc("GET /ready", readyHandler(opts.ReadinessChecks))
	mcpHandler := opts.Authenticator.Middleware(opts.MCPHandler)
	if opts.MCPRateLimit.enabled() {
		limiter, err := newRateLimiter(opts.MCPRateLimit)
		if err == nil {
			mcpHandler = limiter.middleware(mcpHandler)
		}
	}
	mux.Handle("POST /mcp", mcpHandler)

	return requestLogger(opts.Logger, mux)
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"service": "pamie",
		"status":  "ok",
	})
}

func readyHandler(checks []ReadinessCheck) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		results := make([]map[string]string, 0, len(checks))
		for _, check := range checks {
			result := map[string]string{
				"name":   check.Name,
				"status": "ok",
			}
			if check.Check == nil {
				result["status"] = "failed"
				result["error"] = "check failed"
				results = append(results, result)
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{
					"status": "not_ready",
					"checks": results,
				})
				return
			}
			if err := check.Check(r.Context()); err != nil {
				result["status"] = "failed"
				result["error"] = "check failed"
				results = append(results, result)
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{
					"status": "not_ready",
					"checks": results,
				})
				return
			}
			results = append(results, result)
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status": "ready",
			"checks": results,
		})
	}
}

func mcpNotImplementedHandler(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusNotImplemented, map[string]string{
		"error": "mcp endpoint is not implemented yet",
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func requestLogger(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		logger.Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", recorder.status,
			"duration_ms", time.Since(started).Milliseconds(),
		)
	})
}
