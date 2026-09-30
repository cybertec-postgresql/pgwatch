---
description: "Task list for: blocking_locks reports the wait time and the real blockers"
---

# Tasks: blocking_locks wait time and real blockers

**Input**: [`spec/oss-blocking-lock.md`](../oss-blocking-lock.md) (v1.0, 2026-09-30)
**Baseline**: `v7` at `v6.0.0-10-gccd8d10c90`; line numbers are indicative.

**Tests**: INCLUDED. The spec defines a test automation strategy (section 6) and acceptance criteria (section 5). No mock can show what `pg_blocking_pids()` returns, so the tests need a real server.

**Organization**: Tasks are grouped by user story. The spec has no user-story section, so the stories are derived from its two changes: the blocker source and the wait time column.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: US1 or US2 as defined below
- File paths are exact and relative to the repository root
- Requirement IDs (REQ/CON/GUD/AC) from the spec are cited on each task

### User stories

| Story | Title | Priority | Delivers |
|---|---|---|---|
| US1 | Blockers come from `pg_blocking_pids()` | P1 (MVP) | REQ-001, REQ-002, REQ-004, CON-002, AC-002, AC-003, AC-004 |
| US2 | `waiting_seconds` per waiting session | P2 | REQ-003, AC-001 |

REQ-005, REQ-006, CON-001, GUD-001, AC-005 and AC-006 are cross-cutting and are checked in Phase 2 and Phase 5.

---

## Phase 1: Setup

**Purpose**: a known-good baseline to measure the column contract against

- [ ] T001 Run `go test ./pkg/metrics/...` and confirm it is green on the baseline
- [ ] T002 Record the column list the current `blocking_locks` SQL returns (`pkg/metrics/metrics.yaml:277-288`) as the reference for the AC-006 check in T005

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: the integration harness both stories run on, and the column-contract guard that both stories must keep green

**CRITICAL**: No user story work can begin until this phase is complete

- [ ] T003 Create `pkg/metrics/blocking_locks_integration_test.go` that starts PostgreSQL with `testutil.SetupPostgresContainer()` (`internal/testutil/setup.go:24`), opens a `pgx` connection per session, creates a table `t` with one row, and loads the `blocking_locks` SQL for key `14` from `GetDefaultMetrics()` (`pkg/metrics/default.go:17`). Follow the container setup in `pkg/sinks/postgres_feedback_integration_test.go:25-32`
- [ ] T004 Add helpers to the same file: `holdLock(t, conn, stmt)` runs `BEGIN` plus a statement and leaves the transaction open; `waitFor(t, conn, stmt)` runs the statement in a goroutine and returns once `pg_stat_activity.wait_event_type = 'Lock'` for that backend; `runMetric(t, conn)` executes the SQL and returns the rows as `[]map[string]any`; `t.Cleanup` rolls every session back
- [ ] T005 [P] Add `TestBlockingLocksColumns` to `pkg/metrics/metrics_yaml_test.go`: parse the `blocking_locks` SQL for key `14` and assert that every alias of REQ-005 is present, using the list recorded in T002 (REQ-005, AC-006). It must pass before and after the SQL change
- [ ] T006 [P] Add `TestBlockingLocksNoWait` to the integration file: with no waiting session the metric returns zero rows (CON-001, AC-005). It passes on the baseline SQL and must keep passing

**Checkpoint**: the harness starts a container, drives sessions, runs the metric SQL, and the column guard is green on the baseline

---

## Phase 3: User Story 1 - Blockers come from `pg_blocking_pids()` (Priority: P1) MVP

**Goal**: every row names a session that PostgreSQL itself reports as blocking the waiting session, one row per pair, for every lock type.

**Independent Test**: the AC-002, AC-003 and AC-004 tests in `pkg/metrics/blocking_locks_integration_test.go` pass against the new SQL and fail against the baseline SQL.

### Tests for User Story 1

> Write these first and confirm they fail on the baseline SQL before T010.

- [ ] T007 [P] [US1] `TestBlockingLocksModeConflict`: A holds `AccessShareLock` on `t` (`SELECT`), C holds `RowExclusiveLock` on `t` (`INSERT`), B waits in `CREATE INDEX ON t`. Assert rows name C as `other_pid` and no row names A (AC-002). Fails today because the join reports A too
- [ ] T008 [P] [US1] `TestBlockingLocksAdvisory`: A holds `pg_advisory_lock(1)`, B waits in `pg_advisory_lock(1)`. Assert exactly one row with `other_pid` A and `other_locktype` `advisory` (AC-003). Fails today because the join matches only relation or transaction id
- [ ] T009 [P] [US1] `TestBlockingLocksOneRowPerPair`: A holds two granted locks on the object B waits for (an `UPDATE` in an open transaction gives a `RowExclusiveLock` on `t` and a transaction id lock; B updates the same row). Assert exactly one row pairs B and A (REQ-001, AC-004). Fails today because both granted locks join

### Implementation for User Story 1

- [ ] T010 [US1] Rewrite the `blocking_locks` SQL for key `14` in `pkg/metrics/metrics.yaml:268-317`: keep the `sa_snapshot` CTE and its three filters (REQ-006); select waiting locks with `NOT granted` joined to `sa_snapshot`; replace the join on relation or transaction id (`:296-304`) with `cross join lateral unnest(pg_blocking_pids(waiting.pid)) as blocker(pid)`, so the function runs once per waiting session (REQ-002, CON-002); join `sa_snapshot` as `other_stm` on `blocker.pid`
- [ ] T011 [US1] Source `other_locktype`, `other_table` and `other_mode` from a `left join lateral` over `pg_locks` for `blocker.pid` on the same `locktype`, `database`, `relation`, `transactionid`, `virtualxid`, `classid`, `objid` and `objsubid` as the waiting lock, ordered by `granted desc`, `limit 1`, so a granted lock wins over a queued one and a missing row yields nulls without dropping the pair (REQ-001, REQ-004)
- [ ] T012 [US1] Keep every alias, cast and `coalesce` of `pkg/metrics/metrics.yaml:277-288` unchanged in the new select list so T005 stays green (REQ-005)
- [ ] T013 [US1] Run T005 to T009 and confirm T007 to T009 now pass and T005, T006 still pass

**Checkpoint**: `blocking_locks` agrees with `backends.blocked` (`pkg/metrics/metrics.yaml:100`) on who is blocked; no false blockers, no missed lock types

---

## Phase 4: User Story 2 - `waiting_seconds` per waiting session (Priority: P2)

**Goal**: an operator reading a row can tell a wait of 50 ms from one of ten minutes.

**Independent Test**: the AC-001 test passes against the new SQL and fails against the baseline SQL because the column does not exist.

### Tests for User Story 2

> Write this first and confirm it fails before T015.

- [ ] T014 [US2] `TestBlockingLocksWaitingSeconds`: A updates row 1 of `t` in an open transaction, B updates row 1 and the test sleeps two seconds after B is seen waiting. Assert exactly one row with `waiting_pid` B, `other_pid` A and `waiting_seconds >= 2` (AC-001). Also assert the value is a float with at most three decimals (REQ-003)

### Implementation for User Story 2

- [ ] T015 [US2] Add `round(extract(epoch from now() - waiting.waitstart)::numeric, 3)::float8 AS waiting_seconds` to the select list in `pkg/metrics/metrics.yaml`, after `waiting_pid`. `extract` of a null `waitstart` yields null, which REQ-003 requires; add no `coalesce`
- [ ] T016 [US2] Run T005 and T014 and confirm both pass

**Checkpoint**: US1 and US2 both hold; the metric has thirteen data columns plus `epoch_ns`

---

## Phase 5: Polish & Cross-Cutting Concerns

- [ ] T017 [P] Rewrite the `blocking_locks` description in `pkg/metrics/metrics.yaml:262-266` so it names `waiting_seconds` and states that blockers come from `pg_blocking_pids()` (GUD-001)
- [ ] T018 [P] Confirm the integration test runs under `task test` (`Taskfile.yml:76`, no build tag, like `pkg/sinks/postgres_feedback_integration_test.go`) within its 300 s timeout; confirm `TestAllKnownMetricsPresent` (`pkg/metrics/metrics_yaml_test.go:181`) and `TestSQLScalarsAreStructurallyIntact` (`:208`) still pass on the edited scalar
- [ ] T019 Run `task lint` and `task test`; confirm AC-001 through AC-006 all hold and the whole package is green on Windows as well as Linux (commit `c6944da768` made the suite green on Windows; keep it so)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: no dependencies
- **Foundational (Phase 2)**: depends on T002 for T005; BLOCKS both stories. Every story test needs the harness of T003 and T004
- **US1 (Phase 3)**: depends on Phase 2
- **US2 (Phase 4)**: depends on Phase 2 only. It edits the same YAML scalar as US1, so merge US1 first or rebase; the SQL edits do not overlap in meaning
- **Polish (Phase 5)**: T017 depends on US1 and US2; T018 and T019 depend on everything

### Within Each User Story

- Tests are written first and must fail on the baseline SQL before the SQL change
- The select list of REQ-005 is not renamed by either story; T005 is the guard
- Story complete before moving to the next priority

### Parallel Opportunities

- T005 and T006 in parallel once T003 and T004 exist
- T007, T008 and T009 in parallel; they are separate test functions in one file, so land them in one commit
- T017 and T018 in parallel after both stories

---

## Parallel Example: User Story 1

```bash
# Launch all tests for User Story 1 together:
Task: "TestBlockingLocksModeConflict in pkg/metrics/blocking_locks_integration_test.go"
Task: "TestBlockingLocksAdvisory in pkg/metrics/blocking_locks_integration_test.go"
Task: "TestBlockingLocksOneRowPerPair in pkg/metrics/blocking_locks_integration_test.go"
```

---

## Implementation Strategy

### MVP First (US1 only)

1. Phase 1: Setup
2. Phase 2: Foundational (blocks both stories)
3. Phase 3: US1, blockers from `pg_blocking_pids()`
4. **STOP and VALIDATE**: AC-002 to AC-006 hold
5. Ship. Rows are already correct without the new column

### Incremental Delivery

1. Setup + Foundational: harness and column guard in place
2. US1: correct blockers, one row per pair (MVP)
3. US2: `waiting_seconds`
4. Polish: description and CI

Every increment keeps the columns of REQ-005 unchanged, so a dashboard query of today keeps running (AC-006).

---

## Notes

- `[P]` = different files, no dependencies
- Both stories edit one YAML block scalar. Keep the indentation of `pkg/metrics/metrics.yaml` exact; `TestSQLScalarBleedRegression` (`pkg/metrics/metrics_yaml_test.go:253`) fails when a continuation line escapes the scalar
- The SQL key stays `14`; `waitstart` and `pg_blocking_pids()` exist on every supported server
- `pg_blocking_pids()` is called only for rows with `NOT granted`, never for every backend (CON-002)
- Commit after each task or logical group
- Stop at any checkpoint to validate a story independently
