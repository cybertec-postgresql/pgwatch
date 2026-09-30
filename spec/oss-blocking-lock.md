---
title: "blocking_locks reports the wait time and the real blockers"
version: 1.0
date_created: 2026-09-30
date_updated: 2026-09-30
owner: pgwatch maintainers
status: draft
tags: [metrics, locks, blocking]
---

# Introduction

The metric `blocking_locks` answers "who waits for whom" on a monitored database. Each row pairs
a session that waits for a lock with a session that holds one. Two facts about the pair are
missing or wrong today.

The metric has no wait time. It carries the statement texts of both sessions but no column that
says how long the waiting session has waited, so a consumer cannot tell a wait of 50 ms from
one of ten minutes.

The metric guesses the blocker. It joins every lock that is not granted to every granted lock on
the same relation or the same transaction id, whatever the two modes are. A session that holds
a lock compatible with the waiting request is reported as a blocker, and a wait on a lock type
the join does not match (advisory, virtual transaction id, object) yields no row at all.
PostgreSQL computes the exact answer in `pg_blocking_pids()`, which the `backends` metric already
calls.

This specification adds a wait time per waiting session and takes the blockers from
`pg_blocking_pids()`. The column names and tags that exist today keep their names and meaning.

Code references are against branch `v7`. Line numbers are indicative, not contractual.

---

## 1. Purpose & Scope

**Purpose**: every `blocking_locks` row names a session that PostgreSQL itself reports as
blocking the waiting session, and says how long the waiting session has waited.

**In scope**: the SQL of `blocking_locks` in `pkg/metrics/metrics.yaml` (`:261-319`) for
PostgreSQL 14 and newer, its column list and its description.

**Out of scope**: presets and their intervals; the `backends` and `locks` metrics; a lock graph
or root blocker computed in pgwatch; the Grafana dashboards beyond keeping the existing panels
working.

**Audience**: pgwatch maintainers; operators and dashboard authors who read `blocking_locks`.

**Assumptions**: the SQL key stays `14`; `pg_locks.waitstart` and `pg_blocking_pids()` exist on
every server version the metric supports.

---

## 2. Definitions

| Term | Definition |
|---|---|
| **Waiting session** | A backend with a row in `pg_locks` whose `granted` is false. |
| **Blocker** | A PID that `pg_blocking_pids(<waiting pid>)` returns: a session that holds a conflicting lock, or that waits ahead in the queue for a conflicting one. |
| **Pair** | One waiting session and one of its blockers. |
| **Wait time** | Seconds since `pg_locks.waitstart` of the waiting lock. |

---

## 3. Requirements, Constraints & Guidelines

- **REQ-001**: The metric returns one row per pair. A blocker that holds several locks on the
  object the waiting session waits for still gives one row.
- **REQ-002**: Blockers come from `pg_blocking_pids(waiting.pid)`, called once per waiting
  session. The join of today on relation or transaction id (`metrics.yaml:296-304`) is removed.
- **REQ-003**: A new column `waiting_seconds` (float, gauge) holds
  `extract(epoch from now() - waiting.waitstart)`, rounded to three decimals. It is null when
  `waitstart` is null, which PostgreSQL allows for a short time after the wait starts.
- **REQ-004**: `other_mode`, `other_locktype` and `other_table` describe a lock of the blocker on
  the object the waiting session waits for: a granted one when the blocker holds one, otherwise
  the lock it waits for ahead in the queue. When no such lock row exists (the blocker holds a
  lock on a different object of a lock group, or the row vanished between the two reads), the
  three columns are null and the row is still returned.
- **REQ-005**: Every column and tag of today keeps its name, type and meaning:
  `tag_waiting_locktype`, `tag_waiting_user`, `tag_waiting_mode`, `tag_waiting_table`,
  `waiting_query`, `waiting_pid`, `other_locktype`, `other_table`, `other_query`, `other_mode`,
  `other_pid`, `other_user` (`metrics.yaml:277-288`).
- **REQ-006**: The filters of today stay: sessions of the current database only, no autovacuum
  statements, not the collector's own backend (`metrics.yaml:269-274`, `:317`).
- **CON-001**: The query returns no row when no session waits, as today, so a consumer that
  treats a missing sample as "nothing blocked" keeps working.
- **CON-002**: `pg_blocking_pids()` takes the lock manager's partition locks for a short time.
  The query calls it only for waiting sessions, never for every backend.
- **GUD-001**: The description in `metrics.yaml` names `waiting_seconds` and states that blockers
  come from `pg_blocking_pids()`.

---

## 4. Interfaces & Data Contracts

Columns of `blocking_locks` after this change:

| Column | Kind | Source | Change |
|---|---|---|---|
| `tag_waiting_locktype`, `tag_waiting_user`, `tag_waiting_mode`, `tag_waiting_table` | tag | waiting lock and session | none |
| `waiting_query`, `waiting_pid` | gauge | waiting session | none |
| `waiting_seconds` | gauge | `now() - waiting.waitstart` | new |
| `other_pid` | gauge | `unnest(pg_blocking_pids(waiting.pid))` | source changes |
| `other_user`, `other_query` | gauge | blocker session | none |
| `other_locktype`, `other_table`, `other_mode` | gauge | blocker's lock on the same object (REQ-004) | source changes; may be null |

Rows for three waits, today and after the change:

| Situation | Today | After |
|---|---|---|
| B updates a row that A updated in an open transaction | A for B | A for B, with `waiting_seconds` |
| B runs `CREATE INDEX` on `t`; A holds `AccessShareLock` on `t`, C holds `RowExclusiveLock` on `t` | A and C for B | C for B only: `ShareLock` does not conflict with `AccessShareLock` |
| B calls `pg_advisory_lock(1)` that A holds | no row | one row A for B |

---

## 5. Acceptance Criteria

- **AC-001**: Given session A with an open transaction that updated row 1 of `t`, when session B
  updates row 1 and waits two seconds, then the metric returns exactly one row with
  `waiting_pid` B, `other_pid` A and `waiting_seconds` of at least 2.
- **AC-002**: Given A holding `AccessShareLock` and C holding `RowExclusiveLock` on `t`, when B
  waits in `CREATE INDEX ON t`, then the rows name C and not A.
- **AC-003**: Given A holding `pg_advisory_lock(1)`, when B waits in `pg_advisory_lock(1)`, then
  one row names A as `other_pid` with `other_locktype` `advisory`.
- **AC-004**: Given A holding two granted locks on the object B waits for, then one row pairs B
  and A.
- **AC-005**: Given no waiting session, then the metric returns no row.
- **AC-006**: The columns of REQ-005 exist with the same names and types; a dashboard query of
  today over them still runs.

---

## 6. Test Automation Strategy

An integration test in `pkg/metrics` starts PostgreSQL with
`internal/testutil.SetupPostgresContainer` (`internal/testutil/setup.go:24`), opens the sessions
of AC-001 to AC-005 on separate connections, runs the metric SQL from `metrics.yaml` and asserts
the rows. AC-006 is a unit test over the parsed column list. No mock can show what
`pg_blocking_pids()` returns, so the test needs a real server.

---

## 7. Rationale & Context

### Why `pg_blocking_pids()` instead of a better join?

Lock conflicts depend on the pair of modes, the lock type, the wait queue order and parallel
query lock groups. PostgreSQL encodes all of it in `pg_blocking_pids()`; a join in SQL would
have to repeat the conflict table of the documentation and still miss queue order. The
`backends` metric already relies on the function for its `blocked` count (`metrics.yaml:100`),
so the two metrics then agree on who is blocked.

### Why `waitstart` instead of `query_start`?

`query_start` is when the statement began, which for a transaction that ran for minutes before
it hit the lock overstates the wait. `pg_locks.waitstart` is when the backend started waiting for
this lock, which is the number an operator asks for. It exists since PostgreSQL 14, the oldest
key of this metric's SQL.

### Why one row per pair?

A holder with several granted locks on the same object gives several rows today, and a consumer
has to deduplicate before it can count blockers. One row per pair makes a row count a pair
count.

---

## 8. Dependencies & External Integrations

- `pkg/metrics/metrics.yaml:261-319` (`blocking_locks`), `:100` (`backends.blocked` with
  `pg_blocking_pids()`).
- PostgreSQL `pg_locks.waitstart` and `pg_blocking_pids()`, both available from version 14.
- `internal/testutil/setup.go:24` for the integration test.

