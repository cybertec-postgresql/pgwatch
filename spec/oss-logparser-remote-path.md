---
title: "The remote log parser builds server paths with forward slashes"
version: 1.0
date_created: 2026-10-16
date_updated: 2026-10-16
owner: pgwatch maintainers
status: draft
tags: [reaper, logparser, windows]
---

# Introduction

When pgwatch runs on another host than the monitored server, the log parser reads the server
log over SQL with `pg_read_file()`. Before it starts, `checkHasRemotePrivileges` probes one log
file with `pg_read_file($1, 0, 0)`, and builds the path with `filepath.Join(logsDirPath,
logFile)`. On a Windows host `filepath.Join` writes backslashes, so a Linux server receives
`\var\lib\postgresql\18\docker\log\postgresql-….log`, reads it as a relative path under its data
directory, and answers `58P01` (no such file). `parseLogs` then gives up with "couldn't parse
logs remotely, lacking required privileges", and `server_log_event_counts` is never sent.

The path is a path on the database server, not on the host pgwatch runs on. The streaming
parser already treats it so: `logparser_stream.go:283` writes
`filepath.ToSlash(filepath.Join(lp.Directory, name))`. This specification makes the privilege
probe do the same.

Code references are against commit `497861b1aa` of branch `v7`. Line numbers are indicative, not
contractual.

---

## 1. Purpose & Scope

**Purpose**: pgwatch on Windows parses the log of a PostgreSQL server on Linux or in a container
remotely, as it does on Linux and macOS.

**In scope**: every path pgwatch sends to the server in the remote log parser
(`pkg/reaper/logparser.go`, `pkg/reaper/logparser_stream.go`).

**Out of scope**: local parsing (pgwatch and the server on one host), where the host's path
rules are right; a Windows database server.

**Audience**: pgwatch maintainers; users who run pgwatch on Windows.

**Assumptions**: the monitored server uses `/` as its path separator. A Windows server accepts
`/` as well.

---

## 2. Definitions

| Term | Definition |
|---|---|
| **Server path** | A path on the database server's file system, passed to `pg_ls_logdir()` or `pg_read_file()`. |
| **Remote mode** | The log parser mode chosen when pgwatch cannot open the server's files itself (`logparser.go:103`). |

---

## 3. Requirements, Constraints & Guidelines

- **REQ-001**: `checkHasRemotePrivileges` builds the probed file's server path with `path.Join`
  (or `filepath.ToSlash` of `filepath.Join`, as the streaming parser does), so it holds only `/`
  on every host OS (`logparser.go:165`).
- **REQ-002**: Every other server path the remote parser builds follows REQ-001; the review
  lists each call site it checked.
- **CON-001**: Local mode keeps `filepath`: its paths are paths on the host pgwatch runs on.
- **GUD-001**: One helper builds server paths, so the next call site cannot pick the wrong
  package.

---

## 4. Interfaces & Data Contracts

No new option, metric or API. For `log_directory = /var/lib/postgresql/18/docker/log` and the
file `postgresql-2026-10-16_203849.log`:

| Host OS | Probed path today | Probed path after this change |
|---|---|---|
| Linux, macOS | `/var/lib/postgresql/18/docker/log/postgresql-2026-10-16_203849.log` | unchanged |
| Windows | `\var\lib\postgresql\18\docker\log\postgresql-2026-10-16_203849.log` | `/var/lib/postgresql/18/docker/log/postgresql-2026-10-16_203849.log` |

---

## 5. Acceptance Criteria

- **AC-001**: A unit test with a mocked connection asserts that the argument of the
  `pg_read_file($1, 0, 0)` probe holds no `\` for a `log_directory` with `/`, on every host OS
  the CI runs.
- **AC-002**: On a Windows host, pgwatch monitoring PostgreSQL in a Linux container with
  `log_destination = jsonlog` writes `server_log_event_counts`.

---

## 6. Test Automation Strategy

A unit test in `pkg/reaper` next to the existing log parser tests, with `pgxmock`. The OSS CI
runs Windows, which makes AC-001 meaningful there; AC-002 is a manual check, or an integration
test where the CI can run Linux containers on Windows.

---

## 7. Rationale & Context

### Why `path.Join` and not a server-side join?

The server could join the directory and the file name in SQL, but the parser already holds both
strings and the streaming parser already uses forward slashes. One separator is enough, and
PostgreSQL on Windows accepts `/` too.

### Why does it matter?

Running pgwatch on a Windows workstation against PostgreSQL in Docker Desktop is a common
development and evaluation setup. The host is always remote to the container there, so the log
metrics never fill, and the only trace is a misleading message about missing privileges.

---

## 8. Dependencies & External Integrations

- `pkg/reaper/logparser.go:103` (mode choice), `:117-119` (return on failed probe), `:165` (the
  probe).
- `pkg/reaper/logparser_stream.go:283` (the streaming parser's server path).
- `pkg/db/conn.go:118-121` (the local check that fails on a Windows host).
