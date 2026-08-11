---

description: "Task list for WS1 — Bounded Contexts for All Database Round-Trips (spec/design-source-failure-resilience.md)"
---

# Tasks: WS1 — Bounded Contexts for All Database Round-Trips

**Input**: Design document `spec/design-source-failure-resilience.md` (§4 Workstream 1, §3 findings F1–F6)
**Prerequisites**: `spec/design-source-failure-resilience.md` (required)

**Tests**: INCLUDED — mandated by the spec, §9 Test Automation Strategy ("WS1 tests: fake server that accepts TCP and never responds; mock pool that blocks Acquire"). Write story tests FIRST and watch them fail before implementing.

**Organization**: Three user stories, one per code site, each independently shippable:
- US1 (P1, MVP): metric fetch paths — `internal/reaper/database.go`
- US2 (P2): liveness gate + runtime info — `internal/sources/conn.go`
- US3 (P3): discovery resolver — `internal/sources/resolver.go`

**Spec traceability**: REQ-101…REQ-108, CON-101, CON-102, GUD-101. Constraints with no task of their own are verified in the Polish phase (T901, T904).

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1, US2, US3)
- Exact file paths and symbols are included in every description

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: establish a green baseline before any change

- [x] T001 Verify clean-tree build: `go build ./cmd/pgwatch/`
- [x] T002 [P] Verify baseline test suite green: `go test -failfast -p 1 -timeout=300s -parallel=1 ./... -coverprofile=coverage.out`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: the shared deadline helper (GUD-101) and the shared fault-injection test doubles

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [x] T003 Create `internal/db/deadlines.go`: centralized deadline helper per GUD-101. Package-level `var` (NOT `const`, so tests can shrink them): `MinFetchTimeout = 30s` (floor), `ChangeDetectionTimeout = 60s`, `RuntimeInfoTimeout = 30s`, `ResolverTimeout = 15s`, `PingTimeoutMargin = 5s`. Functions: `WithFetchTimeout(ctx, op string, interval time.Duration) (context.Context, context.CancelFunc)` applying `max(interval, MinFetchTimeout)`, and `WithOpTimeout(ctx, op string, d time.Duration)`. Both MUST use `context.WithTimeoutCause` with a cause embedding `op` (e.g. `"fetch db_stats deadline"`) so deadline expiry is distinct and greppable per REQ-107. Constraint: `internal/db` MUST stay a leaf package — no imports of `internal/reaper` or `internal/sources`.
- [x] T004 [P] Create `internal/db/deadlines_test.go`: table-driven unit tests — interval below floor yields floor, above floor yields interval; cause string contains the operation name; parent-context cancellation propagates; deadline actually fires (use shrunk vars).
- [x] T005 Create `internal/testutil/blackhole.go`: `BlackholeListener(t *testing.T) (addr string, close func())` — a TCP listener that accepts connections and never reads/writes/closes (simulates half-open connection per spec §3). Must be safe for many concurrent accepted conns; goroutines exit on `close`.
- [x] T006 [P] Add blocking pool double to `internal/testutil/mocks.go`: `BlockingPool struct { db.PgxPoolIface }` (embedded nil interface; only overridden methods work) with `Ping`, `Query`, `SendBatch`, `Acquire` implementations that block until `ctx.Done()` and then return `ctx.Err()`. Used by US1/US2 tests to simulate wedged pool connections per spec §9.

**Checkpoint**: Foundation ready — `go test ./internal/db/ ./internal/testutil/` passes; story implementation can now begin in parallel ✅

---

## Phase 3: User Story 1 — Metric Fetch Path Deadlines (Priority: P1) 🎯 MVP

**Goal**: a wedged query kills its connection client-side within one metric interval instead of stalling the source's worker for up to kernel TCP limits (spec findings F1–F3)

**Independent Test**: with `BlockingPool.SendBatch` blocking forever, `DbConnReaper.executeBatch` returns an error within the fetch deadline (vars shrunk), logs it, and the worker loop continues to its next tick — no restart, no goroutine leak.

### Tests for User Story 1 ⚠️

> **NOTE: Write these tests FIRST, ensure they FAIL before implementation (they fail by hanging until test timeout / by missing helper symbols)**
- [x] T101 [P] [US1] Add failing test in `internal/reaper/database_test.go`: `executeBatch` over a `DbConn` whose `Conn` is `testutil.BlockingPool` (SendBatch blocks) returns within deadline; shrink `db.MinFetchTimeout` for the test duration. Assert error cause names the operation (REQ-107).
- [x] T102 [P] [US1] Add failing test in `internal/reaper/database_test.go`: `fetchMetric` with blocking `Query` returns within `max(interval, MinFetchTimeout)` where interval comes from `md.GetMetricInterval(entry.metricName)` (REQ-102).
- [x] T103 [P] [US1] Add failing test in `internal/reaper/database_test.go`: `QueryMeasurements` with blocking `Query` returns within `db.ChangeDetectionTimeout` (REQ-103); same for one representative `Detect*Changes` function.
### Implementation for User Story 1

- [x] T104 [US1] Wire `DbConnReaper.executeBatch` (`internal/reaper/database.go`, `SendBatch` call site): derive ctx via `db.WithFetchTimeout(ctx, "batch", sr.calcTickInterval())` per REQ-101; defer cancel. Do NOT hold the deferred `br.Close()` across retries (preserve the #1412 deadlock fix).
- [x] T105 [US1] Wire `DbConnReaper.fetchMetric` (`internal/reaper/database.go`, `Conn.Query` call site): derive ctx via `db.WithFetchTimeout` with the entry's interval per REQ-102.
- [x] T106 [US1] Wire `QueryMeasurements` and the `Detect*Changes` family (`internal/reaper/database.go`) with `db.WithOpTimeout(ctx, op, db.ChangeDetectionTimeout)` per REQ-103.
- [x] T107 [US1] Error surfacing per REQ-107: on `context.DeadlineExceeded` (check `context.Cause`), log at `Error` level with `source` and `operation` fields; verify message is greppable and distinct from ordinary query errors.
- [x] T108 [US1] Run story gate: `go test -race ./internal/reaper/`

**Checkpoint**: US1 fully functional — fetch paths are deadline-bounded; healthy-fleet regression tests unchanged and green (AC-104 for fetch paths)

---

## Phase 4: User Story 2 — Liveness Gate & Runtime Info Deadlines (Priority: P2)

**Goal**: the main-loop `Connect`/`Ping` gate can no longer block forever on a half-open connection or behind wedged pool conns (spec finding F4); runtime-info queries are bounded (F5-adjacent)

**Independent Test**: `DbConn{ConnStr: "postgres://x@" + blackholeAddr}.Ping(ctx)` returns an error within `ConnectTimeout + PingTimeoutMargin`; a Ping queued behind a fully wedged pool (`BlockingPool.Acquire` blocks) fails at the same bound instead of hanging.

### Tests for User Story 2 ⚠️

> **NOTE: Write these tests FIRST, ensure they FAIL before implementation**

- [x] T201 [P] [US2] Add failing test in `internal/sources/conn_test.go`: Ping against `testutil.BlackholeListener` returns within the REQ-104 bound (shrink `db.PingTimeoutMargin` and set a small `connect_timeout` in the conn string).
- [x] T202 [P] [US2] Add failing test in `internal/sources/conn_test.go`: Ping on a `DbConn` whose pool is `testutil.BlockingPool` (Acquire blocks) fails at the REQ-104 bound — covers the queued-behind-wedged-conns case.
- [x] T203 [P] [US2] Add failing test in `internal/sources/conn_test.go`: `FetchRuntimeInfo` with blocking `Query` completes (with error) within `db.RuntimeInfoTimeout` per REQ-105.

### Implementation for User Story 2

- [x] T204 [US2] Wire `DbConn.Ping` (`internal/sources/conn.go:147-150`): derive ctx with `db.WithOpTimeout(ctx, "ping", md.ConnConfig.ConnConfig.ConnectTimeout + db.PingTimeoutMargin)` per REQ-104; guard nil `ConnConfig` (Ping is exported) by falling back to the 10 s default total. Applies to both branches (pgbouncer `SHOW VERSION` and regular `Conn.Ping`).
- [x] T205 [US2] Wire `DbConn.FetchRuntimeInfo` sub-queries (`internal/sources/conn.go`, incl. the extension query at :298 and available-extensions query at :371) with `db.WithOpTimeout(ctx, op, db.RuntimeInfoTimeout)` per REQ-105 — one derived ctx per sub-query, not per call.
- [x] T206 [US2] Warning-level surfacing per REQ-107: Ping deadline expiry must remain visible through the main loop's existing "could not init connection, retrying on next iteration" warning — ensure the wrapped cause is included in the log entry.
- [x] T207 [US2] Run story gate: `go test -race ./internal/sources/`

**Checkpoint**: US1 AND US2 both work independently — sweep liveness gate bounded, fetch paths bounded (AC-101, AC-102)

---

## Phase 5: User Story 3 — Discovery Resolver Deadline (Priority: P3)

**Goal**: the discovery query no longer runs on `context.TODO()` (spec finding F5); a hung discovery target fails within 15 s instead of blocking its resolver goroutine indefinitely

**Independent Test**: `ResolveDatabasesFromPostgres` pointed at a `BlackholeListener` (connect succeeds if the listener completes the pgwire handshake — otherwise point it at a container with the discovery query stalled via `pg_sleep` advisory lock) fails within `db.ResolverTimeout`; other sources' resolution is unaffected.

### Tests for User Story 3 ⚠️

> **NOTE: Write these tests FIRST, ensure they FAIL before implementation**

- [ ] T301 [P] [US3] Add failing test in `internal/sources/resolver_test.go`: resolution against a stalled target fails within `db.ResolverTimeout` (shrink the var) with an error whose cause names the resolver operation; extend the existing container-based `TestMonitoredDatabase_ResolveDatabasesFromPostgres` setup rather than duplicating container boot.

### Implementation for User Story 3

- [ ] T302 [US3] Replace both `context.TODO()` uses in `ResolveDatabasesFromPostgres` (`internal/sources/resolver.go` — pool creation via `NewConn` and the discovery `c.Query`) with a single derived ctx: `db.WithOpTimeout(context.Background(), "resolve "+s.Name, db.ResolverTimeout)`, `defer cancel()` per REQ-106. Function signature unchanged.
- [ ] T303 [US3] Run story gate: `go test -race ./internal/sources/` (covers concurrent `ResolveDatabases` via `wg.Go` against the shared path)

**Checkpoint**: all three user stories independently functional (AC-103 covered)

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: contract verification across stories

- [ ] T901 [P] Verify REQ-108 / CON-101: `grep -rn "statement_timeout" internal/reaper internal/sources internal/db` returns no new enforcement use; no new CLI option/env var added (`internal/cmdopts` untouched).
- [ ] T902 [P] Verify CON-102: no code path retries on the same connection after context expiry (review diff for post-deadline reuse; pgx discard semantics relied upon).
- [ ] T903 Full suite: `go test -failfast -p 1 -timeout=300s -parallel=1 ./... -coverprofile=coverage.out` — no coverage regression vs T002 baseline.
- [ ] T904 [P] Race gate: `go test -race ./internal/reaper/ ./internal/sources/` (AC-105).
- [ ] T905 [P] Lint: `golangci-lint run` — no new findings.
- [ ] T906 Update `spec/design-source-failure-resilience.md`: mark WS1 acceptance criteria AC-101…AC-105 satisfied; note merged PR reference.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — can start immediately
- **Foundational (Phase 2)**: Depends on Setup — BLOCKS all user stories (helper + test doubles are shared)
- **User Stories (Phases 3–5)**: All depend on Foundational completion; then proceed in priority order P1 → P2 → P3, or in parallel (disjoint files: `internal/reaper/database.go` vs `internal/sources/conn.go` vs `internal/sources/resolver.go`)
- **Polish (Phase 6)**: Depends on all stories being complete

### User Story Dependencies

- **US1 (P1)**: No dependencies on other stories. Highest value — converts wedged queries into bounded failures. MVP.
- **US2 (P2)**: No dependency on US1, but note: without US2 the sequential sweep can still stall on a hung Ping even with US1 done (spec finding F4/F6 interaction). Ship together if possible.
- **US3 (P3)**: Fully independent; smallest diff.

### Within Each User Story

- Tests MUST be written first and observed failing (hang/timeout or compile error on missing helper)
- Wiring before logging surfacing
- Story gate (`go test -race ./<pkg>/`) must pass before moving on

### Parallel Opportunities

- T001 + T002 (setup) in parallel
- T004 + T006 in parallel after T003/T005 define their dependencies' shapes — actually T004 depends on T003 (helper API), T006 depends on nothing in T005: run T003+T005 in parallel, then T004+T006 in parallel
- After Foundational: US1, US2, US3 in parallel (different files, shared helper is read-only)
- All Polish tasks in parallel except T906 (last)

---

## Parallel Example: Foundational Phase

```bash
# Wave 1 — independent creations:
Task: "Create internal/db/deadlines.go with deadline helper and vars"        # T003
Task: "Create internal/testutil/blackhole.go with BlackholeListener"         # T005

# Wave 2 — depend on wave 1 shapes:
Task: "Create internal/db/deadlines_test.go"                                 # T004
Task: "Add BlockingPool to internal/testutil/mocks.go"                       # T006

# After checkpoint — all three stories concurrently:
Task: "US1: fetch path deadlines in internal/reaper/database.go"
Task: "US2: Ping/FetchRuntimeInfo deadlines in internal/sources/conn.go"
Task: "US3: resolver deadline in internal/sources/resolver.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (green baseline)
2. Complete Phase 2: Foundational (helper + doubles)
3. Complete Phase 3: US1 — fetch path deadlines
4. **STOP and VALIDATE**: inject a wedged-pool fault; worker logs deadline error and continues to next tick
5. Mergeable as-is; US2/US3 are follow-ups

### Incremental Delivery

1. Setup + Foundational → foundation ready
2. US1 → validate → merge (MVP: fleet no longer stalls on wedged queries)
3. US2 → validate → merge (sweep liveness gate bounded)
4. US3 → validate → merge (resolver bounded)
5. WS1 complete → spec §4 acceptance criteria all satisfied

### Parallel Team Strategy

1. One developer completes Setup + Foundational
2. Then: Dev A → US1, Dev B → US2, Dev C → US3 (disjoint files)
3. Polish phase as final shared gate

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to spec workstream requirement IDs (US1 → REQ-101/102/103/107; US2 → REQ-104/105/107; US3 → REQ-106)
- Deadline vars are package-level `var` deliberately: tests shrink them (e.g. `MinFetchTimeout`) to milliseconds so fault-injection tests run fast; production defaults stay per spec §4.2
- Do not add CLI options (REQ-108) and do not use `statement_timeout` (CON-101) — both are checked in Polish
- Commit after each task or logical group; keep each story's PR independent for clean revert (spec §4.4)
- After WS1 merges, WS3 (discovery cache) may start; WS2 (parallel sweep) REQUIRES WS1 merged (spec §8)
