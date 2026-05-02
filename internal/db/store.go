// SPDX-License-Identifier: AGPL-3.0-only

package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
	_ "modernc.org/sqlite/vec"
)

const (
	driverName         = "sqlite"
	defaultBusyTimeout = 5 * time.Second
)

// Options configures SQLite storage.
type Options struct {
	Path        string
	BusyTimeout time.Duration
}

// Store owns the SQLite connection pool and typed repositories.
type Store struct {
	database *sql.DB
}

// Open opens SQLite, applies safe PRAGMAs, and runs migrations.
func Open(ctx context.Context, opts Options) (*Store, error) {
	if strings.TrimSpace(opts.Path) == "" {
		return nil, errors.New("database path must not be empty")
	}
	if opts.BusyTimeout == 0 {
		opts.BusyTimeout = defaultBusyTimeout
	}
	if opts.BusyTimeout < 0 {
		return nil, errors.New("database busy timeout must not be negative")
	}
	if err := ensureParentDirectory(opts.Path); err != nil {
		return nil, err
	}

	database, err := sql.Open(driverName, opts.Path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)

	store := &Store{database: database}
	if err := store.configure(ctx, opts.BusyTimeout); err != nil {
		_ = database.Close()
		return nil, err
	}
	if err := applyMigrations(ctx, database); err != nil {
		_ = database.Close()
		return nil, err
	}

	return store, nil
}

func ensureParentDirectory(path string) error {
	if path == ":memory:" || strings.HasPrefix(path, "file:") {
		return nil
	}
	dir := filepath.Dir(path)
	if dir == "." || dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create database directory: %w", err)
	}
	return nil
}

func (s *Store) configure(ctx context.Context, busyTimeout time.Duration) error {
	if _, err := s.database.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		return fmt.Errorf("enable foreign keys: %w", err)
	}
	if _, err := s.database.ExecContext(ctx, fmt.Sprintf("PRAGMA busy_timeout = %d", int(busyTimeout.Milliseconds()))); err != nil {
		return fmt.Errorf("set busy timeout: %w", err)
	}
	if _, err := s.database.ExecContext(ctx, "PRAGMA journal_mode = WAL"); err != nil {
		return fmt.Errorf("enable WAL mode: %w", err)
	}
	return nil
}

// Close closes the SQLite connection.
func (s *Store) Close() error {
	if s == nil || s.database == nil {
		return nil
	}
	return s.database.Close()
}

// Ping verifies the database connection.
func (s *Store) Ping(ctx context.Context) error {
	if s == nil || s.database == nil {
		return errors.New("database is not open")
	}
	return s.database.PingContext(ctx)
}

// ApplyMigrations reruns pending migrations. It is safe to call repeatedly.
func (s *Store) ApplyMigrations(ctx context.Context) error {
	if s == nil || s.database == nil {
		return errors.New("database is not open")
	}
	return applyMigrations(ctx, s.database)
}

// JournalMode returns the current SQLite journal mode.
func (s *Store) JournalMode(ctx context.Context) (string, error) {
	var mode string
	if err := s.database.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); err != nil {
		return "", err
	}
	return mode, nil
}

// ForeignKeysEnabled reports whether the current connection enforces foreign keys.
func (s *Store) ForeignKeysEnabled(ctx context.Context) (bool, error) {
	var enabled int
	if err := s.database.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&enabled); err != nil {
		return false, err
	}
	return enabled == 1, nil
}

// SQLiteVecVersion returns the registered sqlite-vec extension version.
func (s *Store) SQLiteVecVersion(ctx context.Context) (string, error) {
	if s == nil || s.database == nil {
		return "", errors.New("database is not open")
	}
	var version string
	if err := s.database.QueryRowContext(ctx, "SELECT vec_version()").Scan(&version); err != nil {
		return "", fmt.Errorf("sqlite-vec unavailable: %w", err)
	}
	return version, nil
}

// ResolveVectorBackend chooses the concrete local vector backend.
func (s *Store) ResolveVectorBackend(ctx context.Context, requested string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(requested)) {
	case "", VectorBackendAuto:
		if _, err := s.SQLiteVecVersion(ctx); err == nil {
			return VectorBackendSQLiteVec, nil
		}
		return VectorBackendSQLiteJSON, nil
	case VectorBackendSQLiteJSON:
		return VectorBackendSQLiteJSON, nil
	case VectorBackendSQLiteVec:
		if _, err := s.SQLiteVecVersion(ctx); err != nil {
			return "", err
		}
		return VectorBackendSQLiteVec, nil
	default:
		return "", fmt.Errorf("unsupported vector backend %q", requested)
	}
}

// AppliedMigrationVersions returns applied migration versions in ascending order.
func (s *Store) AppliedMigrationVersions(ctx context.Context) ([]int, error) {
	rows, err := s.database.QueryContext(ctx, "SELECT version FROM schema_migrations ORDER BY version")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []int
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return versions, nil
}

// Memories returns a repository for memory records.
func (s *Store) Memories() *MemoryRepository {
	return &MemoryRepository{exec: s.database}
}

// Policies returns a repository for retention policies.
func (s *Store) Policies() *RetentionPolicyRepository {
	return &RetentionPolicyRepository{exec: s.database}
}

// AccessLog returns a repository for access log records.
func (s *Store) AccessLog() *AccessLogRepository {
	return &AccessLogRepository{exec: s.database}
}

// WithinTx runs fn inside a database transaction.
func (s *Store) WithinTx(ctx context.Context, fn func(context.Context, *Tx) error) error {
	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	wrapped := &Tx{tx: tx}
	defer func() {
		if recovered := recover(); recovered != nil {
			_ = tx.Rollback()
			panic(recovered)
		}
	}()

	if err := fn(ctx, wrapped); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

// Tx exposes typed repositories inside a transaction.
type Tx struct {
	tx *sql.Tx
}

// Memories returns a transaction-bound memory repository.
func (tx *Tx) Memories() *MemoryRepository {
	return &MemoryRepository{exec: tx.tx}
}

// Policies returns a transaction-bound retention policy repository.
func (tx *Tx) Policies() *RetentionPolicyRepository {
	return &RetentionPolicyRepository{exec: tx.tx}
}

// AccessLog returns a transaction-bound access log repository.
func (tx *Tx) AccessLog() *AccessLogRepository {
	return &AccessLogRepository{exec: tx.tx}
}

type executor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}
