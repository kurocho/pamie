// SPDX-License-Identifier: AGPL-3.0-only

package db

import (
	"context"
	"fmt"
	"time"
)

// AccessLogRepository stores memory access records.
type AccessLogRepository struct {
	exec executor
}

// Record appends an access log entry and returns its generated ID.
func (r *AccessLogRepository) Record(ctx context.Context, entry AccessLogEntry) (int64, error) {
	if err := validateAccessLogEntry(entry); err != nil {
		return 0, err
	}

	result, err := r.exec.ExecContext(ctx, `
INSERT INTO access_log (memory_id, access_type, token_id, created_at)
VALUES (?, ?, ?, ?)`,
		entry.MemoryID,
		entry.AccessType,
		nullString(entry.TokenID),
		formatTime(entry.CreatedAt),
	)
	if err != nil {
		return 0, fmt.Errorf("record access log: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("access log id: %w", err)
	}
	return id, nil
}

// CountSince returns the number of access events for a memory since a timestamp.
func (r *AccessLogRepository) CountSince(ctx context.Context, memoryID string, since time.Time) (int, error) {
	if err := requireID("memory", memoryID); err != nil {
		return 0, err
	}
	if since.IsZero() {
		return 0, fmt.Errorf("%w: since must not be zero", ErrInvalid)
	}

	var count int
	if err := r.exec.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM access_log
WHERE memory_id = ? AND created_at >= ?`, memoryID, formatTime(since)).Scan(&count); err != nil {
		return 0, fmt.Errorf("count access log: %w", err)
	}
	return count, nil
}

func validateAccessLogEntry(entry AccessLogEntry) error {
	if err := requireID("memory", entry.MemoryID); err != nil {
		return err
	}
	if entry.AccessType == "" {
		return fmt.Errorf("%w: access type must not be empty", ErrInvalid)
	}
	if entry.CreatedAt.IsZero() {
		return fmt.Errorf("%w: access created_at must not be zero", ErrInvalid)
	}
	return nil
}
