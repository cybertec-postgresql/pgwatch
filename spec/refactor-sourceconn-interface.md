---
title: Refactor SourceConn from Concrete Struct to Interface Hierarchy
version: 1.1
date_created: 2026-05-08
date_updated: 2026-05-08
tags: [architecture, refactoring, sources, interfaces]
---

# Introduction

This specification defines the refactoring of `sources.SourceConn` from a single concrete struct
with optional nil fields into a `SourceConn` interface implemented by two concrete types:
`DbConn` (postgres, pgbouncer, pgpool, patroni) and `PromConn` (prometheus). The refactoring
eliminates nil-field hazards, removes the runtime `IsPostgresSource()` discriminator need at
call sites, and provides clean extension points for future source kinds (e.g., `RestConn`).

This refactoring is a **prerequisite or parallel track** to implementing the Prometheus exporter
source feature described in `architecture-prometheus-exporter-source.md`. The prometheus spec
assumes this interface hierarchy is in place.

---

## 1. Purpose & Scope

**Purpose**: Replace the single `SourceConn` struct — which mixes DB-specific fields (`Conn`,
`ConnConfig`, `RuntimeInfo`) with an HTTP-specific field (`HTTPClient`) added for prometheus
support — with a clean interface and two focused implementations.

**In scope**:
- New `SourceConn` interface in `internal/sources/`.
- `DbConn` concrete type replacing the existing `SourceConn` struct for all DB-backed sources.
- `PromConn` concrete type for prometheus sources.
- Update of `SourceConns` collection type.
- Update of all call sites in `internal/reaper/`, `internal/sources/`, and any other package
  that holds `*sources.SourceConn`.
- Constructor functions `NewDbConn` and `NewPromConn`.

**Out of scope**:
- Changes to `Source` (the configuration struct) — it remains unchanged.
- Changes to sink implementations.
- The `ScrapeAll` function body and prometheus goroutine logic (specified separately).
- Any new source kind beyond `DbConn` and `PromConn`.

**Audience**: pgwatch maintainers. Also intended for direct consumption by AI coding assistants.

---

## 2. Motivation — Current Problems

| Problem | Location |
|---|---|
| `SourceConn.Conn` is `nil` for prometheus sources | `internal/sources/conn.go` |
| `SourceConn.ConnConfig` is `nil` for prometheus sources | `internal/sources/conn.go` |
| `SourceConn.RuntimeInfo` is meaningless for prometheus sources | `internal/sources/conn.go` |
| `HTTPClient` (added for prometheus) is `nil` for all DB sources | `internal/sources/conn.go` |
| `IsPostgresSource()` is a runtime type-discriminator on a concrete struct | `internal/sources/conn.go` |
| `FetchRuntimeInfo` is a no-op for prometheus, yet must be implemented | `internal/sources/conn.go` |
| `ParseConfig`, `GetClusterIdentifier`, `SetDatabaseName` panic if called on prometheus source | `internal/sources/conn.go` |
| `FunctionExists`, `TryCreateMissingExtensions`, `TryCreateMetricsHelpers` are unreachable for prometheus | `internal/sources/conn.go` |

---

## 3. Requirements

### Interface Definition

- **REQ-001**: A new `SourceConn` interface MUST be defined in `internal/sources/` with the
  following method set — the minimum needed by the reaper's shared dispatch loop:

  ```go
  type SourceConn interface {
      // Connect establishes or validates the underlying transport (DB pool or HTTP client).
      Connect(ctx context.Context, opts cmdopts.CmdOpts) error

      // Ping checks liveness of the underlying transport.
      Ping(ctx context.Context) error

      // IsPostgresSource returns true for kinds that speak the PostgreSQL wire protocol.
      IsPostgresSource() bool

      // GetSource returns the configuration struct for this connection.
      GetSource() Source

      // GetMetricInterval returns the effective collection interval for the named metric,
      // taking standby configuration into account.
      GetMetricInterval(name string) time.Duration
  }
  ```

- **REQ-002**: The existing `SourceConn` struct MUST be renamed to `DbConn`. All its existing
  fields and methods that are DB-specific MUST be preserved on `DbConn` unchanged.

- **REQ-003**: A new `PromConn` struct MUST be defined for prometheus sources. It MUST contain
  only fields relevant to HTTP scraping:

  ```go
  type PromConn struct {
      Source
      HTTPClient *http.Client
      sync.RWMutex
  }
  ```

- **REQ-004**: Both `DbConn` and `PromConn` MUST implement the `SourceConn` interface. The Go
  compiler MUST be able to verify this statically (e.g., via `var _ SourceConn = (*DbConn)(nil)`
  and `var _ SourceConn = (*PromConn)(nil)` compile-time assertions in the package).

- **REQ-005**: `SourceConns` MUST be redefined as `[]SourceConn` (interface slice), replacing
  the current `[]*SourceConn` (concrete pointer slice). `GetMonitoredDatabase` MUST be updated
  accordingly.

### `DbConn`

- **REQ-006**: `DbConn` MUST retain all existing fields:
  - `Source` (embedded)
  - `Conn db.PgxPoolIface`
  - `ConnConfig *pgxpool.Config`
  - `RuntimeInfo` (embedded)
  - `sync.RWMutex` (embedded)

- **REQ-007**: All methods currently on `SourceConn` that access `Conn`, `ConnConfig`, or
  `RuntimeInfo` MUST remain on `DbConn` with identical signatures:
  `ParseConfig`, `GetClusterIdentifier`, `GetDatabaseName`, `SetDatabaseName`,
  `IsClientOnSameHost`, `FetchRuntimeInfo`, `FetchVersion`, `DiscoverPlatform`,
  `FetchApproxSize`, `FunctionExists`, `TryCreateMissingExtensions`, `TryCreateMetricsHelpers`.

- **REQ-008**: `DbConn.IsPostgresSource()` MUST return `true` unless `Kind` is `SourcePgBouncer`
  or `SourcePgPool` (identical to the current implementation).

- **REQ-009**: `DbConn.GetSource()` MUST return a copy of the embedded `Source` struct.

- **REQ-010**: `NewDbConn(s Source) *DbConn` MUST replace the current `NewSourceConn` constructor,
  initialising `RuntimeInfo.Extensions` and `RuntimeInfo.ChangeState` as before.

### `PromConn`

- **REQ-011**: `PromConn.Connect(ctx, opts)` MUST:
  1. Parse TLS query parameters (`tlsrootcert`, `tlsskipverify`) from `Source.ConnStr`.
  2. Construct an `*http.Client` with the derived `tls.Config` and store it in `HTTPClient`.
  3. Issue a HEAD request to the scrape URL (stripped of TLS query parameters) to validate
     reachability. Return an error if the response status is not 2xx or the request fails.
  4. Log a warning if `tlsskipverify=true` is present (SEC-002 from the prometheus spec).

- **REQ-012**: `PromConn.Ping(ctx)` MUST issue a HEAD request to the scrape URL and return an
  error if the status is not 2xx.

- **REQ-013**: `PromConn.IsPostgresSource()` MUST return `false`.

- **REQ-014**: `PromConn.GetSource()` MUST return a copy of the embedded `Source` struct.

- **REQ-015**: `PromConn.GetMetricInterval(name)` MUST return
  `time.Duration(Source.Metrics[name]) * time.Second`. Standby metrics are not applicable to
  prometheus sources; `Source.MetricsStandby` MUST be ignored.

- **REQ-016**: `NewPromConn(s Source) *PromConn` MUST be the constructor. `HTTPClient` MUST be
  `nil` until `Connect` is called.

### Constructor Dispatch

- **REQ-017**: A factory function `NewSourceConn(s Source) SourceConn` MUST be provided that
  returns `NewPromConn(s)` when `s.Kind == SourcePrometheus` and `NewDbConn(s)` otherwise. This
  preserves a single call site for callers that create connections from a `Source` config
  without knowing the kind in advance.

### Call-Site Updates

- **REQ-018**: Every function and struct field in `internal/reaper/` that currently references
  `*sources.SourceConn` MUST be updated to the most specific type it actually needs:
  - `SourceReaper.md` struct field and `NewSourceReaper` constructor parameter MUST be typed
    as `*sources.DbConn`. `SourceReaper` is exclusively a DB-source runner; all its field
    accesses (`Conn`, `RuntimeInfo`, `RLock`/`RUnlock`) are valid on `*DbConn` without any
    type assertion.
  - Functions that only issue SQL queries or access DB-specific runtime info
    (`QueryMeasurements`, `DetectSprocChanges`, `DetectTableChanges`, `DetectIndexChanges`,
    `DetectPrivilegeChanges`, `DetectConfigurationChanges`, `GetInstanceUpMeasurement`,
    `GetObjectChangesMeasurement`, `AddSysinfoToMeasurements`, `CreateSourceHelpers`,
    `FetchStatsDirectlyFromOS`, `NewLogParser`, `checkHasRemotePrivileges`) MUST accept
    `*sources.DbConn`.
  - The new `ScrapeAll` function (prometheus spec) MUST accept `*sources.PromConn`.

- **REQ-019**: `SourceConns.GetMonitoredDatabase` MUST continue to accept a name string and
  return `SourceConn` (interface), searching by `sc.GetSource().Name`.

- **REQ-020**: Any type assertion from `sources.SourceConn` (interface) to `*sources.DbConn`
  or `*sources.PromConn` MUST only appear in the reaper dispatch block (REQ-023), not
  scattered across helper functions or within `SourceReaper` methods.

### Reaper Dispatch Structure

- **REQ-021**: A `SourceRunner` interface MUST be defined in `internal/reaper/` with a single
  method `Run(ctx context.Context)`. Compile-time assertions
  (`var _ SourceRunner = (*SourceReaper)(nil)` and
  `var _ SourceRunner = (*PromSourceReaper)(nil)`) MUST be present.

- **REQ-022**: `Reaper.sourceReapers` MUST be retyped from `map[string]*SourceReaper` to
  `map[string]SourceRunner`.

- **REQ-023**: In `Reaper.Reap()`, the runner creation block MUST dispatch to
  `NewSourceReaper(*DbConn)` for DB sources and `NewPromSourceReaper(*PromConn)` for
  prometheus sources, selected via a `switch` on `source.Kind`.

- **REQ-024**: Type assertions from `sources.SourceConn` (interface) to concrete types MUST
  only appear in the dispatch block described in REQ-023.

---

## 4. Interfaces & Data Contracts

### 4.1 `SourceConn` interface

```go
// internal/sources/conn.go

// SourceConn is the runtime handle for a monitored source. Implementations
// are DbConn (all DB-backed kinds) and PromConn (prometheus).
type SourceConn interface {
    Connect(ctx context.Context, opts cmdopts.CmdOpts) error
    Ping(ctx context.Context) error
    IsPostgresSource() bool
    GetSource() Source
    GetMetricInterval(name string) time.Duration
}
```

### 4.2 `DbConn` struct

```go
// internal/sources/conn.go

// DbConn is the runtime handle for sources that use the PostgreSQL wire
// protocol: postgres, postgres-continuous-discovery, pgbouncer, pgpool, patroni.
type DbConn struct {
    Source
    Conn        db.PgxPoolIface
    ConnConfig  *pgxpool.Config
    RuntimeInfo
    sync.RWMutex
}

func NewDbConn(s Source) *DbConn {
    return &DbConn{
        Source: s,
        RuntimeInfo: RuntimeInfo{
            Extensions:  make(map[string]int),
            ChangeState: make(map[string]map[string]string),
        },
    }
}

// compile-time assertion
var _ SourceConn = (*DbConn)(nil)
```

`DbConn` retains all methods currently on `SourceConn` (see §2 for the full list). Their
signatures are identical; only the receiver type changes from `*SourceConn` to `*DbConn`.

### 4.3 `PromConn` struct

```go
// internal/sources/conn.go

// PromConn is the runtime handle for prometheus sources. HTTPClient is nil
// until Connect is called.
type PromConn struct {
    Source
    HTTPClient *http.Client
    sync.RWMutex
}

func NewPromConn(s Source) *PromConn {
    return &PromConn{Source: s}
}

// compile-time assertion
var _ SourceConn = (*PromConn)(nil)
```

### 4.4 `NewSourceConn` factory

```go
// internal/sources/conn.go

// NewSourceConn returns the correct SourceConn implementation for the given Source.
func NewSourceConn(s Source) SourceConn {
    if s.Kind == SourcePrometheus {
        return NewPromConn(s)
    }
    return NewDbConn(s)
}
```

### 4.5 `SourceConns` collection

```go
// internal/sources/conn.go

type SourceConns []SourceConn

func (mds SourceConns) GetMonitoredDatabase(name string) SourceConn {
    for _, sc := range mds {
        if sc.GetSource().Name == name {
            return sc
        }
    }
    return nil
}
```

### 4.6 `SourceRunner` interface

```go
// internal/reaper/source_runner.go

// SourceRunner is the common interface for per-source collection goroutines.
// It allows Reaper to manage SourceReaper (DB sources) and PromSourceReaper
// (prometheus sources) through a single map without kind-specific branching.
type SourceRunner interface {
    Run(ctx context.Context)
}

// compile-time assertions
var _ SourceRunner = (*SourceReaper)(nil)
var _ SourceRunner = (*PromSourceReaper)(nil)
```

`Reaper.sourceReapers` MUST be declared as `map[string]SourceRunner`. The runner creation
block in `Reaper.Reap()` type-asserts the `SourceConn` interface value to the correct concrete
type and passes it to the appropriate constructor:

```go
if _, exists := r.sourceReapers[source.Name]; !exists {
    var runner SourceRunner
    switch source.Kind {
    case sources.SourcePrometheus:
        runner = NewPromSourceReaper(r, monitoredSource.(*sources.PromConn))
    default:
        runner = NewSourceReaper(r, monitoredSource.(*sources.DbConn))
    }
    sourceCtx, cancel := context.WithCancel(ctx)
    r.cancelFuncs[source.Name] = cancel
    r.sourceReapers[source.Name] = runner
    go runner.Run(sourceCtx)
}
```

### 4.7 Reaper call-site summary

The following functions and struct fields change type.

| Symbol | Kind | Old type | New type |
|---|---|---|---|
| `SourceReaper.md` | struct field | `*sources.SourceConn` | `*sources.DbConn` |
| `NewSourceReaper` | constructor param | `*sources.SourceConn` | `*sources.DbConn` |
| `Reaper.sourceReapers` | struct field | `map[string]*SourceReaper` | `map[string]SourceRunner` |
| `QueryMeasurements` | function param | `*sources.SourceConn` | `*sources.DbConn` |
| `DetectSprocChanges` | function param | `*sources.SourceConn` | `*sources.DbConn` |
| `DetectTableChanges` | function param | `*sources.SourceConn` | `*sources.DbConn` |
| `DetectIndexChanges` | function param | `*sources.SourceConn` | `*sources.DbConn` |
| `DetectPrivilegeChanges` | function param | `*sources.SourceConn` | `*sources.DbConn` |
| `DetectConfigurationChanges` | function param | `*sources.SourceConn` | `*sources.DbConn` |
| `GetInstanceUpMeasurement` | function param | `*sources.SourceConn` | `*sources.DbConn` |
| `GetObjectChangesMeasurement` | function param | `*sources.SourceConn` | `*sources.DbConn` |
| `AddSysinfoToMeasurements` | function param | `*sources.SourceConn` | `*sources.DbConn` |
| `CreateSourceHelpers` | function param | `*sources.SourceConn` | `*sources.DbConn` |
| `FetchStatsDirectlyFromOS` | function param | `*sources.SourceConn` | `*sources.DbConn` |
| `NewLogParser` | function param | `*sources.SourceConn` | `*sources.DbConn` |
| `checkHasRemotePrivileges` | function param | `*sources.SourceConn` | `*sources.DbConn` |
| `ScrapeAll` (new) | function param | — | `*sources.PromConn` |

---

## 5. Acceptance Criteria

- **AC-001**: `var _ sources.SourceConn = (*sources.DbConn)(nil)` compiles without error.
- **AC-002**: `var _ sources.SourceConn = (*sources.PromConn)(nil)` compiles without error.
- **AC-003**: `sources.NewSourceConn(Source{Kind: SourcePrometheus})` returns a `*PromConn`.
- **AC-004**: `sources.NewSourceConn(Source{Kind: SourcePostgres})` returns a `*DbConn`.
- **AC-005**: `(*PromConn).Conn` does not exist as a field; attempting to access it is a
  compile-time error.
- **AC-006**: `(*DbConn).HTTPClient` does not exist as a field; attempting to access it is a
  compile-time error.
- **AC-007**: `PromConn.IsPostgresSource()` returns `false`.
- **AC-008**: `DbConn.IsPostgresSource()` returns `false` only for `SourcePgBouncer` and
  `SourcePgPool`, and `true` for all other kinds.
- **AC-009**: All existing unit tests in `internal/sources/` and `internal/reaper/` pass
  without modification to their test logic (only type references updated).
- **AC-010**: `go vet ./...` and `golangci-lint run` produce no new errors after the refactor.
- **AC-011**: `SourceConns.GetMonitoredDatabase` returns `nil` (not panics) when the name is
  not found.
- **AC-012**: `var _ SourceRunner = (*SourceReaper)(nil)` compiles without error.
- **AC-013**: `var _ SourceRunner = (*PromSourceReaper)(nil)` compiles without error.
- **AC-014**: `Reaper.sourceReapers` is declared as `map[string]SourceRunner`; assigning a
  `*PromSourceReaper` value to it compiles without error.

---

## 6. Test Automation Strategy

- **Approach**: This is a pure refactoring — no new behaviour is introduced. The primary test
  strategy is **compilation + existing test suite green**.
- **Compile-time assertions**: Add `var _ SourceConn = (*DbConn)(nil)` and
  `var _ SourceConn = (*PromConn)(nil)` in `internal/sources/conn.go` so any interface
  drift is caught immediately.
- **Unit tests to add**:
  - `TestNewSourceConnDispatch`: verify `NewSourceConn` returns the correct concrete type for
    each `Kind` value.
  - `TestPromConnIsNotPostgresSource`: verify `PromConn.IsPostgresSource()` is `false`.
  - `TestDbConnIsPostgresSource`: table-driven test across all `Kind` values.
  - `TestSourceConnsGetMonitoredDatabase`: verify nil return for missing name, correct return
    for existing name, works with mixed `DbConn` / `PromConn` in the slice.
- **Coverage**: The `internal/sources` package already has tests; no coverage regression is
  expected. New test functions MUST maintain the existing ≥ 80 % coverage level.

---

## 7. Rationale

### Why an interface instead of a union struct with nil fields?

The nil-field pattern provides no compile-time safety: calling `DbConn`-specific methods on a
`PromConn` value (once the prometheus source is active) would panic at runtime rather than fail
at compile time. An interface moves the error to compile time: a function that accepts
`*DbConn` cannot be accidentally called with a `*PromConn`. This is the standard Go idiom for
polymorphic types with disjoint behaviour.

### Why two concrete types instead of more granular splitting (e.g., one per Kind)?

`pgbouncer`, `pgpool`, `patroni`, and `postgres` all share the same connection mechanism
(`pgx` pool), the same `RuntimeInfo`, and the same `FetchRuntimeInfo` logic with minor
branching on `Kind`. Splitting them further would increase code duplication with no type-safety
gain. The meaningful semantic boundary is "speaks PostgreSQL wire protocol" vs "speaks HTTP +
Prometheus text format", which maps cleanly to `DbConn` vs `PromConn`. A future `RestConn`
would add a third implementation if needed.

### Why keep `IsPostgresSource()` in the interface?

Several places in the reaper need to know whether to call `FetchRuntimeInfo` or skip it, and
whether to use standby metrics. Rather than scattering type-switch boilerplate, a single
interface method keeps that logic readable. The method is cheap and side-effect-free, so the
interface overhead is negligible.

### Why `GetSource() Source` instead of embedding `Source`?

Embedding `Source` in the interface is not possible in Go; interfaces cannot embed structs.
`GetSource()` is the idiomatic alternative. It returns a value copy, which is safe: callers
that need to mutate the source configuration already hold a concrete type pointer.

### Why rename the constructor to `NewDbConn` instead of keeping `NewSourceConn`?

`NewSourceConn` is repurposed as the factory function (REQ-017) that returns the correct
implementation based on `Kind`. Reusing the name for the `DbConn` constructor would conflict.
`NewDbConn` is explicit and self-documenting.

### Blast radius assessment

All affected call sites are within two packages: `internal/sources` and `internal/reaper`. No
external packages (sinks, webserver, cmd) hold `*SourceConn` directly — they receive
`SourceConns` or interact via higher-level reaper APIs. The change is therefore contained and
can be reviewed in a single PR.

### Why a `SourceRunner` interface in the reaper instead of a type switch?

`Reaper.sourceReapers` previously held `map[string]*SourceReaper`. Adding prometheus support
requires storing runners of different concrete types. Two alternatives: (a) `map[string]any`
with runtime type switches at every call site — fragile and error-prone; (b) two separate
maps — duplicates lifecycle management code. The `SourceRunner` interface with a single
`Run(ctx)` method eliminates both problems: the map stays strongly typed, start/stop
lifecycle code in `ShutdownOldWorkers` requires zero kind-specific branching, and future
source kinds (REST, etc.) plug in by implementing only `Run`.

---

## 8. Migration Guide

1. Rename `SourceConn` struct → `DbConn` in `internal/sources/conn.go`.
2. Change all `*SourceConn` receiver types on methods → `*DbConn`.
3. Rename `NewSourceConn` → `NewDbConn`; add the new `NewSourceConn` factory.
4. Add `PromConn` struct and its interface methods.
5. Redefine `SourceConns` as `[]SourceConn`; update `GetMonitoredDatabase`.
6. Add compile-time assertions.
7. Update `internal/reaper/` call sites per the table in §4.6.
8. Run `go build ./...` and fix any remaining type errors.
9. Run `go test ./...` — no test logic changes should be required, only type references.
