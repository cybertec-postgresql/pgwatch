---
title: Reaper Resilience to Source and Network Failures
version: 1.0
date_created: 2026-08-11
owner: pgwatch maintainers
tags: [design, reaper, sources, resilience, issue-890]
---

# Introduction

This specification defines three independent workstreams that make pgwatch survive source-level
and host-level network failures without a global monitoring stall. It originates from the
investigation of [issue #890](https://github.com/cybertec-postgresql/pgwatch/issues/890):
a fleet of ~400 sources experienced repeated total collection stops (1–2 per month) that only
a service restart would clear.

Root-cause verdict from the investigation:

- **Trigger (environmental, not actionable here)**: the pgwatch host's DNS resolver
  (`127.0.0.53`, systemd-resolved) and/or network experienced brownouts. All 400+ independent
  PostgreSQL servers failed simultaneously with DNS i/o timeouts, dial timeouts, and TLS/SASL
  timeouts — a shared-infrastructure failure signature. The sink database (`127.0.0.1`) stayed
  reachable, which is why `instance_up: 0` kept being written while everything else stopped.
- **Amplification (pgwatch, actionable here)**: deadline-free database round-trips let
  half-open TCP connections pin connection-pool slots for up to kernel TCP retransmission
  limits (~15–30 min), the sequential main-loop sweep converts per-source hangs into a
  fleet-wide standstill, and discovery sources are torn down on any transient resolution error.

Each workstream is independently implementable, independently shippable, and independently
revertible. They are ordered by blast radius: WS1 smallest, WS3 medium, WS2 largest.

---

## 1. Purpose & Scope

**Purpose**: convert permanent or long-tail monitoring stalls caused by source/network failures
into bounded, self-recovering, per-source degradation.

**In scope**:

- **WS1** — Client-side deadlines for every database round-trip in the collection paths.
- **WS2** — Bounded-parallel main-loop source sweep in `reaper.Reap()`.
- **WS3** — Last-known-good resolution cache for `postgres-continuous-discovery` sources.

**Out of scope**:

- Fixes to the user's environment (DNS resolver, conntrack, ephemeral ports). The trigger is
  environmental; these workstreams only remove the pgwatch-side amplification.
- The v3 all-or-nothing resolution bug from the original #890 report — already fixed by
  parallel, per-source-isolated resolution (#1378, shipped in v5.2.0).
- `measurementCh` backpressure policy when a sink wedges (buffered 256; senders block on a
  full channel). Related but a separate design decision (drop vs block vs spill).
- `statement_timeout` as a server-side complement. It does nothing for half-open connections
  (the abort never reaches a blackholed client) and is therefore not a substitute for WS1.
  It MAY be added later for slow-but-alive queries (e.g., large `pg_stat_statements`).

**Audience**: pgwatch maintainers. Also intended for direct consumption by AI coding assistants.

**Assumptions**: code references are against `master` (post-v5.2.0, `DbConnReaper` batch
architecture from #1316). Line numbers are indicative, not contractual.

---

## 2. Definitions

| Term | Definition |
|---|---|
| **Source** | Configured monitoring target (`sources.Source`), e.g. a PostgreSQL instance. |
| **Discovery source** | A source of kind `postgres-continuous-discovery` (or patroni variants) whose databases are enumerated at runtime by querying `pg_database` (or DCS). |
| **Reaper** | The component owning the main monitoring loop (`internal/reaper`). |
| **Main-loop sweep** | The per-refresh iteration in `reaper.Reap()` that connects/pings each source, fetches runtime info, and starts/stops per-source workers. |
| **Worker** | Per-source goroutine (`DbConnReaper`) executing metric fetches on a GCD-aligned tick via `pgx.Batch`. |
| **Half-open connection** | A TCP connection whose peer or path silently drops packets: established, no RST, writes retransmit until the kernel gives up (~15–30 min with Linux `tcp_retries2`). |
| **Brownout** | A transient host-level network/DNS degradation affecting many or all sources at once. |
| **Blast radius** | The set of sources whose collection is impacted by a single failure. |

---

## 3. Background — Current Failure Mechanics

Evidence gathered from the v5.2.0 code the reporter runs and from current `master`:

| # | Finding | Location (`master`) |
|---|---|---|
| F1 | Metric batch fetch runs on the worker-lifetime context — no deadline | `DbConnReaper.executeBatch` → `Conn.SendBatch(ctx, …)`, `internal/reaper/database.go` |
| F2 | Single-metric retry path — no deadline | `DbConnReaper.fetchMetric` → `Conn.Query(ctx, …)`, `internal/reaper/database.go` |
| F3 | `QueryMeasurements`, `Detect*Changes` family — no deadline | `internal/reaper/database.go` |
| F4 | `DbConn.Ping` (main-loop gate) — no deadline; can block indefinitely on a half-open conn or behind wedged pool connections | `internal/sources/conn.go` |
| F5 | Discovery query runs on `context.TODO()`; connect is bounded (5 s default `ConnectTimeout`, `internal/db/bootstrap.go`) but the query is not | `ResolveDatabasesFromPostgres`, `internal/sources/resolver.go` |
| F6 | Main-loop sweep is sequential: one hanging source stalls every source behind it | `reaper.Reap()`, `internal/reaper/reaper.go` |
| F7 | Discovery resolution failure drops the source from `monitoredSources`; `CleanupRemovedWorkers` cancels workers, closes pools, issues sink `DeleteOp` — full teardown and re-setup on every transient failure | `reaper.LoadSources`, `reaper.CleanupRemovedWorkers`, `internal/reaper/reaper.go` |
| F8 | Patroni resolver already caches last-known DCS members to survive DCS jitter (`lastFoundClusterMembers`); postgres discovery has no equivalent | `internal/sources/resolver.go` |
| F9 | `lastFoundClusterMembers` is a plain map read/written by concurrent resolver goroutines since #1378 (`wg.Go` per source) — latent data race | `internal/sources/resolver.go` |
| F10 | #1412 deadlock fix (`executeBatch` deferred `br.Close()` holding a pool conn through retries) and the #1316 batch consolidation are on `master` but NOT in the v5.3.0 tag | git history |

Known pgx/pgxpool facts relied upon below:

- A cancelled/expired context during an in-flight query makes pgx tear the connection down
  (protocol stream is desynchronized); the pool discards it and the next acquire dials fresh.
  This is the recovery primitive WS1 depends on.
- `pgxpool.Config` has no `AcquireTimeout`; pool-acquire waits are bounded only by the caller's
  context.
- `pgwatch` already defaults `ConnectTimeout` to 5 s when unset (`internal/db/bootstrap.go`);
  the reporter's `connect_timeout=5` "workaround" was therefore a no-op, and only
  `sslmode=disable` changed behavior (halved dial attempts during the brownout).

---

## 4. Workstream 1 (WS1) — Bounded Contexts for All Database Round-Trips

### 4.1 Goal

No database operation may wait longer than a bounded, predictable deadline. A wedged
connection must be killed client-side so the pool replaces it, converting a multi-hour stall
into a single missed collection interval.

### 4.2 Requirements

- **REQ-101**: `DbConnReaper.executeBatch` MUST execute `SendBatch` under a derived context
  with a deadline. The deadline MUST be the current tick's metric interval, floored at
  **30 s** (very short intervals still get a usable window).

- **REQ-102**: `DbConnReaper.fetchMetric` (retry/degraded path) MUST apply the same deadline
  rule as REQ-101, computed from the entry's metric interval.

- **REQ-103**: `QueryMeasurements` and the `Detect*Changes` family MUST run under a derived
  context with a fixed default deadline of **60 s** unless a tighter caller-provided deadline
  exists.

- **REQ-104**: `DbConn.Ping` MUST apply a deadline of `ConnectTimeout + 5 s` margin (default:
  10 s). This covers both the half-open-round-trip case and the queued-behind-wedged-pool
  case, because pool acquire respects the caller context.

- **REQ-105**: `DbConn.FetchRuntimeInfo` and its sub-queries (version, extensions, approx
  size, platform discovery) MUST each run under a derived context with a fixed default
  deadline of **30 s**.

- **REQ-106**: `ResolveDatabasesFromPostgres` MUST replace `context.TODO()` with a derived
  context with a fixed deadline of **15 s** for both pool creation and the discovery query.
  (Parity with the etcd resolver's existing `WithTimeoutCause(5*time.Second)`.)

- **REQ-107**: Deadline expiry MUST be surfaced in logs as a distinct, greppable message
  (e.g. `context deadline exceeded` wrapped with operation name and source), at `Error` level
  for fetch paths and `Warning` for the main-loop Ping gate (which already has a retry-next-
  iteration path).

- **REQ-108**: No new command-line option or env var is introduced in this workstream.
  Deadlines are derived (interval-based) or fixed defaults as specified above. A tunable MAY
  be added later if field reports demand it.

- **CON-101**: The implementation MUST NOT use `statement_timeout` as the enforcement
  mechanism (server-side; ineffective on half-open connections — see §1 Out of scope).

- **CON-102**: On context expiry, the implementation MUST NOT attempt to reuse the connection;
  rely on pgx/pgxpool discard semantics (see §3 pgx facts).

- **GUD-101**: Centralize deadline construction in one helper (e.g.
  `func fetchCtx(ctx context.Context, interval time.Duration) (context.Context, context.CancelFunc)`)
  so the floor values live in exactly one place.

### 4.3 Acceptance Criteria

- **AC-101** ✅: Given a source whose network silently drops packets mid-query, when a metric
  fetch is in flight, then the fetch returns an error within its deadline and the worker logs
  and continues to its next tick.
- **AC-102** ✅: Given a source whose pool connections are all wedged, when the main loop calls
  `Connect`/`Ping` on it, then `Ping` returns an error within 10 s (default config) and the
  sweep proceeds to the next source.
- **AC-103** ✅: Given a discovery source that accepts TCP but never answers the discovery query,
  when resolution runs, then it fails within 15 s and other sources resolve unaffected.
- **AC-104** ✅: Given a healthy fleet, when all deadlines are in place, then no metric fetch,
  Ping, runtime-info fetch, or resolution fails due to the new deadlines (regression guard:
  defaults are generous relative to p99 fetch times).
- **AC-105** ✅: `go test -race ./internal/reaper/ ./internal/sources/` passes.

### 4.4 Risks & Rollback

- **Risk**: a legitimately slow metric (> interval) now errors instead of completing late.
  Mitigation: floor of 30 s; the existing "fetching time bigger than interval" warning already
  flags such metrics.
- **Risk**: premature conn teardown increases reconnect churn on very flaky links. Bounded by
  pool limits (`MaxParallelConnectionsPerDb`).
- **Rollback**: single-PR revert; no state or schema changes.

---

## 5. Workstream 2 (WS2) — Bounded-Parallel Main-Loop Sweep

### 5.1 Goal

The per-refresh source sweep (`reaper.Reap()` loop body over `r.monitoredSources`) must not be
serialized behind a single hanging source. With 400 sources and ~10–20 s failure cost per
source, a sequential sweep can take 1–2 h during a brownout; a bounded-parallel sweep
degrades this to `ceil(N/limit) × per-source-timeout`.

**Depends on**: WS1. Parallelizing a sweep whose `Ping` can hang forever only multiplies
wedged goroutines; WS1 makes the per-source step bounded, which makes the parallel sweep's
worst case arithmetic.

### 5.2 Requirements

- **REQ-201**: The loop body in `reaper.Reap()` (Connect → FetchRuntimeInfo → FilterSource →
  CreateSourceHelpers → TrackRecoveryStatus → SyncMetricsToSinks → StartWorker) MUST execute
  per source in an `errgroup.Group` (or equivalent) with `SetLimit(32)`. The limit MUST NOT
  scale with fleet size (unbounded parallelism recreates the DNS/reconnect storm this design
  is meant to survive).

- **REQ-202**: `reaper.cancelFuncs` and `reaper.srcRecoveryStatus` map accesses
  (`StartWorker`, `ShutdownWorker`, `TrackRecoveryStatus`, `CleanupRemovedWorkers`) MUST be
  guarded by a new `sync.Mutex` (or `RWMutex`) on `reaper`. All accesses — including the
  sequential phases — MUST go through the guard.

- **REQ-203**: The per-iteration `ctx = log.WithLogger(ctx, srcL)` reassignment MUST become a
  per-source local variable inside the goroutine. (Also fixes the current logger-accumulation
  quirk where iteration N's context carries N loggers.)

- **REQ-204**: `CleanupRemovedWorkers` MUST remain sequential and MUST run only after the
  errgroup's `Wait()` barrier.

- **REQ-205**: `LoadSources` MUST remain sequential and outside the parallel section (it
  already resolves concurrently per source since #1378).

- **REQ-206**: Before enabling parallel `SyncMetricsToSinks`, `PostgresWriter.SyncMetric`
  (and any other writer lacking internal locking) MUST be audited and, if needed, given a
  mutex. `MultiWriter`, `PrometheusWriter`, and `InstanceMetricCache` already hold locks.

- **REQ-207**: Error semantics MUST be unchanged: a per-source failure logs and excludes that
  source from this iteration; it MUST NOT abort sibling goroutines (`errgroup` without
  cancellation-on-error — collect errors via `WithError` logging inside the closure, not via
  `errgroup` error propagation).

- **CON-201**: Per-source worker startup order is no longer deterministic; anything relying on
  source ordering MUST be identified and removed.

- **GUD-201**: Keep the parallelism limit a package-level constant (e.g.
  `maxConcurrentSourceConnects = 32`). Do not add a CLI option in this workstream.

### 5.3 Acceptance Criteria

- **AC-201** ✅: Given 3 sources where the middle one hangs in `Ping` until its deadline, when a
  refresh runs, then the third source's `Connect OK` log appears no later than ~the middle
  source's Ping deadline after the first's (i.e., not serialized behind the full hang plus
  sequential position).
- **AC-202** ✅: Given 400 sources and a host-wide brownout, when a refresh runs, then the sweep
  completes in approximately `ceil(400/32) × ping-deadline` rather than `400 × ping-deadline`.
- **AC-203** ✅: `go test -race ./internal/reaper/` passes, including a new test with concurrent
  Start/Shutdown worker churn.
- **AC-204** ✅: Given a source failing Connect, when the sweep runs, then `instance_up=0` is
  written exactly once for that source per iteration (unchanged behavior).
- **AC-205** ✅: Worker lifecycle is unchanged: a source present in consecutive refreshes keeps
  its running worker (`StartWorker` remains a no-op for existing names).

### 5.4 Risks & Rollback

- **Risk**: startup thundering herd against DNS/network on process start. Mitigation: the
  SetLimit cap is the mitigation; 32 concurrent dials is well below resolver-storm territory
  observed with 400 × 3 pool conns.
- **Risk**: data races in code previously protected by sequential execution. Mitigation:
  REQ-202/REQ-206, mandatory `-race` CI run.
- **Rollback**: single-PR revert.

---

## 6. Workstream 3 (WS3) — Last-Known-Good Cache for Postgres Discovery

### 6.1 Goal

A transient failure of the discovery query (DNS hiccup, connect timeout, or a
permission error that affects only the discovery SQL — e.g. the `btrim` denial from the
original #890 report, where data connections work fine) MUST NOT tear down monitoring of the
source's already-known databases.

### 6.2 Requirements

- **REQ-301**: `ResolveDatabasesFromPostgres` MUST maintain a last-known-good cache of
  resolved database lists, mirroring the existing Patroni `lastFoundClusterMembers` pattern.

- **REQ-302**: On successful resolution, the cache entry MUST be replaced with the fresh list.

- **REQ-303**: On resolution failure with a non-empty cache entry, the resolver MUST return
  the cached list and a `nil` error, and MUST log a `Warning` that stale data is being used.
  This masks the error exactly as the Patroni resolver does, so the main loop neither tears
  down workers nor writes `instance_up=0` for the source.

- **REQ-304**: On resolution failure with an empty cache entry (first start), behavior MUST
  remain as today: error propagates, `WriteInstanceDown(sourceName)` fires, source absent
  until the next refresh.

- **REQ-305**: The cache key MUST incorporate source identity beyond the name: at minimum
  `Name + ConnStr + IncludePattern + ExcludePattern`. A configuration change MUST NOT be
  served stale databases from a previously configured target. (The Patroni cache's name-only
  key has this latent flaw; do not replicate it.)

- **REQ-306**: Both the new cache and the existing `lastFoundClusterMembers` map MUST be
  guarded by a single `sync.Mutex` (or converted to `sync.Map`). This fixes the latent race
  F9 introduced when resolution became concurrent (#1378).

- **REQ-307**: Cached entries carry no TTL. The next successful resolution replaces them.
  (Parity with the Patroni cache; one staleness policy, not two.)

- **CON-301**: A database dropped on the server while its source's discovery keeps failing
  will continue to be monitored from cache: per-cycle connect errors and `instance_up=0` for
  that database until the next successful resolution. This is accepted: it is visible,
  self-healing, and preferable to a full monitoring gap.

- **GUD-301**: Implement inside `internal/sources/resolver.go`. Do not implement reaper-level
  carry-forward of previous `monitoredSources` entries: resolved names are `source_dbname`
  string concatenations, so parentage cannot be reconstructed reliably at the reaper level.

### 6.3 Acceptance Criteria

- **AC-301**: Given a discovery source with 5 previously resolved databases, when the
  discovery query fails once (injected error), then all 5 databases remain in
  `monitoredSources`, their workers keep running, and no `DeleteOp` is synced to sinks.
- **AC-302**: Given the same source, when discovery later succeeds with a changed database
  list, then `monitoredSources` reflects the new list (cache replaced, workers reconciled as
  today).
- **AC-303**: Given a discovery source failing on its first-ever resolution, when pgwatch
  starts, then `instance_up=0` is written for the source and no databases are monitored
  (unchanged behavior).
- **AC-304**: Given a discovery source whose `ConnStr` is edited after a successful
  resolution, when the next resolution fails, then the stale list of the *old* target is NOT
  used.
- **AC-305**: `go test -race ./internal/sources/` passes with concurrent `ResolveDatabases`
  over multiple discovery sources sharing the cache.

### 6.4 Risks & Rollback

- **Risk**: stale topologies are trusted for longer than today. Bounded by the next successful
  refresh (`PW_REFRESH`, typically 120 s) during any period when discovery itself works.
- **Risk**: masking resolution errors reduces visibility. Mitigation: mandatory `Warning` log
  on cache use (REQ-303).
- **Rollback**: single-PR revert.

---

## 7. Non-Goals and Explicitly Rejected Alternatives

| Alternative | Why rejected |
|---|---|
| `statement_timeout` via `RuntimeParams` | Server-side enforcement; the abort packet never reaches a blackholed client. Does not fix the half-open case. |
| pgxpool `AcquireTimeout` | Does not exist in pgx v5; acquire waits are bounded only by context (WS1 covers this). |
| Reaper-level carry-forward of dropped discovery sources | Parentage of resolved DBs is not reconstructible from `source_dbname` concatenated names; resolver-level cache is the clean layer. |
| TTL-based staleness for the discovery cache | Second policy beside the Patroni cache's no-TTL policy; complexity without a demonstrated need. |
| New CLI options for deadlines/parallelism | Scope containment; derive or fix constants now, add knobs only on field demand. |
| Fixing the reporter's DNS/network | Environmental; out of pgwatch's control. |

---

## 8. Dependencies & Ordering

```
WS1 (deadlines) ──► WS2 (parallel sweep)     [WS2 requires bounded per-source steps]
WS1 ──► (independent of WS3)
WS3 (discovery cache) ──► (independent of WS1/WS2; touches only resolver.go + tests)
```

- Recommended implementation order: **WS1 → WS3 → WS2** (ascending blast radius).
- WS3 MAY ship before WS1; it is fully independent.
- Each workstream is a separate PR. Do not combine.

**Note for release planning**: the #1412 deadlock fix and #1316 batch consolidation exist on
`master` but are not contained in the `v5.3.0` tag (F10). Reporters on ≤ v5.2.0 with
`--max-parallel-connections-per-db=1` can hit the #1412 deadlock independently of this spec.

---

## 9. Test Automation Strategy

- **Frameworks**: existing Go test conventions; `testutil` PostgreSQL containers for
  integration tests, `pgxmock` for pool-level unit tests.
- **WS1 tests**: fake server that accepts TCP and never responds (assert deadline-bounded
  failure for Ping, fetch, resolution); mock pool that blocks `Acquire` (assert REQ-104).
- **WS2 tests**: multi-source reaper test with one source stalled; assert sweep completion
  time and per-source isolation. Mandatory `-race` run.
- **WS3 tests**: resolver cache hit/miss/invalidation matrix (AC-301…AC-305), concurrent
  resolution under `-race`.
- **Regression guard**: full existing suite must pass unmodified except where behavior is
  explicitly re-specified above.
- **Commands**:
  ```bash
  go build ./cmd/pgwatch/
  go test -failfast -p 1 -timeout=300s -parallel=1 ./... -coverprofile=coverage.out
  go test -race ./internal/reaper/ ./internal/sources/
  ```

---

## 10. Validation Criteria

The specification is satisfied when, under an injected host-wide network brownout
(e.g. `iptables -A OUTPUT -p tcp --dport 5432 -j DROP` against the test fleet, or a
blackhole-ing DNS resolver):

1. Metrics from unreachable sources fail within their deadlines and resume automatically
   when connectivity returns — without a pgwatch restart.
2. `instance_up=0` is written for unreachable sources only; healthy sources never miss an
   interval because of a sibling's failure.
3. Discovery sources keep monitoring their last-known databases through transient discovery
   failures.
4. No goroutine leak: `runtime.NumGoroutine()` returns to baseline after the brownout clears.

---

## 11. Related Specifications / Further Reading

- [Issue #890 — Single faulty host with postgres-continuous-discovery breaks metrics collection for all hosts](https://github.com/cybertec-postgresql/pgwatch/issues/890)
- [Issue #1412 — deadlock in SourceReaper](https://github.com/cybertec-postgresql/pgwatch/issues/1412)
- `spec/refactor-sourceconn-interface.md` — `DbConn`/`PromConn` interface hierarchy (call-site types referenced above)
- `spec/architecture-prometheus-exporter-source.md` — parallel track; `PromConn` Ping/Connect
  MUST follow the WS1 deadline rules once implemented
