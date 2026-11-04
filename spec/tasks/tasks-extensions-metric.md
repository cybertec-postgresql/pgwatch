---
description: "Task list for the extensions metric"
---

# Tasks: Extensions Metric

**Input**: the specification "A metric lists the installed extensions" (version 1.0, 2026-11-04)
**Prerequisites**: that specification (required for the requirement IDs referenced below)

**Tests**: INCLUDED. Specification section 6 requires an integration test and a preset test.

**Organization**: One user story. The metric definition, its preset entries and its tests ship together in one PR.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which story the task belongs to (US1)
- File paths are exact and relative to the repository root
- Bracketed IDs like **REQ-001** and **AC-001** refer to the specification

---

## Phase 1: Setup

**Purpose**: Confirm the build works before changing anything

- [x] T001 Run `task ui` if `internal/webui/embed/build/` is missing, then `go test ./pkg/metrics/...` to get a green baseline

---

## Phase 2: Foundational

No shared infrastructure is needed. `internal/testutil/setup.go` already provides `SetupPostgresContainer`.

---

## Phase 3: User Story 1 - Operators see installed extensions and versions (Priority: P1) 🎯 MVP

**Goal**: pgwatch records one row per installed extension with its installed and default version.

**Independent Test**: Collect `extensions` from a PostgreSQL container with `pg_stat_statements` installed and get a row for it (**AC-001**). On a database with only `plpgsql`, get exactly one row (**AC-002**).

### Tests for User Story 1

> Write these first and confirm they fail before T005.

- [x] T002 [P] [US1] Add `"extensions"` to `knownMetrics` in `pkg/metrics/metrics_yaml_test.go` so `TestAllKnownMetricsPresent` guards the new key
- [x] T003 [P] [US1] Add `TestExtensionsColumns` to `pkg/metrics/metrics_yaml_test.go`, modeled on `TestBackendsColumns`. Check that the SQL key `14` exists and that the select list has `epoch_ns`, `tag_extension`, `installed_version` and `default_version` (**REQ-001**). Check that `IsInstanceLevel` is false (**REQ-003**)
- [x] T004 [P] [US1] Add `TestDefaultPresetsListExtensions` to `pkg/metrics/default_test.go`. Assert that `exhaustive` and `full` list `extensions` at 3600, `debug` lists it, and `basic`, `standard` and `minimal` do not (**REQ-004**, **AC-003**)
- [x] T005 [US1] Add an integration test in `pkg/metrics/extensions_integration_test.go` using `testutil.SetupPostgresContainer()`. Run the `sqls[14]` text from the default metrics against the container. Assert that a fresh database returns exactly one row, `plpgsql` (**AC-002**). Run `create extension pg_stat_statements`, query again, and assert a row with `tag_extension = 'pg_stat_statements'` and a non-empty `installed_version` (**AC-001**). Assert that `default_version` is not null for it

### Implementation for User Story 1

- [x] T006 [US1] Add the `extensions` metric to `pkg/metrics/metrics.yaml` with the SQL and description from specification section 4, key `14`, no `init_sql`, no `node_status` (**REQ-001**, **REQ-002**, **REQ-003**, **CON-001**). Place it next to the other catalog metrics, keeping the file's existing ordering
- [x] T007 [US1] Add `extensions: 3600` to the `exhaustive` and `full` presets in `pkg/metrics/metrics.yaml` (**REQ-004**)
- [x] T008 [US1] Add `extensions: 30` to the `debug` preset in `pkg/metrics/metrics.yaml`, in alphabetical position (**REQ-004**)
- [x] T009 [US1] Run `go test -failfast -p 1 -timeout=300s ./pkg/metrics/...` and confirm T002 to T005 pass

**Checkpoint**: The metric is defined, in three presets, and verified against a real server.

---

## Phase 4: Polish

- [ ] T010 [P] Add `extensions` to the metric list in `docs/reference/metric_definitions.md` if that file enumerates built-in metrics. Skip it if it does not
- [ ] T011 [P] Optional: add a table panel for `extensions` to a v13 Grafana dashboard under `grafana/postgres/v13/` and the matching `grafana/prometheus/v13/` file. The specification marks this optional, so open a follow-up issue instead if it grows
- [ ] T012 Run `task check` (lint and test) as the pre-PR gate

---

## Dependencies and Execution Order

- Phase 1 has no dependencies.
- T002, T003 and T004 touch different test files or functions and can run in parallel. T002 and T003 both edit `pkg/metrics/metrics_yaml_test.go`, so land them in one commit to avoid conflicts.
- T005 needs Docker.
- T006 must precede T007 and T008, which edit the same file.
- T009 needs T006 to T008.
- T010 and T011 need T006.
- T012 runs last.

## Parallel Example

```bash
# Tests can be written together:
Task: "Add extensions to knownMetrics in pkg/metrics/metrics_yaml_test.go"
Task: "Add TestDefaultPresetsListExtensions in pkg/metrics/default_test.go"
Task: "Add integration test in pkg/metrics/extensions_integration_test.go"
```

## Implementation Strategy

1. Write T002 to T005 and see them fail.
2. Apply T006 to T008 in `metrics.yaml`.
3. Run T009, then the polish tasks, then `task check`.

## Notes

- Do not add AI attribution lines to commits or the PR (project memory rule).
- The metric uses SQL key `14`, so PostgreSQL 13 and older get no data. This follows the specification and needs no code.
- Origin: pgwatch EE feature F016 (config drift) depends on this metric being in the pin.
