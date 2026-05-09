// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOperatorDefaultsUseRunningDaemonDatabasePath(t *testing.T) {
	baseDir := t.TempDir()
	dataDir := filepath.Join(baseDir, "pamie")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	dbPath := filepath.Join(baseDir, "repo", "data", "pamie.db")
	state := daemonState{
		PID:          os.Getpid(),
		Addr:         "127.0.0.1:17683",
		DataDir:      dataDir,
		DatabasePath: dbPath,
		LogPath:      filepath.Join(dataDir, daemonLogFile),
		StartedAt:    time.Now().UTC(),
	}
	if err := writeDaemonState(filepath.Join(dataDir, daemonStateFile), state); err != nil {
		t.Fatalf("writeDaemonState() error = %v", err)
	}

	getenv := func(key string) string {
		if key == "XDG_DATA_HOME" {
			return baseDir
		}
		return ""
	}
	gotDBPath, _ := operatorDefaults(getenv)
	if gotDBPath != dbPath {
		t.Fatalf("operatorDefaults dbPath = %q, want %q", gotDBPath, dbPath)
	}
}

func TestOperatorDefaultsPreferExplicitEnvDatabasePath(t *testing.T) {
	baseDir := t.TempDir()
	explicitDBPath := filepath.Join(baseDir, "explicit.db")
	stateDBPath := filepath.Join(baseDir, "state.db")
	dataDir := filepath.Join(baseDir, "pamie")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	state := daemonState{
		PID:          os.Getpid(),
		Addr:         "127.0.0.1:17683",
		DataDir:      dataDir,
		DatabasePath: stateDBPath,
		LogPath:      filepath.Join(dataDir, daemonLogFile),
		StartedAt:    time.Now().UTC(),
	}
	if err := writeDaemonState(filepath.Join(dataDir, daemonStateFile), state); err != nil {
		t.Fatalf("writeDaemonState() error = %v", err)
	}

	getenv := func(key string) string {
		switch key {
		case "XDG_DATA_HOME":
			return baseDir
		case "PAMIE_DB_PATH":
			return explicitDBPath
		default:
			return ""
		}
	}
	gotDBPath, _ := operatorDefaults(getenv)
	if gotDBPath != explicitDBPath {
		t.Fatalf("operatorDefaults dbPath = %q, want %q", gotDBPath, explicitDBPath)
	}
}
