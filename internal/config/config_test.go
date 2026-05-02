// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"io"
	"strings"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(nil, nil, io.Discard)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Addr != defaultAddr {
		t.Fatalf("Addr = %q, want %q", cfg.Addr, defaultAddr)
	}
	if cfg.LogLevel != defaultLogLevel {
		t.Fatalf("LogLevel = %q, want %q", cfg.LogLevel, defaultLogLevel)
	}
	if cfg.BearerTokenID != defaultTokenID {
		t.Fatalf("BearerTokenID = %q, want %q", cfg.BearerTokenID, defaultTokenID)
	}
	if cfg.BearerTokenScopes != defaultTokenScopes {
		t.Fatalf("BearerTokenScopes = %q, want %q", cfg.BearerTokenScopes, defaultTokenScopes)
	}
	if cfg.DataDir != defaultDataDir {
		t.Fatalf("DataDir = %q, want %q", cfg.DataDir, defaultDataDir)
	}
	if cfg.DatabasePath != "data/pamie.db" {
		t.Fatalf("DatabasePath = %q, want %q", cfg.DatabasePath, "data/pamie.db")
	}
	if cfg.ReadHeaderTimeout != defaultReadHeaderTime {
		t.Fatalf("ReadHeaderTimeout = %v, want %v", cfg.ReadHeaderTimeout, defaultReadHeaderTime)
	}
	if cfg.ShutdownTimeout != defaultShutdownTimeout {
		t.Fatalf("ShutdownTimeout = %v, want %v", cfg.ShutdownTimeout, defaultShutdownTimeout)
	}
	if cfg.MCPRateLimit != defaultMCPRateLimit {
		t.Fatalf("MCPRateLimit = %d, want %d", cfg.MCPRateLimit, defaultMCPRateLimit)
	}
	if cfg.MCPRateBurst != defaultMCPRateBurst {
		t.Fatalf("MCPRateBurst = %d, want %d", cfg.MCPRateBurst, defaultMCPRateBurst)
	}
	if cfg.LifecycleEnabled {
		t.Fatal("LifecycleEnabled = true, want false")
	}
	if cfg.LifecycleInterval != defaultLifecycleInterval {
		t.Fatalf("LifecycleInterval = %v, want %v", cfg.LifecycleInterval, defaultLifecycleInterval)
	}
	if cfg.LifecycleBatchSize != defaultLifecycleBatchSize {
		t.Fatalf("LifecycleBatchSize = %d, want %d", cfg.LifecycleBatchSize, defaultLifecycleBatchSize)
	}
	if cfg.LifecycleRunOnStart {
		t.Fatal("LifecycleRunOnStart = true, want false")
	}
	if cfg.LifecycleStartupDelay != 0 {
		t.Fatalf("LifecycleStartupDelay = %v, want 0", cfg.LifecycleStartupDelay)
	}
	if cfg.VectorSearchEnabled {
		t.Fatal("VectorSearchEnabled = true, want false")
	}
	if cfg.VectorBackend != defaultVectorBackend {
		t.Fatalf("VectorBackend = %q, want %q", cfg.VectorBackend, defaultVectorBackend)
	}
	if cfg.VectorProvider != defaultVectorProvider {
		t.Fatalf("VectorProvider = %q, want %q", cfg.VectorProvider, defaultVectorProvider)
	}
	if cfg.VectorModel != defaultVectorModel {
		t.Fatalf("VectorModel = %q, want %q", cfg.VectorModel, defaultVectorModel)
	}
	if cfg.VectorDimensions != defaultVectorDimensions {
		t.Fatalf("VectorDimensions = %d, want %d", cfg.VectorDimensions, defaultVectorDimensions)
	}
	if cfg.VectorOllamaURL != defaultVectorOllamaURL {
		t.Fatalf("VectorOllamaURL = %q, want %q", cfg.VectorOllamaURL, defaultVectorOllamaURL)
	}
}

func TestLoadEnvironmentAndFlagPrecedence(t *testing.T) {
	env := map[string]string{
		EnvAddr:                  "127.0.0.1:9000",
		EnvToken:                 "env-token",
		EnvTokenID:               "env-token-id",
		EnvTokenScopes:           "memory:read",
		EnvDataDir:               "/tmp/env-data",
		EnvDatabasePath:          "/tmp/env.db",
		EnvLogLevel:              "warn",
		EnvReadHeaderTimeout:     "2s",
		EnvShutdownTimeout:       "3s",
		EnvMCPRateLimit:          "80",
		EnvMCPRateBurst:          "8",
		EnvLifecycleEnabled:      "false",
		EnvLifecycleInterval:     "45m",
		EnvLifecycleBatchSize:    "100",
		EnvLifecycleRunOnStart:   "false",
		EnvLifecycleStartupDelay: "2s",
		EnvVectorSearchEnabled:   "false",
		EnvVectorBackend:         "sqlite-json",
		EnvVectorProvider:        "local-hash",
		EnvVectorModel:           "env-model",
		EnvVectorDimensions:      "128",
		EnvVectorOllamaURL:       "http://127.0.0.1:11435",
		EnvVectorOllamaKeepAlive: "1h",
	}

	cfg, err := Load([]string{
		"--addr", "127.0.0.1:9100",
		"--token", "flag-token",
		"--token-id", "flag-token-id",
		"--token-scopes", "memory:read,memory:write",
		"--data-dir", "/tmp/flag-data",
		"--db-path", "/tmp/flag.db",
		"--log-level", "debug",
		"--read-header-timeout", "4s",
		"--shutdown-timeout", "5s",
		"--mcp-rate-limit", "40",
		"--mcp-rate-burst", "4",
		"--lifecycle-worker=true",
		"--lifecycle-interval", "30m",
		"--lifecycle-batch-size", "250",
		"--lifecycle-run-on-start=true",
		"--lifecycle-startup-delay", "10s",
		"--vector-search=true",
		"--vector-backend", "sqlite-vec",
		"--vector-provider", "ollama",
		"--vector-model", "flag-model",
		"--vector-dimensions", "256",
		"--vector-ollama-url", "http://127.0.0.1:11436",
		"--vector-ollama-keep-alive", "2h",
	}, func(key string) string {
		return env[key]
	}, io.Discard)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Addr != "127.0.0.1:9100" {
		t.Fatalf("Addr = %q", cfg.Addr)
	}
	if cfg.BearerToken != "flag-token" {
		t.Fatalf("BearerToken = %q", cfg.BearerToken)
	}
	if cfg.BearerTokenID != "flag-token-id" {
		t.Fatalf("BearerTokenID = %q", cfg.BearerTokenID)
	}
	if cfg.BearerTokenScopes != "memory:read,memory:write" {
		t.Fatalf("BearerTokenScopes = %q", cfg.BearerTokenScopes)
	}
	if cfg.LogLevel != "debug" {
		t.Fatalf("LogLevel = %q", cfg.LogLevel)
	}
	if cfg.DataDir != "/tmp/flag-data" {
		t.Fatalf("DataDir = %q", cfg.DataDir)
	}
	if cfg.DatabasePath != "/tmp/flag.db" {
		t.Fatalf("DatabasePath = %q", cfg.DatabasePath)
	}
	if cfg.ReadHeaderTimeout != 4*time.Second {
		t.Fatalf("ReadHeaderTimeout = %v", cfg.ReadHeaderTimeout)
	}
	if cfg.ShutdownTimeout != 5*time.Second {
		t.Fatalf("ShutdownTimeout = %v", cfg.ShutdownTimeout)
	}
	if cfg.MCPRateLimit != 40 {
		t.Fatalf("MCPRateLimit = %d", cfg.MCPRateLimit)
	}
	if cfg.MCPRateBurst != 4 {
		t.Fatalf("MCPRateBurst = %d", cfg.MCPRateBurst)
	}
	if !cfg.LifecycleEnabled {
		t.Fatal("LifecycleEnabled = false")
	}
	if cfg.LifecycleInterval != 30*time.Minute {
		t.Fatalf("LifecycleInterval = %v", cfg.LifecycleInterval)
	}
	if cfg.LifecycleBatchSize != 250 {
		t.Fatalf("LifecycleBatchSize = %d", cfg.LifecycleBatchSize)
	}
	if !cfg.LifecycleRunOnStart {
		t.Fatal("LifecycleRunOnStart = false")
	}
	if cfg.LifecycleStartupDelay != 10*time.Second {
		t.Fatalf("LifecycleStartupDelay = %v", cfg.LifecycleStartupDelay)
	}
	if !cfg.VectorSearchEnabled {
		t.Fatal("VectorSearchEnabled = false")
	}
	if cfg.VectorBackend != "sqlite-vec" {
		t.Fatalf("VectorBackend = %q", cfg.VectorBackend)
	}
	if cfg.VectorProvider != "ollama" {
		t.Fatalf("VectorProvider = %q", cfg.VectorProvider)
	}
	if cfg.VectorModel != "flag-model" {
		t.Fatalf("VectorModel = %q", cfg.VectorModel)
	}
	if cfg.VectorDimensions != 256 {
		t.Fatalf("VectorDimensions = %d", cfg.VectorDimensions)
	}
	if cfg.VectorOllamaURL != "http://127.0.0.1:11436" {
		t.Fatalf("VectorOllamaURL = %q", cfg.VectorOllamaURL)
	}
	if cfg.VectorOllamaKeepAlive != "2h" {
		t.Fatalf("VectorOllamaKeepAlive = %q", cfg.VectorOllamaKeepAlive)
	}
}

func TestLoadComputesDatabasePathFromDataDir(t *testing.T) {
	cfg, err := Load([]string{"--data-dir", "/tmp/pamie-data"}, nil, io.Discard)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.DatabasePath != "/tmp/pamie-data/pamie.db" {
		t.Fatalf("DatabasePath = %q", cfg.DatabasePath)
	}
}

func TestLoadVersionFlag(t *testing.T) {
	cfg, err := Load([]string{"--version"}, nil, io.Discard)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !cfg.ShowVersion {
		t.Fatal("ShowVersion = false, want true")
	}
}

func TestLoadRejectsUnexpectedArguments(t *testing.T) {
	_, err := Load([]string{"serve"}, nil, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "unexpected arguments") {
		t.Fatalf("Load() error = %v, want unexpected arguments error", err)
	}
}

func TestValidateRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
	}{
		{name: "empty addr", cfg: validConfig(func(cfg *Config) { cfg.Addr = "" })},
		{name: "empty data dir", cfg: validConfig(func(cfg *Config) { cfg.DataDir = "" })},
		{name: "empty database path", cfg: validConfig(func(cfg *Config) { cfg.DatabasePath = "" })},
		{name: "bad log level", cfg: validConfig(func(cfg *Config) { cfg.LogLevel = "trace" })},
		{name: "token whitespace", cfg: validConfig(func(cfg *Config) { cfg.BearerToken = " secret" })},
		{name: "empty token id", cfg: validConfig(func(cfg *Config) { cfg.BearerTokenID = "" })},
		{name: "empty token scopes", cfg: validConfig(func(cfg *Config) { cfg.BearerTokenScopes = "" })},
		{name: "bad read timeout", cfg: validConfig(func(cfg *Config) { cfg.ReadHeaderTimeout = 0 })},
		{name: "bad shutdown timeout", cfg: validConfig(func(cfg *Config) { cfg.ShutdownTimeout = 0 })},
		{name: "negative rate limit", cfg: validConfig(func(cfg *Config) { cfg.MCPRateLimit = -1 })},
		{name: "negative rate burst", cfg: validConfig(func(cfg *Config) { cfg.MCPRateBurst = -1 })},
		{name: "enabled rate with zero burst", cfg: validConfig(func(cfg *Config) { cfg.MCPRateLimit = 1; cfg.MCPRateBurst = 0 })},
		{name: "bad lifecycle interval", cfg: validConfig(func(cfg *Config) { cfg.LifecycleInterval = 0 })},
		{name: "bad lifecycle batch size", cfg: validConfig(func(cfg *Config) { cfg.LifecycleBatchSize = 0 })},
		{name: "bad lifecycle startup delay", cfg: validConfig(func(cfg *Config) { cfg.LifecycleStartupDelay = -time.Second })},
		{name: "unsupported vector backend", cfg: validConfig(func(cfg *Config) { cfg.VectorBackend = "remote" })},
		{name: "empty vector provider", cfg: validConfig(func(cfg *Config) { cfg.VectorProvider = "" })},
		{name: "unsupported vector provider", cfg: validConfig(func(cfg *Config) { cfg.VectorProvider = "hosted" })},
		{name: "bad vector dimensions", cfg: validConfig(func(cfg *Config) { cfg.VectorDimensions = 0 })},
		{name: "empty ollama model", cfg: validConfig(func(cfg *Config) { cfg.VectorProvider = "ollama"; cfg.VectorModel = "" })},
		{name: "empty ollama url", cfg: validConfig(func(cfg *Config) { cfg.VectorProvider = "ollama"; cfg.VectorOllamaURL = "" })},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.cfg.Validate(); err == nil {
				t.Fatal("Validate() error = nil, want error")
			}
		})
	}
}

func validConfig(mutator func(*Config)) Config {
	cfg := Default()
	if mutator != nil {
		mutator(&cfg)
	}
	return cfg
}
