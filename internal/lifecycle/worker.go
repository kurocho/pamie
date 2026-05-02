// SPDX-License-Identifier: AGPL-3.0-only

// Package lifecycle runs memory lifecycle evaluation on a controlled schedule.
package lifecycle

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/your-org/pamie/internal/audit"
	"github.com/your-org/pamie/internal/memory"
)

// Runner is the lifecycle evaluation behavior required by Worker.
type Runner interface {
	RunLifecycle(context.Context, memory.LifecycleOptions) (memory.LifecycleReport, error)
}

// Ticker is the subset of time.Ticker used by Worker.
type Ticker interface {
	C() <-chan time.Time
	Stop()
}

// Timer is the subset of time.Timer used by Worker.
type Timer interface {
	C() <-chan time.Time
	Stop() bool
}

// Clock provides deterministic time sources for worker tests.
type Clock interface {
	Now() time.Time
	NewTicker(time.Duration) Ticker
	NewTimer(time.Duration) Timer
}

// Options configures a lifecycle Worker.
type Options struct {
	Enabled      bool
	Interval     time.Duration
	BatchSize    int
	RunOnStart   bool
	StartupDelay time.Duration
	Runner       Runner
	Logger       *slog.Logger
	Audit        audit.Logger
	Clock        Clock
}

// Worker runs lifecycle evaluation in the background.
type Worker struct {
	enabled      bool
	interval     time.Duration
	batchSize    int
	runOnStart   bool
	startupDelay time.Duration
	runner       Runner
	logger       *slog.Logger
	audit        audit.Logger
	clock        Clock
	running      atomic.Bool
}

// NewWorker validates options and returns a lifecycle worker.
func NewWorker(opts Options) (*Worker, error) {
	if opts.Interval <= 0 {
		return nil, errors.New("lifecycle interval must be positive")
	}
	if opts.BatchSize <= 0 {
		return nil, errors.New("lifecycle batch size must be positive")
	}
	if opts.StartupDelay < 0 {
		return nil, errors.New("lifecycle startup delay must not be negative")
	}
	if opts.Enabled && opts.Runner == nil {
		return nil, errors.New("lifecycle runner must not be nil")
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.Audit == nil {
		opts.Audit = audit.Noop()
	}
	if opts.Clock == nil {
		opts.Clock = realClock{}
	}
	return &Worker{
		enabled:      opts.Enabled,
		interval:     opts.Interval,
		batchSize:    opts.BatchSize,
		runOnStart:   opts.RunOnStart,
		startupDelay: opts.StartupDelay,
		runner:       opts.Runner,
		logger:       opts.Logger,
		audit:        opts.Audit,
		clock:        opts.Clock,
	}, nil
}

// Start runs the worker in a background goroutine and closes the returned
// channel after shutdown completes.
func (w *Worker) Start(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		w.Run(ctx)
	}()
	return done
}

// Run blocks until ctx is canceled or the disabled worker exits.
func (w *Worker) Run(ctx context.Context) {
	if !w.enabled {
		w.logger.Info("lifecycle worker disabled")
		return
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var runs sync.WaitGroup
	startRun := func(trigger string) {
		if ctx.Err() != nil {
			return
		}
		if !w.running.CompareAndSwap(false, true) {
			w.logSkipped(ctx, trigger)
			return
		}
		runs.Add(1)
		go func() {
			defer runs.Done()
			defer w.running.Store(false)
			w.runOnce(runCtx, trigger)
		}()
	}

	w.logger.Info("lifecycle worker started",
		"interval", w.interval.String(),
		"batch_size", w.batchSize,
		"run_on_start", w.runOnStart,
		"startup_delay", w.startupDelay.String(),
	)
	defer w.logger.Info("lifecycle worker stopped")

	if w.runOnStart {
		startRun("startup")
	} else if w.startupDelay > 0 {
		timer := w.clock.NewTimer(w.startupDelay)
		select {
		case <-ctx.Done():
			cancel()
			timer.Stop()
			runs.Wait()
			return
		case <-timer.C():
			startRun("startup_delay")
		}
	}

	ticker := w.clock.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			cancel()
			runs.Wait()
			return
		case <-ticker.C():
			startRun("interval")
		}
	}
}

func (w *Worker) runOnce(ctx context.Context, trigger string) {
	started := w.clock.Now()
	w.logger.Info("lifecycle worker run started",
		"trigger", trigger,
		"batch_size", w.batchSize,
	)

	report, err := w.runner.RunLifecycle(ctx, memory.LifecycleOptions{Limit: w.batchSize})
	durationMs := w.clock.Now().Sub(started).Milliseconds()
	if err != nil {
		outcome := "error"
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			outcome = "canceled"
		}
		fields := map[string]any{
			"trigger":     trigger,
			"batch_size":  w.batchSize,
			"duration_ms": durationMs,
			"error":       err.Error(),
		}
		audit.Log(ctx, w.audit, audit.Event{
			Type:    "lifecycle_worker",
			Outcome: outcome,
			Action:  "run",
			Subject: "lifecycle",
			Fields:  fields,
		})
		if outcome == "canceled" {
			w.logger.Info("lifecycle worker run canceled", auditFields(fields)...)
			return
		}
		w.logger.Error("lifecycle worker run failed", auditFields(fields)...)
		return
	}

	fields := lifecycleAuditFields(trigger, w.batchSize, durationMs, report)
	audit.Log(ctx, w.audit, audit.Event{
		Type:    "lifecycle_worker",
		Outcome: "success",
		Action:  "run",
		Subject: "lifecycle",
		Fields:  fields,
	})
	w.logger.Info("lifecycle worker run completed", auditFields(fields)...)
}

func (w *Worker) logSkipped(ctx context.Context, trigger string) {
	fields := map[string]any{
		"trigger": trigger,
		"reason":  "already_running",
	}
	audit.Log(ctx, w.audit, audit.Event{
		Type:    "lifecycle_worker",
		Outcome: "skipped",
		Action:  "run",
		Subject: "lifecycle",
		Fields:  fields,
	})
	w.logger.Warn("lifecycle worker run skipped", auditFields(fields)...)
}

func lifecycleAuditFields(trigger string, batchSize int, durationMs int64, report memory.LifecycleReport) map[string]any {
	return map[string]any{
		"trigger":           trigger,
		"batch_size":        batchSize,
		"duration_ms":       durationMs,
		"evaluated":         report.Evaluated,
		"promoted":          report.Promoted,
		"demoted":           report.Demoted,
		"archived":          report.Archived,
		"deleted":           report.Deleted,
		"skipped_pinned":    report.SkippedPinned,
		"skipped_important": report.SkippedImportant,
	}
}

func auditFields(fields map[string]any) []any {
	attrs := make([]any, 0, len(fields)*2)
	for key, value := range fields {
		attrs = append(attrs, key, value)
	}
	return attrs
}

type realClock struct{}

func (realClock) Now() time.Time {
	return time.Now().UTC()
}

func (realClock) NewTicker(interval time.Duration) Ticker {
	return realTicker{Ticker: time.NewTicker(interval)}
}

func (realClock) NewTimer(delay time.Duration) Timer {
	return realTimer{Timer: time.NewTimer(delay)}
}

type realTicker struct {
	*time.Ticker
}

func (t realTicker) C() <-chan time.Time {
	return t.Ticker.C
}

type realTimer struct {
	*time.Timer
}

func (t realTimer) C() <-chan time.Time {
	return t.Timer.C
}
