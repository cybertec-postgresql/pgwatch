---
title: Sink Feedback Interface — Querying Sinks for the Last Stored Measurement
version: 1.0
date_created: 2026-09-02
owner: pgwatch maintainers
tags: [design, sinks, feedback, grpc, postgres]
---

# Introduction

pgwatch is currently a strictly one-way pipeline: reapers produce `metrics.MeasurementEnvelope`
values and push them into sinks. No component can ask a sink what it already holds.

This is a gap for **stateful, resumable collectors** — collectors whose output depends on where
the previous run stopped rather than only on the current state of the source. A collector of that
kind cannot resume at the right point after a restart, because nothing in pgwatch can answer
"what is the newest measurement of metric *M* you hold for source *S*?".

This specification defines the mechanism that answers that question: an **optional,
capability-negotiated feedback interface** that a sink may implement. It is deliberately narrow —
the only fact exchanged is *the epoch of the newest measurement a sink holds for a given
(source, metric) pair*.

**This specification covers the producer side of that contract only: the interface, the sink
implementations, the `MultiWriter` aggregation semantics, the gRPC wire extension, and the
configuration switch.** It deliberately introduces **no consumer**. Wiring a collector to
actually use feedback is out of scope. The deliverable is a complete, tested, unused-by-default
capability that a consumer can adopt without further changes to `internal/sinks`.

---

## 1. Purpose & Scope

**Purpose**: Give sinks a way to report the timestamp of the most recent measurement they already
hold for a (source, metric) pair, and give callers a way to discover whether a given sink can
answer that question at all.

**In scope**:

- A new optional Go interface `sinks.Feedbacker` in `internal/sinks`.
- Two-level capability negotiation: static (does this sink kind do feedback at all?) and dynamic
  (does it do feedback for *this* source and *this* metric?).
- Aggregation semantics for `sinks.MultiWriter` across heterogeneous sinks.
- Implementations for the Postgres sink and the RPC/gRPC sink; explicit non-implementation
  rationale for the Prometheus and JSON-file sinks.
- A backward-compatible `api/pb/pgwatch.proto` extension.
- A single configuration switch that disables the capability process-wide.
- The normative contract that any future caller must observe.

**Out of scope** (each is a separate piece of work):

- **Any consumer of the interface.** No file outside `internal/sinks`, `api/pb`, `docs/`, and the
  corresponding tests may call a `Feedbacker` method as a result of this work.
- Rewind policy — how far back a collector may resume, and what it does with the epoch it
  receives — belongs to the consumer, not to the sink layer.
- Reading measurement *values* back from a sink (only the epoch is exchanged).
- Deduplication of measurements already present in a sink. Sinks remain responsible for their own
  duplicate tolerance.
- Any change to the `Writer.Write` or `Writer.SyncMetric` contracts.
- Cross-source or fleet-wide feedback queries (e.g. "newest measurement of any kind").
- Changing the Prometheus sink from a pull model to a push model.

**Audience**: pgwatch maintainers and AI coding assistants implementing the feature. The
document is self-contained and assumes only familiarity with Go and the pgwatch repository
layout.

**Assumptions**:

- Repository module path is `github.com/cybertec-postgresql/pgwatch/v6`, Go 1.26.
- The measurement epoch column is `epoch_ns` (`metrics.EpochColumnName`), Unix nanoseconds,
  `int64`.
- Sinks are constructed once at start-up by `sinks.NewSinkWriter` and live for the process
  lifetime.

---

## 2. Definitions

| Term | Definition |
|---|---|
| **sink** | A component implementing `sinks.Writer`; a destination for measurements. Current implementations: `PostgresWriter`, `PrometheusWriter`, `JSONWriter`, `RPCWriter`, and the aggregating `MultiWriter`. |
| **source** | A monitored entity. Identified across the pipeline by `MeasurementEnvelope.DBName`, which equals `sources.Source.Name`. Referred to in this spec as `sourceName`. |
| **metric** | A named measurement set. Identified by `MeasurementEnvelope.MetricName`. Referred to as `metricName`. |
| **feedback** | Information flowing *from* a sink *back* to a producer. In this specification, exactly one datum: the epoch of the newest stored measurement for a (source, metric) pair. |
| **Feedbacker** | The optional Go interface defined in §4.1 that a sink implements to provide feedback. |
| **feedback-capable sink** | A sink whose concrete type implements `sinks.Feedbacker` **and** whose `CanFeedback` returns `true` for the pair being queried. |
| **epoch_ns** | Unix timestamp in nanoseconds, `int64`. The value of the `metrics.EpochColumnName` key in a `metrics.Measurement`. |
| **pair** | The tuple `(sourceName, metricName)`. |
| **caller** | Any code that invokes a `Feedbacker` method. No caller is introduced by this specification; §3.5 defines the contract every future caller must honour. |
| **MultiWriter** | `sinks.MultiWriter`; fans one `Write` out to several sinks. Returned by `NewSinkWriter` when more than one `--sink` is configured. |
| **stateful collector** | A collector whose next output depends on its own previous output position (e.g. a log tail), as opposed to a stateless collector that re-queries current state each interval. The eventual consumer of this interface. |
| **at-least-once** | A delivery guarantee under which a measurement may be written more than once but is never lost. |
| **storage name** | The metric name under which measurements are actually persisted, which may differ from the definition name (see `metricNameForStorage` in `internal/reaper/reaper.go`). |
| **RPC sink** | `sinks.RPCWriter`; forwards measurements to a user-supplied gRPC server implementing `api/pb.Receiver`. |

---

## 3. Requirements, Constraints & Guidelines

### 3.1 Interface and Capability Negotiation

- **REQ-001**: `internal/sinks` MUST define an optional interface `Feedbacker` (exact signature in §4.1). A sink MUST NOT be required to implement it; `Writer` MUST remain unchanged.
- **REQ-002**: Callers MUST discover feedback support by Go type assertion on the `sinks.Writer` value, following the existing pattern used for `sinks.MetricsDefiner` (`internal/reaper/metric.go:84`) and `db.Migrator` (`internal/cmdopts/cmdoptions.go:188`).
- **REQ-003**: `Feedbacker` MUST expose a **pair-level** capability predicate `CanFeedback(sourceName, metricName string) bool`. Implementing the interface declares the sink *kind* capable; `CanFeedback` declares whether *this specific pair* can be answered.
- **REQ-004**: `CanFeedback` MUST be side-effect free, MUST NOT perform network or disk I/O, MUST NOT block, and MUST be safe for concurrent use. It answers from in-memory state only.
- **REQ-005**: `Feedbacker` MUST expose `LastMeasurement(ctx context.Context, sourceName, metricName string) (int64, error)` returning the `epoch_ns` of the newest measurement the sink holds for the pair.
- **REQ-006**: `LastMeasurement` MUST return `(0, ErrFeedbackUnsupported)` when the sink cannot answer for the pair. Callers MUST tolerate this even when a preceding `CanFeedback` returned `true` (the state may have changed in between).
- **REQ-007**: `LastMeasurement` MUST return `(0, ErrNoFeedbackData)` when the pair is supported but the sink holds no measurement for it (e.g. a metric never yet written for that source).
- **REQ-008**: `LastMeasurement` MUST honour `ctx` cancellation and deadlines and MUST return promptly when the context is done.
- **REQ-009**: `LastMeasurement` MUST be safe for concurrent use by multiple goroutines.
- **REQ-010**: The returned epoch MUST be expressed in Unix nanoseconds (`int64`), consistent with `metrics.EpochColumnName`, even when the sink's native storage uses a different precision. Conversion is the sink's responsibility.
- **REQ-011**: A returned epoch MUST refer to a measurement the sink has **durably accepted**, not one merely queued in an in-process buffer. Sinks with asynchronous write paths (e.g. `PostgresWriter.input`) MUST NOT report epochs for measurements still in flight.
- **REQ-012**: `LastMeasurement` MUST be monotonically non-decreasing for a given pair over the process lifetime under normal operation, except after external data removal (retention, manual `DELETE`, sink truncation), which MAY cause it to decrease.
- **REQ-013**: The returned epoch MUST be strictly positive when `err` is `nil`. A sink that would return `0` MUST return `ErrNoFeedbackData` instead.

### 3.2 MultiWriter Aggregation

- **REQ-020**: `MultiWriter` MUST implement `Feedbacker`.
- **REQ-021**: `MultiWriter.CanFeedback` MUST return `true` if and only if **at least one** contained writer implements `Feedbacker` and returns `true` from its own `CanFeedback` for the pair.
- **REQ-022**: `MultiWriter.LastMeasurement` MUST query every contained feedback-capable writer and return the **minimum** epoch across those that answered successfully.
- **REQ-023**: Writers returning `ErrFeedbackUnsupported` MUST be excluded from the minimum without failing the aggregate call.
- **REQ-024**: If any contained writer returns `ErrNoFeedbackData`, `MultiWriter.LastMeasurement` MUST return `(0, ErrNoFeedbackData)`. Rationale: that sink holds nothing, so resuming from any later point would leave it permanently short.
- **REQ-025**: If a contained writer returns any other error, `MultiWriter.LastMeasurement` MUST return that error (joined with any others via `errors.Join`, matching the existing `MultiWriter` error style) and MUST NOT return a partial minimum.
- **REQ-026**: If no contained writer is feedback-capable for the pair, `MultiWriter.LastMeasurement` MUST return `(0, ErrFeedbackUnsupported)`.
- **REQ-027**: `MultiWriter` MUST NOT consider writers that do not implement `Feedbacker` when computing the minimum; their absence MUST NOT suppress feedback from writers that do. The consequence — duplicate measurements delivered to non-capable sinks once a consumer acts on the epoch — is accepted (see §7.2).
- **REQ-028**: `MultiWriter.LastMeasurement` MUST pass the caller's `ctx` unchanged to each contained writer and MUST NOT parallelise the queries. Sink counts are small (typically one or two) and sequential iteration matches the existing `Write`/`SyncMetric` implementations.

### 3.3 Per-Sink Implementation

**Postgres sink**

- **PGS-001**: `PostgresWriter` MUST implement `Feedbacker`.
- **PGS-002**: `PostgresWriter.CanFeedback` MUST return `true` only when the sink's in-memory metric map (`partitionMapMetric`, guarded by `PostgresWriter.mu`) contains an entry for the metric's storage name, indicating the metric table is known to exist. It MUST return `false` for an empty `sourceName` or an empty `metricName`.
- **PGS-003**: `PostgresWriter.LastMeasurement` MUST execute the bounded query in §4.4 against `sinkDb`, MUST quote the metric table identifier, and MUST pass `sourceName` as a bind parameter. String concatenation of `sourceName` into SQL is prohibited.
- **PGS-004**: `PostgresWriter.LastMeasurement` MUST bound the scan by the configured retention interval so the query cannot degrade into a full scan of all partitions.
- **PGS-005**: `PostgresWriter.LastMeasurement` MUST convert the returned `timestamptz` to `epoch_ns` (`UnixNano`).
- **PGS-006**: A `SELECT` returning zero rows MUST yield `ErrNoFeedbackData`, not a zero epoch with a `nil` error.
- **PGS-007**: A missing metric table (`SQLSTATE 42P01`, undefined_table) MUST be mapped to `ErrFeedbackUnsupported`, not propagated as a raw error.
- **PGS-008**: `LastMeasurement` MUST NOT acquire `PostgresWriter.mu` for the duration of the SQL round-trip. The mutex serialises partition DDL; holding it across a network call would stall `SyncMetric`.

**Prometheus and JSON sinks**

- **PRM-001**: `PrometheusWriter` MUST NOT implement `Feedbacker`. Rationale in §7.3.
- **JSN-001**: `JSONWriter` MUST NOT implement `Feedbacker`. Rationale in §7.4.

**RPC sink**

- **RPC-001**: `RPCWriter` MUST implement `Feedbacker`.
- **RPC-002**: The `Receiver` gRPC service MUST gain a `GetLastMeasurement` method (§4.5). Existing servers that do not implement it return `codes.Unimplemented`, which MUST be mapped to `ErrFeedbackUnsupported`.
- **RPC-003**: `RPCWriter` MUST cache the "remote server does not implement feedback" verdict after the first `codes.Unimplemented` response and MUST return `false` from `CanFeedback` from then on, for every pair, without further round-trips.
- **RPC-004**: Before the first `GetLastMeasurement` call, `RPCWriter.CanFeedback` MUST return `true` for any non-empty pair (optimistic), so that the single probing call can occur.
- **RPC-005**: A `codes.NotFound` response MUST be mapped to `ErrNoFeedbackData`.
- **RPC-006**: `RPCWriter.LastMeasurement` MUST apply a request deadline derived from the caller's `ctx`; if `ctx` carries no deadline, the implementation MUST impose `CON-002`'s default.
- **RPC-007**: A negative or zero `EpochNs` in an otherwise successful reply MUST be treated as `ErrNoFeedbackData`.
- **RPC-008**: The capability verdict cached by **RPC-003** MUST be guarded for concurrent access (atomic or mutex) and MUST NOT be reset by transient errors.
- **RPC-009**: `RPCWriter.LastMeasurement` MUST use the writer's existing authenticated context (`rw.ctx`, which carries the credential metadata built in `NewRPCWriter`) as the parent for the outgoing call, so the feedback RPC is authenticated exactly like `UpdateMeasurements`.

### 3.4 Configuration

- **CFG-001**: `sinks.CmdOpts` MUST gain a boolean field `Feedback` bound to `--sink-feedback` / `PW_SINK_FEEDBACK`. The effective default MUST be `true`.
- **CFG-002**: When `Feedback` is `false`, `CanFeedback` MUST return `false` on every sink for every pair, and `LastMeasurement` MUST return `ErrFeedbackUnsupported` without performing I/O. Disabling the switch MUST be sufficient to guarantee no feedback query ever reaches a sink backend.
- **CFG-003**: The flag MUST follow the existing `CmdOpts` struct-tag conventions (`long`, `mapstructure`, `description`, `env`). See the note in §4.6 on default-`true` booleans under `go-flags`.
- **CFG-004**: No rewind, horizon, or replay-bound configuration is introduced by this specification. Such policy belongs to the consumer and MUST NOT be added to `sinks.CmdOpts` here.

### 3.5 Caller Contract (normative for future consumers; no caller added here)

- **CAL-001**: A caller MUST type-assert to `sinks.Feedbacker` and MUST degrade to its pre-existing behaviour when the assertion fails.
- **CAL-002**: A caller MUST treat `ErrFeedbackUnsupported` and `ErrNoFeedbackData` as ordinary, expected outcomes — never as faults, and never as a reason to fail start-up or abort collection.
- **CAL-003**: A caller MUST apply its own upper bound on how far it acts on a returned epoch. The sink layer does not bound this and offers no configuration for it (**CFG-004**).
- **CAL-004**: A caller MUST validate the returned epoch before use, including rejecting epochs in the future (clock skew between the pgwatch host and the sink backend is expected).
- **CAL-005**: A caller MUST query at most once per collector instance, at start-up. Per-interval querying is prohibited by **CON-003**.
- **CAL-006**: A caller MUST use the metric's **storage name**, not its definition name (**DAT-003**).
- **CAL-007**: A caller MUST assume at-least-once semantics: acting on a returned epoch may cause a measurement to be written twice. It MUST NOT assume the sink deduplicates.

### 3.6 Constraints

- **CON-001**: No breaking change to `sinks.Writer`, `sinks.MetricsDefiner`, or any existing exported signature in `internal/sinks`.
- **CON-002**: A feedback query MUST complete within 5 seconds by default. Implementations MUST impose this deadline when the caller's context carries none.
- **CON-003**: Feedback queries MUST NOT be issued on the measurement hot path. Permitted call sites are collector start-up and explicit administrative/diagnostic paths only.
- **CON-004**: The `api/pb/pgwatch.proto` change MUST be additive: a new RPC method and new messages only. Existing message field numbers MUST NOT be renumbered, reused, or removed.
- **CON-005**: pgwatch MUST start and operate normally when every configured sink is feedback-incapable.
- **CON-006**: This change MUST be behaviour-neutral at runtime. With no consumer wired up, `pgwatch` MUST produce byte-identical collection behaviour to the pre-change build. The only observable differences permitted are the new flag in `--help` and the new gRPC method in server reflection.

### 3.7 Security

- **SEC-001**: `sourceName` and `metricName` MUST be passed to SQL as bind parameters; only the metric table identifier may be interpolated, and it MUST be quoted with `pgx.Identifier{...}.Sanitize()` or an equivalent.
- **SEC-002**: The gRPC feedback call MUST reuse the existing `RPCWriter` connection, credentials, and TLS configuration (`internal/sinks/rpc.go`). It MUST NOT open a second connection or weaken transport security.
- **SEC-003**: Feedback replies MUST be treated as untrusted input. A sink MUST NOT panic, loop unboundedly, or allocate unboundedly on a malformed, negative, zero, or absurdly large epoch. Sinks normalise per **REQ-013** and **RPC-007**; callers validate per **CAL-004**.
- **SEC-004**: Feedback queries and their outcomes MUST be logged using the existing sink logging conventions (`log.GetLogger(ctx).WithField("sink", ...)`), at `Debug` level for successful queries and no higher than `Info` for expected `ErrFeedbackUnsupported` / `ErrNoFeedbackData` outcomes.

### 3.8 Guidelines

- **GUD-001**: Prefer adding feedback support to a sink over adding local state files to producers. A sink is the authoritative record of what was stored.
- **GUD-002**: Keep `Feedbacker` minimal. Resist adding value-returning or range-query methods; add a separate optional interface if such a need arises.
- **GUD-003**: When implementing `Feedbacker` for a new sink, place the implementation in the same file as the sink and add a compile-time assertion `var _ Feedbacker = (*XWriter)(nil)`.
- **GUD-004**: Keep the sentinel errors comparable with `errors.Is` and never wrap them in a way that changes their identity across the `MultiWriter` boundary.

- **PAT-001**: Follow the existing optional-capability pattern: define a small interface, type-assert at the call site, degrade silently when absent. Do not add methods to `Writer`.
- **PAT-002**: Follow the existing `MultiWriter` aggregation pattern (iterate writers, `errors.Join` the failures) while applying the min/short-circuit rules of §3.2.
- **PAT-003**: Sentinel errors, compared with `errors.Is`, not typed errors or magic epoch values.

---

## 4. Interfaces & Data Contracts

### 4.1 `sinks.Feedbacker` (new, in `internal/sinks/feedback.go`)

```go
package sinks

import (
    "context"
    "errors"
)

// ErrFeedbackUnsupported indicates that the sink cannot report a last-written
// epoch for the requested (sourceName, metricName) pair. It is not a failure:
// callers are expected to fall back to their default behaviour.
var ErrFeedbackUnsupported = errors.New("sink does not support feedback for this source/metric")

// ErrNoFeedbackData indicates that the pair is supported but the sink holds no
// measurement for it yet.
var ErrNoFeedbackData = errors.New("sink holds no measurements for this source/metric")

// Feedbacker is an optional interface that a Writer may implement to report
// back what it has already durably stored. It exists so that stateful,
// resumable collectors can continue from the last persisted measurement
// instead of restarting from the current instant.
//
// Implementing Feedbacker declares the sink kind capable of feedback;
// CanFeedback declares whether one specific source/metric pair can be answered.
//
// No pgwatch component calls these methods today. See the caller contract in
// the sink feedback specification before wiring up a consumer.
type Feedbacker interface {
    // CanFeedback reports whether LastMeasurement can be answered for this
    // pair. It must not perform I/O, must not block, and must be safe for
    // concurrent use. A true result is advisory: LastMeasurement may still
    // return ErrFeedbackUnsupported if state changed in between.
    CanFeedback(sourceName, metricName string) bool

    // LastMeasurement returns the epoch_ns (Unix nanoseconds) of the newest
    // measurement the sink durably holds for the pair.
    //
    // Returns ErrFeedbackUnsupported when the pair cannot be answered, and
    // ErrNoFeedbackData when the pair is supported but empty. Both are
    // expected outcomes, not faults. The returned epoch is 0 whenever err is
    // non-nil, and strictly positive whenever err is nil.
    LastMeasurement(ctx context.Context, sourceName, metricName string) (int64, error)
}
```

### 4.2 Reference call-site pattern (documentation only — not added to the tree)

This is the shape every future consumer is expected to take. It is reproduced here and in the
package documentation so the contract is unambiguous; **no such function is introduced by this
work** (**CON-006**).

```go
func lastStoredEpoch(ctx context.Context, w sinks.Writer, source, metric string) (int64, bool) {
    fb, ok := w.(sinks.Feedbacker)          // CAL-001
    if !ok || !fb.CanFeedback(source, metric) {
        return 0, false
    }
    ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()
    epoch, err := fb.LastMeasurement(ctx, source, metric)
    if err != nil {
        // ErrFeedbackUnsupported, ErrNoFeedbackData, deadline, transport error:
        // all mean "fall back to the default behaviour".  CAL-002
        return 0, false
    }
    return epoch, true
}
```

### 4.3 `MultiWriter` aggregation table

| Contained writer outcomes | `CanFeedback` | `LastMeasurement` result |
|---|---|---|
| No writer implements `Feedbacker` | `false` | `(0, ErrFeedbackUnsupported)` |
| All capable writers return `ErrFeedbackUnsupported` | `false` | `(0, ErrFeedbackUnsupported)` |
| Capable writers return epochs `{T1, T2}`; others unsupported | `true` | `(min(T1, T2), nil)` |
| One capable writer returns `ErrNoFeedbackData` | `true` | `(0, ErrNoFeedbackData)` |
| One capable writer returns a transport/SQL error | `true` | `(0, joined error)` |
| Single capable writer returns `T1` | `true` | `(T1, nil)` |
| `Feedback` config is `false` | `false` | `(0, ErrFeedbackUnsupported)` |

Note: `NewSinkWriter` unwraps a single-sink `MultiWriter` and returns the sink directly
(`internal/sinks/multiwriter.go:60`), so the single-sink case exercises the sink's own
implementation, not the aggregate. Tests MUST construct `MultiWriter` explicitly via
`AddWriter` to cover the aggregation rules.

### 4.4 Postgres sink query

Measurements live in `public."<metric>"` with the columns of `admin.metrics_template`
(`time timestamptz`, `dbname text`, `data jsonb`, `tag_data jsonb`) and an index on
`(dbname, time)` (`internal/sinks/sql/admin_schema.sql:69`).

```sql
-- $1 = sourceName, $2 = retention interval (e.g. '14 days')
-- Table identifier is quoted by the caller; never concatenated from user input.
SELECT time
FROM public."db_stats"
WHERE dbname = $1
  AND time > now() - $2::interval
ORDER BY time DESC
LIMIT 1;
```

The `time > now() - retention` predicate enables partition pruning and bounds the worst case to
the retained partitions. Zero rows ⇒ `ErrNoFeedbackData` (**PGS-006**). `SQLSTATE 42P01` ⇒
`ErrFeedbackUnsupported` (**PGS-007**).

### 4.5 gRPC / protobuf extension (`api/pb/pgwatch.proto`)

Additive only (**CON-004**):

```proto
service Receiver {
    rpc UpdateMeasurements(MeasurementEnvelope) returns (Reply);
    rpc SyncMetric(SyncReq) returns (Reply);
    rpc DefineMetrics(google.protobuf.Struct) returns (Reply);
    rpc GetLastMeasurement(FeedbackReq) returns (FeedbackReply); // new
}

message FeedbackReq {
    string DBName = 1;
    string MetricName = 2;
}

message FeedbackReply {
    // Unix nanoseconds of the newest measurement the server durably holds for
    // the pair. Servers that hold nothing must return codes.NotFound rather
    // than 0.
    int64 EpochNs = 1;
}
```

Server-side status-code contract:

| gRPC status | `RPCWriter` mapping |
|---|---|
| `OK`, `EpochNs > 0` | `(EpochNs, nil)` |
| `OK`, `EpochNs <= 0` | `(0, ErrNoFeedbackData)` (**RPC-007**) |
| `codes.Unimplemented` | `(0, ErrFeedbackUnsupported)`; capability cached off (**RPC-003**) |
| `codes.NotFound` | `(0, ErrNoFeedbackData)` (**RPC-005**) |
| `codes.Unavailable`, `DeadlineExceeded`, other | `(0, err)` — transient, capability NOT cached off |

Receiver implementers are the audience for this table; it MUST be reproduced in the user-facing
gRPC sink documentation so third-party servers know which codes carry which meaning.

### 4.6 `sinks.CmdOpts` addition

```go
type CmdOpts struct {
    // ... existing fields unchanged ...
    Feedback bool `long:"sink-feedback" mapstructure:"sink-feedback" description:"Allow sinks to report the last stored measurement epoch to collectors that can resume from it" env:"PW_SINK_FEEDBACK"`
}
```

Note on the boolean default: `go-flags` treats a bare `--sink-feedback` as `true`, so a
`default:"true"` tag alone would make the flag impossible to turn off from the command line.
Implement **CFG-001**'s default the way the codebase already handles default-on booleans —
either a value-taking flag or a matching `--no-sink-feedback` negation — and keep the effective
default `true`. Whichever form is chosen MUST be reflected in `docs/`.

Sinks read this value from the `*CmdOpts` they already hold (`PostgresWriter.opts`). Sinks that
are constructed without `CmdOpts` — `RPCWriter`, built by `NewRPCWriter(ctx, connStr)` — MUST be
given access to the switch; the smallest change consistent with `NewSinkWriter`'s current shape
is to pass `opts` to `NewRPCWriter` alongside the connection string, mirroring
`NewPostgresWriter`.

---

## 5. Acceptance Criteria

- **AC-001**: Given a sink that does not implement `Feedbacker`, When the §4.2 reference pattern runs against it, Then no feedback call is attempted and the caller's default path is taken.
- **AC-002**: Given a `PostgresWriter` holding rows for source `S` and metric `M` with newest `time` = `T`, When `LastMeasurement(ctx, "S", "M")` is called, Then it returns `T.UnixNano()` and a `nil` error.
- **AC-003**: Given a `PostgresWriter` whose metric table exists but holds no rows for source `S`, When `LastMeasurement` is called for `S`, Then it returns `(0, ErrNoFeedbackData)`.
- **AC-004**: Given a `PostgresWriter` whose metric table does not exist, When `LastMeasurement` is called, Then it returns `(0, ErrFeedbackUnsupported)` and nothing is logged at `Error` level.
- **AC-005**: Given a `MultiWriter` over a capable writer reporting `T2` and another reporting `T1` with `T1 < T2`, When `LastMeasurement` is called, Then it returns `T1`.
- **AC-006**: Given a `MultiWriter` over a `PostgresWriter` reporting `T2` and a `JSONWriter`, When `LastMeasurement` is called, Then it returns `T2` — the non-capable writer does not suppress the answer.
- **AC-007**: Given a `MultiWriter` where one capable writer returns `ErrNoFeedbackData`, When `LastMeasurement` is called, Then it returns `(0, ErrNoFeedbackData)` regardless of the other writers' epochs.
- **AC-008**: Given an RPC sink whose remote server returns `codes.Unimplemented`, When `LastMeasurement` is called, Then it returns `ErrFeedbackUnsupported`, and every subsequent `CanFeedback` returns `false` without a network round-trip.
- **AC-009**: Given an RPC sink whose remote server returns `codes.Unavailable`, When `LastMeasurement` is called, Then it returns the transport error and a subsequent `CanFeedback` still returns `true` (**RPC-008**).
- **AC-010**: Given `PrometheusWriter` or `JSONWriter`, When asserted to `sinks.Feedbacker`, Then the assertion fails.
- **AC-011**: Given `Feedback` is `false`, When `CanFeedback` is called on any sink for any pair, Then it returns `false`, and `LastMeasurement` returns `ErrFeedbackUnsupported` with no query reaching the backend (**CFG-002**).
- **AC-012**: Given a `LastMeasurement` call whose context is cancelled mid-flight, When the sink is a `PostgresWriter`, Then the call returns a context error within 100 ms and leaves no leaked connection or goroutine.
- **AC-013**: Given a `PostgresWriter` with measurements still buffered in `input` and not yet flushed, When `LastMeasurement` is called, Then the returned epoch does not include those buffered measurements (**REQ-011**).
- **AC-014**: Given concurrent `LastMeasurement` and `SyncMetric` calls on the same `PostgresWriter`, When both run under `-race`, Then no data race is reported and neither blocks the other for the duration of a SQL round-trip (**PGS-008**).
- **AC-015**: Given a `LastMeasurement` returning `err == nil`, When the epoch is inspected, Then it is strictly greater than zero (**REQ-013**).
- **AC-016**: Given an existing gRPC receiver built against the pre-change `pgwatch.proto`, When pgwatch writes measurements to it, Then `UpdateMeasurements`, `SyncMetric`, and `DefineMetrics` behave exactly as before.
- **AC-017**: Given the full change set, When `grep -rn "Feedbacker\|LastMeasurement\|CanFeedback" --include=*.go` is run over the repository, Then every non-test hit is inside `internal/sinks` (**CON-006**).
- **AC-018**: The system shall pass `go vet ./...` and `gofmt -l internal/ api/` with no output after the change.

---

## 6. Test Automation Strategy

- **Test Levels**: Unit (per-sink `CanFeedback`/`LastMeasurement`, `MultiWriter` aggregation, config gating), Integration (Postgres sink against a real database; RPC sink against an in-process gRPC server). No end-to-end level applies, since no consumer exists.
- **Frameworks**: Go standard `testing`; `github.com/stretchr/testify` (`assert`, `require`) as already used across `internal/sinks/*_test.go`; `pgxmock` for `PostgresWriter` unit tests, mirroring `internal/sinks/postgres_test.go`; `google.golang.org/grpc/test/bufconn` for `RPCWriter` tests, mirroring `internal/sinks/rpc_test.go`.
- **Test Data Management**: Postgres integration tests create metric tables through the sink's own `SyncMetric`/`AddOp` path and drop them in `t.Cleanup`. Unit tests use `pgxmock` expectations rather than a live database. No fixtures are shared between tests.
- **Mocks**: A `fakeFeedbacker` test helper in `internal/sinks` MUST allow scripting `CanFeedback` results, epochs, errors, and call counts, so that **AC-011** (no backend call when disabled) and **AC-008** (capability caching) are assertable. The same helper drives every row of the §4.3 aggregation table without needing four real sinks.
- **gRPC server double**: A `bufconn`-backed `Receiver` implementation MUST be scriptable to return each row of the §4.5 status-code table, including a variant that omits `GetLastMeasurement` entirely so `codes.Unimplemented` is produced by gRPC itself rather than by the double.
- **CI/CD Integration**: Tests run in the existing GitHub Actions Go workflow. Integration tests requiring a live Postgres MUST follow the repository's existing `*_integration_test.go` convention so the default `go test ./...` remains hermetic.
- **Coverage Requirements**: New code in `internal/sinks` MUST reach at least 80% statement coverage. Every row of the §4.3 aggregation table and every row of the §4.5 status-code table MUST have a dedicated test case.
- **Race Testing**: `go test -race` MUST cover concurrent `CanFeedback` + `LastMeasurement` + `Write` + `SyncMetric` on `PostgresWriter`, and concurrent `LastMeasurement` on `RPCWriter` exercising the cached-capability flag (**RPC-008**), extending the pattern of `internal/sinks/prometheus_race_test.go`.
- **Performance Testing**: A timed integration assertion MUST show that `PostgresWriter.LastMeasurement` on a table with ≥ 30 daily partitions completes in under 100 ms, demonstrating that **PGS-004**'s retention bound prunes partitions.
- **Regression Guards**:
  - A test MUST assert that `PrometheusWriter` and `JSONWriter` do *not* satisfy `Feedbacker`, so the deliberate non-implementation (**PRM-001**, **JSN-001**) is not silently reversed.
  - A test or CI step MUST enforce **AC-017**, so a consumer cannot be wired up inside this change without the guard failing.

---

## 7. Rationale & Context

### 7.1 Why an optional interface rather than extending `Writer`

`Writer` has exactly two methods and four implementations, two of which (Prometheus, JSON) cannot
meaningfully answer a feedback query. Adding methods to `Writer` would force every implementation
— including third-party ones, since the sink set is effectively an extension point — to write
stub methods, and would make "unsupported" indistinguishable from "supported but empty" without
sentinel values anyway. The repository already establishes the optional-interface pattern for
`MetricsDefiner` and `db.Migrator`; this specification reuses it verbatim so there is one idiom
to learn.

### 7.2 Why `MultiWriter` returns the minimum epoch

Consider two sinks that received measurements at different points because one was briefly
unavailable: Postgres holds data through `T2`, the RPC receiver only through `T1`, `T1 < T2`.
A consumer resuming from `max = T2` means the RPC receiver never receives the `T1..T2` span —
permanent data loss. Resuming from `min = T1` means Postgres receives the `T1..T2` span twice —
duplication. pgwatch's pipeline is at-least-once end to end (a failed `Write` is logged and the
envelope is dropped, `internal/reaper/reaper.go:430`), and duplicate measurements are visibly
wrong but recoverable, whereas a hole in a monitoring timeline is not. Minimum is therefore the
correct conservative choice. **REQ-024** extends the same logic: a sink holding *nothing* is the
extreme case of lagging, so the aggregate reports "no data" and the consumer falls back to its
default.

### 7.3 Why `PrometheusWriter` does not implement `Feedbacker`

The Prometheus sink is pull-based. `PrometheusWriter.Write` stores an envelope in an in-memory
cache with a 10-minute TTL (`promCacheTTL`) that a Prometheus server may or may not scrape. The
newest epoch in that cache says what pgwatch *offered*, not what any Prometheus server *stored* —
answering with it would be actively misleading, and it would cause the `MultiWriter` minimum to be
dominated by an unreliable value. A Prometheus-backed feedback path would require querying the
Prometheus HTTP API, which means a new configuration surface (query URL, auth, tenancy) with no
guarantee the scraper and the sink even refer to the same server. That is a separate feature.

### 7.4 Why `JSONWriter` does not implement `Feedbacker`

`JSONWriter` writes newline-delimited JSON through `lumberjack` with compression and size-based
rotation. Answering "newest epoch for pair X" would require decompressing and scanning rotated
files backwards with no index — unbounded work with an unbounded worst case, violating
**CON-002**. The JSON sink is documented as a debugging and pipeline-integration sink; consumers
downstream of it own their own offsets.

### 7.5 Why the pair-level `CanFeedback` predicate exists

Sink capability is not uniform across metrics. A Postgres sink can answer for a metric whose table
exists and cannot for one never yet written; an RPC receiver may support feedback for the metrics
it persists and not for those it forwards to an alerting system. Making the predicate part of the
interface — and requiring it to be I/O-free — lets a caller cheaply decide whether a query is
worth issuing at all, and keeps the expensive path (`LastMeasurement`) off the decision tree when
the answer is known in advance. The redundancy with `ErrFeedbackUnsupported` is intentional:
`CanFeedback` is an optimisation and a hint, `LastMeasurement`'s error is the authority
(**REQ-006**).

### 7.6 Why the epoch and nothing else

Every richer contract considered — returning the last measurement's payload, a range of stored
epochs, a per-metric watermark map — either duplicates what a sink's native query interface
already offers or commits pgwatch to a read API across four dissimilar backends. The epoch is the
smallest datum that solves the motivating problem, is expressible in every backend, and is cheap
to compute in each. **GUD-002** records the intent to keep it that way.

### 7.7 Why the capability ships without a consumer

Splitting the interface from its first consumer is deliberate, not incidental. The two changes
have different review surfaces: this one is a self-contained addition to `internal/sinks` plus an
additive `.proto` method, reviewable against the tables in §4 and provably behaviour-neutral
(**CON-006**); a consumer change is about resume correctness, replay bounds, and duplicate
tolerance in one specific collector. Landing the interface first also means the consumer branch
can rebase onto a stable contract instead of two branches editing the same package. **AC-017**
exists so the separation is mechanically enforced rather than merely intended.

### 7.8 Why start-up only

**CON-003** confines feedback queries to collector start-up. A per-interval feedback query would
add a synchronous round-trip to every collection cycle for every source — at fleet scale
(hundreds of sources) that is a meaningful load on the sink and a new failure mode on the hot
path. Once a stateful collector is running, it already knows its own position; it only needs the
sink to tell it where to begin. The constraint is stated here, rather than left to the consumer,
because it shapes what the sink implementations are allowed to assume about query frequency —
notably **PGS-004**, whose retention-bounded query is acceptable at start-up and would not be at
every tick.

### 7.9 Relationship to source-failure resilience work

`spec/design-source-failure-resilience.md` bounds contexts for database round-trips and introduces
a last-known-good cache so that a single slow source cannot stall the main loop. Sink feedback is
the mirror-image concern on the write side, and follows the same principles: every round-trip is
bounded (**CON-002**), and every failure degrades to previous behaviour rather than blocking
(**CAL-002**, **CON-005**).

### 7.10 Anticipated consumers

Recorded so the interface is not narrowed to a single use case. None is authorised by this
specification; each needs its own:

- Resumable log-event collection — the first beneficiary.
- `change_events` detection resuming from the last stored change snapshot.
- A diagnostic `pgwatch config` subcommand reporting per-source, per-metric staleness.
- Gap detection raising an alert when a sink's newest measurement for a pair is older than several
  collection intervals.

---

## 8. Dependencies & External Integrations

### External Systems

- **EXT-001**: PostgreSQL (and compatible: TimescaleDB, Citus, CockroachDB) as measurement store — must accept a parameterised `SELECT` against `public."<metric>"` and expose the `time`/`dbname` columns of `admin.metrics_template`.
- **EXT-002**: User-supplied gRPC receivers implementing `api/pb.Receiver` — may optionally implement `GetLastMeasurement`; those that do not remain fully supported.

### Third-Party Services

- **SVC-001**: None. This specification introduces no dependency on any external service.

### Infrastructure Dependencies

- **INF-001**: The metric table index on `(dbname, time)` (`admin.metrics_template`) — required for **PGS-004**'s latency bound. If a deployment drops it, feedback queries degrade and **CON-002**'s deadline will cut them off, falling back safely.
- **INF-002**: Time partitioning by `time` — required for the retention predicate to prune partitions.

### Data Dependencies

- **DAT-001**: `epoch_ns` semantics — measurements carry Unix nanoseconds in `metrics.EpochColumnName`; the Postgres sink persists this as `timestamptz`. Round-tripping through `timestamptz` truncates to microsecond precision, so a returned epoch may be up to 999 ns older than the value originally written. This is immaterial and MUST NOT be treated as a defect.
- **DAT-002**: Metric storage names — feedback queries operate on the storage name (`metricNameForStorage`), not the definition name, matching what `SyncMetric` and `Write` use. Sinks receive whatever name the caller supplies; **CAL-006** places the obligation on the caller.
- **DAT-003**: `PostgresWriter.opts.RetentionInterval` — a PostgreSQL interval string, already validated at sink init (`internal/sinks/postgres.go:123`). Reused verbatim as the query bound; no new validation is required.

### Technology Platform Dependencies

- **PLT-001**: Go 1.26 — the module's declared version; no newer language feature is required beyond the builtin `min`, available since Go 1.21.
- **PLT-002**: gRPC and protobuf toolchain — regenerating `api/pb` after the additive `.proto` change requires `protoc` with `protoc-gen-go` and `protoc-gen-go-grpc`, per the repository's existing generation step.

### Compliance Dependencies

- **COM-001**: None. Feedback exchanges only source names, metric names, and timestamps — no measurement values and no end-user data leave the sink.

---

## 9. Examples & Edge Cases

### 9.1 Postgres sink implementation sketch

```go
var _ Feedbacker = (*PostgresWriter)(nil)

func (pgw *PostgresWriter) CanFeedback(sourceName, metricName string) bool {
    if !pgw.opts.Feedback || sourceName == "" || metricName == "" { // CFG-002
        return false
    }
    pgw.mu.Lock()
    defer pgw.mu.Unlock()
    _, ok := pgw.partitionMapMetric[metricName]
    return ok
}

func (pgw *PostgresWriter) LastMeasurement(ctx context.Context, sourceName, metricName string) (int64, error) {
    if !pgw.CanFeedback(sourceName, metricName) { // PGS-008: mutex released before the round-trip
        return 0, ErrFeedbackUnsupported
    }
    ctx, cancel := context.WithTimeout(ctx, feedbackTimeout) // CON-002
    defer cancel()

    sql := `SELECT time FROM public.` + pgx.Identifier{metricName}.Sanitize() +
        ` WHERE dbname = $1 AND time > now() - $2::interval ORDER BY time DESC LIMIT 1`

    var ts time.Time
    err := pgw.sinkDb.QueryRow(ctx, sql, sourceName, pgw.opts.RetentionInterval).Scan(&ts)
    switch {
    case errors.Is(err, pgx.ErrNoRows):
        return 0, ErrNoFeedbackData // PGS-006
    case isUndefinedTable(err): // SQLSTATE 42P01
        return 0, ErrFeedbackUnsupported // PGS-007
    case err != nil:
        return 0, err
    }
    if epoch := ts.UnixNano(); epoch > 0 { // REQ-013
        return epoch, nil
    }
    return 0, ErrNoFeedbackData
}
```

### 9.2 MultiWriter aggregation sketch

```go
func (mw *MultiWriter) CanFeedback(sourceName, metricName string) bool {
    for _, w := range mw.writers {
        if fb, ok := w.(Feedbacker); ok && fb.CanFeedback(sourceName, metricName) {
            return true
        }
    }
    return false
}

func (mw *MultiWriter) LastMeasurement(ctx context.Context, sourceName, metricName string) (int64, error) {
    var (
        oldest  int64 = math.MaxInt64
        answers int
        errs    error
    )
    for _, w := range mw.writers {
        fb, ok := w.(Feedbacker)
        if !ok {
            continue // REQ-027: silent sinks do not veto
        }
        epoch, err := fb.LastMeasurement(ctx, sourceName, metricName) // REQ-028: caller ctx, sequential
        switch {
        case errors.Is(err, ErrFeedbackUnsupported):
            continue // REQ-023
        case errors.Is(err, ErrNoFeedbackData):
            return 0, ErrNoFeedbackData // REQ-024: short-circuit
        case err != nil:
            errs = errors.Join(errs, err) // REQ-025
        default:
            answers++
            oldest = min(oldest, epoch)
        }
    }
    if errs != nil {
        return 0, errs
    }
    if answers == 0 {
        return 0, ErrFeedbackUnsupported // REQ-026
    }
    return oldest, nil
}
```

### 9.3 RPC sink implementation sketch

```go
var _ Feedbacker = (*RPCWriter)(nil)

// unsupported is set once the remote server answers Unimplemented. RPC-003, RPC-008.
func (rw *RPCWriter) CanFeedback(sourceName, metricName string) bool {
    if !rw.opts.Feedback || sourceName == "" || metricName == "" { // CFG-002
        return false
    }
    return !rw.unsupported.Load() // RPC-004: optimistic until proven otherwise
}

func (rw *RPCWriter) LastMeasurement(ctx context.Context, sourceName, metricName string) (int64, error) {
    if !rw.CanFeedback(sourceName, metricName) {
        return 0, ErrFeedbackUnsupported
    }
    // RPC-009: rw.ctx carries the credential metadata; inherit the caller's deadline only.
    callCtx, cancel := contextWithDeadlineFrom(rw.ctx, ctx, feedbackTimeout) // CON-002, RPC-006
    defer cancel()

    reply, err := rw.client.GetLastMeasurement(callCtx, &pb.FeedbackReq{
        DBName: sourceName, MetricName: metricName,
    })
    if err != nil {
        switch status.Code(err) {
        case codes.Unimplemented:
            rw.unsupported.Store(true) // RPC-003
            return 0, ErrFeedbackUnsupported
        case codes.NotFound:
            return 0, ErrNoFeedbackData // RPC-005
        }
        return 0, err // transient: capability NOT cached off
    }
    if reply.GetEpochNs() <= 0 {
        return 0, ErrNoFeedbackData // RPC-007
    }
    return reply.GetEpochNs(), nil
}
```

### 9.4 Edge cases

| # | Situation | Required behaviour |
|---|---|---|
| E-01 | `CanFeedback` called for a metric whose table was never created | `false`; no query issued |
| E-02 | Metric table exists but the source was never written to it | `CanFeedback` → `true`; `LastMeasurement` → `ErrNoFeedbackData` |
| E-03 | Retention deleted every row for the pair | `ErrNoFeedbackData`, not a stale epoch from outside the retention window |
| E-04 | Sink backend clock is ahead of the pgwatch host | The sink returns the stored epoch verbatim; rejecting future epochs is the caller's duty (**CAL-004**), not the sink's |
| E-05 | `--sink-feedback` disabled | Every `CanFeedback` → `false`; every `LastMeasurement` → `ErrFeedbackUnsupported`; no backend traffic (**AC-011**) |
| E-06 | Two `--sink` targets, one Postgres and one `jsonfile` | Aggregate answers from the Postgres sink alone (**REQ-027**) |
| E-07 | `MultiWriter` contains two Postgres sinks pointed at the same database | Both answer; minimum is their common value; no special handling |
| E-08 | gRPC receiver implements `GetLastMeasurement` but is temporarily unavailable | Transport error returned; capability NOT cached off, so a later call retries (**AC-009**) |
| E-09 | gRPC receiver returns `EpochNs = 0` with status `OK` | Normalised to `ErrNoFeedbackData` (**RPC-007**) |
| E-10 | `SyncMetric` is creating a partition while `LastMeasurement` runs | Both proceed; `LastMeasurement` must not hold `mu` across its round-trip (**PGS-008**, **AC-014**) |
| E-11 | Caller passes a definition name where a storage name differs | The sink answers about the table it was asked for; correctness is the caller's obligation (**CAL-006**, **DAT-002**) |
| E-12 | Metric name contains a double quote or other identifier-hostile character | `pgx.Identifier.Sanitize()` quotes it correctly; no injection and no error (**SEC-001**) |
| E-13 | Caller passes an already-cancelled context | Return the context error promptly; do not issue a backend query (**REQ-008**) |

### 9.5 Worked aggregation example

```text
Configuration: --sink postgresql://…  --sink jsonfile:///var/log/pgwatch.json  --sink grpc://…

Query: LastMeasurement(ctx, "prod-db", "db_stats")

  PostgresWriter   CanFeedback → true   LastMeasurement → 1756800000000000000  (10:00:00)
  JSONWriter       not a Feedbacker     skipped entirely                        (REQ-027)
  RPCWriter        CanFeedback → true   LastMeasurement → 1756799880000000000  (09:58:00)

  answers = 2 ; minimum = 1756799880000000000  → returned

A consumer acting on this replays from 09:58:00. The Postgres sink receives the 09:58–10:00
span a second time; the RPC receiver receives it for the first time. No sink loses the span,
which is the property REQ-022 is chosen to guarantee (§7.2).
```

---

## 10. Validation Criteria

A conforming implementation MUST satisfy all of the following:

1. `sinks.Feedbacker`, `sinks.ErrFeedbackUnsupported`, and `sinks.ErrNoFeedbackData` exist with the exact names and signatures of §4.1.
2. `sinks.Writer` is unchanged; the diff shows no modification to its method set (**CON-001**).
3. `PostgresWriter`, `RPCWriter`, and `MultiWriter` satisfy `Feedbacker` (compile-time assertions present); `PrometheusWriter` and `JSONWriter` do not.
4. Every row of the §4.3 aggregation table and the §4.5 status-code table has a passing test.
5. Every acceptance criterion **AC-001** … **AC-018** has at least one corresponding test, traceable by name or comment.
6. `api/pb/pgwatch.proto` diff is additive only; field numbers 1–4 of `MeasurementEnvelope` and 1–3 of `SyncReq` are untouched (**CON-004**).
7. A gRPC receiver compiled against the previous `.proto` interoperates with the new pgwatch for all three pre-existing methods (**AC-016**).
8. No `Feedbacker` method is called anywhere outside `internal/sinks` and its tests (**AC-017**).
9. No `Feedbacker` method is invoked from `PostgresWriter.poll`, `PostgresWriter.flush`, or any other per-measurement path (**CON-003**).
10. All SQL built for feedback passes `sourceName` as a bind parameter; the only interpolated element is the sanitised table identifier (**SEC-001**).
11. `go test -race ./internal/sinks/...` passes.
12. `gofmt -l internal/ api/` produces no output and `go vet ./...` is clean (**AC-018**).
13. Documentation under `docs/` describes the new flag, the two-level capability model, which sinks support feedback, and the §4.5 status-code contract for third-party gRPC receiver implementers.
14. Running pgwatch with an unchanged configuration produces the same collection behaviour as the pre-change build; the only user-visible additions are the new flag in `--help` and the new gRPC method (**CON-006**).

---

## 11. Related Specifications / Further Reading

- [`spec/design-source-failure-resilience.md`](design-source-failure-resilience.md) — bounded contexts and last-known-good caching on the source side; shares the "bound every round-trip, degrade instead of blocking" principle (§7.9).
- [`spec/architecture-prometheus-exporter-source.md`](architecture-prometheus-exporter-source.md) — Prometheus as a pgwatch source; explains the pull-model characteristics that make `PrometheusWriter` unsuitable as a feedback provider (§7.3).
- [`spec/refactor-sourceconn-interface.md`](refactor-sourceconn-interface.md) — source connection abstraction; context for how sinks and sources are wired together.
- `internal/sinks/doc.go` — package overview of the sink connectors; MUST be extended to mention the optional feedback capability.
- `internal/sinks/multiwriter.go` — `Writer`, `MetricsDefiner`, and the existing optional-capability pattern this specification extends.
- `internal/sinks/sql/admin_schema.sql` — `admin.metrics_template`, the column and index layout the Postgres feedback query relies on.
- `api/pb/pgwatch.proto` — the `Receiver` service contract extended in §4.5.
- [gRPC status codes](https://grpc.io/docs/guides/status-codes/) — `Unimplemented` and `NotFound` semantics used by §4.5.
