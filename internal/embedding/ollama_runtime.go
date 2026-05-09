// SPDX-License-Identifier: AGPL-3.0-only

package embedding

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// OllamaRuntime manages only an Ollama process started by Pamie.
type OllamaRuntime struct {
	cmd *exec.Cmd
}

// OllamaRuntimeOptions controls optional local Ollama autostart.
type OllamaRuntimeOptions struct {
	BaseURL        string
	Autostart      bool
	Command        string
	StartupTimeout time.Duration
	Logger         *slog.Logger
}

// EnsureOllama checks local Ollama and optionally starts `ollama serve`.
func EnsureOllama(ctx context.Context, opts OllamaRuntimeOptions) (*OllamaRuntime, error) {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	if ollamaHealthy(ctx, opts.BaseURL) {
		return nil, nil
	}
	if !opts.Autostart {
		logger.Warn("ollama is unavailable; vector indexing will fall back to FTS-only until it is running", "url", opts.BaseURL)
		return nil, nil
	}
	command := strings.TrimSpace(opts.Command)
	if command == "" {
		return nil, errors.New("ollama command must not be empty")
	}
	cmd := exec.CommandContext(context.Background(), command, "serve")
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		logger.Warn("failed to start ollama; vector indexing will fall back to FTS-only", "error", err)
		return nil, nil
	}
	runtime := &OllamaRuntime{cmd: cmd}
	timeout := opts.StartupTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	deadline, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		if ollamaHealthy(deadline, opts.BaseURL) {
			logger.Info("started ollama for local vector embeddings", "command", command, "url", opts.BaseURL)
			return runtime, nil
		}
		select {
		case <-deadline.Done():
			logger.Warn("ollama autostart did not become ready before timeout", "timeout", timeout)
			return runtime, nil
		case <-ticker.C:
		}
	}
}

// Stop terminates the Ollama process only if Pamie started it.
func (r *OllamaRuntime) Stop(ctx context.Context) error {
	if r == nil || r.cmd == nil || r.cmd.Process == nil {
		return nil
	}
	if err := r.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		_ = r.cmd.Process.Kill()
		return fmt.Errorf("stop ollama: %w", err)
	}
	done := make(chan error, 1)
	go func() { done <- r.cmd.Wait() }()
	select {
	case <-ctx.Done():
		_ = r.cmd.Process.Kill()
		return ctx.Err()
	case err := <-done:
		return err
	}
}

func ollamaHealthy(ctx context.Context, baseURL string) bool {
	parsed, err := url.Parse(strings.TrimRight(baseURL, "/") + "/api/tags")
	if err != nil || !parsed.IsAbs() {
		return false
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return false
	}
	client := http.Client{Timeout: 750 * time.Millisecond}
	response, err := client.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)
	return response.StatusCode >= 200 && response.StatusCode < 300
}
