---
description: "Task list for Prometheus Exporter as a pgwatch Source"
---

# Tasks: Prometheus Exporter as a pgwatch Source

**Input**: `spec/architecture-prometheus-exporter-source.md`, `spec/refactor-sourceconn-interface.md`
**Prerequisites**: Both spec files must be read before implementation begins.

**Tests**: TDD — write failing tests first, then implement. All test tasks are mandatory.

**Organization**: Tasks are grouped by delivery phase. The interface refactoring (Phase 2) is a
hard prerequisite for all subsequent phases.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story or spec section this task belongs to

---

## Phase 1: Setup

**Purpose**: Verify the build is clean and add the test infrastructure helpers that all phases share.

- [x] T001 [P] Confirm `go build ./...` passes on current `main` branch as baseline
- [x] T002 [P] Add `httptest` helper `newFakeExporter(t, body string) *httptest.Server` to `internal/testutil/` for reuse across reaper and sources tests

---

## Phase 2: Foundation — `SourceConn` Interface Refactoring

**Purpose**: Replace the single `SourceConn` concrete struct with a `SourceConn` interface, a
`DbConn` implementation for DB-backed sources, and a `PromConn` implementation for prometheus
sources. No user-facing behaviour changes; all existing tests must still pass.

**Spec reference**: `spec/refactor-sourceconn-interface.md`

**⚠️ CRITICAL**: Phases 3–6 depend on `PromConn` and `DbConn` being in place. No phase 3+ work
can begin until this phase is complete and the full test suite is green.

### Tests for Phase 2

> **Write these tests FIRST; ensure they FAIL before implementation.**

- [x] T003 [P] Write compile-time interface checks (these will fail until T008–T011 are done):
  - `var _ sources.SourceConn = (*sources.DbConn)(nil)` in `internal/sources/conn_test.go`
  - `var _ sources.SourceConn = (*sources.PromConn)(nil)` in `internal/sources/conn_test.go`
  - `var _ SourceRunner = (*SourceReaper)(nil)` in `internal/reaper/source_reaper_test.go`
- [x] T004 [P] Table-driven test for `DbConn.IsPostgresSource()` returns `true` for all DB kinds in `internal/sources/conn_test.go`
- [x] T005 [P] Table-driven test for `PromConn.IsPostgresSource()` returns `false` in `internal/sources/conn_test.go`
- [x] T006 [P] Test that `PromConn.FetchRuntimeInfo()` sets `VersionStr = "prometheus"` and `Version = 0` in `internal/sources/conn_test.go`

### Implementation for Phase 2

- [x] T007 Define `SourceConn` interface in `internal/sources/conn.go` with methods: `Connect`, `Ping`, `IsPostgresSource`, `GetSource`, `GetMetricInterval` (REQ-001 from refactor spec)
- [x] T008 Rename existing `SourceConn` struct → `DbConn` in `internal/sources/conn.go`; preserve all DB-specific fields and methods unchanged (REQ-002)
- [x] T009 [P] Create `PromConn` struct in `internal/sources/conn.go` with `Source`, `HTTPClient *http.Client`, `sync.RWMutex`; add `NewPromConn` constructor (REQ-003)
- [x] T010 [P] Add `NewDbConn` constructor aliasing the existing `NewSourceConn`; update `SourceConns` to `[]SourceConn` (interface slice) (REQ-004/REQ-005)
- [x] T011 Update all call sites in `internal/reaper/`, `internal/sources/`, and other packages that hold `*sources.SourceConn` to use the interface or concrete type as required
- [x] T012 Define `SourceRunner` interface in `internal/reaper/runner.go`:
  ```go
  type SourceRunner interface { Run(ctx context.Context) }
  var _ SourceRunner = (*SourceReaper)(nil)
  ```
- [x] T013 Change `Reaper.sourceReapers` field type from `map[string]*SourceReaper` to `map[string]SourceRunner` in `internal/reaper/reaper.go`

**Checkpoint**: `go test ./...` is green; interface checks in T003 compile and pass.

---

## Phase 3: Source Kind Registration (US1 — Source Definition)

**Purpose**: Register `SourcePrometheus` as a valid `Kind`, add it to `Kinds`, and ensure YAML
round-trip works. Covers REQ-001, REQ-004, REQ-005, REQ-034.

### Tests for Phase 3

> **Write FIRST; ensure they FAIL before implementation.**

- [x] T014 [P] Table-driven test: `Kind("prometheus").IsValid()` returns `true`; existing kinds still valid; `Kind("unknown").IsValid()` returns `false` — in `internal/sources/types_test.go`
- [x] T015 [P] YAML unmarshal test: a `prometheus` source with `conn_str`, `custom_metrics`, and `custom_tags` round-trips correctly through `sources.Sources.Validate()` — in `internal/sources/yaml_test.go`
- [x] T016 [P] Test `sources.Validate()` with `kind: prometheus` does NOT error on `IncludePattern`, `ExcludePattern`, `OnlyIfMaster`, `MetricsStandby`, `PresetMetricsStandby` being empty — in `internal/sources/types_test.go` (CON-001)

### Implementation for Phase 3

- [x] T017 Add `SourcePrometheus Kind = "prometheus"` constant and append it to `Kinds` slice in `internal/sources/types.go` (REQ-001, REQ-005)
- [x] T018 Update `Sources.Validate()` in `internal/sources/types.go` to accept `kind: prometheus`; no special validation needed for ignored fields (CON-001)

**Checkpoint**: `go test ./internal/sources/...` is green.

---

## Phase 4: PromConn Connection Lifecycle (US2 — Authentication & TLS)

**Purpose**: Implement `PromConn.Connect()`, `Ping()`, `FetchRuntimeInfo()` with TLS query-parameter
parsing, Basic Auth from userinfo, and password redaction. Covers REQ-006, REQ-007, REQ-011–REQ-015,
SEC-001.

### Tests for Phase 4

> **Write FIRST; ensure they FAIL before implementation.**

- [x] T019 [P] Test `PromConn.Connect()` against `httptest.NewTLSServer` with `tlsskipverify=true` in `internal/sources/conn_test.go`:
  - Connection succeeds
  - TLS params are stripped from the actual request URL
- [x] T020 [P] Test `PromConn.Connect()` with `http://user:secret@host/metrics`: Basic Auth header sent, password not logged (SEC-001); use a capturing handler
- [x] T021 [P] Test `PromConn.Connect()` returns error when exporter is unreachable (REQ-012)
- [x] T022 [P] Test `PromConn.Ping()` returns nil for 2xx, error for 4xx/5xx in `internal/sources/conn_test.go` (REQ-013)
- [x] T023 [P] Test `redactURL(url)` helper strips password from userinfo component, leaves URL otherwise unchanged — in `internal/sources/conn_test.go` (SEC-001)

### Implementation for Phase 4

- [x] T024 Add `redactURL(rawURL string) string` helper to `internal/sources/conn.go` that clears the password from `url.URL.User` before returning the URL string (SEC-001)
- [x] T025 Implement `PromConn.Connect(ctx, opts)` in `internal/sources/conn.go`:
  - Parse `tlsrootcert` and `tlsskipverify` query params from `ConnStr`
  - Build `*http.Client` with derived `tls.Config`
  - Strip TLS params from URL before making reachability HEAD/GET
  - Validate reachability; return error if unreachable (REQ-012)
- [x] T026 Implement `PromConn.Ping(ctx)` — HEAD request; error if status ≥ 300 (REQ-013)
- [x] T027 Implement `PromConn.FetchRuntimeInfo()` — no-op (REQ-015); note: `PromConn` has no `RuntimeInfo` field, so this may just be a method stub returning these values

**Checkpoint**: `go test ./internal/sources/...` is green.

---

## Phase 5: `ScrapeAll` Core (US3 — Scrape & Measurement Rows)

**Purpose**: Implement `ScrapeAll` in `internal/reaper/prometheus.go` and extend
`MeasurementEnvelope` with `SourceKind`. Covers REQ-021–REQ-026, REQ-023, REQ-024, REQ-025.

### Tests for Phase 5

> **Write FIRST; ensure they FAIL before implementation.**

- [ ] T028 [P] Table-driven test `TestScrapeAll` in `internal/reaper/prometheus_test.go`:
  - Serve fixture exposition text via `httptest.NewServer`
  - Assert each sample maps to one `Measurement` with `tag_<label>` columns and value column named after the family
  - Assert `epoch_ns` uses sample timestamp when present
  - Assert `epoch_ns` falls back to `time.Now()` when timestamp absent (approximate check)
  - Assert `__name__` label is NOT present as a column (REQ-024)
- [ ] T029 [P] Test non-finite values (`+Inf`, `-Inf`, `NaN`) are preserved as-is in the value column (REQ-025)
- [ ] T030 [P] Test that `ScrapeAll` sends `Accept: text/plain` in the request header (CON-003)
- [ ] T031 [P] Test that `MeasurementEnvelope.SourceKind` field exists and is correctly marshalled/used — in `internal/metrics/types_test.go` (REQ-026)

### Implementation for Phase 5

- [ ] T032 Add `SourceKind string` field to `MeasurementEnvelope` in `internal/metrics/types.go` (REQ-026)
- [ ] T033 Implement `ScrapeAll(ctx context.Context, sc *sources.PromConn) (map[string]metrics.Measurements, error)` in `internal/reaper/prometheus.go` using `github.com/prometheus/common/expfmt` (REQ-021–REQ-025):
  - Single GET with `Accept: text/plain`
  - For each family and each sample: build `Measurement` with `tag_<label>`, value column, `epoch_ns`
  - Skip `__name__` label
  - Use sample timestamp when available; `time.Now().UnixNano()` otherwise
  - Preserve non-finite floats as-is

**Checkpoint**: `go test ./internal/reaper/... ./internal/metrics/...` is green.

---

## Phase 6: `PromSourceReaper` (US3 — Scrape Loop & Emit Gating)

**Purpose**: Implement the per-source goroutine that drives scrapes on a GCD tick loop and
applies per-family emit-interval gating. Covers REQ-016–REQ-020, REQ-017 (scrape-all mode).

### Tests for Phase 6

> **Write FIRST; ensure they FAIL before implementation.**

- [ ] T034 [P] Table-driven test `TestCalcScrapeInterval` in `internal/reaper/prom_source_reaper_test.go`:
  - Multiple emit intervals → GCD result
  - Single interval → that value
  - Empty intervals (scrape-all) → 60 s
  - All intervals below `minTickInterval` → floored to `minTickInterval`
- [ ] T035 [P] Test scrape-all mode: when `Metrics` is empty, `Run` activates 60 s interval and logs warning — use a context with timeout and a counting fake exporter (REQ-010, REQ-017)
- [ ] T036 [P] Test per-family emit gating in `internal/reaper/prom_source_reaper_test.go`:
  - Family with `emitInterval = 60 s`: emitted on first tick, NOT emitted on second tick at 30 s, emitted on tick at 60 s
  - `lastEmitted` is zero on first tick → always emit (REQ-019)
- [ ] T037 [P] Test `ScrapeAll` error path: warning logged, `lastEmitted` unchanged, loop continues without crash (REQ-020)
- [ ] T038 [P] Compile-time check `var _ SourceRunner = (*PromSourceReaper)(nil)` in `internal/reaper/prom_source_reaper_test.go` (REQ-016)

### Implementation for Phase 6

- [ ] T039 Create `internal/reaper/prom_source_reaper.go`:
  - `PromSourceReaper` struct with `reaper *Reaper`, `md *sources.PromConn`, `lastEmitted map[string]time.Time`
  - `NewPromSourceReaper(r *Reaper, md *sources.PromConn) *PromSourceReaper`
  - `calcScrapeInterval() time.Duration` using `GCDSlice`; defaults to 60 s when `md.Metrics` is empty
  - `Run(ctx context.Context)` tick loop with family filtering and `lastEmitted` gating; sets `SourceKind: string(sources.SourcePrometheus)` on every emitted envelope
- [ ] T040 Wire `PromSourceReaper` into `Reaper.Reap()` in `internal/reaper/reaper.go`: when `source.Kind == sources.SourcePrometheus`, instantiate `PromSourceReaper` and store as `SourceRunner` in `sourceReapers` map (REQ-016)

**Checkpoint**: `go test ./internal/reaper/...` is green; no data races (`go test -race ./internal/reaper/...`).

---

## Phase 7: Prom→Prom Proxy Path (US5 — PrometheusWriter)

**Purpose**: Add fast-path in `PrometheusWriter` to emit prom-sourced envelopes with original
metric names and labels, without prepending the pgwatch namespace. Covers REQ-027–REQ-031, GUD-003.

### Tests for Phase 7

> **Write FIRST; ensure they FAIL before implementation.**

- [ ] T041 [P] Test `isPromSourcedEnvelope` returns `true` only when `SourceKind == "prometheus"` — in `internal/sinks/prometheus_test.go` (REQ-027)
- [ ] T042 [P] Test `PrometheusWriter.Write()` + `Collect()` for a prom-sourced envelope in `internal/sinks/prometheus_test.go`:
  - Value column is used as the metric name (envelope.MetricName) without namespace prefix (GUD-003)
  - `tag_*` columns become Prometheus label key-value pairs (REQ-029)
  - `epoch_ns` converted to `time.Time` for metric timestamp (REQ-030)
  - Duplicate `(metric_name, label_set)` pairs deduplicated within one `Collect()` call (REQ-031)
- [ ] T043 [P] Test that pgwatch namespace IS still prepended for non-prometheus-sourced envelopes (regression guard, GUD-003)

### Implementation for Phase 7

- [ ] T044 Add `isPromSourcedEnvelope(envelope metrics.MeasurementEnvelope) bool` helper in `internal/sinks/prometheus.go` (REQ-027)
- [ ] T045 Add prom-sourced branch in `PrometheusWriter.Write()`: cache envelope as-is, tagged with original metric name (REQ-028)
- [ ] T046 Add prom-sourced branch in `PrometheusWriter.Collect()`: for each cached prom-sourced envelope, iterate rows, build `prometheus.Desc` from metric name (no namespace) + sorted `tag_*` label keys, emit `prometheus.MustNewConstMetric`; deduplicate using the existing dedup mechanism (REQ-028–REQ-031, GUD-003)

**Checkpoint**: `go test ./internal/sinks/...` is green; no data races.

---

## Phase 8: Preset & Configuration (US4 — Preset Support & YAML)

**Purpose**: Add the `postgres-exporter-basic` built-in preset and update configuration examples.
Covers REQ-032, REQ-033, GUD-004.

### Tests for Phase 8

> **Write FIRST; ensure they FAIL before implementation.**

- [ ] T047 [P] Test that preset `postgres-exporter-basic` is resolvable from the default metrics store and yields `MetricIntervals` with at least `pg_stat_activity_count`, `pg_stat_bgwriter_checkpoints_timed`, `pg_stat_replication_pg_wal_lsn_diff` — in `internal/metrics/default_test.go` (REQ-032)
- [ ] T048 [P] Test YAML unmarshal of the full example from REQ-033 (including `custom_tags`) produces a valid `Sources` list accepted by `Validate()` — in `internal/sources/yaml_test.go`

### Implementation for Phase 8

- [ ] T049 Add `postgres-exporter-basic` preset entry to `internal/metrics/metrics.yaml` in a dedicated `# Prometheus-targeted presets` section (REQ-032, GUD-004)
- [ ] T050 Add a `prometheus` source example to `contrib/sample.sources.yaml` (REQ-033)

**Checkpoint**: `go test ./internal/metrics/... ./internal/sources/...` is green.

---

## Phase 9: Cross-Cutting Concerns

**Purpose**: Race-detector sweep, build verification, and documentation.

- [ ] T051 Run `go test -race -failfast -p 1 -timeout=300s -parallel=1 ./...` and fix any detected races
- [ ] T052 [P] Run `go build ./cmd/pgwatch/` and confirm binary builds without errors
- [ ] T053 [P] Update `docs/reference/` or `docs/howto/` with a short section on configuring prometheus sources and the `postgres-exporter-basic` preset

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: No dependencies — start immediately
- **Phase 2 (Interface Refactor)**: Depends on Phase 1 — **BLOCKS** Phases 3–7
- **Phase 3 (Kind Registration)**: Depends on Phase 2
- **Phase 4 (PromConn Lifecycle)**: Depends on Phase 2
- **Phase 5 (ScrapeAll)**: Depends on Phase 2
- **Phase 6 (PromSourceReaper)**: Depends on Phases 3, 4, and 5
- **Phase 7 (Prom→Prom Proxy)**: Depends on Phase 5 (`SourceKind` field on envelope)
- **Phase 8 (Preset & Config)**: Depends on Phase 3 (`SourcePrometheus` kind)
- **Phase 9 (Cross-Cutting)**: Depends on all previous phases

### Parallel Opportunities Within a Phase

- Phases 3, 4, and 5 can proceed in parallel once Phase 2 is complete
- Phases 7 and 8 can proceed in parallel once their respective prerequisites are met
- All [P]-marked tasks within a phase operate on independent files and can be worked concurrently

---

## Modern Go Notes

Use Go 1.26 idioms throughout:

- `slices.Contains(sources.Kinds, k)` — already used for `Kind.IsValid()`; extend with new constant
- `maps.Keys` / `range over map` for iterating `MetricIntervals`
- `max(a, b)` built-in for interval floor logic in `calcScrapeInterval`
- `errors.Join` to combine multiple connect/parse errors
- Table-driven tests with `t.Run` and typed subtests for all HTTP behaviour
- `httptest.NewServer` / `httptest.NewTLSServer` for all HTTP interaction tests — no live network
- `context.WithCancelCause` for test goroutine shutdown in `Run()` loop tests
