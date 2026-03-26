# Reaper Batch Consolidation — Implementation Summary

## Overview

This document summarizes the implementation of the pgwatch reaper goroutine consolidation, which replaces the previous one-goroutine-per-(source × metric) architecture with a one-goroutine-per-source model using pgx Batch queries.

---

## What Changed

### Architecture: Before vs After

| Aspect | Before | After |
|--------|--------|-------|
| Goroutine model | 1 goroutine per (source × metric) | 1 goroutine per source |
| SQL execution | 1 `Query()` call per metric per tick | 1 `SendBatch()` call per source per tick |
| Query protocol | Individual queries, each a network round-trip | pgx pipeline protocol — multiple queries in one round-trip |
| Cancel granularity | Per `source¤¤¤metric` key | Per source name |
| Config hot-reload | Cancel + re-spawn per metric goroutine | `UpdateSchedules()` on existing `SourceReaper` |

### Goroutine Reduction

| Scenario | Before | After | Reduction |
|----------|--------|-------|-----------|
| 10 sources × exhaustive (32 metrics) | 320 | 10 | **97%** |
| 50 sources × exhaustive | 1,600 | 50 | **97%** |
| 1 source × basic (4 metrics) | 4 | 1 | **75%** |

### Network Round-Trip Reduction

With the `exhaustive` preset at 60-second alignment, ~12 SQL metrics are due simultaneously.

| Before | After | Reduction |
|--------|-------|-----------|
| 12 separate `Query()` calls | 1 `SendBatch()` call | **~92%** |

At peak alignment (t = 7200s, all 32 metrics due): **32 → 1 round-trip = 97% reduction**.

---

## Implementation Phases

### Phase 1: Core Infrastructure ✅

- Added `SendBatch(ctx, *pgx.Batch) pgx.BatchResults` to `PgxPoolIface` interface
- Created `SourceReaper` struct with per-source state: metric schedules, tick interval, connection
- Implemented `GCD()` / `GCDSlice()` for computing tick interval from metric intervals
- Implemented `isDue()` check with zero-value = "never fetched" semantics
- Minimum tick interval floor: **5 seconds** (prevents excessive wake-ups for coprime intervals)

### Phase 2: Batch Query Execution ✅

- `executeBatch()`: Builds `pgx.Batch` from due metrics, sends in one round-trip, dispatches results per-metric
- Preserves: instance-level caching, primary/standby filtering, `AddSysinfoToMeasurements`, server restart detection
- `fetchSequentialMetric()`: Fallback for non-Postgres sources (pgbouncer, pgpool) using simple protocol
- `fetchOSMetric()`, `fetchSpecialMetric()`: Handle gopsutil and special metrics inline
- `BatchQueryMeasurements()`: Standalone batch helper with deterministic (sorted) key ordering

### Phase 3: Main Loop Integration ✅

- `Reap()` now spawns `go sr.Run(sourceCtx)` per source instead of per-metric goroutines
- `cancelFuncs` map simplified from `map[string]context.CancelFunc` keyed by `db¤¤¤metric` to keyed by source name
- Added `sourceReapers map[string]*SourceReaper` to `Reaper` struct
- `ShutdownOldWorkers()` simplified — only checks if source was removed from config
- Removed dead `reapMetricMeasurements()` function (was ~100 lines)

### Phase 4: Change Detection Batching ✅

- `GetObjectChangesMeasurement()` now calls `prefetchChangeDetectionData()` to batch-fetch all hash queries (`sproc_hashes`, `table_hashes`, `index_hashes`, `privilege_changes`) in one `pgx.Batch`
- Added `Detect*ChangesWithData()` variants that accept pre-fetched data, falling back to original methods if nil
- Configuration changes (`DetectConfigurationChanges`) remain unbatched — different `Scan()` pattern with typed variables

### Phase 5: Cleanup & Observability ✅

- New Prometheus metrics in `observability.go`:
  - `pgwatch_reaper_batch_size` (histogram) — queries per batch
  - `pgwatch_reaper_batch_duration_seconds` (histogram) — wall-clock time per batch
  - `pgwatch_reaper_metric_fetch_total` (counter, labels: source, status) — success/error counts
  - `pgwatch_reaper_active_source_reapers` (gauge) — currently running source reapers
- Batch timeout: 80% of tick interval, prevents slow queries from blocking the next tick

---

## Files Changed

### New Files

| File | Lines | Purpose |
|------|-------|---------|
| `internal/reaper/source_reaper.go` | 494 | SourceReaper struct, GCD, batch execution, Run loop |
| `internal/reaper/source_reaper_test.go` | 471 | pgxmock unit tests (12 test functions, 20+ subtests) |
| `internal/reaper/source_reaper_integration_test.go` | 301 | testcontainers integration tests (6 test functions) |
| `internal/reaper/observability.go` | 38 | Prometheus metrics for batch observability |

### Modified Files

| File | Changes | Purpose |
|------|---------|---------|
| `internal/db/conn.go` | +1 line | Added `SendBatch` to `PgxPoolIface` interface |
| `internal/reaper/reaper.go` | +21 / -186 lines | Per-source goroutines, simplified cancel management |
| `internal/reaper/database.go` | +303 lines | Batched change detection, `prefetchChangeDetectionData` |
| `internal/reaper/reaper_test.go` | +15 / -14 lines | Updated tests for per-source cancel pattern |

**Total: 4 new files (1,304 lines), 4 modified files (+340 / -200 net)**

---

## Test Coverage

### Unit Tests (pgxmock) — 12 test functions, 20+ subtests

| Test | What it verifies |
|------|-----------------|
| `TestGCD` | Euclidean GCD for two integers |
| `TestGCDSlice` | GCD across slices: empty, single, coprime, exhaustive preset (30s) |
| `TestCalcTickInterval` | Tick interval calculation: normal, floor to 5s, single metric, empty |
| `TestIsDue` | Never-fetched (always due), recently fetched (not due), past interval (due) |
| `TestSourceReaper_DueMetrics` | Correct partitioning of due vs not-yet-due metrics |
| `TestNewSourceReaper` | Constructor sets schedules and tick interval |
| `TestUpdateSchedules` | Hot-reload preserves lastFetch for retained metrics, purges removed |
| `TestSourceReaper_ExecuteBatch` | Full batch path: 2 metrics → pgxmock batch → 2 envelopes on channel |
| `TestSourceReaper_RunOneIteration` | Run() loop fires, collects metrics, exits on context cancel |
| `TestSourceReaper_DetectServerRestart` | Detects uptime regression → emits `object_changes` envelope |
| `TestSourceReaper_NonPostgresSequential` | Sequential fallback path for postgres source |
| `TestBatchQueryMeasurements` | Standalone batch (Postgres) and sequential (non-Postgres) paths |

### Integration Tests (testcontainers) — 6 test functions

| Test | What it verifies |
|------|-----------------|
| `TestIntegration_BatchQueryMeasurements` | 4 real SQL queries batched against Postgres 18, all return correct data |
| `TestIntegration_ExecuteBatch` | Full `executeBatch()` path with 2 metric definitions → envelopes arrive |
| `TestIntegration_SourceReaper_RunCollectsMetrics` | `Run()` loop starts, collects 2 metrics within 15s, exits cleanly |
| `TestIntegration_BatchVsSequentialConsistency` | Batch and sequential paths return identical results for same query |
| `TestIntegration_BatchEmptySQL` | Empty/whitespace SQL queries are silently skipped |
| `TestIntegration_BatchMultipleMetricsSameRoundTrip` | 10 queries sent in one batch, all 10 return results |

### Existing Tests — 0 regressions

All 152 test cases in `internal/reaper/` pass, including all pre-existing tests for `DetectSprocChanges`, `DetectTableChanges`, `DetectIndexChanges`, `DetectPrivilegeChanges`, `DetectConfigurationChanges`, `FetchMetric`, `LoadSources`, `LoadMetrics`, log parser tests, and OS metric tests.

---

## Design Decisions

| Decision | Rationale |
|----------|-----------|
| GCD-based tick loop (Option A) | Zero external dependencies, natural fit with `context` cancellation, simple reasoning |
| No external scheduler (gocron) | gocron v2 uses one goroutine per job, defeating the consolidation purpose |
| 5-second minimum tick | Prevents excessive wake-ups for coprime intervals (e.g., GCD(7, 13) = 1) |
| Sequential fallback for non-Postgres | pgbouncer/pgpool use `SimpleProtocol` and don't support pipeline batching |
| `server_log_event_counts` keeps its own goroutine | Streaming CSV parser with long-running I/O, not batchable |
| Batch timeout = 80% of tick interval | Prevents slow queries from blocking the next tick cycle |
| Sorted keys in `BatchQueryMeasurements` | Ensures deterministic batch ordering for testing and debugging |
| `Detect*ChangesWithData()` variants | Allow pre-fetched batch data while preserving original methods as fallbacks |

---

## Expected Production Impact

### Resource Usage

- **Memory**: ~97% reduction in goroutine stack allocations (320 × 8KB default stack → 10 × 8KB)
- **CPU scheduling**: Fewer goroutines means less scheduler overhead and context switching
- **Connection pool**: Batch acquires 1 connection per tick instead of N concurrent acquires; reduces pool contention

### Network

- **Round-trips**: ~92% reduction at 60s alignment for exhaustive preset
- **Latency**: Batch queries benefit from TCP connection reuse and PostgreSQL's pipeline protocol
- **Bandwidth**: Slight reduction from fewer TCP handshakes and acknowledgements

### Observability

- `pgwatch_reaper_batch_size` histogram reveals how many queries are batched per tick
- `pgwatch_reaper_batch_duration_seconds` tracks end-to-end batch latency
- `pgwatch_reaper_metric_fetch_total` with source/status labels enables per-source error rate alerting
- `pgwatch_reaper_active_source_reapers` gauge shows current source count

---

## Future Enhancements

1. **Overflow workers (Option D)**: Offload known-slow metrics (e.g., `table_bloat_approx_summary_sql`) to a separate goroutine if they exceed a time threshold, preventing them from blocking the batch
2. **Adaptive tick interval**: Dynamically adjust tick interval based on observed query latency
3. **Per-metric batch timeout**: Use `SET LOCAL statement_timeout` within the batch for metrics with `StatementTimeoutSeconds` configured
4. **Batch configuration changes**: Batch the `DetectConfigurationChanges` hash queries (currently excluded due to different `Scan()` pattern)
