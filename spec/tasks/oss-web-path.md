---

description: "Task list for accepting the web base path with or without slashes"
---

# Tasks: The web base path is accepted with or without slashes

**Input**: `spec/oss-web-path.md`
**Prerequisites**: branch `v7`

**Tests**: The specification requires tests (AC-001 to AC-003, section 6). Each story writes its tests first.

**Organization**: Tasks are grouped by user story. Each story can be implemented and tested on its own.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: The user story this task belongs to (US1, US2, US3)

## Stories

| Story | Priority | Requirements | Acceptance |
|---|---|---|---|
| US1: the four spellings of `pgwatch`, and `/`, give the same routes and `BasePath` | P1 | REQ-001, REQ-002, CON-001 | AC-001, AC-002 |
| US2: a malformed value fails at startup with an error naming the option | P2 | REQ-003 | AC-003 |
| US3: the option help and the docs name the accepted forms | P3 | GUD-001 | none |

---

## Phase 1: Setup

No setup. The change is string handling inside the existing `pkg/webserver` package.

---

## Phase 2: Foundational (blocking prerequisites)

**Purpose**: One function that turns the option value into the base path. US1 and US2 both call it.

- [x] T001 Add `normalizeBasePath(p string) (string, error)` in `pkg/webserver/webserver.go`. It trims every leading and trailing `/` with `strings.Trim(p, "/")` and returns the result. For now it never returns an error; US2 adds validation.
- [x] T002 Call `normalizeBasePath` at the top of `Init` in `pkg/webserver/webserver.go` and store the result back into `opts.WebBasePath` before `s` is built. `s.CmdOpts` then holds the normalized value, so the route prefix (`webserver.go:75-78`), the template `BasePath` (`webserver.go:133`), the prefix `handleStatic` strips (`webserver.go:162`) and the `basePath` passed to the route hook (`webserver.go:97`) all read one value. Normalizing in `Init` rather than in `cmdopts.ValidateConfig` also covers embedders that build `CmdOpts` themselves.

**Checkpoint**: Existing tests in `pkg/webserver` pass unchanged.

---

## Phase 3: User Story 1, every spelling gives the same routes (Priority: P1), MVP

**Goal**: `pgwatch`, `/pgwatch`, `pgwatch/` and `/pgwatch/` register routes under `/pgwatch/`; `/` and the empty value register under `/`.

**Independent test**: `go test ./pkg/webserver -run 'BasePath'` passes, and `pgwatch --web-base-path=/pgwatch` answers `200` on `GET /pgwatch/liveness`.

### Tests for User Story 1

> Write these first and confirm they fail against the current code.

- [x] T003 [P] [US1] Table test `TestNormalizeBasePath` in `pkg/webserver/webserver_test.go` over the six values of spec section 4. It asserts the base path and the route prefix built from it (`/` for empty, `/pgwatch/` otherwise).
- [x] T004 [P] [US1] Test `TestBasePathSpellings` in `pkg/webserver/routes_test.go`, following `TestWithRoutes`. For each of the four spellings of `pgwatch` it calls `webserver.Init` and asserts `GET /pgwatch/liveness` answers `200` (AC-001). Use a distinct `WebAddr` per case or `:0`.
- [x] T005 [US1] In the same test, assert that `GET /pgwatch/` returns an `index.html` whose rendered `BasePath` is `pgwatch` for all four spellings (AC-002). Use a test UI provider whose `index.html` prints `{{.BasePath}}`.
- [x] T006 [US1] In the same test, assert that a route registered through `webserver.WithRoutes` receives `basePath == "/pgwatch/"` for all four spellings.

### Implementation for User Story 1

- [x] T007 [US1] Replace the prefix construction at `pkg/webserver/webserver.go:75-78` so it reads the normalized `opts.WebBasePath` from T002. Keep the current form: `"/"` plus the base path plus `"/"` when the base path is not empty.
- [x] T008 [US1] Confirm `handleStatic` (`pkg/webserver/webserver.go:162`) strips `/pgwatch` for every spelling, and that `TestServer_handleStatic` still passes with an empty base path (CON-001).

**Checkpoint**: T003 to T006 pass. `--web-base-path=pgwatch` gives the same routes and `BasePath` as before.

---

## Phase 4: User Story 2, malformed values fail at startup (Priority: P2)

**Goal**: A value with an empty inner segment (`a//b`) or a character outside the URL path set stops the server at startup with an error naming `--web-base-path` and the value.

**Independent test**: `pgwatch --web-base-path=a//b` exits non-zero and prints a message containing `--web-base-path` and `a//b`.

### Tests for User Story 2

- [x] T009 [P] [US2] Extend `TestNormalizeBasePath` in `pkg/webserver/webserver_test.go` with error cases: `a//b`, `/a//b/`, `pg watch`, `pg?watch`, `pg#watch`. Assert the error text contains `--web-base-path` and the original value (AC-003).
- [x] T010 [P] [US2] Test in `pkg/webserver/routes_test.go` that `webserver.Init` with `WebBasePath: "a//b"` returns an error and does not start listening.

### Implementation for User Story 2

- [x] T011 [US2] In `normalizeBasePath`, after trimming, split on `/` and reject any empty segment. Reject any segment that contains a character outside RFC 3986 `pchar` (unreserved, `%`-encoded, sub-delims, `:`, `@`). Return `fmt.Errorf("invalid --web-base-path %q: ...", p)` with the original, untrimmed value.
- [x] T012 [US2] Make `Init` return the error from T002 before it builds the mux, so nothing is registered or bound on failure.

**Checkpoint**: T009 and T010 pass. US1 tests still pass.

---

## Phase 5: User Story 3, help text and docs (Priority: P3)

**Goal**: The option help and the reference docs name the accepted forms.

**Independent test**: `pgwatch --help` shows the new description.

- [x] T013 [P] [US3] Change the `description` tag of `WebBasePath` in `pkg/webserver/cmdopts.go:16` to `Base path for web UI and API endpoints ('pgwatch' or '/pgwatch/')` (GUD-001).
- [x] T014 [P] [US3] Update `docs/reference/cli_env.md:180-185` to the new description. Its example `--web-base-path=/pgwatch` is broken today and works after US1, so keep it.
- [x] T015 [P] [US3] In `docs/howto/reverse_proxy.md:13-27`, state that leading and trailing slashes are ignored.
- [x] T016 [P] [US3] In `docs/developer/embedding.md:67-68`, state that the `basePath` passed to `WithRoutes` is always `/` or `/<base path>/`, whatever spelling the operator used.

**Checkpoint**: All stories done.

---

## Phase 6: Polish

- [x] T017 Run `go test ./pkg/webserver/... ./pkg/app/...` and `golangci-lint run ./pkg/webserver/...`.
- [x] T018 Manual check: run `pgwatch --web-base-path=/pgwatch/ --web-addr=:8080` behind no proxy and open `http://localhost:8080/pgwatch/`. The UI loads and its API calls go to `/pgwatch/...`.

---

## Dependencies and execution order

### Phase dependencies

- Phase 2 (T001, T002) blocks US1 and US2.
- US1 and US2 both touch `normalizeBasePath`; do US1 first, then US2 extends it.
- US3 has no code dependency and can run at any time, but its doc text describes US1 behaviour, so merge it with or after US1.
- Phase 6 runs after the stories you ship.

### Within each story

- Tests first, and they must fail before the implementation.
- T005 and T006 extend the test added in T004, so they run after it.
- T012 depends on T011.

### Parallel opportunities

- T003 and T004 touch different files.
- T009 and T010 touch different files.
- T013 to T016 touch different files.

---

## Implementation strategy

1. T001 and T002.
2. US1 (T003 to T008). This alone fixes the silent `404` for `/pgwatch`, `pgwatch/` and `/pgwatch/`, and can ship.
3. US2 (T009 to T012).
4. US3 (T013 to T016).
5. Phase 6.

---

## Notes

- Commit after each story.
- The UI's handling of the injected `BasePath` is out of scope (spec section 1). AC-002 checks only the rendered `index.html`.
