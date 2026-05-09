// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	EnvAddr                           = "PAMIE_ADDR"
	EnvToken                          = "PAMIE_TOKEN"
	EnvTokenID                        = "PAMIE_TOKEN_ID"
	EnvTokenScopes                    = "PAMIE_TOKEN_SCOPES"
	EnvDataDir                        = "PAMIE_DATA_DIR"
	EnvDatabasePath                   = "PAMIE_DB_PATH"
	EnvLogLevel                       = "PAMIE_LOG_LEVEL"
	EnvReadHeaderTimeout              = "PAMIE_READ_HEADER_TIMEOUT"
	EnvShutdownTimeout                = "PAMIE_SHUTDOWN_TIMEOUT"
	EnvMCPRateLimit                   = "PAMIE_MCP_RATE_LIMIT"
	EnvMCPRateBurst                   = "PAMIE_MCP_RATE_BURST"
	EnvLifecycleEnabled               = "PAMIE_LIFECYCLE_WORKER_ENABLED"
	EnvLifecycleInterval              = "PAMIE_LIFECYCLE_INTERVAL"
	EnvLifecycleBatchSize             = "PAMIE_LIFECYCLE_BATCH_SIZE"
	EnvLifecycleRunOnStart            = "PAMIE_LIFECYCLE_RUN_ON_START"
	EnvLifecycleStartupDelay          = "PAMIE_LIFECYCLE_STARTUP_DELAY"
	EnvVectorSearchEnabled            = "PAMIE_VECTOR_SEARCH_ENABLED"
	EnvVectorBackend                  = "PAMIE_VECTOR_BACKEND"
	EnvVectorProvider                 = "PAMIE_VECTOR_PROVIDER"
	EnvVectorModel                    = "PAMIE_VECTOR_MODEL"
	EnvVectorDimensions               = "PAMIE_VECTOR_DIMENSIONS"
	EnvVectorEmbeddingScope           = "PAMIE_VECTOR_EMBEDDING_SCOPE"
	EnvVectorOllamaURL                = "PAMIE_VECTOR_OLLAMA_URL"
	EnvVectorOllamaKeepAlive          = "PAMIE_VECTOR_OLLAMA_KEEP_ALIVE"
	EnvVectorOllamaAutostart          = "PAMIE_VECTOR_OLLAMA_AUTOSTART"
	EnvVectorOllamaCommand            = "PAMIE_VECTOR_OLLAMA_COMMAND"
	EnvVectorOllamaStartupTimeout     = "PAMIE_VECTOR_OLLAMA_STARTUP_TIMEOUT"
	defaultAddr                       = "127.0.0.1:17683"
	defaultTokenID                    = "default"
	defaultTokenScopes                = "all"
	defaultDataDir                    = "data"
	defaultDatabaseName               = "pamie.db"
	defaultLogLevel                   = "info"
	defaultReadHeaderTime             = 5 * time.Second
	defaultShutdownTimeout            = 10 * time.Second
	defaultMCPRateLimit               = 120
	defaultMCPRateBurst               = 30
	defaultLifecycleInterval          = time.Hour
	defaultLifecycleBatchSize         = 500
	defaultVectorBackend              = "auto"
	defaultVectorProvider             = "local-hash"
	defaultVectorModel                = "embeddinggemma"
	defaultVectorDimensions           = 384
	defaultVectorEmbeddingScope       = "title_keywords"
	defaultVectorOllamaURL            = "http://127.0.0.1:11434"
	defaultVectorOllamaCommand        = "ollama"
	defaultVectorOllamaStartupTimeout = 10 * time.Second
)

// Config contains process configuration parsed during startup.
type Config struct {
	ShowVersion                bool
	Addr                       string
	BearerToken                string
	BearerTokenID              string
	BearerTokenScopes          string
	DataDir                    string
	DatabasePath               string
	LogLevel                   string
	ReadHeaderTimeout          time.Duration
	ShutdownTimeout            time.Duration
	MCPRateLimit               int
	MCPRateBurst               int
	LifecycleEnabled           bool
	LifecycleInterval          time.Duration
	LifecycleBatchSize         int
	LifecycleRunOnStart        bool
	LifecycleStartupDelay      time.Duration
	VectorSearchEnabled        bool
	VectorBackend              string
	VectorProvider             string
	VectorModel                string
	VectorDimensions           int
	VectorEmbeddingScope       string
	VectorOllamaURL            string
	VectorOllamaKeepAlive      string
	VectorOllamaAutostart      bool
	VectorOllamaCommand        string
	VectorOllamaStartupTimeout time.Duration
}

// Default returns safe local development defaults.
func Default() Config {
	return Config{
		Addr:                       defaultAddr,
		BearerTokenID:              defaultTokenID,
		BearerTokenScopes:          defaultTokenScopes,
		DataDir:                    defaultDataDir,
		DatabasePath:               filepath.Join(defaultDataDir, defaultDatabaseName),
		LogLevel:                   defaultLogLevel,
		ReadHeaderTimeout:          defaultReadHeaderTime,
		ShutdownTimeout:            defaultShutdownTimeout,
		MCPRateLimit:               defaultMCPRateLimit,
		MCPRateBurst:               defaultMCPRateBurst,
		LifecycleInterval:          defaultLifecycleInterval,
		LifecycleBatchSize:         defaultLifecycleBatchSize,
		VectorSearchEnabled:        true,
		VectorBackend:              defaultVectorBackend,
		VectorProvider:             defaultVectorProvider,
		VectorModel:                defaultVectorModel,
		VectorDimensions:           defaultVectorDimensions,
		VectorEmbeddingScope:       defaultVectorEmbeddingScope,
		VectorOllamaURL:            defaultVectorOllamaURL,
		VectorOllamaCommand:        defaultVectorOllamaCommand,
		VectorOllamaStartupTimeout: defaultVectorOllamaStartupTimeout,
	}
}

// Load parses configuration from environment and command-line flags.
// Environment values establish defaults and explicit flags take precedence.
func Load(args []string, getenv func(string) string, output io.Writer) (Config, error) {
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	if output == nil {
		output = io.Discard
	}

	cfg := Default()
	databasePathExplicit := false
	if value := getenv(EnvAddr); value != "" {
		cfg.Addr = value
	}
	if value := getenv(EnvToken); value != "" {
		cfg.BearerToken = value
	}
	if value := getenv(EnvTokenID); value != "" {
		cfg.BearerTokenID = value
	}
	if value := getenv(EnvTokenScopes); value != "" {
		cfg.BearerTokenScopes = value
	}
	if value := getenv(EnvDataDir); value != "" {
		cfg.DataDir = value
	}
	if value := getenv(EnvDatabasePath); value != "" {
		cfg.DatabasePath = value
		databasePathExplicit = true
	}
	if value := getenv(EnvLogLevel); value != "" {
		cfg.LogLevel = value
	}
	if value := getenv(EnvReadHeaderTimeout); value != "" {
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return Config{}, fmt.Errorf("%s: %w", EnvReadHeaderTimeout, err)
		}
		cfg.ReadHeaderTimeout = parsed
	}
	if value := getenv(EnvShutdownTimeout); value != "" {
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return Config{}, fmt.Errorf("%s: %w", EnvShutdownTimeout, err)
		}
		cfg.ShutdownTimeout = parsed
	}
	if value := getenv(EnvMCPRateLimit); value != "" {
		parsed, err := parseNonNegativeInt(value)
		if err != nil {
			return Config{}, fmt.Errorf("%s: %w", EnvMCPRateLimit, err)
		}
		cfg.MCPRateLimit = parsed
	}
	if value := getenv(EnvMCPRateBurst); value != "" {
		parsed, err := parseNonNegativeInt(value)
		if err != nil {
			return Config{}, fmt.Errorf("%s: %w", EnvMCPRateBurst, err)
		}
		cfg.MCPRateBurst = parsed
	}
	if value := getenv(EnvLifecycleEnabled); value != "" {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return Config{}, fmt.Errorf("%s: %w", EnvLifecycleEnabled, err)
		}
		cfg.LifecycleEnabled = parsed
	}
	if value := getenv(EnvLifecycleInterval); value != "" {
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return Config{}, fmt.Errorf("%s: %w", EnvLifecycleInterval, err)
		}
		cfg.LifecycleInterval = parsed
	}
	if value := getenv(EnvLifecycleBatchSize); value != "" {
		parsed, err := parseNonNegativeInt(value)
		if err != nil {
			return Config{}, fmt.Errorf("%s: %w", EnvLifecycleBatchSize, err)
		}
		cfg.LifecycleBatchSize = parsed
	}
	if value := getenv(EnvLifecycleRunOnStart); value != "" {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return Config{}, fmt.Errorf("%s: %w", EnvLifecycleRunOnStart, err)
		}
		cfg.LifecycleRunOnStart = parsed
	}
	if value := getenv(EnvLifecycleStartupDelay); value != "" {
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return Config{}, fmt.Errorf("%s: %w", EnvLifecycleStartupDelay, err)
		}
		cfg.LifecycleStartupDelay = parsed
	}
	if value := getenv(EnvVectorSearchEnabled); value != "" {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return Config{}, fmt.Errorf("%s: %w", EnvVectorSearchEnabled, err)
		}
		cfg.VectorSearchEnabled = parsed
	}
	if value := getenv(EnvVectorBackend); value != "" {
		cfg.VectorBackend = value
	}
	if value := getenv(EnvVectorProvider); value != "" {
		cfg.VectorProvider = value
	}
	if value := getenv(EnvVectorModel); value != "" {
		cfg.VectorModel = value
	}
	if value := getenv(EnvVectorDimensions); value != "" {
		parsed, err := parseNonNegativeInt(value)
		if err != nil {
			return Config{}, fmt.Errorf("%s: %w", EnvVectorDimensions, err)
		}
		cfg.VectorDimensions = parsed
	}
	if value := getenv(EnvVectorEmbeddingScope); value != "" {
		cfg.VectorEmbeddingScope = value
	}
	if value := getenv(EnvVectorOllamaURL); value != "" {
		cfg.VectorOllamaURL = value
	}
	if value := getenv(EnvVectorOllamaKeepAlive); value != "" {
		cfg.VectorOllamaKeepAlive = value
	}
	if value := getenv(EnvVectorOllamaAutostart); value != "" {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return Config{}, fmt.Errorf("%s: %w", EnvVectorOllamaAutostart, err)
		}
		cfg.VectorOllamaAutostart = parsed
	}
	if value := getenv(EnvVectorOllamaCommand); value != "" {
		cfg.VectorOllamaCommand = value
	}
	if value := getenv(EnvVectorOllamaStartupTimeout); value != "" {
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return Config{}, fmt.Errorf("%s: %w", EnvVectorOllamaStartupTimeout, err)
		}
		cfg.VectorOllamaStartupTimeout = parsed
	}

	fs := flag.NewFlagSet("pamie", flag.ContinueOnError)
	fs.SetOutput(output)
	fs.BoolVar(&cfg.ShowVersion, "version", cfg.ShowVersion, "print version and exit")
	fs.StringVar(&cfg.Addr, "addr", cfg.Addr, "HTTP listen address")
	fs.StringVar(&cfg.BearerToken, "token", cfg.BearerToken, "Bearer token for protected MCP endpoint")
	fs.StringVar(&cfg.BearerTokenID, "token-id", cfg.BearerTokenID, "identifier used for the configured Bearer token in audit logs")
	fs.StringVar(&cfg.BearerTokenScopes, "token-scopes", cfg.BearerTokenScopes, "comma-separated scopes for the configured Bearer token, or all")
	fs.StringVar(&cfg.DataDir, "data-dir", cfg.DataDir, "directory for local Pamie data")
	fs.StringVar(&cfg.DatabasePath, "db-path", cfg.DatabasePath, "SQLite database path")
	fs.StringVar(&cfg.LogLevel, "log-level", cfg.LogLevel, "log level: debug, info, warn, or error")
	fs.DurationVar(&cfg.ReadHeaderTimeout, "read-header-timeout", cfg.ReadHeaderTimeout, "HTTP read header timeout")
	fs.DurationVar(&cfg.ShutdownTimeout, "shutdown-timeout", cfg.ShutdownTimeout, "graceful shutdown timeout")
	fs.IntVar(&cfg.MCPRateLimit, "mcp-rate-limit", cfg.MCPRateLimit, "per-client /mcp requests per minute; 0 disables rate limiting")
	fs.IntVar(&cfg.MCPRateBurst, "mcp-rate-burst", cfg.MCPRateBurst, "per-client /mcp rate limit burst")
	fs.BoolVar(&cfg.LifecycleEnabled, "lifecycle-worker", cfg.LifecycleEnabled, "enable scheduled lifecycle evaluation")
	fs.DurationVar(&cfg.LifecycleInterval, "lifecycle-interval", cfg.LifecycleInterval, "interval between scheduled lifecycle evaluations")
	fs.IntVar(&cfg.LifecycleBatchSize, "lifecycle-batch-size", cfg.LifecycleBatchSize, "maximum memories evaluated per lifecycle run")
	fs.BoolVar(&cfg.LifecycleRunOnStart, "lifecycle-run-on-start", cfg.LifecycleRunOnStart, "run lifecycle evaluation as soon as the worker starts")
	fs.DurationVar(&cfg.LifecycleStartupDelay, "lifecycle-startup-delay", cfg.LifecycleStartupDelay, "delay before the first lifecycle run when lifecycle-run-on-start is false")
	fs.BoolVar(&cfg.VectorSearchEnabled, "vector-search", cfg.VectorSearchEnabled, "enable local vector embedding storage and hybrid search")
	fs.StringVar(&cfg.VectorBackend, "vector-backend", cfg.VectorBackend, "local vector backend: auto, sqlite-json, or sqlite-vec")
	fs.StringVar(&cfg.VectorProvider, "vector-provider", cfg.VectorProvider, "local embedding provider: local-hash or ollama")
	fs.StringVar(&cfg.VectorModel, "vector-model", cfg.VectorModel, "embedding model name for providers that require one")
	fs.IntVar(&cfg.VectorDimensions, "vector-dimensions", cfg.VectorDimensions, "embedding dimensions for the local vector provider")
	fs.StringVar(&cfg.VectorEmbeddingScope, "vector-embedding-scope", cfg.VectorEmbeddingScope, "embedding scope: title_keywords")
	fs.StringVar(&cfg.VectorOllamaURL, "vector-ollama-url", cfg.VectorOllamaURL, "base URL for a local Ollama server")
	fs.StringVar(&cfg.VectorOllamaKeepAlive, "vector-ollama-keep-alive", cfg.VectorOllamaKeepAlive, "Ollama keep_alive value for loaded embedding models")
	fs.BoolVar(&cfg.VectorOllamaAutostart, "vector-ollama-autostart", cfg.VectorOllamaAutostart, "start local `ollama serve` when vector provider is ollama and Ollama is unavailable")
	fs.StringVar(&cfg.VectorOllamaCommand, "vector-ollama-command", cfg.VectorOllamaCommand, "command used for Ollama autostart")
	fs.DurationVar(&cfg.VectorOllamaStartupTimeout, "vector-ollama-startup-timeout", cfg.VectorOllamaStartupTimeout, "maximum wait for Ollama autostart readiness")

	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	if fs.NArg() > 0 {
		return Config{}, fmt.Errorf("unexpected arguments: %v", fs.Args())
	}
	fs.Visit(func(flag *flag.Flag) {
		if flag.Name == "db-path" {
			databasePathExplicit = true
		}
	})
	if !databasePathExplicit {
		cfg.DatabasePath = filepath.Join(cfg.DataDir, defaultDatabaseName)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate checks configuration invariants that must hold before startup.
func (c Config) Validate() error {
	if strings.TrimSpace(c.Addr) == "" {
		return errors.New("addr must not be empty")
	}
	if strings.TrimSpace(c.DataDir) == "" {
		return errors.New("data dir must not be empty")
	}
	if strings.TrimSpace(c.DatabasePath) == "" {
		return errors.New("database path must not be empty")
	}
	if strings.TrimSpace(c.LogLevel) == "" {
		return errors.New("log level must not be empty")
	}
	switch strings.ToLower(c.LogLevel) {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("unsupported log level %q", c.LogLevel)
	}
	if c.BearerToken != "" && strings.TrimSpace(c.BearerToken) != c.BearerToken {
		return errors.New("bearer token must not have leading or trailing whitespace")
	}
	if strings.TrimSpace(c.BearerTokenID) == "" {
		return errors.New("bearer token id must not be empty")
	}
	if strings.TrimSpace(c.BearerTokenScopes) == "" {
		return errors.New("bearer token scopes must not be empty")
	}
	if c.ReadHeaderTimeout <= 0 {
		return errors.New("read header timeout must be positive")
	}
	if c.ShutdownTimeout <= 0 {
		return errors.New("shutdown timeout must be positive")
	}
	if c.MCPRateLimit < 0 {
		return errors.New("mcp rate limit must not be negative")
	}
	if c.MCPRateBurst < 0 {
		return errors.New("mcp rate limit burst must not be negative")
	}
	if c.MCPRateLimit > 0 && c.MCPRateBurst == 0 {
		return errors.New("mcp rate limit burst must be positive when rate limit is enabled")
	}
	if c.LifecycleInterval <= 0 {
		return errors.New("lifecycle interval must be positive")
	}
	if c.LifecycleBatchSize <= 0 {
		return errors.New("lifecycle batch size must be positive")
	}
	if c.LifecycleStartupDelay < 0 {
		return errors.New("lifecycle startup delay must not be negative")
	}
	switch c.VectorBackend {
	case "auto", "sqlite-json", "sqlite-vec":
	default:
		return fmt.Errorf("unsupported vector backend %q", c.VectorBackend)
	}
	if strings.TrimSpace(c.VectorProvider) == "" {
		return errors.New("vector provider must not be empty")
	}
	switch c.VectorProvider {
	case "local-hash", "ollama":
	default:
		return fmt.Errorf("unsupported vector provider %q", c.VectorProvider)
	}
	if c.VectorDimensions <= 0 {
		return errors.New("vector dimensions must be positive")
	}
	switch c.VectorEmbeddingScope {
	case "title_keywords":
	default:
		return fmt.Errorf("unsupported vector embedding scope %q", c.VectorEmbeddingScope)
	}
	if c.VectorProvider == "ollama" {
		if strings.TrimSpace(c.VectorModel) == "" {
			return errors.New("vector model must not be empty for ollama provider")
		}
		if strings.TrimSpace(c.VectorOllamaURL) == "" {
			return errors.New("vector ollama URL must not be empty")
		}
	}
	if c.VectorOllamaAutostart && strings.TrimSpace(c.VectorOllamaCommand) == "" {
		return errors.New("vector ollama command must not be empty when autostart is enabled")
	}
	if c.VectorOllamaStartupTimeout <= 0 {
		return errors.New("vector ollama startup timeout must be positive")
	}
	return nil
}

func parseNonNegativeInt(value string) (int, error) {
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, err
	}
	if parsed < 0 {
		return 0, errors.New("must not be negative")
	}
	return parsed, nil
}
