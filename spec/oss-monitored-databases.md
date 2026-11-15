---
title: "The embedder reads the monitored sources"
version: 1.0
date_created: 2026-11-15
date_updated: 2026-11-15
owner: pgwatch maintainers
status: implemented
tags: [app, reaper, sources, embedding]
---

# Introduction

The reaper knows, for every monitored source, its name, its tags, its PostgreSQL version and
whether it is in recovery. An embedder of `pkg/app` cannot ask for any of it. The list lives in
the private field `monitoredSources` of the private type `reaper`, and `RuntimeInfo` is only
reachable through a cast to `*sources.DbConn`. An embedder that needs version and role
therefore reads them back from the sink.

This specification adds two read-only accessors: `App.MonitoredSources()` and
`SourceConn.GetRuntimeInfo()`.

Code references are against branch `v7`. Line numbers are indicative, not contractual.

---

## 1. Purpose & Scope

**Purpose**: an embedder reads the sources the reaper monitors, with their runtime info, from
memory.

**In scope**: one method on `SourceConn` and its two implementations, one method on the reaper,
one method on `App`.

**Out of scope**: refreshing runtime info on demand; new types; changes to what or how often
the reaper collects; exporting the type `reaper`.

**Audience**: pgwatch maintainers and embedders of `pkg/app`.

---

## 2. Definitions

| Term | Definition |
|---|---|
| **Monitored sources** | `reaper.monitoredSources`: the `SourceConns` of the last `LoadSources`, discovered databases included. |
| **Runtime info** | `sources.RuntimeInfo`, filled by `DbConn.FetchRuntimeInfo` at most every 5 minutes. |
| **Embedder** | A program that builds `app.New(...)` and runs it in its own process. |

---

## 3. Requirements, Constraints & Guidelines

- **REQ-001**: `SourceConn` gains `GetRuntimeInfo() RuntimeInfo`, the twin of `GetSource()`.
  `DbConn` returns a copy read under its `RLock`, with `Extensions` and `ChangeState` cloned.
  `PromConn` returns `RuntimeInfo{}`.
- **REQ-002**: The reaper publishes a copy of `monitoredSources` at the end of every
  `LoadSources`, the emergency pause included, and returns it from `MonitoredSources()
  sources.SourceConns`. Before the first `LoadSources` it returns nil.
- **REQ-003**: `ReadierReaper` gains `MonitoredSources()`. `App` gains
  `MonitoredSources() sources.SourceConns`, which delegates as `App.Ready()` does and returns nil
  while the reaper does not exist.
- **CON-001**: The main loop keeps reading `monitoredSources` without a lock. The published copy
  is a separate slice behind an `atomic.Pointer`.
- **CON-002**: No new exported type. The type `reaper` stays private.
- **GUD-001**: A caller trusts `RuntimeInfo` only when `IsPostgresSource()` is true and
  `Version > 0`. A pooler `DbConn` holds the pooler's version, and an info never fetched holds
  zeros, so `IsInRecovery` is `false` in both cases.

---

## 4. Interfaces & Data Contracts

```go
// pkg/sources
type SourceConn interface {
 // ... unchanged
 GetSource() Source
 GetRuntimeInfo() RuntimeInfo
}

// pkg/reaper
type ReadierReaper interface {
 Reaper
 Readier
 MonitoredSources() sources.SourceConns
}

// pkg/app
func (a *App) MonitoredSources() sources.SourceConns
```

Use by an embedder:

```go
for _, c := range a.MonitoredSources() {
 s := c.GetSource() // Name, CustomTags
 if !c.IsPostgresSource() {
  continue
 }
 if ri := c.GetRuntimeInfo(); ri.Version > 0 {
  // ri.Version, ri.IsInRecovery
 }
}
```

---

## 5. Acceptance Criteria

- **AC-001**: After `LoadSources`, `MonitoredSources()` returns one entry per monitored source
  with the same names as `monitoredSources`.
- **AC-002**: `GetRuntimeInfo()` of a `DbConn` after `FetchRuntimeInfo` returns its `Version` and
  `IsInRecovery`; changing a map of the copy does not change the connection.
- **AC-003**: `GetRuntimeInfo()` of a `PromConn` returns `RuntimeInfo{}`.
- **AC-004**: `go test -race` passes with `MonitoredSources()` and `GetRuntimeInfo()` called
  from another goroutine while the reaper loops.

---

## 6. Test Automation Strategy

Unit tests in `pkg/sources` for AC-002 and AC-003, and in `pkg/reaper` for AC-001 and AC-004
with the existing mock sources reader.

---

## 7. Rationale & Context

### Why the whole list and not one source by name?

An embedder groups sources by tag, version or role, so it needs all of them at once. A lookup
by name still needs the list of names.

### Why return `SourceConns` and not a new struct?

`Source`, `SourceConn`, `DbConn` and `PromConn` already exist. `GetSource()` and
`GetRuntimeInfo()` give the plain values. The entries are the live connections, so an
embedder can reach the pool through a cast; it runs in the same process and is trusted.

### Why no on-demand refresh of the role?

`IsInRecovery` selects the standby metric intervals (`conn.go:118`, `:226`). A refresh started
by the embedder would change what the reaper collects. A role that is up to 5 minutes late after
a failover is accepted; a faster refresh belongs in the reaper, for both.

### Blocking

`GetRuntimeInfo()` waits while `FetchRuntimeInfo` holds the write lock, which is at most the
runtime info timeouts of its four statements.

---

## 8. Dependencies & External Integrations

- `pkg/sources/conn.go`, `pkg/reaper/reaper.go`, `pkg/app/app.go`.

**Origin**: pgwatch Enterprise Edition reads version and role of every source from the sink on
each request to group sources; with this change it reads them from memory.
