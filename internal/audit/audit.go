// SPDX-License-Identifier: AGPL-3.0-only

package audit

import (
	"context"
	"log/slog"
)

// Event describes one security-relevant audit event.
type Event struct {
	Type    string
	Outcome string
	TokenID string
	Action  string
	Subject string
	Fields  map[string]any
}

// Logger records audit events.
type Logger interface {
	Log(context.Context, Event)
}

type noopLogger struct{}

func (noopLogger) Log(context.Context, Event) {}

// Noop returns an audit logger that drops events.
func Noop() Logger {
	return noopLogger{}
}

type slogLogger struct {
	logger *slog.Logger
}

// NewSlogLogger records audit events through slog without special token fields.
func NewSlogLogger(logger *slog.Logger) Logger {
	if logger == nil {
		logger = slog.Default()
	}
	return slogLogger{logger: logger}
}

func (l slogLogger) Log(ctx context.Context, event Event) {
	attrs := []any{
		"audit_type", event.Type,
		"outcome", event.Outcome,
	}
	if event.TokenID != "" {
		attrs = append(attrs, "token_id", event.TokenID)
	}
	if event.Action != "" {
		attrs = append(attrs, "action", event.Action)
	}
	if event.Subject != "" {
		attrs = append(attrs, "subject", event.Subject)
	}
	for key, value := range event.Fields {
		if key == "" || key == "token" || key == "authorization" {
			continue
		}
		attrs = append(attrs, key, value)
	}
	l.logger.InfoContext(ctx, "audit event", attrs...)
}

// Log records an event when logger is configured.
func Log(ctx context.Context, logger Logger, event Event) {
	if logger == nil {
		return
	}
	logger.Log(ctx, event)
}
