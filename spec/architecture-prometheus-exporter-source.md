---
title: Prometheus Exporter as a pgwatch Source
version: 1.3
date_created: 2026-04-26
date_updated: 2026-05-08
tags: [architecture, design, prometheus, sources, metrics, sinks]
---

# Introduction

This specification defines how pgwatch fetches metrics from external Prometheus exporters
(e.g., `node_exporter`, `postgres_exporter`, custom exporters) and routes them through the
standard pgwatch pipeline. It covers the new source kind, the scrape lifecycle,
label-to-column conventions, and a lightweight proxy path that avoids double-conversion
when the configured sink is also Prometheus.

**No dedicated pgwatch metric definitions are required for Prometheus sources.** The Prometheus
metric family names are used directly as pgwatch metric names via the standard `custom_metrics`
/ `preset_metrics` configuration on the source.

---

## 1. Purpose & Scope

**Purpose**: Allow pgwatch to scrape arbitrary Prometheus exporters, apply family-level
filtering, and write measurements to any configured sink — including acting as a low-overhead
proxy when the sink is a Prometheus endpoint.

**In scope**:
- New source `Kind` value `prometheus`.
- Use of `custom_metrics` / `preset_metrics` to specify Prometheus family names and their emit intervals.
- `PromSourceReaper`: a dedicated runner type implementing the same `SourceRunner` interface as `SourceReaper` (DB sources), with a GCD-based tick loop, one HTTP GET per tick, and per-family emit cadence gated by `lastEmitted` state.
- Measurement row structure for Prometheus samples.
- Prometheus-to-Prometheus (Prom→Prom) proxy optimisation in `PrometheusWriter`.
- Authentication and TLS configuration.
- "Scrape-all" mode when no metrics are configured.

**Out of scope**:
- Push-gateway integration.
- Prometheus alerting rules.
- Remote-write protocol (only the `/metrics` exposition format is targeted).
- Grafana dashboard changes.
- Any extension to the `metrics.Metric` struct.

**Audience**: pgwatch maintainers implementing the feature. The document is also intended to be
consumed directly by AI coding assistants.

---

## 2. Definitions

| Term | Definition |
|---|---|
| **Prometheus exporter** | An HTTP service that exposes metrics in Prometheus exposition format (text or protobuf) at a `/metrics` endpoint. |
| **Metric family** | A single named group of Prometheus samples sharing the same `__name__` label (e.g., `pg_stat_activity_count`). |
| **Sample** | A single (label-set, float64 value, optional timestamp) triple within a metric family. |
| **pgwatch metric** | For `prometheus` sources: the Prometheus metric family name used as the key in `custom_metrics`. For other source kinds: a named, SQL-based measurement set. |
| **pgwatch source** | A configured entity from which measurements are collected. |
| **scrape** | An HTTP GET request to a Prometheus exporter's `/metrics` endpoint. Always returns all families. |
| **scrape interval** | The HTTP request cadence for a `prometheus` source: `min` of all configured emit intervals (or 60 s in scrape-all mode). Drives how often the goroutine wakes and issues an HTTP GET. |
| **emit interval** | The per-family cadence at which measurements are forwarded to the sink. Configured as the value in `custom_metrics`. May be longer than the scrape interval. |
| **lastEmitted** | Per-family timestamp of the most recent successful sink write. Used to gate whether a family's measurements are forwarded on a given scrape tick. |
| **scrape-all mode** | Activated when a `prometheus` source has neither `custom_metrics` nor `preset_metrics` configured. The reaper scrapes all available families at a fixed 60 s interval and emits every family on every tick. |
| **MeasurementEnvelope** | The internal pgwatch structure `{DBName, MetricName, CustomTags, Data Measurements}` passed from reaper to sinks. |
| **Prom→Prom path** | The code path activated when `source.Kind == "prometheus"` and the active sink is `PrometheusWriter`. |
| **tag column** | A pgwatch measurement column whose name starts with `tag_`. Treated as an indexed dimension. |
| **value column** | A numeric measurement column not prefixed `tag_`. For `prometheus` sources, named after the metric family. |
| **epoch_ns** | Mandatory measurement column: Unix timestamp in nanoseconds (int64). |
| **ConnStr** | The `Source.ConnStr` field, repurposed as the scrape URL for `prometheus` sources. |

---

## 3. Requirements, Constraints & Guidelines

### Source Definition

- **REQ-001**: `sources.Kind` MUST include a new constant `SourcePrometheus = "prometheus"`.
- **REQ-002**: A `prometheus` source MUST use `Source.ConnStr` as the HTTP(S) scrape URL (e.g., `http://host:9187/metrics`). No PostgreSQL DSN is required or expected.
- **REQ-003**: `Source.Metrics` (type `metrics.MetricIntervals`) MUST map Prometheus metric family name → emit interval in seconds. Each key is an exact family name; the value is how often that family's measurements are forwarded to the sink.
- **REQ-004**: `Source.PresetMetrics` MUST be supported for `prometheus` sources. A preset resolves to a `MetricIntervals` map whose keys are Prometheus family names and whose values are emit intervals.
- **REQ-005**: `sources.Kinds` slice MUST include `SourcePrometheus` so that `Kind.IsValid()` returns `true`.
- **CON-001**: Fields `Source.IncludePattern`, `Source.ExcludePattern`, `Source.OnlyIfMaster`, `Source.MetricsStandby`, and `Source.PresetMetricsStandby` are irrelevant for `prometheus` sources and MUST be ignored without error.
- **GUD-001**: `Source.Name` SHOULD be set to a human-readable identifier for the exporter (e.g., `node-exporter-prod-db01`). It maps to `MeasurementEnvelope.DBName`.

### Authentication and TLS

- **REQ-006**: The scrape URL in `ConnStr` MUST support standard URL userinfo for Basic Auth: `http://user:password@host:9187/metrics`.
- **REQ-007**: TLS options MUST be configurable via URL query parameters appended to `ConnStr`:

  | Query parameter | Description |
  |---|---|
  | `tlsrootcert=<path>` | Path to a PEM-encoded CA certificate file used to verify the server certificate. Omit to use system roots. |
  | `tlsskipverify=true` | When `true`, TLS certificate verification is disabled. Default: absent / `false`. |

  Example: `https://host:9187/metrics?tlsrootcert=/etc/ssl/ca.pem`

  The `ConnStr` URL MUST be stripped of these query parameters before being used as the actual HTTP request URL, so they are never forwarded to the exporter.

- **SEC-001**: Passwords stored in `ConnStr` MUST NOT be logged. The logger MUST redact the userinfo component before printing any URL.
- **SEC-002**: `tlsskipverify=true` in the scrape URL MUST produce a warning-level log entry each time the HTTP client is constructed for that source.
- **CON-002**: Custom headers (e.g., `Authorization: Bearer`) are out of scope for v1.0.

### Metric Filtering

- **REQ-008**: For `prometheus` sources, each key in the resolved `MetricIntervals` map (from `custom_metrics` or `preset_metrics`) MUST be treated as an exact Prometheus metric family name. No `metrics.Metric` definition file entry is required or consulted for `prometheus` sources.
- **REQ-009**: Metric families present in the scrape response but NOT listed in the resolved `MetricIntervals` map MUST be silently discarded when the map is non-empty.
- **REQ-010**: When both `Source.Metrics` and `Source.PresetMetrics` are empty, the source MUST operate in **scrape-all mode**: every metric family returned by the exporter is collected at a default interval of 60 seconds. The reaper MUST emit a warning-level log message when activating scrape-all mode, noting the potential for an unbounded number of measurement tables in the sink.
- **GUD-002**: Regex and glob patterns in family names are not supported in v1.0. Each name MUST be an exact match.

### Scrape Lifecycle

- **REQ-011**: `SourceConn` MUST carry an `HTTPClient *http.Client` field alongside the existing `Conn db.PgxPoolIface`. For `prometheus` sources, `Conn` MUST remain `nil`; `HTTPClient` MUST be non-nil after `Connect()`.
- **REQ-012**: `SourceConn.Connect()` MUST parse TLS query parameters (`tlsrootcert`, `tlsskipverify`) from `ConnStr`, construct an `*http.Client` with the derived TLS config, and validate reachability by issuing a HEAD or GET to the scrape URL (with TLS parameters stripped). If the exporter is unreachable, `Connect()` MUST return an error.
- **REQ-013**: `SourceConn.Ping()` for a `prometheus` source MUST issue a HEAD request to the scrape URL and return an error if the status code is not 2xx.
- **REQ-014**: `SourceConn.IsPostgresSource()` MUST return `false` for `Kind == "prometheus"`.
- **REQ-015**: `SourceConn.FetchRuntimeInfo()` for `prometheus` sources MUST be a no-op that sets `RuntimeInfo.VersionStr = "prometheus"` and `RuntimeInfo.Version = 0` without error.
- **REQ-016**: The reaper MUST instantiate a `PromSourceReaper` for each `prometheus` source (not one per family) and start it via `go psr.Run(sourceCtx)`, mirroring the `NewSourceReaper` / `go sr.Run(sourceCtx)` pattern used for DB sources. `PromSourceReaper` MUST implement a `Run(ctx context.Context)` method satisfying the `SourceRunner` interface (see `refactor-sourceconn-interface.md` §4.6).
- **REQ-017**: `PromSourceReaper.Run` MUST compute the **scrape interval** using the same `GCDSlice` helper as `SourceReaper.calcTickInterval`: `min` of all emit interval values in the resolved `MetricIntervals` map, floored at `minTickInterval`. In scrape-all mode the scrape interval MUST default to 60 seconds.
- **REQ-018**: On every scrape tick `PromSourceReaper.Run` MUST issue a single HTTP GET to the exporter (via `ScrapeAll`) and receive measurements for all available families in one response.
- **REQ-019**: The goroutine MUST maintain a `lastEmitted map[string]time.Time` keyed by family name, initialised empty. After a successful scrape, for each family present in the response:
  - In **filtered mode**: skip families not present in the resolved `MetricIntervals` map.
  - Emit a `MeasurementEnvelope` to the sink for family `f` only when `time.Since(lastEmitted[f]) >= emitInterval[f]`. On first scrape (zero `lastEmitted` value) the family MUST always be emitted.
  - Update `lastEmitted[f]` to the current time after a successful sink write.
  - In **scrape-all mode**: all families are emitted on every tick (emit interval equals the 60 s scrape interval for all).
- **REQ-020**: If `ScrapeAll` returns an error, the goroutine MUST log the error at warning level, leave all `lastEmitted` values unchanged, and retry on the next tick without crashing.

### `ScrapeAll` Behaviour

- **REQ-021**: `ScrapeAll` MUST issue a single HTTP GET to the scrape URL (derived from `source.ConnStr` with TLS parameters stripped), parse the full Prometheus text exposition response, and return measurements for every discovered family.
- **REQ-022**: The function signature MUST be:
  ```go
  // internal/reaper/prometheus.go

  // ScrapeAll fetches the exporter's /metrics endpoint and returns one
  // Measurements slice per discovered metric family, keyed by family name.
  func ScrapeAll(ctx context.Context, sc *sources.PromConn) (map[string]metrics.Measurements, error)
  ```
- **REQ-023**: The parser MUST use `github.com/prometheus/common/expfmt` (already an indirect dependency via the Prometheus client library). No new major dependencies are permitted.
- **REQ-024**: Each sample from a metric family becomes **one measurement row**:
  - Each Prometheus label becomes a `tag_<label_name>` column (string). The `__name__` label MUST be omitted.
  - A single value column named after the metric family holds the sample's float64 value.
  - `epoch_ns` (int64, Unix nanoseconds) is taken from the sample timestamp when present; otherwise `time.Now().UnixNano()`.
- **REQ-025**: Non-finite values (`+Inf`, `-Inf`, `NaN`) in the value column MUST be preserved as-is. Sinks are responsible for handling them.
- **CON-003**: Protobuf exposition format (`application/vnd.google.apis.metrics.protobuf`) is out of scope for v1.0. The scrape request MUST only advertise `text/plain` in the `Accept` header.

### Prometheus → Prometheus Proxy Path

- **REQ-026**: A field `SourceKind string` MUST be added to `MeasurementEnvelope` to carry the originating source kind. The reaper MUST set it to `string(sources.SourcePrometheus)` for `prometheus`-sourced envelopes. All other sinks MUST ignore it.
- **REQ-027**: `PrometheusWriter.Write()` MUST detect a Prom-sourced envelope by checking `envelope.SourceKind == string(sources.SourcePrometheus)`.
- **REQ-028**: When a Prom-sourced envelope is detected, `PrometheusWriter` MUST emit one `prometheus.MustNewConstMetric` per row, using the **value column name** (the family name) as the Prometheus metric name. Since each envelope carries exactly one family's measurements, the value column name equals `envelope.MetricName`.
- **REQ-029**: All `tag_*` columns MUST become Prometheus label key-value pairs on every emitted metric.
- **REQ-030**: The column's float64 value MUST be used as the metric's value. The `epoch_ns` column MUST be converted to a `time.Time` and passed as the metric's timestamp.
- **REQ-031**: Each unique `(metric_name, label_set)` within a single `Collect()` call MUST produce a distinct `prometheus.Desc`. Duplicates MUST be deduplicated (same mechanism already used for postgres-sourced metrics).
- **GUD-003**: The Prometheus namespace (`PrometheusWriter.Namespace`) MUST NOT be prepended to metric names in Prom-sourced envelopes, since those names already carry the exporter's namespace (e.g., `pg_stat_activity_count`). Prepending would produce names like `pgwatch_pg_stat_activity_count`, which breaks existing dashboards.

### Preset Support

- **REQ-032**: A new built-in preset SHOULD be provided for common `postgres_exporter` metric families. Example:

  ```yaml
  presets:
    postgres-exporter-basic:
      description: "Core metrics from postgres_exporter"
      metrics:
        pg_stat_activity_count: 30
        pg_stat_bgwriter_checkpoints_timed: 60
        pg_stat_replication_pg_wal_lsn_diff: 30
  ```

- **GUD-004**: Built-in Prom-targeted presets SHOULD be kept in a dedicated section or file separate from SQL-based presets to avoid confusion.

### YAML / Configuration

- **REQ-033**: The YAML representation of a `prometheus` source MUST follow the existing `Source` YAML schema with `kind: prometheus`. Example:

  ```yaml
  sources:
    - name: postgres-exporter-prod
      kind: prometheus
      conn_str: "https://user:secret@localhost:9187/metrics?tlsrootcert=/etc/ssl/certs/my-ca.pem"
      is_enabled: true
      # emit intervals differ per family; HTTP scrape cadence = min(30, 60) = 30 s
      custom_metrics:
        pg_stat_activity_count: 30
        pg_stat_bgwriter_checkpoints_timed: 60
      custom_tags:
        env: production

    - name: node-exporter-prod   # scrape-all: no metrics configured, scrapes every 60 s
      kind: prometheus
      conn_str: "http://localhost:9100/metrics"
      is_enabled: true
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

### 4.2 `sources.Source` (unchanged)

No new fields are required. The existing `Metrics` (`custom_metrics`) and `PresetMetrics` fields
carry Prometheus family names and intervals for `prometheus` sources exactly as they carry SQL
metric names and intervals for postgres sources.

```go
// internal/sources/types.go — no structural changes
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
}
```

`CustomTags` values are propagated verbatim into every `MeasurementEnvelope.CustomTags` for that
source, enriching measurement rows with extra dimensions.

### 4.3 `metrics.Metric` (unchanged)

No extension to `metrics.Metric` is required. `prometheus` sources do not reference any
`metrics.Metric` definition; the family name from `custom_metrics` is used directly as both
the collection key and the value column name in measurement rows.

### 4.4 `sources.SourceConn` (extended)

```go
// internal/sources/conn.go
type SourceConn struct {
    Source
    Conn        db.PgxPoolIface  // nil for prometheus sources
    ConnConfig  *pgxpool.Config  // nil for prometheus sources
    HTTPClient  *http.Client     // NEW: non-nil only for prometheus sources
    RuntimeInfo
    sync.RWMutex
}
```

### 4.5 `ScrapeAll` function signature

```go
// internal/reaper/prometheus.go

// ScrapeAll fetches the exporter's /metrics endpoint and returns one
// Measurements slice per discovered metric family, keyed by family name.
// It is the only HTTP call made per scrape tick; family filtering and
// per-family emit gating are performed by the caller.
func ScrapeAll(ctx context.Context, sc *sources.PromConn) (map[string]metrics.Measurements, error)
```

### 4.6 `PromSourceReaper` struct and `Run` pseudo-code

```go
// internal/reaper/prom_source_reaper.go

// PromSourceReaper manages metric collection for a single prometheus source.
// It mirrors the structure of SourceReaper (DB sources) but issues HTTP scrapes
// instead of SQL queries, using per-family emit gating via lastEmitted.
type PromSourceReaper struct {
    reaper      *Reaper
    md          *sources.PromConn
    lastEmitted map[string]time.Time
}

func NewPromSourceReaper(r *Reaper, md *sources.PromConn) *PromSourceReaper {
    return &PromSourceReaper{
        reaper:      r,
        md:          md,
        lastEmitted: make(map[string]time.Time),
    }
}

// calcScrapeInterval computes GCD of all emit intervals using the same
// GCDSlice helper as SourceReaper.calcTickInterval.
func (psr *PromSourceReaper) calcScrapeInterval() time.Duration {
    intervals := make([]int, 0, len(psr.md.Metrics))
    for _, v := range psr.md.Metrics {
        intervals = append(intervals, max(v, minTickInterval))
    }
    return time.Duration(max(GCDSlice(intervals), minTickInterval)) * time.Second
}

// Run is the main loop for a single prometheus source. Satisfies SourceRunner.
func (psr *PromSourceReaper) Run(ctx context.Context) {
    l := log.GetLogger(ctx).WithField("source", psr.md.Name)
    ctx = log.WithLogger(ctx, l)

    intervals := psr.md.Metrics // family name → emit interval in seconds
    if len(intervals) == 0 {
        l.Warning("no metrics configured for prometheus source, activating scrape-all mode at 60s")
    }

    scrapeInterval := psr.calcScrapeInterval()
    ticker := time.NewTicker(scrapeInterval)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case now := <-ticker.C:
            allFamilies, err := ScrapeAll(ctx, psr.md)
            if err != nil {
                l.WithError(err).Warning("scrape failed")
                continue
            }
            for family, measurements := range allFamilies {
                emitSecs := intervals[family]
                emitInterval := time.Duration(emitSecs) * time.Second
                if emitInterval == 0 && len(intervals) > 0 {
                    continue // family not in configured list; discard
                }
                if emitInterval == 0 {
                    emitInterval = scrapeInterval // scrape-all: emit every tick
                } else if last, ok := psr.lastEmitted[family]; ok && now.Sub(last) < emitInterval {
                    continue // not yet due
                }
                psr.reaper.measurementCh <- metrics.MeasurementEnvelope{
                    DBName:     psr.md.Name,
                    MetricName: family,
                    Data:       measurements,
                    CustomTags: psr.md.CustomTags,
                    SourceKind: string(sources.SourcePrometheus),
                }
                psr.lastEmitted[family] = now
            }
        }
    }
}
```

### 4.7 Measurement row shape

For family `pg_stat_activity_count` with label set `{datname="mydb", state="active"}`:

```json
{
  "epoch_ns": 1714123456789000000,
  "tag_datname": "mydb",
  "tag_state": "active",
  "pg_stat_activity_count": 42.0
}
```

Each sample produces one row. The single value column is named after the metric family.
Different label combinations within the same family produce separate rows.

For a histogram family `my_histogram_bucket`, each bucket is a distinct row because `le` is a
label and therefore becomes `tag_le`:

| `epoch_ns` | `tag_datname` | `tag_le` | `my_histogram_bucket` |
|---|---|---|---|
| 1714… | mydb | 0.005 | 0.0 |
| 1714… | mydb | 0.01  | 3.0 |
| 1714… | mydb | +Inf  | 42.0 |

### 4.8 `MeasurementEnvelope` (extended)

```go
// internal/metrics/types.go
type MeasurementEnvelope struct {
    DBName     string
    MetricName string
    CustomTags map[string]string
    Data       Measurements
    SourceKind string // NEW: "prometheus" for prometheus-sourced envelopes; empty otherwise
}
```

### 4.9 `PrometheusWriter` proxy detection predicate

```go
// Detects whether the envelope originated from a Prometheus exporter source.
func isPromSourcedEnvelope(envelope metrics.MeasurementEnvelope) bool {
    return envelope.SourceKind == string(sources.SourcePrometheus)
}
```

---

## 5. Acceptance Criteria

- **AC-001**: Given a valid `prometheus` source pointing at a running exporter, when the reaper starts, then **exactly one** goroutine is launched for that source; measurements appear in the sink with one row per sample.
- **AC-002**: Given `custom_metrics: {pg_stat_activity_count: 30, pg_stat_bgwriter_checkpoints_timed: 60}`, when the reaper runs, then the HTTP scrape cadence is 30 s (the minimum); `pg_stat_activity_count` measurements are forwarded every 30 s and `pg_stat_bgwriter_checkpoints_timed` measurements are forwarded every 60 s, but only one HTTP GET is made per 30 s tick.
- **AC-003**: Given `custom_metrics: {pg_stat_activity_count: 30}` and a scrape returning samples with `{datname="mydb", state="active"}` and `{datname="mydb", state="idle"}`, when the goroutine processes the scrape, then the result is **two rows** — one per label combination — each with a single value column `pg_stat_activity_count`.
- **AC-004**: Given `custom_metrics: {pg_stat_activity_count: 30}` and a scrape response that also contains `pg_stat_bgwriter_checkpoints_timed`, when the goroutine processes the scrape, then `pg_stat_bgwriter_checkpoints_timed` measurements are discarded and never forwarded to the sink.
- **AC-005**: Given `conn_str` containing `?tlsskipverify=true`, when the scrape HTTP client is constructed, then TLS verification is disabled and a warning is logged.
- **AC-006**: Given `conn_str: "http://user:secret@localhost:9187/metrics"`, when any log message referencing the URL is produced, then the password `secret` MUST NOT appear in the log output.
- **AC-007**: Given `source.Kind == "prometheus"` and `PrometheusWriter` as the active sink, when `Write()` is called with a Prom-sourced envelope for family `pg_stat_activity_count`, then the emitted Prometheus metric is named `pg_stat_activity_count` without any pgwatch namespace prefix.
- **AC-008**: Given a `prometheus` source with neither `custom_metrics` nor `preset_metrics`, when the reaper initialises, then a warning is logged and all families from each scrape are written to the sink as separate envelopes at a 60 s cadence (scrape-all mode).
- **AC-009**: Given the exporter is temporarily unreachable during a scrape tick, when `ScrapeAll` returns an error, then the reaper logs the error, leaves all `lastEmitted` values unchanged, does not write to the sink for that tick, and retries on the next tick without crashing.
- **AC-010**: Given a `prometheus` source with `preset_metrics: postgres-exporter-basic`, when the reaper resolves the preset, then only the metric families listed in that preset's `metrics` map are forwarded to the sink.
- **AC-011**: Given a Prometheus histogram family `my_histogram_bucket` in `custom_metrics`, when the goroutine processes the scrape, then each bucket produces a distinct row with `tag_le` holding the bucket boundary and `my_histogram_bucket` as the single value column.

---

## 6. Test Automation Strategy

- **Test Levels**: Unit, Integration.
- **Unit Tests**:
  - `ScrapeAll` with a table-driven test using a mock HTTP server (`httptest.NewServer`) serving pre-recorded Prometheus exposition text. Cover: normal gauges, counters, histograms (verify `le` → `tag_le`), non-finite values, missing timestamp (defaults to `time.Now()`), all families returned in the map.
  - `PromSourceReaper` emit-gating logic: mock `ScrapeAll` to return two families with intervals `{A: 30s, B: 60s}`; advance a fake clock and assert that after 30 s only A is emitted, after 60 s both are emitted, and only one `ScrapeAll` call is made per 30 s tick.
  - Filtered mode: assert families not in `MetricIntervals` are discarded after `ScrapeAll`.
  - `PromConn.Connect()` with mock TLS server: Basic Auth redaction in logs, `tlsskipverify` warning.
  - `PrometheusWriter.Write()` with Prom-sourced envelopes: verify emitted metric names match family names and the pgwatch namespace prefix is absent.
- **Integration Tests**:
  - Start a minimal Prometheus text-format HTTP server in the test, register a `prometheus` source with two families at different intervals, run the reaper loop, and assert the correct per-family emit cadence in the JSON sink file with a single HTTP call per tick.
  - Test Prom→Prom proxy: assert that the Prometheus scrape endpoint of pgwatch re-exposes the original metric family names.
  - Test scrape-all mode: register a `prometheus` source with no metrics configured, verify all families appear in the sink and a warning was logged.
- **Frameworks**: Standard `testing` package; `httptest`; `testify/assert` (already used in the codebase).
- **Coverage**: New packages/files introduced by this feature MUST achieve ≥ 80 % statement coverage.
- **CI**: Existing `go test ./...` pipeline covers new tests automatically.

---

## 7. Rationale & Context

### Why one goroutine per source, not one per family?

A Prometheus `/metrics` endpoint is a single HTTP resource that always returns all metric
families in one response body. There is no protocol mechanism to request individual families.
Launching one goroutine per configured family would therefore issue `N` full HTTP GET requests
per collection cycle where each response is identical — O(N) bandwidth and O(N) exporter load
for the same data. With a busy exporter (e.g., `node_exporter` with 200+ families) and small
intervals this is a significant waste. The single-goroutine model issues exactly one HTTP GET
per `min(intervals)` tick and fans out the result in memory, which is the correct abstraction
for a scrape-based data source.

### Why `min(intervals)` as the scrape cadence?

Scraping less frequently than the shortest configured emit interval would cause data to be
stale by more than the operator intended. Scraping at `min(intervals)` guarantees that every
family can be forwarded to the sink as soon as its emit interval elapses. Families with longer
intervals are simply filtered at sink-write time using `lastEmitted` state; the extra scrape
response data they produce is discarded in memory at negligible cost.

### Why `PromSourceReaper` instead of a standalone goroutine function?

The existing `SourceReaper` consolidates N per-metric goroutines into a single per-source
goroutine with a GCD-based tick loop and `pgx.Batch` SQL execution. Introducing a parallel
`PromSourceReaper` type follows the same pattern for HTTP sources: one goroutine per source,
GCD-based cadence, per-family emit gating. Keeping the same structural shape means:

1. **Uniform dispatch**: `Reaper.sourceReapers` holds `SourceRunner` values regardless of
   kind. Start/stop lifecycle management in `ShutdownOldWorkers` requires zero kind-specific
   branching.
2. **Shared helpers**: `GCDSlice`, `minTickInterval`, and the cancel-func map are reused
   without duplication.
3. **Testability**: `PromSourceReaper` can be unit-tested in isolation via a mock `Reaper`
   and mock HTTP server, exactly as `SourceReaper` is tested with a mock DB.

### Why use `custom_metrics` keys as family names instead of a dedicated selector field?

Prometheus exporters expose a fixed set of metric families determined by the exporter binary —
pgwatch cannot define or alter them. Introducing a `PromSelectors` field in `metrics.Metric`
would require a parallel metric definition file for Prometheus sources, adding schema complexity
with no practical benefit. The `custom_metrics` / `preset_metrics` mechanism already provides
exactly the right abstraction: a named set of things to collect at a given interval. For
`prometheus` sources, "things to collect" are family names rather than SQL metric names; the
data model is identical, and no new struct fields are needed.

### Why one row per sample instead of joining families into wide rows?

An earlier design grouped multiple Prometheus families sharing a label set into a single wide
row (one row per unique label combination, multiple value columns). This was abandoned because:

1. **Unnecessary coupling**: Prometheus families are independently named, versioned, and may
   appear or disappear between scrapes. Joining by label set forces choosing a primary family
   and handling partial overlap with `0.0` fill — complexity with no added value.
2. **Natural sink granularity**: one family → one table in PostgreSQL/TimescaleDB is the
   correct granularity. It matches how `postgres_exporter` itself models metrics and how
   existing Grafana dashboards query them.
3. **Prom→Prom fidelity**: in the proxy path the original family name must be the emitted
   metric name. With one-row-per-sample, the value column name IS the family name, making the
   proxy path trivial. Wide rows would require reconstructing the family name from context.

### Why allow scrape-all mode instead of requiring explicit metric lists?

Exploration is a common workflow: operators want to quickly connect pgwatch to an unfamiliar
exporter and see what it exposes before committing to a fixed list. Requiring explicit
configuration for every family would make exploration impossible. The mandatory warning log
ensures operators understand the schema implications before deploying to production.

### Why reuse `Source` struct instead of a new struct?

All existing configuration readers (YAML, PostgreSQL config DB, gRPC API) operate on `Source`.
The `prometheus` kind follows the same pattern as `pgbouncer` and `pgpool`, which also reuse
`Source` with non-standard connection strings.

### Why keep the same `MeasurementEnvelope` for the proxy path?

Introducing a separate code path (e.g., a direct HTTP relay bypassing the reaper) would split
lifecycle management, filtering, and custom-tag injection into two places. The chosen approach
(parse → `MeasurementEnvelope` → lighter Prom sink write) keeps a single pipeline. The
performance difference is negligible because the dominant cost is the HTTP scrape itself, not
the in-process row transformation.

### Why not prepend the pgwatch namespace in Prom→Prom mode?

When pgwatch proxies an existing exporter, users expect to query metrics under their original
names (e.g., `pg_stat_activity_count`, not `pgwatch_pg_stat_activity_count`). Prepending a
namespace would break existing Grafana dashboards and alerting rules.

---

## 8. Dependencies & External Integrations

### External Systems

- **EXT-001**: Any HTTP(S) service exposing Prometheus text exposition format on a `/metrics` endpoint (e.g., `node_exporter`, `postgres_exporter`, `redis_exporter`).

### Third-Party Services

None beyond what is already used.

### Infrastructure Dependencies

- **INF-001**: `github.com/prometheus/common/expfmt` — Prometheus text format parser. Already present as an indirect dependency via `github.com/prometheus/client_golang`. Must be promoted to a direct dependency in `go.mod`.
- **INF-002**: Standard library `net/http` for scraping. No additional HTTP client library is required.
