// SPDX-License-Identifier: AGPL-3.0-only

package httpserver

import (
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/your-org/pamie/internal/audit"
)

// RateLimitOptions configures per-client request limiting for /mcp.
type RateLimitOptions struct {
	RequestsPerMinute int
	Burst             int
	Now               func() time.Time
	Audit             audit.Logger
}

func (o RateLimitOptions) enabled() bool {
	return o.RequestsPerMinute > 0
}

func (o RateLimitOptions) validate() error {
	if o.RequestsPerMinute < 0 {
		return errors.New("mcp rate limit must not be negative")
	}
	if o.Burst < 0 {
		return errors.New("mcp rate limit burst must not be negative")
	}
	if o.RequestsPerMinute > 0 && o.Burst == 0 {
		return errors.New("mcp rate limit burst must be positive when rate limit is enabled")
	}
	return nil
}

type rateLimiter struct {
	mu       sync.Mutex
	now      func() time.Time
	rate     float64
	burst    float64
	clients  map[string]*rateBucket
	auditLog audit.Logger
}

type rateBucket struct {
	tokens  float64
	updated time.Time
}

func newRateLimiter(opts RateLimitOptions) (*rateLimiter, error) {
	if err := opts.validate(); err != nil {
		return nil, err
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &rateLimiter{
		now:      opts.Now,
		rate:     float64(opts.RequestsPerMinute) / float64(time.Minute),
		burst:    float64(opts.Burst),
		clients:  map[string]*rateBucket{},
		auditLog: opts.Audit,
	}, nil
}

func (l *rateLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		client := clientKey(r)
		if !l.allow(client) {
			audit.Log(r.Context(), l.auditLog, audit.Event{
				Type:    "rate_limit",
				Outcome: "blocked",
				Action:  "mcp_request",
				Subject: client,
			})
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "rate limit exceeded"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (l *rateLimiter) allow(client string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	bucket, ok := l.clients[client]
	if !ok {
		l.clients[client] = &rateBucket{tokens: l.burst - 1, updated: now}
		return true
	}

	elapsed := now.Sub(bucket.updated)
	if elapsed > 0 {
		bucket.tokens += float64(elapsed) * l.rate
		if bucket.tokens > l.burst {
			bucket.tokens = l.burst
		}
		bucket.updated = now
	}
	if bucket.tokens < 1 {
		return false
	}
	bucket.tokens--
	return true
}

func clientKey(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	if r.RemoteAddr != "" {
		return r.RemoteAddr
	}
	return "unknown"
}
