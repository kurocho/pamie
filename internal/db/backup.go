// SPDX-License-Identifier: AGPL-3.0-only

package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// BackupSQLite creates a consistent SQLite backup at destinationPath.
//
// The implementation uses SQLite's VACUUM INTO command instead of copying the
// database file, which is safe for databases running with WAL enabled.
func BackupSQLite(ctx context.Context, sourcePath, destinationPath string) error {
	sourcePath = strings.TrimSpace(sourcePath)
	destinationPath = strings.TrimSpace(destinationPath)
	if sourcePath == "" {
		return errors.New("source database path must not be empty")
	}
	if err := RequireExistingDatabasePath(sourcePath); err != nil {
		return err
	}
	if destinationPath == "" {
		return errors.New("backup destination path must not be empty")
	}
	if destinationPath == ":memory:" || strings.HasPrefix(destinationPath, "file:") {
		return errors.New("backup destination must be a filesystem path")
	}
	if err := ensureBackupDestination(destinationPath); err != nil {
		return err
	}

	database, err := sql.Open(driverName, sourcePath)
	if err != nil {
		return fmt.Errorf("open source database: %w", err)
	}
	defer database.Close()
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)

	if _, err := database.ExecContext(ctx, fmt.Sprintf("PRAGMA busy_timeout = %d", int(defaultBusyTimeout.Milliseconds()))); err != nil {
		return fmt.Errorf("set source busy timeout: %w", err)
	}
	if _, err := database.ExecContext(ctx, "VACUUM INTO "+quoteSQLiteString(destinationPath)); err != nil {
		return fmt.Errorf("sqlite backup: %w", err)
	}
	if err := integrityCheck(ctx, destinationPath); err != nil {
		_ = os.Remove(destinationPath)
		return err
	}
	return nil
}

// RestoreSQLiteBackup validates a SQLite backup and restores it to destinationPath.
//
// The destination must not already exist. Operators should stop Pamie and move
// any existing database files aside before committing a SQLite restore.
func RestoreSQLiteBackup(ctx context.Context, sourcePath, destinationPath string) error {
	sourcePath = strings.TrimSpace(sourcePath)
	destinationPath = strings.TrimSpace(destinationPath)
	if sourcePath == "" {
		return errors.New("restore source path must not be empty")
	}
	if sourcePath == ":memory:" || strings.HasPrefix(sourcePath, "file:") {
		return errors.New("restore source must be a filesystem path")
	}
	if destinationPath == "" {
		return errors.New("restore destination path must not be empty")
	}
	if destinationPath == ":memory:" || strings.HasPrefix(destinationPath, "file:") {
		return errors.New("restore destination must be a filesystem path")
	}
	if err := ValidateSQLiteDatabase(ctx, sourcePath); err != nil {
		return err
	}
	if err := ensureBackupDestination(destinationPath); err != nil {
		return err
	}

	source, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open restore source: %w", err)
	}
	defer source.Close()

	destination, err := os.OpenFile(destinationPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create restore destination: %w", err)
	}
	if _, err := io.Copy(destination, source); err != nil {
		_ = destination.Close()
		_ = os.Remove(destinationPath)
		return fmt.Errorf("copy restore database: %w", err)
	}
	if err := destination.Close(); err != nil {
		_ = os.Remove(destinationPath)
		return fmt.Errorf("close restore destination: %w", err)
	}
	if err := ValidateSQLiteDatabase(ctx, destinationPath); err != nil {
		_ = os.Remove(destinationPath)
		return err
	}
	return nil
}

// ValidateSQLiteDatabase verifies that path points at a readable SQLite database.
func ValidateSQLiteDatabase(ctx context.Context, path string) error {
	if err := RequireExistingDatabasePath(path); err != nil {
		return err
	}
	return integrityCheck(ctx, path)
}

// RequireExistingDatabasePath verifies that path points at an existing database file.
func RequireExistingDatabasePath(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("database path must not be empty")
	}
	if path == ":memory:" {
		return errors.New("database path must be a filesystem path")
	}
	if strings.HasPrefix(path, "file:") {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("database path %q does not exist", path)
		}
		return fmt.Errorf("check database path: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("database path %q is a directory", path)
	}
	return nil
}

func ensureBackupDestination(path string) error {
	info, err := os.Stat(path)
	if err == nil {
		if info.IsDir() {
			return fmt.Errorf("backup destination %q is a directory", path)
		}
		return fmt.Errorf("backup destination %q already exists", path)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("check backup destination: %w", err)
	}
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("create backup directory: %w", err)
		}
	}
	return nil
}

func integrityCheck(ctx context.Context, path string) error {
	database, err := sql.Open(driverName, path)
	if err != nil {
		return fmt.Errorf("open backup for integrity check: %w", err)
	}
	defer database.Close()
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)

	checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var result string
	if err := database.QueryRowContext(checkCtx, "PRAGMA integrity_check").Scan(&result); err != nil {
		return fmt.Errorf("backup integrity check: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("backup integrity check failed: %s", result)
	}
	return nil
}

func quoteSQLiteString(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
