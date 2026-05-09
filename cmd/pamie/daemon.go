// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/your-org/pamie/internal/config"
	"github.com/your-org/pamie/internal/db"
)

const (
	daemonStateFile = "pamie.pid"
	daemonLogFile   = "pamie.log"
)

type daemonState struct {
	PID          int       `json:"pid"`
	Addr         string    `json:"addr"`
	DataDir      string    `json:"data_dir"`
	DatabasePath string    `json:"database_path"`
	LogPath      string    `json:"log_path"`
	StartedAt    time.Time `json:"started_at"`
}

func runStartCommand(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) error {
	cfg, err := config.Load(args, getenvWithDefaultDataDir(getenv), stderr)
	if err != nil {
		return err
	}
	if cfg.ShowVersion {
		fmt.Fprintf(stdout, "pamie %s\n", version)
		return nil
	}
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	statePaths := daemonStatePaths(cfg.DataDir, getenv)

	if state, path, ok := runningDaemonState(statePaths); ok {
		if strings.TrimSpace(state.DatabasePath) != "" && filepath.Clean(cfg.DatabasePath) != filepath.Clean(state.DatabasePath) {
			return fmt.Errorf("pamie is already running with database %s; stop it before starting with %s", state.DatabasePath, cfg.DatabasePath)
		}
		fmt.Fprintf(stdout, "Pamie is already running.\npid: %d\naddr: %s\ndata: %s\ndatabase: %s\nlog: %s\nstate: %s\n", state.PID, state.Addr, state.DataDir, state.DatabasePath, state.LogPath, path)
		return nil
	}
	removeDaemonStateFiles(statePaths)
	if _, err := getHTTPStatusWithTimeout(ctx, cfg.Addr, "/health", 200*time.Millisecond); err == nil {
		return fmt.Errorf("address %s already has a service answering /health; run `pamie status` or choose --addr", cfg.Addr)
	}

	generatedToken, err := ensureStartupToken(ctx, cfg, getenv)
	if err != nil {
		return err
	}

	logPath := filepath.Join(cfg.DataDir, daemonLogFile)
	logFile, err := os.OpenFile(logPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open daemon log: %w", err)
	}
	defer logFile.Close()

	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}
	childArgs := append([]string{"serve"}, args...)
	cmd := exec.CommandContext(ctx, executable, childArgs...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = mergeEnv(os.Environ(), map[string]string{
		config.EnvAddr:         cfg.Addr,
		config.EnvDataDir:      cfg.DataDir,
		config.EnvDatabasePath: cfg.DatabasePath,
	})
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start pamie: %w", err)
	}
	exited := make(chan error, 1)
	go func() {
		exited <- cmd.Wait()
	}()

	state := daemonState{
		PID:          cmd.Process.Pid,
		Addr:         cfg.Addr,
		DataDir:      cfg.DataDir,
		DatabasePath: cfg.DatabasePath,
		LogPath:      logPath,
		StartedAt:    time.Now().UTC(),
	}
	if err := writeDaemonStateFiles(statePaths, state); err != nil {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		return err
	}

	health := waitForDaemonHealth(ctx, cfg.Addr, exited, 5*time.Second)
	if health != nil {
		removeDaemonStateFiles(statePaths)
		return health
	}
	fmt.Fprintln(stdout, "Pamie started in background.")
	fmt.Fprintf(stdout, "pid: %d\n", state.PID)
	fmt.Fprintf(stdout, "MCP endpoint: %s\n", localHTTPURL(cfg.Addr, "/mcp"))
	if generatedToken != "" {
		fmt.Fprintf(stdout, "Bearer token: %s\n", generatedToken)
		fmt.Fprintln(stdout, "The token is stored hashed and is shown only once. Run `pamie token` to rotate it.")
	}
	fmt.Fprintf(stdout, "data: %s\n", cfg.DataDir)
	fmt.Fprintf(stdout, "log: %s\n", logPath)
	fmt.Fprintln(stdout, "health: ok")
	return nil
}

func runStatusCommand(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dataDir := fs.String("data-dir", defaultLocalDataDir(getenv), "directory for local Pamie data")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("status: unexpected arguments: %v", fs.Args())
	}
	dataDirExplicit := flagWasSet(fs, "data-dir")

	statePaths := statusStatePaths(*dataDir, getenv, dataDirExplicit)
	state, _, ok := runningDaemonState(statePaths)
	if !ok {
		removeDaemonStateFiles(statePaths)
		fmt.Fprintln(stdout, "Pamie is stopped.")
		return nil
	}

	fmt.Fprintln(stdout, "Pamie is running.")
	fmt.Fprintf(stdout, "pid: %d\n", state.PID)
	fmt.Fprintf(stdout, "addr: %s\n", state.Addr)
	fmt.Fprintf(stdout, "MCP endpoint: %s\n", localHTTPURL(state.Addr, "/mcp"))
	fmt.Fprintf(stdout, "data: %s\n", state.DataDir)
	fmt.Fprintf(stdout, "database: %s\n", state.DatabasePath)
	fmt.Fprintf(stdout, "log: %s\n", state.LogPath)
	if status, err := getHTTPStatus(ctx, state.Addr, "/health"); err == nil {
		fmt.Fprintf(stdout, "health: %s\n", status)
	} else {
		fmt.Fprintf(stdout, "health: unavailable (%v)\n", err)
	}
	if status, err := getHTTPStatus(ctx, state.Addr, "/ready"); err == nil {
		fmt.Fprintf(stdout, "ready: %s\n", status)
	} else {
		fmt.Fprintf(stdout, "ready: unavailable (%v)\n", err)
	}
	return nil
}

func runStopCommand(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("stop", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dataDir := fs.String("data-dir", defaultLocalDataDir(getenv), "directory for local Pamie data")
	timeout := fs.Duration("timeout", 10*time.Second, "maximum time to wait for graceful stop")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("stop: unexpected arguments: %v", fs.Args())
	}
	dataDirExplicit := flagWasSet(fs, "data-dir")

	statePaths := statusStatePaths(*dataDir, getenv, dataDirExplicit)
	state, _, ok := runningDaemonState(statePaths)
	if !ok {
		removeDaemonStateFiles(statePaths)
		fmt.Fprintln(stdout, "Pamie is already stopped.")
		return nil
	}
	process, err := os.FindProcess(state.PID)
	if err != nil {
		return fmt.Errorf("find pamie process: %w", err)
	}
	if err := process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("stop pamie: %w", err)
	}

	deadline := time.Now().Add(*timeout)
	for time.Now().Before(deadline) {
		if !processRunning(state.PID) {
			removeDaemonStateFiles(statePaths)
			fmt.Fprintln(stdout, "Pamie stopped.")
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	return errors.New("pamie did not stop before timeout")
}

func ensureStartupToken(ctx context.Context, cfg config.Config, getenv func(string) string) (string, error) {
	if cfg.BearerToken != "" || getenvValue(getenv, config.EnvToken) != "" {
		return "", nil
	}
	store, err := db.Open(ctx, db.Options{Path: cfg.DatabasePath})
	if err != nil {
		return "", err
	}
	defer store.Close()
	active, err := store.Tokens().CountActive(ctx, time.Now().UTC())
	if err != nil {
		return "", err
	}
	if active > 0 {
		return "", nil
	}
	return rotateStoredToken(ctx, store, "default", "all", 0)
}

func readDaemonState(path string) (daemonState, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return daemonState{}, err
	}
	var state daemonState
	if err := json.Unmarshal(body, &state); err != nil {
		return daemonState{}, err
	}
	if state.PID <= 0 {
		return daemonState{}, errors.New("invalid pamie pid file")
	}
	return state, nil
}

func writeDaemonState(path string, state daemonState) error {
	body, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	if err := os.WriteFile(path, body, 0o600); err != nil {
		return fmt.Errorf("write pamie pid file: %w", err)
	}
	return nil
}

func writeDaemonStateFiles(paths []string, state daemonState) error {
	for _, path := range paths {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return fmt.Errorf("create state directory: %w", err)
		}
		if err := writeDaemonState(path, state); err != nil {
			return err
		}
	}
	return nil
}

func daemonStatePaths(dataDir string, getenv func(string) string) []string {
	paths := []string{
		filepath.Join(dataDir, daemonStateFile),
		filepath.Join(defaultLocalDataDir(getenv), daemonStateFile),
	}
	unique := paths[:0]
	seen := map[string]struct{}{}
	for _, path := range paths {
		clean := filepath.Clean(path)
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		unique = append(unique, clean)
	}
	return unique
}

func statusStatePaths(dataDir string, getenv func(string) string, explicit bool) []string {
	if explicit {
		return []string{filepath.Join(dataDir, daemonStateFile)}
	}
	return []string{filepath.Join(defaultLocalDataDir(getenv), daemonStateFile)}
}

func flagWasSet(fs *flag.FlagSet, name string) bool {
	found := false
	fs.Visit(func(flag *flag.Flag) {
		if flag.Name == name {
			found = true
		}
	})
	return found
}

func runningDaemonState(paths []string) (daemonState, string, bool) {
	for _, path := range paths {
		state, err := readDaemonState(path)
		if err == nil && processRunning(state.PID) {
			return state, path, true
		}
	}
	return daemonState{}, "", false
}

func removeDaemonStateFiles(paths []string) {
	for _, path := range paths {
		_ = os.Remove(path)
	}
}

func processRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

func waitForDaemonHealth(ctx context.Context, addr string, exited <-chan error, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		select {
		case err := <-exited:
			if err != nil {
				return fmt.Errorf("pamie exited during startup: %w", err)
			}
			return errors.New("pamie exited during startup")
		default:
		}
		if _, err := getHTTPStatus(ctx, addr, "/health"); err == nil {
			return nil
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	if lastErr == nil {
		lastErr = errors.New("health check timed out")
	}
	return lastErr
}

func getHTTPStatus(ctx context.Context, addr, path string) (string, error) {
	return getHTTPStatusWithTimeout(ctx, addr, path, 2*time.Second)
}

func getHTTPStatusWithTimeout(ctx context.Context, addr, path string, timeout time.Duration) (string, error) {
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, localHTTPURL(addr, path), nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.Status, nil
	}
	return "ok", nil
}

func localHTTPURL(addr, path string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "http://" + strings.TrimRight(addr, "/") + path
	}
	if host == "" || host == "0.0.0.0" || host == "::" || host == "[::]" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port) + path
}

func defaultLocalDataDir(getenv func(string) string) string {
	if value := getenvValue(getenv, config.EnvDataDir); strings.TrimSpace(value) != "" {
		return value
	}
	if value := getenvValue(getenv, "XDG_DATA_HOME"); strings.TrimSpace(value) != "" {
		return filepath.Join(value, "pamie")
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		return filepath.Join(home, ".local", "share", "pamie")
	}
	return "data"
}

func getenvWithDefaultDataDir(getenv func(string) string) func(string) string {
	return func(key string) string {
		if key == config.EnvDataDir && getenvValue(getenv, key) == "" {
			return defaultLocalDataDir(getenv)
		}
		return getenvValue(getenv, key)
	}
}

func getenvValue(getenv func(string) string, key string) string {
	if getenv == nil {
		return ""
	}
	return getenv(key)
}

func mergeEnv(base []string, values map[string]string) []string {
	merged := make([]string, 0, len(base)+len(values))
	seen := map[string]struct{}{}
	for _, item := range base {
		key, _, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		if value, replace := values[key]; replace {
			merged = append(merged, key+"="+value)
			seen[key] = struct{}{}
			continue
		}
		merged = append(merged, item)
		seen[key] = struct{}{}
	}
	for key, value := range values {
		if _, ok := seen[key]; !ok {
			merged = append(merged, key+"="+value)
		}
	}
	return merged
}
