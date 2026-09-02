---
description: "Task list for implementing the sink feedback interface"
---

# Tasks: Sink Feedback Interface

**Input**: [`spec/design-sink-feedback.md`](../design-sink-feedback.md)
**Prerequisites**: that specification (required for requirement IDs referenced below)

**Tests**: INCLUDED. The specification mandates them — §6 Test Automation Strategy, §10 items 4, 5, 11 — so test tasks are first-class here, not optional.

**Organization**: Grouped by deliverable increment. Each increment is independently implementable, testable, and mergeable; each ends in a working sink capability that adds value without the later ones.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which increment this task belongs to (US1…US3)
- File paths are exact and relative to the repository root
- Bracketed IDs like **PGS-003** refer to requirements in the specification

---

## Phase 1: Setup

**Purpose**: Toolchain and a known-good baseline to measure behaviour neutrality against

- [x] T001 Run `task tools` to install `protoc-gen-go` and `protoc-gen-go-grpc`, then confirm `protoc --version` succeeds — required by US3 (**PLT-002**)
- [x] T002 Capture the baseline: `go test ./internal/sinks/...` green, and record current `pgwatch --help` output for the **CON-006** behaviour-neutrality comparison in T038

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The interface itself, the config switch, and the shared test doubles. Every increment depends on these.

**⚠️ CRITICAL**: No increment work can begin until this phase is complete

- [x] T003 Create `internal/sinks/feedback.go` with the `Feedbacker` interface and the `ErrFeedbackUnsupported` / `ErrNoFeedbackData` sentinels, copied verbatim from spec §4.1 including doc comments (**REQ-001**, **REQ-003**, **REQ-005**, **REQ-006**, **REQ-007**, **REQ-013**)
- [x] T004 Add `NoFeedback bool` to `sinks.CmdOpts` in `internal/sinks/cmdopts.go` plus a `FeedbackEnabled()` accessor (**CFG-001**, **CFG-003**); see D-01 for why the flag is negated
- [x] T005 [P] Add the `fakeFeedbacker` test double to `internal/sinks/feedback_test.go`: scriptable `CanFeedback` result, epoch, error, and per-method call counters (§6 Mocks). It drives every row of the §4.3 table without needing four real sinks
- [x] T006 [P] Add negative capability guards to `internal/sinks/feedback_test.go` asserting `PrometheusWriter` and `JSONWriter` do **not** satisfy `Feedbacker`, and that both still satisfy `Writer` (**PRM-001**, **JSN-001**, **AC-010**)
- [x] T007 [P] Add the scope guard enforcing **AC-017**: a test (or CI step) that greps `Feedbacker|LastMeasurement|CanFeedback` across `--include=*.go` and fails if any non-test hit falls outside `internal/sinks`

**Checkpoint**: The interface compiles, the switch exists, the doubles are ready — increments can start in parallel

---

## Phase 3: US1 — Postgres Sink Feedback (Priority: P1) 🎯 MVP

**Goal**: The Postgres sink can report the newest stored measurement epoch for a (source, metric) pair, bounded by retention and safe under concurrent DDL.

**Independent Test**: With a single `--sink postgresql://…`, call `LastMeasurement` for a metric that has rows, one that has none, and one whose table does not exist; get the epoch, `ErrNoFeedbackData`, and `ErrFeedbackUnsupported` respectively.

**Why first**: Postgres is the default and most-deployed sink. On its own it makes the capability real, and `NewSinkWriter` unwraps a single-sink `MultiWriter`, so this increment needs nothing from US2.

### Tests for US1

> Write these first and confirm they fail before implementing T012–T016.

- [x] T008 [P] [US1] `pgxmock` unit tests for `CanFeedback` in `internal/sinks/postgres_feedback_test.go`: non-empty pair → true; empty `sourceName` → false; empty `metricName` → false; feedback disabled → false; no lock taken (**PGS-002**, **PGS-008**)
- [x] T009 [P] [US1] `pgxmock` unit tests for `LastMeasurement` in the same file, covering: row returned → `epoch = time.UnixNano()` (**AC-002**); `pgx.ErrNoRows` → `ErrNoFeedbackData` (**AC-003**); `SQLSTATE 42P01` → `ErrFeedbackUnsupported` and nothing logged at `Error` (**AC-004**); already-cancelled context → context error with no query issued (**E-13**); `err == nil` implies epoch > 0 (**AC-015**); feedback disabled → `ErrFeedbackUnsupported` with zero `pgxmock` expectations consumed (**AC-011**)
- [x] T010 [P] [US1] Assert the generated SQL passes `sourceName` as a bind parameter and interpolates only the sanitised identifier; include a metric name containing a double quote (**SEC-001**, **E-12**)
- [x] T011 [US1] Integration test in `internal/sinks/postgres_feedback_integration_test.go` using `testutil.SetupPostgresContainer()`, following the `internal/reaper/database_integration_test.go` convention: real table via the sink's own `SyncMetric`/`AddOp` path, dropped in `t.Cleanup`. Cover the live epoch round-trip (**AC-002**), buffered-but-unflushed measurements excluded (**AC-013**, **REQ-011**), and a ≥ 30-partition table answering in under 100 ms (**PGS-004**, §6 Performance)
- [x] T012 [US1] Race test in `internal/sinks/postgres_feedback_integration_test.go` (a real backend is needed; pgxmock is not concurrency-safe) (pattern: `internal/sinks/prometheus_race_test.go`) running `CanFeedback` + `LastMeasurement` + `Write` + `SyncMetric` concurrently under `-race`; assert no race and that `LastMeasurement` does not block `SyncMetric` for the round-trip duration (**PGS-008**, **AC-014**, **E-10**)

### Implementation for US1

- [x] T013 [US1] Implement `PostgresWriter.CanFeedback` in `internal/sinks/postgres.go`: gate on `pgw.opts.FeedbackEnabled()` and reject empty names; optimistic and lock-free, since `partitionMapMetric` is empty at process start (**PGS-002**, **PGS-008**, **CFG-002**, **REQ-004**)
- [x] T014 [US1] Implement `PostgresWriter.LastMeasurement` in `internal/sinks/postgres.go` per the §9.1 sketch — build the §4.4 query with `pgx.Identifier{...}.Sanitize()` and bind parameters (**PGS-003**, **SEC-001**), bound by `pgw.opts.RetentionInterval` (**PGS-004**, **DAT-003**), convert via `UnixNano` (**PGS-005**), apply the 5 s default deadline (**CON-002**)
- [x] T015 [US1] Add the `isUndefinedTable` helper mapping `SQLSTATE 42P01` to `ErrFeedbackUnsupported` in `internal/sinks/postgres.go` (**PGS-007**)
- [x] T016 [US1] Add `var _ Feedbacker = (*PostgresWriter)(nil)` (**GUD-003**) and query logging: `Debug` on success, no higher than `Info` for expected `ErrFeedbackUnsupported` / `ErrNoFeedbackData` (**SEC-004**)

**Checkpoint**: Postgres sink feedback is fully functional and testable on its own

---

## Phase 4: US2 — MultiWriter Aggregation (Priority: P2)

**Goal**: A multi-sink configuration answers with the minimum epoch across feedback-capable sinks, without letting non-capable sinks veto the answer.

**Independent Test**: Build a `MultiWriter` from two `fakeFeedbacker`s and a `JSONWriter`; walk every row of the §4.3 table and confirm the documented result.

**Depends on**: Phase 2 only. It is exercised entirely through `fakeFeedbacker`, so it does not require US1 or US3 to be merged — though shipping it alongside US1 is what makes it observable in a real deployment.

### Tests for US2

- [x] T017 [P] [US2] Table-driven test in `internal/sinks/multiwriter_test.go` covering all seven rows of spec §4.3, built with `AddWriter` rather than `NewSinkWriter` (which unwraps the single-sink case): no capable writers → `ErrFeedbackUnsupported` (**REQ-026**); all unsupported → `ErrFeedbackUnsupported`; two epochs → minimum (**AC-005**); mixed capable + non-`Feedbacker` → capable answer survives (**AC-006**, **REQ-027**); one `ErrNoFeedbackData` → short-circuit regardless of other epochs (**AC-007**, **REQ-024**); one transport error → joined error, no partial minimum (**REQ-025**); feedback disabled → `ErrFeedbackUnsupported`
- [x] T018 [P] [US2] Test that `MultiWriter.CanFeedback` is true iff at least one contained writer is capable for the pair (**REQ-021**)
- [x] T019 [P] [US2] Test that the caller's `ctx` reaches each contained writer unchanged and that writers are queried sequentially (**REQ-028**), asserted via `fakeFeedbacker` call ordering

### Implementation for US2

- [x] T020 [US2] Implement `MultiWriter.CanFeedback` in `internal/sinks/multiwriter.go` (**REQ-020**, **REQ-021**)
- [x] T021 [US2] Implement `MultiWriter.LastMeasurement` per the §9.2 sketch: skip non-`Feedbacker` writers, skip `ErrFeedbackUnsupported`, short-circuit on `ErrNoFeedbackData`, `errors.Join` other errors, return the minimum (**REQ-022**…**REQ-028**, **PAT-002**)
- [x] T022 [US2] Add `var _ Feedbacker = (*MultiWriter)(nil)` (**GUD-003**)

**Checkpoint**: US1 and US2 both work; a Postgres + jsonfile deployment answers correctly

---

## Phase 5: US3 — RPC/gRPC Sink Feedback (Priority: P3)

**Goal**: A gRPC receiver can answer feedback queries, and receivers that do not implement the new method keep working with a single probe and no repeat cost.

**Independent Test**: Point the RPC sink at a `bufconn` receiver scripted to each row of the §4.5 status table; confirm each mapping, and that an `Unimplemented` reply flips `CanFeedback` to false permanently.

**Depends on**: Phase 2 plus T001 (protoc toolchain).

### Wire and plumbing for US3

- [x] T023 [US3] Add `GetLastMeasurement`, `FeedbackReq`, and `FeedbackReply` to `api/pb/pgwatch.proto` exactly as in spec §4.5 — additive only, no renumbering of existing fields (**CON-004**)
- [x] T024 [US3] Regenerate stubs with `task proto` and confirm `UnimplementedReceiverServer` gained the `GetLastMeasurement` fallback (**PLT-002**). The `*.pb.go` files are gitignored (`.gitignore:16`) and generated at build time, so there is nothing to commit
- [ ] T025 [US3] Pass `*CmdOpts` into `NewRPCWriter` in `internal/sinks/rpc.go` (mirroring `NewPostgresWriter`) so the sink can honour the kill switch; update the call site in `internal/sinks/multiwriter.go` and every constructor call in `internal/sinks/rpc_test.go` (§4.6)

### Tests for US3

- [ ] T026 [P] [US3] Extend `internal/testutil/mocks.go` with a scriptable `GetLastMeasurement` on `testutil.Receiver` — settable epoch and gRPC status — for use by `testutil.SetupRPCServers()` (§6 gRPC server double)
- [ ] T027 [P] [US3] `bufconn` tests in `internal/sinks/rpc_test.go` covering all five rows of §4.5: `OK` + positive epoch; `OK` + `EpochNs <= 0` → `ErrNoFeedbackData` (**RPC-007**, **E-09**); `codes.Unimplemented` → `ErrFeedbackUnsupported` and `CanFeedback` false thereafter with **zero** further round-trips (**AC-008**, **RPC-003**); `codes.NotFound` → `ErrNoFeedbackData` (**RPC-005**); `codes.Unavailable` → transport error with `CanFeedback` still true (**AC-009**, **E-08**)
- [ ] T028 [P] [US3] Test the `Unimplemented` path against a receiver that omits `GetLastMeasurement` entirely, so the status comes from gRPC itself rather than the double — `testutil.Receiver` embeds `pb.UnimplementedReceiverServer`, so this case works before T026 lands
- [ ] T029 [P] [US3] Race test: concurrent `LastMeasurement` calls exercising the cached-capability flag under `-race` (**RPC-008**)
- [ ] T030 [P] [US3] Backward-compatibility test: a receiver built against the pre-change service still handles `UpdateMeasurements`, `SyncMetric`, and `DefineMetrics` unchanged (**AC-016**)

### Implementation for US3

- [ ] T031 [US3] Add the atomic `unsupported` flag to `RPCWriter` and implement `CanFeedback`: gate on `opts.FeedbackEnabled()`, reject empty names, return optimistic true until the flag is set (**RPC-004**, **RPC-008**, **CFG-002**)
- [ ] T032 [US3] Implement `RPCWriter.LastMeasurement` per the §9.3 sketch: parent the call on `rw.ctx` so credential metadata is carried (**RPC-009**, **SEC-002**), derive the deadline from the caller's context with the 5 s default (**RPC-006**, **CON-002**), map every status per §4.5, set `unsupported` only on `Unimplemented` (**RPC-002**, **RPC-003**, **RPC-005**, **RPC-007**)
- [ ] T033 [US3] Add `var _ Feedbacker = (*RPCWriter)(nil)` (**GUD-003**)

**Checkpoint**: All three sink increments are independently functional

---

## Phase 6: Polish & Cross-Cutting Concerns

- [ ] T034 [P] Extend `internal/sinks/doc.go` to mention the optional feedback capability and which sinks provide it (§11)
- [ ] T035 [P] Document the new flag in `docs/reference/cli_env.md` and the two-level capability model plus the per-sink support matrix in `docs/reference/sinks_options.md` (§10 item 13)
- [ ] T036 [P] Add the §4.5 status-code contract to `docs/howto/implement_grpc_server.md` so third-party receiver authors know which codes carry which meaning (§10 item 13)
- [ ] T037 Run `task lint`, `gofmt -l internal/ api/`, and `go vet ./...`; all must be clean (**AC-018**, §10 item 12)
- [ ] T038 Verify **CON-006** behaviour neutrality against the T002 baseline: identical collection behaviour, with only the new flag in `--help` and the new gRPC method as visible additions (§10 item 14)
- [ ] T039 Walk spec §10 items 1–14 as a release checklist and confirm every **AC-001**…**AC-018** maps to a named test (§10 item 5)

---

## Open Decisions

Resolve before T004; each blocks a specific task.

| # | Decision | Blocks | Options |
|---|---|---|---|
| D-01 | ✅ **Resolved**: how to express a default-`true` bool under `go-flags` | T004, T035 | `go-flags` rejects `default:` on a bool outright (*"boolean flag may not have default values, they always default to `false' and can only be turned on"*), so the value-flag option does not exist. Implemented as the negation `--no-sink-feedback` / `PW_NO_SINK_FEEDBACK` plus a `FeedbackEnabled()` accessor, matching every other bool flag in the codebase |
| D-02 | Whether `NewRPCWriter` takes `*CmdOpts` (spec §4.6 recommendation) or reads a package-level switch | T025 | Passing `opts` mirrors `NewPostgresWriter` and keeps the sink self-contained; it costs one signature change plus test call-site updates |

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: no dependencies — start immediately
- **Foundational (Phase 2)**: depends on Setup — **blocks all increments**
- **US1 / US2 / US3 (Phases 3–5)**: all depend only on Phase 2; may proceed in parallel or in priority order P1 → P2 → P3
- **Polish (Phase 6)**: depends on whichever increments are being shipped

### Increment Dependencies

- **US1 (P1)**: no dependency on US2 or US3. `NewSinkWriter` unwraps a single-sink `MultiWriter`, so a Postgres-only deployment exercises US1 directly
- **US2 (P2)**: no dependency on US1 or US3 — its tests run entirely on `fakeFeedbacker`. It becomes *observable* in a real deployment once at least one real sink is capable
- **US3 (P3)**: depends on T001 (protoc) from Setup; independent of US1 and US2

### Within Each Increment

- Tests are written and confirmed failing before implementation
- Interface and errors (T003) before any sink implements them
- `CanFeedback` before `LastMeasurement` — the latter calls the former
- Proto change and regeneration (T023, T024) before any RPC implementation
- Compile-time assertions land with the implementation they assert

### Parallel Opportunities

- T005, T006, T007 run in parallel — three different concerns in `feedback_test.go` and the guard
- T008, T009, T010 run in parallel — same new test file, so coordinate or split by function
- All of US2's tests (T017–T019) run in parallel with each other and with US1
- US3's test tasks T026–T030 run in parallel; T026 and T027 touch different files
- All Polish doc tasks (T034–T036) run in parallel
- With three developers: A takes US1, B takes US2, C takes US3, immediately after Phase 2

---

## Parallel Example: Phase 2 → US1

```bash
# After T003 and T004, launch the foundational test scaffolding together:
Task: "Add fakeFeedbacker double in internal/sinks/feedback_test.go"
Task: "Add negative capability guards for Prometheus and JSON sinks"
Task: "Add the AC-017 scope guard"

# Then launch US1's unit tests together:
Task: "pgxmock CanFeedback tests in internal/sinks/postgres_feedback_test.go"
Task: "pgxmock LastMeasurement path tests"
Task: "SQL parameterisation and identifier-sanitisation assertions"
```

---

## Implementation Strategy

### MVP First (US1 only)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational (blocks everything)
3. Complete Phase 3: US1 — Postgres sink feedback
4. **STOP and VALIDATE**: run the US1 tests plus T037; confirm behaviour neutrality per T038
5. Mergeable on its own — the capability exists, is tested, and nothing calls it

### Incremental Delivery

1. Setup + Foundational → interface and switch in place
2. Add US1 → Postgres answers → merge (MVP)
3. Add US2 → multi-sink aggregation correct → merge
4. Add US3 → gRPC receivers can answer → merge
5. Polish → docs, lint, full §10 checklist

Each increment leaves the tree green and adds no runtime behaviour.

---

## Notes

- **This change ships no consumer.** T007's guard exists precisely so one cannot be added here by accident; if it fails, something outside `internal/sinks` started calling the interface (**AC-017**, **CON-006**)
- `[P]` tasks touch different files or independent concerns — no dependencies
- Verify tests fail before implementing
- Commit after each task or logical group
- Stop at any checkpoint to validate an increment independently
- `task test` spins up Postgres/etcd testcontainers, so Docker must be running for T011
