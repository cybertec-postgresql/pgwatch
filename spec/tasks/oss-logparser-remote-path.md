---
description: "Task list for: The remote log parser builds server paths with forward slashes"
---

# Tasks: Remote log parser server paths

**Input**: `spec/oss-logparser-remote-path.md` (v1.0, 2026-10-16)
**Baseline**: `v7` at `7725207aac`; line numbers are indicative.

**Tests**: INCLUDED — the spec defines a test automation strategy (§6) and acceptance criteria (§5).

**Organization**: One user story; the spec has no user-story section, so it is derived from §1.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: US1 as defined below
- Requirement IDs (REQ/CON/GUD/AC) from the spec are cited on each task

### User stories

| Story | Title | Priority | Delivers |
|---|---|---|---|
| US1 | pgwatch on Windows parses a Linux server's log remotely | P1 (MVP) | REQ-001, REQ-002, CON-001, GUD-001, AC-001, AC-002 |

### Server path call sites (REQ-002)

| Call site | Path sent to the server | Status at baseline |
|---|---|---|
| `pkg/reaper/logparser.go:165` `checkHasRemotePrivileges` | `pg_read_file($1, 0, 0)` argument | **bug**: `filepath.Join` |
| `pkg/reaper/logparser_stream.go:283` `remoteFileSizes` | offset map key, compared with pgremote's paths | correct: `filepath.ToSlash(filepath.Join(...))` |
| `pkg/reaper/logparser_stream.go:289` `remoteFileSizes` lookup | path handed back by pgremote | correct: already `/` |
| `pkg/reaper/logparser_stream.go:164` `openRemote` | `pgremote.Config.Dir`; pgremote joins with `path.Join` (`joinRemote`) | correct |
| `pkg/reaper/logparser_stream.go:149` `openLocal` | local mode | out of scope (CON-001) |

---

## Phase 1: Setup

No setup: no new dependency, option or generated file.

---

## Phase 2: Foundational

**Purpose**: The single helper every server path goes through (GUD-001).

- [x] T001 [US1] Add `serverPath(dir, name string) string` built on `path.Join` in `pkg/reaper/logparser.go` (GUD-001, REQ-001)

**Checkpoint**: The helper's signature is fixed; tests (T003, T004) code against it.

---

## Phase 3: User Story 1 — pgwatch on Windows parses a Linux server's log remotely (Priority: P1) 🎯 MVP

**Goal**: Every path the remote parser sends to the server holds only `/` on every host OS.

**Independent Test**: `go test -p 1 ./pkg/reaper -run 'Remote|LogParse'` passes on Windows and Linux;
pgwatch on Windows writes `server_log_event_counts` for PostgreSQL in a Linux container.

### Tests for User Story 1

- [x] T003 [P] [US1] Unit test in `pkg/reaper/logparser_test.go`: with `pgxmock`, `checkHasRemotePrivileges` for `log_directory = /var/lib/postgresql/18/docker/log` sends exactly `/var/lib/postgresql/18/docker/log/postgresql-2026-10-16_203849.log` (no `\`) to `pg_read_file($1, 0, 0)` (AC-001)
- [x] T004 [P] [US1] In `pkg/reaper/logparser_stream_test.go` and the existing remote tests in `pkg/reaper/logparser_test.go`, the mocks expect the probe argument as `path.Join`, not `filepath.Join`, so it asserts the server path on Windows too (AC-001)

### Implementation for User Story 1

- [x] T002 [US1] Use `serverPath` in `checkHasRemotePrivileges` (`pkg/reaper/logparser.go:165`) and in `remoteFileSizes` (`pkg/reaper/logparser_stream.go:283`); drop the `path/filepath` imports that become unused; leave local mode on `filepath` (REQ-001, REQ-002, CON-001; depends on T001)
- [x] T005 [US1] Manual check on this Windows host: pgwatch against PostgreSQL 18 in a Linux container with `logging_collector = on`, `log_destination = jsonlog`, connected over TCP so remote mode is chosen; confirm the log no longer says "lacking required privileges" and `server_log_event_counts` is written (AC-002; depends on T002)

**Checkpoint**: US1 is complete; AC-001 and AC-002 hold.

---

## Phase N: Polish

- [x] T006 `task lint` and `go test -p 1 ./pkg/reaper` pass; the PR body lists the REQ-002 call sites above (depends on T002–T005)

---

## Dependencies & Execution Order

- T001 → T002 → T005 → T006
- T003, T004 run in parallel with T001/T002 (different files, code against the fixed `serverPath` contract); they must fail against the baseline on Windows and pass after T002.

## Parallel Example

```text
Task: "T001+T002 helper and call sites in logparser.go, logparser_stream.go"
Task: "T003 probe-path unit test in logparser_test.go"
Task: "T004 probe-path expectation in logparser_stream_test.go"
```
