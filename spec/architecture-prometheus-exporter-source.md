---
title: Prometheus Exporter as a pgwatch Source
version: 1.0
date_created: 2026-04-26
tags: [architecture, design, prometheus, sources, metrics, sinks]
---

# Introduction

This specification defines how pgwatch fetches metrics from external Prometheus exporters
(e.g., `node_exporter`, `postgres_exporter`, custom exporters) and routes them through the
standard pgwatch pipeline. It covers the new source kind, the metric definition extension
that maps Prometheus metric families to pgwatch logical metrics, label-to-column conventions,
and a lightweight proxy path that avoids double-conversion when the configured sink is also
Prometheus.

---

## 1. Purpose & Scope

**Purpose**: Allow pgwatch to scrape arbitrary Prometheus exporters, apply metric grouping and
filtering, and write measurements to any configured sink — including acting as a low-overhead
proxy when the sink is a Prometheus endpoint.

**In scope**:
- New source `Kind` value `prometheus`.
- Extension of `metrics.Metric` for Prometheus selector lists.
- HTTP scrape lifecycle inside `SourceConn` and the reaper.
- Measurement row structure for Prometheus samples.
- Prometheus-to-Prometheus (Prom→Prom) proxy optimisation in `PrometheusWriter`.
- Authentication and TLS configuration.

**Out of scope**:
- Push-gateway integration.
- Prometheus alerting rules.
- Remote-write protocol (only the `/metrics` exposition format is targeted).
- Grafana dashboard changes.

**Audience**: pgwatch maintainers implementing the feature. The document is also intended to be
consumed directly by AI coding assistants.

---

## 2. Definitions

| Term | Definition |
|---|---|
| **Prometheus exporter** | An HTTP service that exposes metrics in Prometheus exposition format (text or protobuf) at a `/metrics` endpoint. |
| **Metric family** | A single named group of Prometheus samples sharing the same `__name__` label (e.g., `pg_stat_activity_count`). |
| **Sample** | A single (label-set, float64 value, optional timestamp) triple within a metric family. |
| **pgwatch metric** | A named, schema-fixed set of measurements stored as one table/series in the sink. |
| **pgwatch source** | A configured entity from which measurements are collected. |
| **scrape** | An HTTP GET request to a Prometheus exporter's `/metrics` endpoint. |
| **selector list** | The ordered list of Prometheus metric family names that are grouped into one pgwatch metric. |
| **MeasurementEnvelope** | The internal pgwatch structure `{DBName, MetricName, CustomTags, Data Measurements}` passed from reaper to sinks. |
| **Prom→Prom path** | The code path activated when `source.Kind == "prometheus"` and the active sink is `PrometheusWriter`. |
| **tag column** | A pgwatch measurement column whose name starts with `tag_`. Treated as an indexed dimension. |
| **value column** | A numeric measurement column not prefixed `tag_`. |
| **epoch_ns** | Mandatory measurement column: Unix timestamp in nanoseconds (int64). |
| **ConnStr** | The `Source.ConnStr` field, repurposed as the scrape URL for `prometheus` sources. |

---

## 3. Requirements, Constraints & Guidelines

### Source Definition

- **REQ-001**: `sources.Kind` MUST include a new constant `SourcePrometheus = "prometheus"`.
- **REQ-002**: A `prometheus` source MUST use `Source.ConnStr` as the HTTP(S) scrape URL (e.g., `http://host:9187/metrics`). No PostgreSQL DSN is required or expected.
- **REQ-003**: `Source.Metrics` (type `metrics.MetricIntervals`) MUST continue to map pgwatch metric name → scrape interval in seconds, identical to existing source kinds.
- **REQ-004**: `Source.PresetMetrics` MUST be supported for `prometheus` sources (preset resolves to a `MetricIntervals` map as usual).
- **CON-001**: Fields `Source.IncludePattern`, `Source.ExcludePattern`, `Source.OnlyIfMaster`, `Source.MetricsStandby`, and `Source.PresetMetricsStandby` are irrelevant for `prometheus` sources and MUST be ignored without error.
- **GUD-001**: `Source.Name` SHOULD be set to a human-readable identifier for the exporter (e.g., `node-exporter-prod-db01`). It maps to `MeasurementEnvelope.DBName`.
- **REQ-005**: `sources.Kinds` slice MUST include `SourcePrometheus` so that `Kind.IsValid()` returns `true`.

### Authentication and TLS

- **REQ-006**: The scrape URL in `ConnStr` MUST support standard URL userinfo for Basic Auth: `http://user:password@host:9187/metrics`.
- **REQ-007**: TLS options MUST be configurable via URL query parameters appended to `ConnStr`, following the same convention as the PostgreSQL DSN (`sslmode`, `sslrootcert`, etc.):

  | Query parameter | Description |
  |---|---|
  | `tlsrootcert=<path>` | Path to a PEM-encoded CA certificate file used to verify the server certificate. Omit to use system roots. |
  | `tlsskipverify=true` | When `true`, TLS certificate verification is disabled. Default: absent / `false`. |

  Example: `https://host:9187/metrics?tlsrootcert=/etc/ssl/ca.pem`

  The `ConnStr` URL MUST be stripped of these query parameters before being used as the actual HTTP request URL, so they are never forwarded to the exporter.

- **SEC-001**: Passwords stored in `ConnStr` MUST NOT be logged. The logger MUST redact the userinfo component before printing any URL.
- **SEC-002**: `tlsskipverify=true` in the scrape URL MUST produce a warning-level log entry each time the HTTP client is constructed for that source.
- **CON-002**: Custom headers (e.g., `Authorization: Bearer`) are out of scope for v1.0.

### Metric Definition Extension

- **REQ-008**: `metrics.Metric` struct MUST have a new field `PromSelectors []string` (YAML tag `prom_selectors`). Example:

  ```yaml
  metrics:
    pg_connections:
      description: "Connection state summary from postgres_exporter"
      prom_selectors:
        - pg_stat_activity_count
        - pg_stat_activity_max_tx_duration
      storage_name: pg_connections
  ```

- **REQ-009**: `PromSelectors` MUST be ignored for non-`prometheus` source kinds. A metric that has both `SQLs` and `PromSelectors` is valid; `SQLs` is used for postgres-family sources and `PromSelectors` for `prometheus` sources.
- **REQ-010**: If a metric definition referenced by a `prometheus` source has neither `PromSelectors` nor `SQLs`, the reaper MUST log a warning and skip that metric for the affected source.
- **GUD-002**: Each entry in `PromSelectors` MUST be an exact Prometheus metric family name (no glob, no regex). Regex support may be added in a future version.
- **REQ-011**: Metric families present in the scrape response but NOT listed in any `PromSelectors` of the active metric set MUST be silently discarded.

### Scrape Lifecycle

- **REQ-012**: `SourceConn` MUST carry an `HTTPClient *http.Client` field alongside the existing `Conn db.PgxPoolIface`. For `prometheus` sources, `Conn` MUST remain `nil`; `HTTPClient` MUST be non-nil after `Connect()`.
- **REQ-013**: `SourceConn.Connect()` MUST parse TLS query parameters (`tlsrootcert`, `tlsskipverify`) from `ConnStr`, construct an `*http.Client` with the derived TLS config, and validate reachability by issuing a HEAD or GET to the scrape URL (with TLS parameters stripped). If the exporter is unreachable, `Connect()` MUST return an error.
- **REQ-014**: `SourceConn.Ping()` for a `prometheus` source MUST issue a HEAD request to the scrape URL and return an error if the status code is not 2xx.
- **REQ-015**: `SourceConn.IsPostgresSource()` MUST return `false` for `Kind == "prometheus"`.
- **REQ-016**: `SourceConn.FetchRuntimeInfo()` for `prometheus` sources MUST be a no-op that sets `RuntimeInfo.VersionStr = "prometheus"` and `RuntimeInfo.Version = 0` without error.
- **REQ-017**: The reaper MUST dispatch `reapMetricMeasurements` for `prometheus` sources using the same goroutine/interval mechanism used for postgres sources.
- **REQ-018**: Inside `reapMetricMeasurements`, when `source.Kind == "prometheus"`, the reaper MUST call a new function `ScrapeMeasurements(ctx, sourceConn, metricDef)` instead of `QueryMeasurements`.

### `ScrapeMeasurements` Behaviour

- **REQ-019**: `ScrapeMeasurements` MUST issue an HTTP GET to `source.ConnStr`, parse the Prometheus text exposition format, and return `metrics.Measurements`.
- **REQ-020**: The function signature MUST be:
  ```go
  func ScrapeMeasurements(ctx context.Context, md *sources.SourceConn, m metrics.Metric) (metrics.Measurements, error)
  ```
- **REQ-021**: The parser MUST use `github.com/prometheus/common/expfmt` (already an indirect dependency via the Prometheus client library). No new major dependencies are permitted.
- **REQ-022**: Only metric families whose `Name` appears in `m.PromSelectors` MUST be retained. All others MUST be discarded before constructing measurement rows.
- **REQ-023**: Samples from all selected metric families MUST be joined by their label set into a single measurement row per unique label combination. This mirrors how a SQL SELECT returns multiple named columns in one row:
  - Each Prometheus label becomes a `tag_<label_name>` column (string). The `__name__` label MUST be omitted.
  - Each selected metric family name becomes a **value column** whose name is the metric family name and whose value is the sample's float64.
  - `epoch_ns` (int64, Unix nanoseconds) is taken from the sample timestamp when present; otherwise `time.Now().UnixNano()`.

  Example: given `prom_selectors: [pg_stat_activity_count, pg_stat_activity_max_tx_duration]` and two samples both labelled `{datname="mydb", state="active"}`, the result is **one row**:

  | `epoch_ns` | `tag_datname` | `tag_state` | `pg_stat_activity_count` | `pg_stat_activity_max_tx_duration` |
  |---|---|---|---|---|
  | 1714123456… | mydb | active | 42.0 | 5.3 |

- **REQ-023a**: When selected metric families have partially overlapping label sets, rows are formed by the **union** of all label names. Columns absent for a given label combination MUST be set to `0.0`.
- **REQ-024**: Non-finite values (`+Inf`, `-Inf`, `NaN`) in any value column MUST be preserved as-is. Sinks are responsible for handling them.
- **REQ-025**: Histogram and Summary metric families expose synthetic per-bucket/per-quantile samples with extra labels (`le`, `quantile`). Because these extra labels are part of the label set, different bucket samples already produce distinct rows naturally via the join-by-label-set rule in REQ-023. Users who select `my_histogram_bucket` in `prom_selectors` will receive one row per `(label_set ∪ {le})` combination. The `_sum` and `_count` families may be selected independently.
- **CON-003**: Protobuf exposition format (`application/vnd.google.apis.metrics.protobuf`) is out of scope for v1.0. The scrape request MUST only advertise `text/plain` in the `Accept` header.

### Prometheus → Prometheus Proxy Path

- **REQ-026**: A minimal field `SourceKind sources.Kind` MUST be added to `MeasurementEnvelope` to carry the originating source kind. The reaper MUST populate it when building envelopes from `prometheus` sources. All other sinks MUST ignore it.
- **REQ-027**: `PrometheusWriter.Write()` MUST detect a Prom-sourced envelope by checking `envelope.SourceKind == sources.SourcePrometheus`.
- **REQ-028**: When a Prom-sourced envelope is detected, `PrometheusWriter` MUST iterate over every non-`tag_*`, non-`epoch_ns` column in each row and emit one `prometheus.MustNewConstMetric` per column, using the **column name** as the Prometheus metric name. The column name is the original Prometheus metric family name, so the exporter's naming is preserved verbatim.
- **REQ-029**: All `tag_*` columns MUST become Prometheus label key-value pairs on every emitted metric.
- **REQ-030**: The column's float64 value MUST be used as the metric's value. The `epoch_ns` column MUST be converted to a `time.Time` and passed as the metric's timestamp.
- **REQ-031**: Each unique `(column_name, label_set)` within a single `Collect()` call MUST produce a distinct `prometheus.Desc`. Duplicates MUST be deduplicated (same mechanism already used for postgres-sourced metrics).
- **GUD-003**: The pgwatch metric name (from `MetricName` in the envelope) SHOULD be emitted as an additional label `pgwatch_metric` to allow Prometheus queries to filter by pgwatch grouping. This is a guideline, not a hard requirement; implementations MAY omit it.
- **GUD-004**: The Prometheus namespace (`PrometheusWriter.Namespace`) MUST NOT be prepended to column names in Prom-sourced envelopes, since those column names already carry the exporter's namespace (e.g., `pg_stat_activity_count`). Prepending would produce names like `pgwatch_pg_stat_activity_count`, which breaks existing dashboards.

### Preset Support

- **REQ-032**: A new built-in preset SHOULD be provided for common postgres_exporter metrics. Example:

  ```yaml
  presets:
    postgres-exporter-basic:
      description: "Core metrics from postgres_exporter"
      metrics:
        pg_connections: 30
        pg_replication_lag: 30
        pg_stat_bgwriter: 60
  ```

- **GUD-005**: Built-in Prom-targeted metric definitions and presets SHOULD be placed in a separate `metrics_prometheus.yaml` file and merged at startup alongside the main `metrics.yaml`.

### YAML / Configuration

- **REQ-033**: The YAML representation of a `prometheus` source MUST follow the existing `Source` YAML schema with `kind: prometheus`. Example:

  ```yaml
  sources:
    - name: postgres-exporter-prod
      kind: prometheus
      # TLS options are query parameters; Basic Auth is URL userinfo
      conn_str: "https://user:secret@localhost:9187/metrics?tlsrootcert=/etc/ssl/certs/my-ca.pem"
      is_enabled: true
      preset_metrics: postgres-exporter-basic
      custom_tags:
        env: production   # enriches every measurement row
  ```

- **REQ-034**: Existing YAML and database configuration readers MUST continue to function without modification for all existing source kinds.

---

## 4. Interfaces & Data Contracts

### 4.1 `sources.Kind`

```go
// internal/sources/types.go
const (
    SourcePostgres           Kind = "postgres"
    SourcePostgresContinuous Kind = "postgres-continuous-discovery"
    SourcePgBouncer          Kind = "pgbouncer"
    SourcePgPool             Kind = "pgpool"
    SourcePatroni            Kind = "patroni"
    SourcePrometheus         Kind = "prometheus"   // NEW
)
```

### 4.2 `sources.Source` (extended)

```go
// internal/sources/types.go
type Source struct {
    Name                 string                  `yaml:"name"             db:"name"`
    Group                string                  `yaml:"group"            db:"group"`
    ConnStr              string                  `yaml:"conn_str"         db:"connstr"`
    Metrics              metrics.MetricIntervals `yaml:"custom_metrics"   db:"config"`
    MetricsStandby       metrics.MetricIntervals `yaml:"custom_metrics_standby" db:"config_standby"`
    Kind                 Kind                    `yaml:"kind"             db:"dbtype"`
    IncludePattern       string                  `yaml:"include_pattern"  db:"include_pattern"`
    ExcludePattern       string                  `yaml:"exclude_pattern"  db:"exclude_pattern"`
    PresetMetrics        string                  `yaml:"preset_metrics"   db:"preset_config"`
    PresetMetricsStandby string                  `yaml:"preset_metrics_standby" db:"preset_config_standby"`
    IsEnabled            bool                    `yaml:"is_enabled"       db:"is_enabled"`
    CustomTags           map[string]string       `yaml:"custom_tags"      db:"custom_tags"`
    OnlyIfMaster         bool                    `yaml:"only_if_master"   db:"only_if_master"`
    // No new fields required for prometheus TLS — options are encoded as URL query
    // parameters in ConnStr (e.g. ?tlsrootcert=/ca.pem&tlsskipverify=true).
}
```

`CustomTags` values are propagated verbatim into every `MeasurementEnvelope.CustomTags` for that
source, enriching measurement rows with extra dimensions. They MUST NOT be used to pass
configuration parameters.

### 4.3 `metrics.Metric` (extended)

```go
// internal/metrics/types.go
type Metric struct {
    SQLs            SQLs     `yaml:",omitempty"`
    InitSQL         string   `yaml:"init_sql,omitempty"`
    NodeStatus      string   `yaml:"node_status,omitempty"`
    Gauges          []string `yaml:",omitempty"`
    IsInstanceLevel bool     `yaml:"is_instance_level,omitempty"`
    StorageName     string   `yaml:"storage_name,omitempty"`
    Description     string   `yaml:"description,omitempty"`
    PromSelectors   []string `yaml:"prom_selectors,omitempty"` // NEW
}
```

### 4.4 `sources.SourceConn` (extended)

```go
// internal/sources/conn.go
type SourceConn struct {
    Source
    Conn       db.PgxPoolIface    // nil for prometheus sources
    ConnConfig *pgxpool.Config    // nil for prometheus sources
    HTTPClient *http.Client       // NEW: non-nil only for prometheus sources
    RuntimeInfo
    sync.RWMutex
}
```

### 4.5 `ScrapeMeasurements` function signature

```go
// internal/reaper/database.go (or new file internal/reaper/prometheus.go)
func ScrapeMeasurements(ctx context.Context, md *sources.SourceConn, m metrics.Metric) (metrics.Measurements, error)
```

### 4.6 Measurement row shape for a Prometheus metric

Given `prom_selectors: [pg_stat_activity_count, pg_stat_activity_max_tx_duration]` and a scrape
containing both families with the same label set `{datname="mydb", state="active"}`, the result
is a **single row** — identical in structure to what a SQL metric query would return:

```json
{
  "epoch_ns": 1714123456789000000,
  "tag_datname": "mydb",
  "tag_state": "active",
  "pg_stat_activity_count": 42.0,
  "pg_stat_activity_max_tx_duration": 5.3
}
```

Each Prometheus metric family name becomes a value column. Prometheus labels become `tag_` columns.
There is no `value` column and no `tag_metric_family` column. This is the same shape as a pgwatch
SQL metric that returns multiple named columns.

### 4.7 `MeasurementEnvelope` (extended)

```go
// internal/metrics/types.go
type MeasurementEnvelope struct {
    DBName     string
    MetricName string
    CustomTags map[string]string
    Data       Measurements
    SourceKind string // NEW: set to sources.SourcePrometheus for prometheus-sourced envelopes; empty otherwise
}
```

### 4.8 `PrometheusWriter` proxy detection predicate

```go
// Detects whether the envelope originated from a Prometheus exporter source.
func isPromSourcedEnvelope(envelope metrics.MeasurementEnvelope) bool {
    return envelope.SourceKind == string(sources.SourcePrometheus)
}
```

---

## 5. Acceptance Criteria

- **AC-001**: Given a valid `prometheus` source pointing at a running exporter, when the reaper starts, then a scrape goroutine is launched per configured metric at the specified interval, and measurements appear in the sink.
- **AC-002**: Given a `prometheus` source with `preset_metrics: postgres-exporter-basic`, when the reaper resolves the preset, then only metric families listed under the resolved metric definitions are collected; all other families from the scrape are discarded.
- **AC-003**: Given `prom_selectors: [pg_stat_activity_count, pg_stat_activity_max_tx_duration]` and both families returning samples with `{datname="mydb", state="active"}`, when `ScrapeMeasurements` processes the scrape, then the result is **one row** containing `tag_datname = "mydb"`, `tag_state = "active"`, `pg_stat_activity_count = 42.0`, and `pg_stat_activity_max_tx_duration = 5.3` — not two separate rows.
- **AC-004**: Given `conn_str` containing `?tlsskipverify=true`, when the scrape HTTP client is constructed, then TLS verification is disabled and a warning is logged.
- **AC-005**: Given `conn_str: "http://user:secret@localhost:9187/metrics"`, when any log message referencing the URL is produced, then the password `secret` MUST NOT appear in the log output.
- **AC-006**: Given `source.Kind == "prometheus"` and `PrometheusWriter` as the active sink, when `Write()` is called with a Prom-sourced envelope, then each value column in each row is emitted as a separate Prometheus metric whose name is the column name (original metric family name) without the pgwatch namespace prefix.
- **AC-007**: Given a metric definition with `prom_selectors: [pg_stat_activity_count]` assigned to a `postgres` source, when the reaper collects that metric, then `PromSelectors` is ignored and the standard SQL path is used.
- **AC-008**: Given a `prometheus` source with a metric that has neither `PromSelectors` nor `SQLs`, when the reaper initialises, then a warning is logged and no goroutine is started for that metric.
- **AC-009**: Given the exporter is temporarily unreachable during a scrape, when `ScrapeMeasurements` returns an error, then the reaper logs the error, does not write to the sink for that interval, and retries on the next interval without crashing.
- **AC-010**: Given a Prometheus histogram metric family `my_histogram_bucket` in `prom_selectors`, when `ScrapeMeasurements` processes the scrape, then each bucket produces a distinct row because the `le` label is part of the label set used for joining, and `my_histogram_bucket` appears as a value column in each row.

---

## 6. Test Automation Strategy

- **Test Levels**: Unit, Integration.
- **Unit Tests**:
  - `ScrapeMeasurements` with a table-driven test using a mock HTTP server (`httptest.NewServer`) serving pre-recorded Prometheus exposition text. Cover: normal gauges, counters, histograms, summaries, missing families (selector filtering), non-finite values, missing timestamp (defaults to `time.Now()`).
  - `isPromSourcedEnvelope` with empty data and data containing/lacking `tag_metric_family`.
  - `SourceConn.Connect()` with mock TLS server for Basic Auth and `tls_skip_verify` cases.
  - `PrometheusWriter.Write()` with Prom-sourced envelopes: verify emitted metric names and absence of namespace prefix.
- **Integration Tests**:
  - Start a minimal Prometheus text-format HTTP server in the test, register a `prometheus` source, run the reaper loop for N seconds, and assert measurements appear in the JSON sink file.
  - Test Prom→Prom proxy: assert that the Prometheus scrape endpoint of pgwatch re-exposes the original metric family names.
- **Frameworks**: Standard `testing` package; `httptest`; `testify/assert` (already used in the codebase).
- **Coverage**: New packages/files introduced by this feature MUST achieve ≥ 80 % statement coverage.
- **CI**: Existing `go test ./...` pipeline covers new tests automatically.

---

## 7. Rationale & Context

### Why reuse `Source` struct instead of a new struct?

All existing configuration readers (YAML, PostgreSQL config DB, gRPC API) operate on `Source`.
Adding a new struct would require changes in every reader and writer. The `prometheus` kind follows
the same pattern as `pgbouncer` and `pgpool`, which also reuse `Source` with non-standard connection strings.

### Why selector lists instead of regex or auto-discovery?

Auto-discovery (all families → individual pgwatch metrics) would create an unbounded, unpredictable
set of tables in the sink on first scrape. Selector lists make the schema explicit, predictable, and
consistent with the philosophy that a pgwatch metric is a deliberately designed measurement set.
Regex can be added later without breaking backward compatibility.

### Why metric family names become value columns instead of producing one row per sample?

A pgwatch metric is modelled after a SQL query result: multiple named value columns in a single row,
distinguished by their tag columns. For example, a SQL metric for connection state returns
`pg_stat_activity_count` and `pg_stat_activity_max_tx_duration` as two columns of the same row for
each `(datname, state)` pair. The Prometheus counterpart should produce the same shape so that sink
schemas, Grafana dashboards, and downstream tooling are identical regardless of whether the source
is a PostgreSQL connection or a Prometheus exporter. One-row-per-sample would produce a tall/narrow
table incompatible with this convention.

### Why keep the same `MeasurementEnvelope` for the proxy path?

Introducing a separate code path (e.g., a direct HTTP relay bypassing the reaper) would split the
lifecycle management, filtering, and custom-tag injection logic into two places. The chosen approach
(parse → `MeasurementEnvelope` → lighter Prom sink write) keeps a single pipeline. The performance
difference is negligible because the dominant cost is the HTTP scrape itself, not the in-process
row transformation.

### Why not prepend the pgwatch namespace in Prom→Prom mode?

When pgwatch proxies an existing exporter, users expect to query metrics under their original names
(e.g., `pg_stat_activity_count`, not `pgwatch_pg_stat_activity_count`). Prepending a namespace would
break existing Grafana dashboards and alerting rules that target those metrics.

---

## 8. Dependencies & External Integrations

### External Systems

- **EXT-001**: Any HTTP(S) service exposing Prometheus text exposition format on a `/metrics` endpoint (e.g., `node_exporter`, `postgres_exporter`, `redis_exporter`).

### Third-Party Services

None beyond what is already used.

### Infrastructure Dependencies

- **INF-001**: `github.com/prometheus/common/expfmt` — Prometheus text format parser. Already present as an indirect dependency via `github.com/prometheus/client_golang`. Must be promoted to a direct dependency in `go.mod`.
- **INF-002**: Standard library `net/http` for scraping. No additional HTTP client library is required.
