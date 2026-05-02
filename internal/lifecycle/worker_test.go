// SPDX-License-Identifier: AGPL-3.0-only

package lifecycle

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/your-org/pamie/internal/audit"
	"github.com/your-org/pamie/internal/memory"
)

func TestWorkerRunsOnPeriodicTick(t *testing.T) {
	clock := newFakeClock()
	runner := newFakeRunner()
	worker := newTestWorker(t, Options{
		Enabled:    true,
		Interval:   time.Minute,
		BatchSize:  25,
		Runner:     runner,
		Clock:      clock,
		RunOnStart: false,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := worker.Start(ctx)

	clock.ticker.tick()
	runner.waitStarted(t)
	runner.waitCalls(t, 1)
	if got := runner.lastLimit(); got != 25 {
		t.Fatalf("lifecycle limit = %d, want 25", got)
	}

	cancel()
	waitDone(t, done)
}

func TestWorkerRunsAfterStartupDelay(t *testing.T) {
	clock := newFakeClock()
	runner := newFakeRunner()
	worker := newTestWorker(t, Options{
		Enabled:      true,
		Interval:     time.Minute,
		BatchSize:    10,
		StartupDelay: 5 * time.Second,
		Runner:       runner,
		Clock:        clock,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := worker.Start(ctx)

	if got := runner.calls(); got != 0 {
		t.Fatalf("calls before startup delay = %d, want 0", got)
	}
	clock.timer.fire()
	runner.waitStarted(t)
	runner.waitCalls(t, 1)

	cancel()
	waitDone(t, done)
}

func TestWorkerStopsGracefully(t *testing.T) {
	clock := newFakeClock()
	runner := newFakeRunner()
	runner.block = true
	worker := newTestWorker(t, Options{
		Enabled:    true,
		Interval:   time.Minute,
		BatchSize:  10,
		RunOnStart: true,
		Runner:     runner,
		Clock:      clock,
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := worker.Start(ctx)

	runner.waitStarted(t)
	cancel()
	waitDone(t, done)
	runner.waitCalls(t, 1)
}

func TestWorkerContinuesAfterFailedRun(t *testing.T) {
	clock := newFakeClock()
	runner := newFakeRunner()
	runner.setErr(errors.New("boom"))
	auditor := newCaptureAudit()
	worker := newTestWorker(t, Options{
		Enabled:   true,
		Interval:  time.Minute,
		BatchSize: 10,
		Runner:    runner,
		Clock:     clock,
		Audit:     auditor,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := worker.Start(ctx)

	clock.ticker.tick()
	auditor.waitOutcome(t, "error")
	waitIdle(t, worker)
	runner.setErr(nil)
	clock.ticker.tick()
	auditor.waitOutcome(t, "success")

	cancel()
	waitDone(t, done)
}

func TestWorkerDoesNotOverlapLifecycleRuns(t *testing.T) {
	clock := newFakeClock()
	runner := newFakeRunner()
	runner.block = true
	auditor := newCaptureAudit()
	worker := newTestWorker(t, Options{
		Enabled:   true,
		Interval:  time.Minute,
		BatchSize: 10,
		Runner:    runner,
		Clock:     clock,
		Audit:     auditor,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := worker.Start(ctx)

	clock.ticker.tick()
	runner.waitStarted(t)
	clock.ticker.tick()
	auditor.waitOutcome(t, "skipped")
	if got := runner.maxConcurrent(); got != 1 {
		t.Fatalf("max concurrent runs = %d, want 1", got)
	}
	if got := runner.calls(); got != 1 {
		t.Fatalf("calls = %d, want 1 while first run is blocked", got)
	}

	runner.release()
	cancel()
	waitDone(t, done)
}

func TestWorkerDisabled(t *testing.T) {
	worker := newTestWorker(t, Options{
		Enabled:   false,
		Interval:  time.Minute,
		BatchSize: 10,
		Clock:     newFakeClock(),
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := worker.Start(ctx)
	waitDone(t, done)
}

func TestWorkerValidation(t *testing.T) {
	tests := []struct {
		name string
		opts Options
	}{
		{name: "missing runner when enabled", opts: Options{Enabled: true, Interval: time.Minute, BatchSize: 1}},
		{name: "bad interval", opts: Options{Enabled: true, Interval: 0, BatchSize: 1, Runner: newFakeRunner()}},
		{name: "bad batch size", opts: Options{Enabled: true, Interval: time.Minute, BatchSize: 0, Runner: newFakeRunner()}},
		{name: "bad startup delay", opts: Options{Enabled: true, Interval: time.Minute, BatchSize: 1, StartupDelay: -time.Second, Runner: newFakeRunner()}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewWorker(tt.opts); err == nil {
				t.Fatal("NewWorker() error = nil, want error")
			}
		})
	}
}

func newTestWorker(t *testing.T, opts Options) *Worker {
	t.Helper()
	opts.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	if opts.Audit == nil {
		opts.Audit = audit.Noop()
	}
	worker, err := NewWorker(opts)
	if err != nil {
		t.Fatalf("NewWorker() error = %v", err)
	}
	return worker
}

type fakeRunner struct {
	mu             sync.Mutex
	callCount      int
	current        int
	highestCurrent int
	limit          int
	block          bool
	started        chan struct{}
	released       chan struct{}
	report         memory.LifecycleReport
	err            error
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{
		started:  make(chan struct{}, 10),
		released: make(chan struct{}),
		report:   memory.LifecycleReport{Evaluated: 1},
	}
}

func (r *fakeRunner) RunLifecycle(ctx context.Context, opts memory.LifecycleOptions) (memory.LifecycleReport, error) {
	r.mu.Lock()
	r.callCount++
	r.current++
	if r.current > r.highestCurrent {
		r.highestCurrent = r.current
	}
	r.limit = opts.Limit
	err := r.err
	r.mu.Unlock()

	r.started <- struct{}{}
	defer func() {
		r.mu.Lock()
		r.current--
		r.mu.Unlock()
	}()

	if r.block {
		select {
		case <-r.released:
		case <-ctx.Done():
			return memory.LifecycleReport{}, ctx.Err()
		}
	}
	if err != nil {
		return memory.LifecycleReport{}, err
	}
	return r.report, nil
}

func (r *fakeRunner) waitStarted(t *testing.T) {
	t.Helper()
	select {
	case <-r.started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for lifecycle run")
	}
}

func (r *fakeRunner) waitCalls(t *testing.T, want int) {
	t.Helper()
	deadline := time.After(time.Second)
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		if r.calls() == want {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("calls = %d, want %d", r.calls(), want)
		case <-ticker.C:
		}
	}
}

func (r *fakeRunner) calls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.callCount
}

func (r *fakeRunner) lastLimit() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.limit
}

func (r *fakeRunner) maxConcurrent() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.highestCurrent
}

func (r *fakeRunner) setErr(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.err = err
}

func (r *fakeRunner) release() {
	close(r.released)
}

type fakeClock struct {
	now    time.Time
	ticker *fakeTicker
	timer  *fakeTimer
}

func newFakeClock() *fakeClock {
	return &fakeClock{
		now:    time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC),
		ticker: &fakeTicker{ch: make(chan time.Time, 10)},
		timer:  &fakeTimer{ch: make(chan time.Time, 1)},
	}
}

func (c *fakeClock) Now() time.Time {
	return c.now
}

func (c *fakeClock) NewTicker(time.Duration) Ticker {
	return c.ticker
}

func (c *fakeClock) NewTimer(time.Duration) Timer {
	return c.timer
}

type fakeTicker struct {
	ch chan time.Time
}

func (t *fakeTicker) C() <-chan time.Time {
	return t.ch
}

func (t *fakeTicker) Stop() {}

func (t *fakeTicker) tick() {
	t.ch <- time.Now()
}

type fakeTimer struct {
	ch chan time.Time
}

func (t *fakeTimer) C() <-chan time.Time {
	return t.ch
}

func (t *fakeTimer) Stop() bool {
	return true
}

func (t *fakeTimer) fire() {
	t.ch <- time.Now()
}

type captureAudit struct {
	ch chan audit.Event
}

func newCaptureAudit() *captureAudit {
	return &captureAudit{ch: make(chan audit.Event, 10)}
}

func (c *captureAudit) Log(_ context.Context, event audit.Event) {
	c.ch <- event
}

func (c *captureAudit) waitOutcome(t *testing.T, outcome string) {
	t.Helper()
	for {
		select {
		case event := <-c.ch:
			if event.Outcome == outcome {
				return
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for audit outcome %q", outcome)
		}
	}
}

func waitDone(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for worker shutdown")
	}
}

func waitIdle(t *testing.T, worker *Worker) {
	t.Helper()
	deadline := time.After(time.Second)
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		if !worker.running.Load() {
			return
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for worker to become idle")
		case <-ticker.C:
		}
	}
}
