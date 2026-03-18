# Reaper Goroutine Consolidation via pgx Batch Queries

## Problem Statement

The current Reaper spawns **one goroutine per (source × metric)** combination. With the `exhaustive` preset (32 metrics) and 10 sources, this produces **320 long-lived goroutines**, each independently issuing SQL queries. This causes goroutine saturation, excessive network round-trips, connection pool pressure, and scheduling jitter.

---

## Current Architecture

Each metric goroutine (`reapMetricMeasurements` in [reaper.go](internal/reaper/reaper.go#L295)) runs an infinite loop: sleep for `interval` → try OS fetch → fall back to SQL `Query()` → send result to `measurementCh`. All SQL goes through [QueryMeasurements()](internal/reaper/database.go#L17) which calls `md.Conn.Query(ctx, sql)` — one network round-trip per metric per tick. The connection is a `pgxpool.Pool` ([conn.go](internal/sources/conn.go#L41)) supporting batching, but `SendBatch` is never used.

---

## Architectural Options

### Option A: Internal GCD-Based Tick Loop (Recommended)

- One goroutine per source, ticking at `GCD(all_metric_intervals)`. On each tick: collect due metrics → build `pgx.Batch` → `SendBatch()` → dispatch results.
- **Pros:** Zero dependencies, full control, natural fit with context cancellation, simple to reason about.
- **Cons:** Must handle GCD recalculation on config changes, partial batch failure handling.

### Option B: External Scheduler (github.com/go-co-op/gocron)

- One gocron Scheduler per source, each metric as a job.
- **Pros:** Battle-tested cron semantics, built-in job lifecycle, supports cron expressions (useful for `DisabledDays`/`DisableTimes` in `MetricAttrs`).
- **Cons:** **Batching is NOT built-in** — still need custom grouping logic. gocron v2 uses one goroutine per job (defeats the purpose). New dependency. **Verdict: Overkill.** The core problem is batching queries, not scheduling complexity.

### Option C: Time-Window Batching (Collect & Flush)

- Per-source goroutine wakes every N seconds (e.g., 5s), batches all due metrics.
- **Pros:** Simpler than GCD. Naturally groups aligned metrics.
- **Cons:** Up to N seconds latency, less predictable timing.

### Option D: Hybrid — GCD Loop + Overflow Workers

- GCD main loop (Option A) + offload slow metrics (e.g., `table_bloat_approx_summary_sql`) to separate goroutine if they exceed a time threshold.
- **Pros:** Best of both worlds. **Cons:** More complex.

**Recommendation: Option A** as the starting point, with Option D's overflow mechanism as a future enhancement if needed.

---

## pgx Batch API (v5.8.0)

`pgx.Batch` queues multiple queries, `pool.SendBatch(ctx, batch)` acquires **one connection** and sends all queries via PostgreSQL's pipeline protocol. Results are read **in queue order** via `results.Query()`. Individual query errors don't abort the batch. Currently **zero usage** of Batch API in pgwatch.

**Compatibility concerns:**

- Non-Postgres sources (pgbouncer, pgpool) use `QueryExecModeSimpleProtocol` → **skip batching**, fall back to sequential
- `lock_timeout` set at connection level → inherited by batch (no change needed)
- Per-metric `StatementTimeoutSeconds` → prepend `SET LOCAL statement_timeout` in the batch

---

## Non-Batchable Metrics

| Type | Examples | Reason |
|------|----------|--------|
| OS metrics | `psutil_cpu`, `psutil_mem`, `psutil_disk`, `cpu_load` | gopsutil system calls, not SQL |
| Log parser | `server_log_event_counts` | Streaming CSV parser, keeps own goroutine |
| Change events | `change_events` | Multi-step stateful detection (5 sub-queries + state diff). *Can* be batched in Phase 4 |
| Health check | `instance_up` | Simple `Ping()`, no SQL |

---

## GCD Interval Calculation

For the `exhaustive` preset, intervals are: `[30, 60, 120, 180, 300, 600, 900, 3600, 7200]` → **GCD = 30 seconds**. At each tick, only "due" metrics fire. Peak batch at t=60s: ~12 SQL queries in one round-trip instead of 12 separate connections.

---

## Impact

| Scenario | Current Goroutines | Proposed | Reduction |
|----------|-------------------|----------|-----------|
| 10 sources × exhaustive (32 metrics) | 320 | 10 | **97%** |
| 50 sources × exhaustive | 1600 | 50 | **97%** |
| 1 source × basic (4 metrics) | 4 | 1 | **75%** |

Network round-trips at 60s alignment: 12→1 = **~92% reduction**.

---

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| Slow query blocks entire batch | Timeout per batch (80% of tick interval). Overflow to separate goroutine for known slow metrics. |
| Partial batch failure | pgx `BatchResults` lets each result be read independently. Log error, continue. |
| GCD too small (coprime intervals → GCD=1) | Floor GCD to minimum 5s |
| Config hot-reload | Recalculate GCD + update schedule map via config channel / atomic pointer |
| Non-Postgres sources | Detect via `IsPostgresSource()` → sequential simple-protocol queries |

---

## Development Roadmap

### Phase 1: Core Infrastructure *(blocking)*

1. Define `SourceReaper` struct — per-source state: `md`, `metricSchedule`, `tickInterval`, `nonBatchableMetrics`
2. Implement GCD calculator with 5s floor
3. Implement `isDue(metric, now)` check
4. Add `SendBatch(ctx, *pgx.Batch) pgx.BatchResults` to `PgxPoolIface` in [internal/db/conn.go](internal/db/conn.go)

- New file: `internal/reaper/source_reaper.go`
- Modified: [internal/db/conn.go](internal/db/conn.go)
- **Verify:** Unit tests for GCD, due-check logic

### Phase 2: Batch Query Execution *(depends on Phase 1)*

1. Implement `buildBatch(dueMetrics)` — queue SQL, track metric→position mapping, skip non-batchable
2. Implement `processBatchResults(results, metricOrder)` — per-result: `CollectRows` → `MeasurementEnvelope` → `measurementCh`
3. Preserve: server restart detection, instance cache, `AddSysinfoToMeasurements`, primary/standby filtering

- Modified: `internal/reaper/source_reaper.go`, [internal/reaper/database.go](internal/reaper/database.go)
- **Verify:** Integration test with mock pgxpool

### Phase 3: Main Loop Integration *(depends on Phase 2)*

1. Refactor [Reap()](internal/reaper/reaper.go#L74) — spawn `go sourceReaper.Run(ctx)` per source instead of per-metric goroutines
2. `cancelFuncs` simplifies from `[db+metric]` to `[db]`
3. Dynamic reconfiguration via config channel to `SourceReaper`
4. Simplify `ShutdownOldWorkers`

- Modified: [internal/reaper/reaper.go](internal/reaper/reaper.go)
- **Verify:** `runtime.NumGoroutine()` reduction, hot-reload test, metric output comparison

### Phase 4: Change Detection Batching *(parallel with Phase 5)*

1. Batch the 5 hash queries (`sproc_hashes`, `table_hashes`, `index_hashes`, `configuration_hashes`, `privilege_changes`) into one `pgx.Batch`
2. Refactor `Detect*Changes` methods to accept pre-fetched data

- Modified: [internal/reaper/database.go](internal/reaper/database.go)
- **Verify:** Unit tests with mock data

### Phase 5: Cleanup & Observability *(parallel with Phase 4)*

1. Remove old `reapMetricMeasurements()` and per-metric cancel management
2. Add batch size histogram, execution time, per-metric latency metrics
3. Edge cases: graceful fallback if `SendBatch` fails, `go test -race`, goleak

---

## Key Decisions

- **No external scheduler** — gocron adds goroutine-per-job overhead that defeats the purpose
- **pgx Batch only for Postgres sources** — pgbouncer/pgpool stay sequential
- **Non-batchable metrics execute inline** in the source goroutine (OS metrics are fast)
- **`server_log_event_counts` keeps its own goroutine** (streaming parser)
- **Minimum tick interval: 5 seconds** to prevent excessive wake-ups
- **Phases 1→3 are sequential; Phases 4-5 can run in parallel**
