# internal/lifecycle

Owner of scheduled lifecycle worker execution.

## Responsibilities

- Run `memory.RunLifecycle` on an operator-controlled schedule.
- Keep lifecycle runs non-overlapping.
- Stop through caller-provided context cancellation.
- Emit structured logs and audit events for worker runs.
- Provide injectable clocks and tickers for deterministic tests.

## Non-Responsibilities

- Defining tier or retention rules.
- Reading environment variables directly.
- Owning database connections or transactions.
- Adding distributed locking.

## Current Implementation

The worker is disabled by default in process configuration. When enabled, `cmd/pamie` starts it with the same root context used by the HTTP server. The worker can run immediately on startup, after a startup delay, and then on each interval tick. Ticks that arrive while a lifecycle run is active are skipped and audited.
