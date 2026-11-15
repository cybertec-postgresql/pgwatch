---
description: "Task list for: the embedder reads the monitored sources"
---

# Tasks: the embedder reads the monitored sources

**Input**: `spec/oss-monitored-databases.md` (v1.0, 2026-11-15)
**Baseline**: branch `v7`. Line numbers are indicative.

**Tests**: included. The spec defines acceptance criteria (§5) and a test strategy (§6).

**Organization**: the spec has no user-story section, so the stories below come from its two
accessors. Each task cites the REQ, CON, GUD or AC ids it covers.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: can run in parallel (different files, no dependencies)
- **[Story]**: US1 or US2, defined below

### User stories

| Story | Title | Priority | Delivers |
|---|---|---|---|
| US1 | A connection returns a copy of its runtime info | P1 (MVP) | REQ-001, GUD-001, AC-002, AC-003 |
| US2 | `App` returns the sources the reaper monitors | P2 | REQ-002, REQ-003, CON-001, CON-002, AC-001, AC-004 |

US2 builds without US1. The embedder example in spec §4 needs both.

---

## Phase 1: Setup

**Purpose**: a green baseline before the interface changes.

- [x] T001 Produce `internal/webui/embed/build/` with `task ui` if it is missing, then run `go test -failfast -p 1 -timeout=300s ./pkg/sources ./pkg/reaper ./pkg/app` and record the result. Docker must be running for the testcontainers tests.

---

## Phase 2: Foundational

None. The two stories touch different methods and share no new code.

---

## Phase 3: User story 1, a connection returns a copy of its runtime info (P1) MVP

**Goal**: `SourceConn.GetRuntimeInfo()` returns the `RuntimeInfo` of a connection without a cast to `*sources.DbConn`.

**Independent test**: `go test -race -p 1 ./pkg/sources -run 'GetRuntimeInfo'` passes.

### Tests for user story 1

Write these first. They must fail to compile or fail until T005 and T006 land.

- [x] T002 [P] [US1] Add `TestDbConn_GetRuntimeInfo` in `pkg/sources/conn_test.go`. Fill the runtime info through `FetchRuntimeInfo` with `pgxmock`, or set `RuntimeInfo` directly. Assert that `Version` and `IsInRecovery` match. Then write to `Extensions` and to an inner map of `ChangeState` in the returned copy, and assert that the connection's maps did not change. Covers AC-002.
- [x] T003 [P] [US1] Add `TestPromConn_GetRuntimeInfo` in `pkg/sources/conn_test.go`. Assert that the result equals `sources.RuntimeInfo{}`. Covers AC-003.

### Implementation for user story 1

- [x] T004 [US1] Add `GetRuntimeInfo() RuntimeInfo` to the `SourceConn` interface in `pkg/sources/conn.go:43`, after `GetSource()`. Its doc comment states GUD-001: trust the value only when `IsPostgresSource()` is true and `Version > 0`, because a pooler holds the pooler's version and an info never fetched holds zeros. Covers REQ-001 and GUD-001.
- [x] T005 [US1] Implement `(*DbConn).GetRuntimeInfo()` in `pkg/sources/conn.go`, next to `GetSource()` at `:107`. Take `md.RLock()`, copy `md.RuntimeInfo`, clone `Extensions` with `maps.Clone`, and clone `ChangeState` together with each inner map. A nil map stays nil. Depends on T004. Covers REQ-001.
- [x] T006 [US1] Implement `(*PromConn).GetRuntimeInfo()` returning `RuntimeInfo{}` in `pkg/sources/conn.go`, next to `GetSource()` at `:613`. Depends on T004. Covers REQ-001.
- [x] T007 [US1] Add `GetRuntimeInfo()` returning `sources.RuntimeInfo{}` to `fakeSourceConn` in `pkg/reaper/reaper_test.go:1032`, so that `pkg/reaper` tests compile again. Depends on T004.

**Checkpoint**: T002 and T003 pass, and `go vet ./...` is clean.

---

## Phase 4: User story 2, `App` returns the sources the reaper monitors (P2)

**Goal**: `App.MonitoredSources()` returns a copy of the list from the last `LoadSources`, discovered databases included.

**Independent test**: `go test -race -p 1 ./pkg/reaper ./pkg/app -run 'MonitoredSources'` passes.

### Tests for user story 2

Write these first. They must fail until T013 to T016 land.

- [x] T008 [P] [US2] Add `TestReaper_MonitoredSources` in `pkg/reaper/reaper_test.go` with the mock sources reader used by `TestReaper_LoadSources` (`:27`). Subtests:
  - before the first `LoadSources`, the result is nil (REQ-002);
  - after `LoadSources`, the result has one entry per element of `r.monitoredSources`, with the same names (AC-001);
  - with the emergency pause triggerfile present, the result is empty (REQ-002);
  - replacing an entry of the result does not change `r.monitoredSources` (CON-001).
- [x] T009 [US2] Add `TestReaper_MonitoredSourcesRace` in `pkg/reaper/reaper_test.go`. One goroutine calls `LoadSources` in a loop while another calls `MonitoredSources()` and `GetRuntimeInfo()` on every entry. Run it with `go test -race`. Depends on T008 (same file) and on US1 for `GetRuntimeInfo()`. Covers AC-004.
- [x] T010 [P] [US2] Add `TestApp_MonitoredSources` in `pkg/app/app_test.go`. Assert that an `App` without a reaper returns nil, and that an `App` whose reaper is a stub `ReadierReaper` returns what the stub returns. Covers REQ-003.

### Implementation for user story 2

- [x] T011 [US2] Add a field `published atomic.Pointer[sources.SourceConns]` to `reaper` in `pkg/reaper/reaper.go:49`. Its comment says that only `LoadSources` writes it and that readers on other goroutines load it. Covers CON-001.
- [x] T012 [US2] Add a private method `publishSources()` in `pkg/reaper/reaper.go` that stores a pointer to `r.monitoredSources` in `r.published`, with nil stored as an empty slice. It does not clone: `monitoredSources` is replaced, never changed in place (CON-001). Depends on T011.
- [x] T013 [US2] Call `publishSources()` in `LoadSources` (`pkg/reaper/reaper.go:364`) on both successful exits: after the emergency pause reset at `:367` and after `r.monitoredSources = newSrcs` at `:401`. The error return from `GetSources()` does not publish. Depends on T012. Covers REQ-002.
- [x] T014 [US2] Add `func (r *reaper) MonitoredSources() sources.SourceConns` in `pkg/reaper/reaper.go`. It returns `slices.Clone(*p)` when `r.published.Load()` is not nil, and nil otherwise, so a caller that changes the result cannot change what other readers see. Depends on T011. Covers REQ-002.
- [x] T015 [US2] Add `MonitoredSources() sources.SourceConns` to `ReadierReaper` in `pkg/reaper/reaper.go:43`. The type `reaper` stays private. Depends on T014. Covers REQ-003 and CON-002.
- [x] T016 [US2] Add `func (a *App) MonitoredSources() sources.SourceConns` in `pkg/app/app.go`, next to `Ready()` at `:65`. It returns nil while `a.reaper` is nil and delegates otherwise. Depends on T015. Covers REQ-003.

**Checkpoint**: T008 to T010 pass under `-race`.

---

## Phase 5: Polish

- [x] T017 [P] Document `App.MonitoredSources()` and `SourceConn.GetRuntimeInfo()` in `docs/developer/embedding.md`, with the loop from spec §4 and the GUD-001 rule.
- [x] T018 [P] Use `MonitoredSources()` in `docs/howto/embedding/main.go`, if the example has a place where it fits. Skip the task if it does not.
- [x] T019 [P] Check `docs/developer/api_stability.md` and list the two methods if the page lists public API by method. Checked: the page lists packages, not methods, so nothing to add.
- [x] T020 Run `task check`. Lint and tests must pass. Depends on all tasks above. `task check` stopped in its `ui` step on a corrupt yarn cache; `golangci-lint run` and the full `go test -p 1` suite passed when run directly.
- [x] T021 Set `status` in `spec/oss-monitored-databases.md` to the value the maintainers use for an implemented spec, and update `date_updated`.

---

## Dependencies and execution order

### Phase dependencies

- Setup (phase 1) blocks everything else.
- Phase 2 is empty.
- US1 (phase 3) and US2 (phase 4) can start after phase 1, in either order.
- T009 needs `GetRuntimeInfo()` from US1. Without US1 it can only call `MonitoredSources()`.
- Polish (phase 5) runs after both stories.

### Within each story

- Tests come first and must fail.
- In US1, T004 changes the interface, so T005, T006 and T007 must follow it in the same commit or the build breaks.
- In US2, the order is T011, T012, T013, T014, T015, T016. T015 breaks every other implementation of `ReadierReaper`; today `reaper` is the only one, and the stub in T010 must have the method.

### Parallel opportunities

- T002 and T003 share `conn_test.go`, so one person writes both. They run in parallel with T008 and T010.
- US1 (`pkg/sources`) and US2 (`pkg/reaper`, `pkg/app`) touch different packages. The exception is T007, which edits `pkg/reaper/reaper_test.go`, and T008 and T009 edit the same file. Merge carefully.
- T017, T018 and T019 edit different files.

---

## Implementation strategy

1. Phase 1: confirm the baseline.
2. US1: interface method, two implementations, fake update, tests. Commit.
3. US2: published pointer, publish in `LoadSources`, accessors on reaper and `App`, tests including the race test. Commit.
4. Polish: docs and `task check`.

## Notes

- Keep `-p 1` on every `go test` run. Packages share containers and ports.
- Out of scope, per spec §1: an on-demand refresh of runtime info, new exported types, and changes to what or how often the reaper collects.
- `GetRuntimeInfo()` blocks while `FetchRuntimeInfo` holds the write lock (spec §7). Do not add a timeout or a try-lock.
