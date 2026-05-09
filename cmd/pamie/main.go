// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/your-org/pamie/internal/audit"
	"github.com/your-org/pamie/internal/auth"
	"github.com/your-org/pamie/internal/config"
	"github.com/your-org/pamie/internal/db"
	"github.com/your-org/pamie/internal/embedding"
	"github.com/your-org/pamie/internal/httpserver"
	"github.com/your-org/pamie/internal/lifecycle"
	"github.com/your-org/pamie/internal/mcp"
	"github.com/your-org/pamie/internal/memory"
	"github.com/your-org/pamie/internal/resources"
	"github.com/your-org/pamie/internal/tools"
)

var version = "dev"

const (
	operatorFormatSQLite = "sqlite"
	operatorFormatNDJSON = "ndjson"
)

func main() {
	args := os.Args[1:]
	if isTopLevelHelp(args) {
		printTopLevelUsage(os.Stdout)
		return
	}
	if len(args) == 0 {
		printTopLevelUsage(os.Stdout)
		return
	}
	if args[0] == "serve" {
		if err := runServer(args[1:], os.Getenv, os.Stdout, os.Stderr); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return
			}
			fmt.Fprintf(os.Stderr, "server error: %v\n", err)
			os.Exit(1)
		}
		return
	}
	if strings.HasPrefix(args[0], "-") {
		if err := runServer(args, os.Getenv, os.Stdout, os.Stderr); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return
			}
			fmt.Fprintf(os.Stderr, "server error: %v\n", err)
			os.Exit(1)
		}
		return
	}
	if isOperatorCommand(args[0]) {
		if err := runOperatorCommand(context.Background(), args[0], args[1:], os.Getenv, os.Stdout, os.Stderr); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return
			}
			fmt.Fprintf(os.Stderr, "operation error: %v\n", err)
			os.Exit(1)
		}
		return
	}
	fmt.Fprintf(os.Stderr, "unknown command %q\n\n", args[0])
	printTopLevelUsage(os.Stderr)
	os.Exit(2)
}

func runServer(args []string, getenv func(string) string, stdout, stderr io.Writer) error {
	cfg, err := config.Load(args, getenv, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return err
		}
		return fmt.Errorf("configuration: %w", err)
	}

	if cfg.ShowVersion {
		fmt.Fprintf(stdout, "pamie %s\n", version)
		return nil
	}

	logger, err := newLoggerTo(cfg.LogLevel, stdout)
	if err != nil {
		return fmt.Errorf("configuration: %w", err)
	}
	auditLogger := audit.NewSlogLogger(logger)

	store, err := db.Open(context.Background(), db.Options{Path: cfg.DatabasePath})
	if err != nil {
		logger.Error("failed to open database", "error", err, "path", cfg.DatabasePath)
		return err
	}
	defer func() {
		if err := store.Close(); err != nil {
			logger.Error("failed to close database", "error", err)
		}
	}()

	scopes, err := auth.ParseScopes(cfg.BearerTokenScopes)
	if err != nil {
		return errors.New("configuration: invalid bearer token scopes")
	}
	authenticator, err := auth.NewBearerAuthenticatorWithOptions(cfg.BearerToken, cfg.BearerTokenID, scopes, auditLogger)
	if err != nil {
		return errors.New("configuration: invalid bearer token")
	}
	authenticator.UseTokenSource(databaseTokenSource{repo: store.Tokens()})
	activeTokens, err := store.Tokens().CountActive(context.Background(), time.Now().UTC())
	if err != nil {
		return err
	}
	if cfg.BearerToken == "" && activeTokens == 0 {
		logger.Warn("bearer token is not configured; /mcp will reject requests")
	}

	var embeddingProvider embedding.Provider
	var ollamaRuntime *embedding.OllamaRuntime
	vectorBackend := db.VectorBackendSQLiteJSON
	if cfg.VectorSearchEnabled {
		vectorBackend, err = store.ResolveVectorBackend(context.Background(), cfg.VectorBackend)
		if err != nil {
			logger.Error("failed to configure vector backend", "error", err)
			return err
		}
		if cfg.VectorProvider == embedding.OllamaProviderName {
			ollamaRuntime, err = embedding.EnsureOllama(context.Background(), embedding.OllamaRuntimeOptions{
				BaseURL:        cfg.VectorOllamaURL,
				Autostart:      cfg.VectorOllamaAutostart,
				Command:        cfg.VectorOllamaCommand,
				StartupTimeout: cfg.VectorOllamaStartupTimeout,
				Logger:         logger,
			})
			if err != nil {
				logger.Warn("ollama runtime setup failed; vector indexing will fall back to FTS-only when embedding calls fail", "error", err)
			}
		}
		embeddingProvider, err = newEmbeddingProvider(cfg.VectorProvider, cfg.VectorModel, cfg.VectorDimensions, cfg.VectorOllamaURL, cfg.VectorOllamaKeepAlive)
		if err != nil {
			logger.Error("failed to configure vector search", "error", err)
			return err
		}
		logger.Info("vector search enabled", "provider", embeddingProvider.Name(), "model", embeddingProvider.Model(), "dimensions", embeddingProvider.Dimensions(), "backend", vectorBackend, "embedding_scope", cfg.VectorEmbeddingScope)
	}

	memoryService := memory.NewServiceWithOptions(store, memory.Options{
		EmbeddingProvider:    embeddingProvider,
		VectorSearchEnabled:  cfg.VectorSearchEnabled,
		VectorBackend:        vectorBackend,
		VectorEmbeddingScope: cfg.VectorEmbeddingScope,
	})
	toolRegistry := tools.NewRegistry(memoryService)
	resourceRegistry := resources.NewRegistry(memoryService)
	mcpHandler := mcp.NewHandler(mcp.Options{
		Version:      version,
		Instructions: resources.UsageInstructionsForEmbeddingScope(cfg.VectorEmbeddingScope, cfg.VectorSearchEnabled),
		Tools:        toolRegistry,
		Resources:    resourceRegistry,
		Logger:       logger,
		Audit:        auditLogger,
	})

	server, err := httpserver.New(httpserver.Options{
		Addr:              cfg.Addr,
		Authenticator:     authenticator,
		MCPHandler:        mcpHandler,
		Logger:            logger,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		ShutdownTimeout:   cfg.ShutdownTimeout,
		MCPRateLimit: httpserver.RateLimitOptions{
			RequestsPerMinute: cfg.MCPRateLimit,
			Burst:             cfg.MCPRateBurst,
			Audit:             auditLogger,
		},
		ReadinessChecks: []httpserver.ReadinessCheck{
			{
				Name: "sqlite",
				Check: func(ctx context.Context) error {
					return store.Ping(ctx)
				},
			},
		},
	})
	if err != nil {
		logger.Error("failed to configure server", "error", err)
		return err
	}

	var lifecycleWorker *lifecycle.Worker
	if cfg.LifecycleEnabled {
		lifecycleWorker, err = lifecycle.NewWorker(lifecycle.Options{
			Enabled:      cfg.LifecycleEnabled,
			Interval:     cfg.LifecycleInterval,
			BatchSize:    cfg.LifecycleBatchSize,
			RunOnStart:   cfg.LifecycleRunOnStart,
			StartupDelay: cfg.LifecycleStartupDelay,
			Runner:       memoryService,
			Logger:       logger,
			Audit:        auditLogger,
		})
		if err != nil {
			logger.Error("failed to configure lifecycle worker", "error", err)
			return err
		}
	}

	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithCancel(signalCtx)
	defer cancel()

	var lifecycleDone <-chan struct{}
	if lifecycleWorker != nil {
		lifecycleDone = lifecycleWorker.Start(ctx)
	}

	logger.Info("starting pamie", "version", version, "addr", cfg.Addr, "db_path", cfg.DatabasePath)
	err = server.ListenAndServe(ctx)
	cancel()
	if lifecycleDone != nil {
		<-lifecycleDone
	}
	if ollamaRuntime != nil {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		if stopErr := ollamaRuntime.Stop(stopCtx); stopErr != nil {
			logger.Warn("failed to stop ollama process started by pamie", "error", stopErr)
		}
		stopCancel()
	}
	if err != nil {
		logger.Error("server stopped with error", "error", err)
		return err
	}
	logger.Info("pamie stopped")
	return nil
}

func isTopLevelHelp(args []string) bool {
	if len(args) != 1 {
		return false
	}
	switch args[0] {
	case "-h", "--help", "help":
		return true
	default:
		return false
	}
}

func printTopLevelUsage(output io.Writer) {
	fmt.Fprint(output, `Usage:
  pamie
  pamie start [server options]
  pamie serve [server options]
  pamie status
  pamie stop
  pamie token [rotate|create|list|revoke]
  pamie backup --db-path PATH --out PATH [--format sqlite|ndjson]
  pamie restore --db-path PATH --in PATH [--format sqlite|ndjson] (--dry-run|--confirm)
  pamie embeddings backfill [--db-path PATH] [--limit N] [--embedding-scope title_keywords] [--reindex]

Commands:
  start       Start Pamie in the background.
  serve       Run Pamie in the foreground for Docker, systemd, and development.
  status      Show background process and health status.
  stop        Stop the background Pamie process.
  token       Create, rotate, list, or revoke persistent tokens.
  backup      Create a SQLite backup, or NDJSON with --format ndjson.
  restore     Validate or restore a SQLite backup, or NDJSON with --format ndjson.
  embeddings  Manage local embedding indexes.

Use "pamie <command> -h" for command-specific options.

Server options:
`)
	_, _ = config.Load([]string{"-h"}, nil, output)
}

func isOperatorCommand(command string) bool {
	switch command {
	case "start", "status", "stop", "token", "backup", "restore", "embeddings":
		return true
	default:
		return false
	}
}

func runOperatorCommand(ctx context.Context, command string, args []string, getenv func(string) string, stdout, stderr io.Writer) error {
	defaultDBPath, defaultLogLevel := operatorDefaults(getenv)
	switch command {
	case "start":
		return runStartCommand(ctx, args, getenv, stdout, stderr)
	case "status":
		return runStatusCommand(ctx, args, getenv, stdout, stderr)
	case "stop":
		return runStopCommand(ctx, args, getenv, stdout, stderr)
	case "token":
		return runTokenCommand(ctx, args, stdout, stderr, defaultDBPath)
	case "backup":
		return runBackupCommand(ctx, args, stdout, stderr, defaultDBPath, defaultLogLevel)
	case "restore":
		return runRestoreCommand(ctx, args, stdout, stderr, defaultDBPath, defaultLogLevel)
	case "embeddings":
		return runEmbeddingsCommand(ctx, args, getenv, stdout, stderr, defaultDBPath, defaultLogLevel)
	default:
		return fmt.Errorf("unknown operator command %q", command)
	}
}

func operatorDefaults(getenv func(string) string) (string, string) {
	if getenv == nil {
		getenv = os.Getenv
	}
	cfg := config.Default()
	dbPath := filepath.Join(defaultLocalDataDir(getenv), filepath.Base(cfg.DatabasePath))
	if value := getenv(config.EnvDatabasePath); value != "" {
		dbPath = value
	} else if state, _, ok := runningDaemonState(daemonStatePaths(defaultLocalDataDir(getenv), getenv)); ok && strings.TrimSpace(state.DatabasePath) != "" {
		dbPath = state.DatabasePath
	}
	logLevel := cfg.LogLevel
	if value := getenv(config.EnvLogLevel); value != "" {
		logLevel = value
	}
	return dbPath, logLevel
}

func runBackupCommand(ctx context.Context, args []string, stdout, stderr io.Writer, defaultDBPath, defaultLogLevel string) error {
	fs := flag.NewFlagSet("backup", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dbPath := fs.String("db-path", defaultDBPath, "SQLite database path")
	outPath := fs.String("out", "", "backup destination path, or - for NDJSON stdout; must not already exist")
	formatFlag := fs.String("format", operatorFormatSQLite, "backup format: sqlite or ndjson")
	logLevel := fs.String("log-level", defaultLogLevel, "log level: debug, info, warn, or error")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("backup: unexpected arguments: %v", fs.Args())
	}
	if strings.TrimSpace(*outPath) == "" {
		return errors.New("backup: --out is required")
	}
	format, err := normalizeOperatorFormat(*formatFlag)
	if err != nil {
		return err
	}
	auditLogger, err := newOperatorAuditLogger(*logLevel, stderr)
	if err != nil {
		return err
	}

	switch format {
	case operatorFormatSQLite:
		return runSQLiteBackup(ctx, *dbPath, *outPath, stdout, auditLogger)
	case operatorFormatNDJSON:
		return runNDJSONBackup(ctx, *dbPath, *outPath, stdout, auditLogger)
	default:
		return fmt.Errorf("unsupported backup format %q", format)
	}
}

func runSQLiteBackup(ctx context.Context, dbPath, outPath string, stdout io.Writer, auditLogger audit.Logger) error {
	if outPath == "-" {
		return errors.New("backup --format sqlite requires a filesystem --out path")
	}
	err := db.BackupSQLite(ctx, dbPath, outPath)
	auditOperatorEvent(ctx, auditLogger, "backup", dbPath, err, map[string]any{
		"db_path": dbPath,
		"format":  operatorFormatSQLite,
		"out":     outPath,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "backup written to %s\n", outPath)
	return nil
}

func runNDJSONBackup(ctx context.Context, dbPath, outPath string, stdout io.Writer, auditLogger audit.Logger) error {
	if err := db.RequireExistingDatabasePath(dbPath); err != nil {
		auditOperatorEvent(ctx, auditLogger, "backup", dbPath, err, map[string]any{
			"db_path": dbPath,
			"format":  operatorFormatNDJSON,
			"out":     outPath,
		})
		return err
	}

	writer, closeWriter, err := openOperatorOutput(outPath, stdout)
	if err != nil {
		auditOperatorEvent(ctx, auditLogger, "backup", dbPath, err, map[string]any{
			"db_path": dbPath,
			"format":  operatorFormatNDJSON,
			"out":     outPath,
		})
		return err
	}
	defer func() {
		_ = closeWriter()
	}()

	store, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		auditOperatorEvent(ctx, auditLogger, "backup", dbPath, err, map[string]any{
			"db_path": dbPath,
			"format":  operatorFormatNDJSON,
			"out":     outPath,
		})
		return err
	}
	defer store.Close()

	summary, err := store.ExportNDJSON(ctx, writer, db.ExportOptions{PamieVersion: version})
	if closeErr := closeWriter(); closeErr != nil && err == nil {
		err = fmt.Errorf("close output: %w", closeErr)
	}
	closeWriter = func() error { return nil }
	fields := recordCountAuditFields(dbPath, outPath, summary.Manifest.Counts)
	fields["format"] = operatorFormatNDJSON
	fields["records_sha256"] = summary.Manifest.Checksums.RecordsSHA256
	auditOperatorEvent(ctx, auditLogger, "backup", dbPath, err, fields)
	if err != nil {
		if outPath != "-" {
			_ = os.Remove(outPath)
		}
		return err
	}
	if outPath != "-" {
		fmt.Fprintf(stdout, "backup written to %s format=ndjson records_sha256=%s\n", outPath, summary.Manifest.Checksums.RecordsSHA256)
	}
	return nil
}

func runRestoreCommand(ctx context.Context, args []string, stdout, stderr io.Writer, defaultDBPath, defaultLogLevel string) error {
	fs := flag.NewFlagSet("restore", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dbPath := fs.String("db-path", defaultDBPath, "SQLite target database path")
	inPath := fs.String("in", "", "restore source path, or - for NDJSON stdin")
	formatFlag := fs.String("format", operatorFormatSQLite, "restore format: sqlite or ndjson")
	dryRun := fs.Bool("dry-run", false, "validate the restore without committing rows")
	confirm := fs.Bool("confirm", false, "commit the restore")
	logLevel := fs.String("log-level", defaultLogLevel, "log level: debug, info, warn, or error")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("restore: unexpected arguments: %v", fs.Args())
	}
	if *dryRun == *confirm {
		return errors.New("restore requires exactly one of --dry-run or --confirm")
	}
	if strings.TrimSpace(*inPath) == "" {
		return errors.New("restore: --in is required")
	}
	format, err := normalizeOperatorFormat(*formatFlag)
	if err != nil {
		return err
	}
	auditLogger, err := newOperatorAuditLogger(*logLevel, stderr)
	if err != nil {
		return err
	}

	switch format {
	case operatorFormatSQLite:
		return runSQLiteRestore(ctx, *dbPath, *inPath, *dryRun, stdout, auditLogger)
	case operatorFormatNDJSON:
		return runNDJSONRestore(ctx, *dbPath, *inPath, *dryRun, stdout, auditLogger)
	default:
		return fmt.Errorf("unsupported restore format %q", format)
	}
}

func runSQLiteRestore(ctx context.Context, dbPath, inPath string, dryRun bool, stdout io.Writer, auditLogger audit.Logger) error {
	if inPath == "-" {
		return errors.New("restore --format sqlite requires a filesystem --in path")
	}
	var err error
	if dryRun {
		err = db.ValidateSQLiteDatabase(ctx, inPath)
	} else {
		err = db.RestoreSQLiteBackup(ctx, inPath, dbPath)
	}
	auditOperatorEvent(ctx, auditLogger, restoreAuditAction(dryRun), dbPath, err, map[string]any{
		"artifact_path": inPath,
		"db_path":       dbPath,
		"dry_run":       dryRun,
		"format":        operatorFormatSQLite,
	})
	if err != nil {
		return err
	}
	if dryRun {
		fmt.Fprintf(stdout, "restore validated: format=sqlite source=%s\n", inPath)
		return nil
	}
	fmt.Fprintf(stdout, "restore committed: format=sqlite path=%s\n", dbPath)
	return nil
}

func runNDJSONRestore(ctx context.Context, dbPath, inPath string, dryRun bool, stdout io.Writer, auditLogger audit.Logger) error {
	reader, closeReader, err := openOperatorInput(inPath, os.Stdin)
	if err != nil {
		auditOperatorEvent(ctx, auditLogger, restoreAuditAction(dryRun), dbPath, err, map[string]any{
			"db_path": dbPath,
			"dry_run": dryRun,
			"format":  operatorFormatNDJSON,
			"in":      inPath,
		})
		return err
	}
	defer closeReader()

	store, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		auditOperatorEvent(ctx, auditLogger, restoreAuditAction(dryRun), dbPath, err, map[string]any{
			"db_path": dbPath,
			"dry_run": dryRun,
			"format":  operatorFormatNDJSON,
			"in":      inPath,
		})
		return err
	}
	defer store.Close()

	summary, err := store.ImportNDJSON(ctx, reader, db.ImportOptions{DryRun: dryRun})
	fields := recordCountAuditFields(dbPath, inPath, summary.Counts)
	fields["dry_run"] = dryRun
	fields["format"] = operatorFormatNDJSON
	auditOperatorEvent(ctx, auditLogger, restoreAuditAction(dryRun), dbPath, err, fields)
	if err != nil {
		return err
	}
	if dryRun {
		fmt.Fprintf(stdout, "restore validated: format=ndjson memory_items=%d memory_chunks=%d memory_keywords=%d memory_events=%d retention_policies=%d access_logs=%d\n",
			summary.Counts.MemoryItems,
			summary.Counts.MemoryChunks,
			summary.Counts.MemoryKeywords,
			summary.Counts.MemoryEvents,
			summary.Counts.RetentionPolicies,
			summary.Counts.AccessLogs,
		)
		return nil
	}
	fmt.Fprintf(stdout, "restore committed: format=ndjson memory_items=%d memory_chunks=%d memory_keywords=%d memory_events=%d retention_policies=%d access_logs=%d\n",
		summary.Counts.MemoryItems,
		summary.Counts.MemoryChunks,
		summary.Counts.MemoryKeywords,
		summary.Counts.MemoryEvents,
		summary.Counts.RetentionPolicies,
		summary.Counts.AccessLogs,
	)
	return nil
}

func runEmbeddingsCommand(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer, defaultDBPath, defaultLogLevel string) error {
	if len(args) == 0 {
		return errors.New("embeddings: subcommand is required; supported subcommand: backfill")
	}
	switch args[0] {
	case "-h", "--help", "help":
		fmt.Fprint(stdout, `Usage:
  pamie embeddings backfill [options]

Subcommands:
  backfill  Index missing title/keywords embeddings, or all embeddings with --reindex.
`)
		return nil
	case "backfill":
		return runEmbeddingsBackfillCommand(ctx, args[1:], getenv, stdout, stderr, defaultDBPath, defaultLogLevel)
	default:
		return fmt.Errorf("embeddings: unknown subcommand %q", args[0])
	}
}

func runEmbeddingsBackfillCommand(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer, defaultDBPath, defaultLogLevel string) error {
	defaults := config.Default()
	if loaded, err := config.Load(nil, getenv, io.Discard); err == nil {
		defaults = loaded
	}
	if defaultDBPath != "" {
		defaults.DatabasePath = defaultDBPath
	}
	if defaultLogLevel != "" {
		defaults.LogLevel = defaultLogLevel
	}

	fs := flag.NewFlagSet("embeddings backfill", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dbPath := fs.String("db-path", defaults.DatabasePath, "SQLite database path")
	limit := fs.Int("limit", 500, "maximum chunks to index in this run")
	reindex := fs.Bool("reindex", false, "recompute embeddings even when rows already exist")
	providerName := fs.String("provider", defaults.VectorProvider, "embedding provider: local-hash or ollama")
	model := fs.String("model", defaults.VectorModel, "embedding model for providers that require one")
	dimensions := fs.Int("dimensions", defaults.VectorDimensions, "embedding vector dimensions")
	embeddingScope := fs.String("embedding-scope", defaults.VectorEmbeddingScope, "embedding scope: title_keywords")
	backend := fs.String("backend", defaults.VectorBackend, "vector backend: auto, sqlite-json, or sqlite-vec")
	ollamaURL := fs.String("ollama-url", defaults.VectorOllamaURL, "base URL for a local Ollama server")
	ollamaKeepAlive := fs.String("ollama-keep-alive", defaults.VectorOllamaKeepAlive, "Ollama keep_alive value")
	logLevel := fs.String("log-level", defaults.LogLevel, "log level: debug, info, warn, or error")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("embeddings backfill: unexpected arguments: %v", fs.Args())
	}
	if *limit <= 0 {
		return errors.New("embeddings backfill: --limit must be positive")
	}
	auditLogger, err := newOperatorAuditLogger(*logLevel, stderr)
	if err != nil {
		return err
	}
	store, err := db.Open(ctx, db.Options{Path: *dbPath})
	if err != nil {
		auditOperatorEvent(ctx, auditLogger, "embeddings_backfill", *dbPath, err, map[string]any{"db_path": *dbPath})
		return err
	}
	defer store.Close()

	resolvedBackend, err := store.ResolveVectorBackend(ctx, *backend)
	if err != nil {
		auditOperatorEvent(ctx, auditLogger, "embeddings_backfill", *dbPath, err, map[string]any{"db_path": *dbPath, "backend": *backend})
		return err
	}
	provider, err := newEmbeddingProvider(*providerName, *model, *dimensions, *ollamaURL, *ollamaKeepAlive)
	if err != nil {
		auditOperatorEvent(ctx, auditLogger, "embeddings_backfill", *dbPath, err, map[string]any{"db_path": *dbPath, "provider": *providerName})
		return err
	}
	service := memory.NewServiceWithOptions(store, memory.Options{
		EmbeddingProvider:    provider,
		VectorSearchEnabled:  true,
		VectorBackend:        resolvedBackend,
		VectorEmbeddingScope: *embeddingScope,
	})
	var result memory.EmbeddingBackfillResult
	if *reindex {
		result, err = service.ReindexEmbeddings(ctx, *limit)
	} else {
		result, err = service.BackfillEmbeddings(ctx, *limit)
	}
	fields := map[string]any{
		"db_path":    *dbPath,
		"provider":   provider.Name(),
		"model":      provider.Model(),
		"dimensions": provider.Dimensions(),
		"backend":    resolvedBackend,
		"scope":      *embeddingScope,
		"limit":      *limit,
		"reindex":    *reindex,
		"scanned":    result.Scanned,
		"indexed":    result.Indexed,
		"failed":     result.Failed,
		"skipped":    result.Skipped,
	}
	auditOperatorEvent(ctx, auditLogger, "embeddings_backfill", *dbPath, err, fields)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "embeddings backfill completed: scanned=%d indexed=%d failed=%d skipped=%d provider=%s model=%s dimensions=%d backend=%s scope=%s reindex=%t\n",
		result.Scanned,
		result.Indexed,
		result.Failed,
		result.Skipped,
		provider.Name(),
		provider.Model(),
		provider.Dimensions(),
		resolvedBackend,
		*embeddingScope,
		*reindex,
	)
	return nil
}

func newOperatorAuditLogger(level string, output io.Writer) (audit.Logger, error) {
	logger, err := newLoggerTo(level, output)
	if err != nil {
		return nil, err
	}
	return audit.NewSlogLogger(logger), nil
}

func newEmbeddingProvider(providerName, model string, dimensions int, ollamaURL, ollamaKeepAlive string) (embedding.Provider, error) {
	switch providerName {
	case embedding.LocalHashProviderName:
		return embedding.NewLocalHashProvider(dimensions)
	case embedding.OllamaProviderName:
		return embedding.NewOllamaProvider(embedding.OllamaOptions{
			BaseURL:    ollamaURL,
			Model:      model,
			Dimensions: dimensions,
			KeepAlive:  ollamaKeepAlive,
		})
	default:
		return nil, fmt.Errorf("unsupported vector provider %q", providerName)
	}
}

func normalizeOperatorFormat(format string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", operatorFormatSQLite:
		return operatorFormatSQLite, nil
	case operatorFormatNDJSON:
		return operatorFormatNDJSON, nil
	default:
		return "", fmt.Errorf("unsupported format %q; expected sqlite or ndjson", format)
	}
}

func openOperatorOutput(path string, stdout io.Writer) (io.Writer, func() error, error) {
	if path == "-" {
		return stdout, func() error { return nil }, nil
	}
	if strings.TrimSpace(path) == "" {
		return nil, nil, errors.New("output path must not be empty")
	}
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, nil, fmt.Errorf("create output directory: %w", err)
		}
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, nil, fmt.Errorf("open output: %w", err)
	}
	return file, file.Close, nil
}

func openOperatorInput(path string, stdin io.Reader) (io.Reader, func() error, error) {
	if path == "-" {
		return stdin, func() error { return nil }, nil
	}
	if strings.TrimSpace(path) == "" {
		return nil, nil, errors.New("input path must not be empty")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("open input: %w", err)
	}
	return file, file.Close, nil
}

func auditOperatorEvent(ctx context.Context, logger audit.Logger, action, subject string, operationErr error, fields map[string]any) {
	outcome := "success"
	if operationErr != nil {
		outcome = "failure"
		if fields == nil {
			fields = map[string]any{}
		}
		fields["error"] = operationErr.Error()
	}
	audit.Log(ctx, logger, audit.Event{
		Type:    "operator",
		Outcome: outcome,
		Action:  action,
		Subject: subject,
		Fields:  fields,
	})
}

func recordCountAuditFields(dbPath, artifactPath string, counts db.ExportRecordCounts) map[string]any {
	return map[string]any{
		"db_path":            dbPath,
		"artifact_path":      artifactPath,
		"memory_items":       counts.MemoryItems,
		"memory_chunks":      counts.MemoryChunks,
		"memory_keywords":    counts.MemoryKeywords,
		"memory_events":      counts.MemoryEvents,
		"retention_policies": counts.RetentionPolicies,
		"access_logs":        counts.AccessLogs,
	}
}

func restoreAuditAction(dryRun bool) string {
	if dryRun {
		return "restore_validate"
	}
	return "restore"
}

func newLogger(level string) (*slog.Logger, error) {
	return newLoggerTo(level, os.Stdout)
}

func newLoggerTo(level string, output io.Writer) (*slog.Logger, error) {
	var slogLevel slog.Level
	switch strings.ToLower(level) {
	case "debug":
		slogLevel = slog.LevelDebug
	case "info":
		slogLevel = slog.LevelInfo
	case "warn":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	default:
		return nil, fmt.Errorf("unsupported log level %q", level)
	}
	if output == nil {
		output = io.Discard
	}

	return slog.New(slog.NewJSONHandler(output, &slog.HandlerOptions{
		Level: slogLevel,
	})), nil
}
