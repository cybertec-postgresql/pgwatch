---
description: "Task list for: Public extension API — published engine packages and a pluggable web UI"
---

# Tasks: Public extension API (published engine packages + pluggable web UI)

**Input**: `spec/oss-published-packages.md` (v1.0, 2026-09-07)
**Baseline**: `master` at `v6.0.0-beta-56-gae677c8028`; line numbers are indicative.

**Tests**: INCLUDED — the spec defines a test automation strategy (§6) and acceptance criteria (§5).

**Organization**: Tasks are grouped by user story. Stories are derived from the spec's capabilities,
since the spec has no user-story section.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: US1–US5 as defined below
- Requirement IDs (REQ/CON/GUD/AC) from the spec are cited on each task

### User stories

| Story | Title | Priority | Delivers |
|---|---|---|---|
| US1 | Published engine packages, buildable from the module proxy | P1 (MVP) | REQ-001…003, 012…015, CON-001, GUD-001 |
| US2 | Provider-driven SPA routes and index template data | P2 | REQ-007, REQ-008, CON-002 |
| US3 | Route extension hook and configurable CORS origin | P3 | REQ-009…011 |
| US4 | `pkg/app` bootstrap | P4 | REQ-016, GUD-002 |
| US5 | `cmdopts` extensions (embedder flag groups and subcommands) | P5 | REQ-018 |

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: metadata corrections and tooling needed by the relocation

- [x] T001 [P] Correct license metadata to `BSD-3-Clause`: `.goreleaser.yml:58` (`license: MIT Licence`) and `internal/webui/package.json:6` (`"license": "MIT"`) — REQ-017
- [x] T002 [P] Create `pkg/doc.go` declaring `pkg/*` as public API and pointing at the stability policy — REQ-001
- [x] T003 [P] Add a `rewrite-imports` target to `Taskfile.yml` that rewrites `github.com/cybertec-postgresql/pgwatch/v6/internal/<pkg>` → `.../pkg/<pkg>` across the tree, backed by `tools/rewriteimports` (consumed by Phase 3; delete both once the relocation lands)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: get the React `//go:embed` out of `webserver` behind a `ui.Provider`. Until this is
done, `webserver` cannot be published (REQ-013) and no route/UI extension point has a home.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [ ] T004 Create `pkg/ui/ui.go` with the `Provider` interface — `FS() fs.FS`, `SPARoutes() []string`, `IndexData() map[string]any` — REQ-004, §4.1
- [ ] T005 Redirect the Vite build output at the default provider: `internal/webui/vite.config.ts:48` `outDir: '../webserver/build'` → `'../webui/embed/build'`; update the artifact path in `.github/workflows/build.yml:95` and any `Taskfile.yml`/`docker/` reference to `internal/webserver/build` — REQ-006
- [ ] T006 Create `internal/webui/embed/embed.go`: `//go:embed build`, `Provider() ui.Provider` with `FS: fs.Sub(buildFS, "build")`, `SPARoutes: "/", "/sources", "/metrics", "/presets", "/logs"`, `IndexData: nil` — REQ-006, §4.4
- [ ] T007 Delete `//go:embed build`, `buildFS`, `uiFS` and `init()` from `internal/webserver/webserver.go:26-33`; add a `uiProvider ui.Provider` field to `WebUIServer` — REQ-005, REQ-013
- [ ] T008 Add functional options to `webserver.Init` — `type Option func(*WebUIServer)`, variadic `options ...Option`, and `WithUI(p ui.Provider) Option`; `Init` MUST fail with a clear error when no provider is configured and `WebDisable != WebDisableUI` — REQ-005, §4.2
- [ ] T009 Make `prepareIndexHTML` (`internal/webserver/webserver.go:105-120`) and `handleStatic` (`:122-165`) read from `s.uiProvider.FS()` instead of the package-level `uiFS` — REQ-005
- [ ] T010 Pass `embed.Provider()` at the `webserver.Init` call in `cmd/pgwatch/main.go:104` — REQ-006
- [ ] T011 Update `internal/webserver/webserver_test.go` and `server_test.go` to inject a fake provider (they currently depend on the package-level `uiFS`)

**Checkpoint**: binary behaviour unchanged; `webserver` no longer embeds React assets

---

## Phase 3: User Story 1 - Published engine packages (Priority: P1) 🎯 MVP

**Goal**: an external Go module can `go get` pgwatch and import the engine packages, and the module
builds straight from the Go module proxy.

**Independent Test**: from an empty directory, `go build github.com/cybertec-postgresql/pgwatch/v6/cmd/pgwatch@<tag>`
succeeds, and a scratch module importing `pkg/reaper` + `pkg/cmdopts` compiles (AC-005).

### Tests for User Story 1

- [ ] T012 [P] [US1] Add a generated-code freshness step to `.github/workflows/build.yml` after `go generate ./api/pb/`: `git diff --exit-code` — REQ-012, AC-004
- [ ] T013 [P] [US1] Add a CI job that builds `cmd/pgwatch` from a scratch module directory with `GOFLAGS=-mod=mod` and `GOWORK=off` — AC-005, §6

### Implementation for User Story 1

- [ ] T014 [US1] Remove both `*.pb.go` ignore rules from `.gitignore` (under "# Protobuf files" and "# Generated protobuf files") and commit `api/pb/pgwatch.pb.go` and `api/pb/pgwatch_grpc.pb.go` — REQ-012
- [ ] T015 [US1] Relocate the leaf packages: `git mv internal/log pkg/log`, `git mv internal/db pkg/db`; run `task rewrite-imports` — REQ-001, REQ-002
- [ ] T016 [US1] Relocate `internal/metrics` → `pkg/metrics`, keeping the embedded `metrics.yaml` alongside it — REQ-001, §4.1
- [ ] T017 [US1] Relocate `internal/sources` → `pkg/sources` — REQ-001
- [ ] T018 [US1] Relocate `internal/sinks` → `pkg/sinks`, keeping the embedded `sql/*.sql` alongside it — REQ-001, §4.1
- [ ] T019 [US1] Relocate `internal/cmdopts` → `pkg/cmdopts` — REQ-001
- [ ] T020 [US1] Relocate `internal/reaper` → `pkg/reaper` — REQ-001
- [ ] T021 [US1] Relocate `internal/webserver` → `pkg/webserver` (already embed-free after Phase 2) — REQ-001, REQ-013
- [ ] T022 [US1] Fix up the remaining import sites — `cmd/pgwatch/*.go`, `internal/webui/embed`, `internal/testutil`, `api/`, `contrib/` — then `go mod tidy`; confirm `internal/` holds only `testutil` and `webui` — CON-001, GUD-001
- [ ] T023 [US1] Move `configSchema`/`sinkSchema` (`cmd/pgwatch/version.go:10-11`) into `pkg/cmdopts` as exported `ConfigSchema`/`SinkSchema` next to `NeedsSchemaUpgrade`, and make `printVersion` use them — REQ-015, AC-007
- [ ] T024 [US1] Write the stability policy in `docs/developer/` (Go compatibility promise within a major version for `pkg/*`; `// Experimental:` marks the exceptions) and add it to `mkdocs.yml` nav — REQ-003
- [ ] T025 [US1] Annotate not-yet-stable exported symbols in `pkg/*` with `// Experimental:` doc comments — REQ-003
- [ ] T026 [US1] Update every path reference to the moved packages in `.goreleaser.yml`, `Taskfile.yml`, `docker/`, `.github/workflows/*.yml` and `docs/`
- [ ] T027 [US1] Confirm `go test ./...` passes with no test changes beyond import paths — AC-001

**Checkpoint**: engine packages are importable, module builds from the proxy, binary unchanged

---

## Phase 4: User Story 2 - Provider-driven SPA routes and index data (Priority: P2)

**Goal**: an embedder's provider fully controls the client-side route set and the index template
data, without touching pgwatch.

**Independent Test**: a fake provider returning `["*"]` and `IndexData{"Foo":"bar"}` gets its routes
answered with `index.html` and its data rendered, while `BasePath` still comes from the server.

### Tests for User Story 2

- [ ] T028 [P] [US2] Table-driven test in `pkg/webserver` for `SPARoutes()` fallback, including the `"*"` wildcard (any extension-less path → `index.html`) — REQ-008, §6
- [ ] T029 [P] [US2] Test that `IndexData()` is merged into the template data and that a provider key named `BasePath` does not override the server's value — REQ-007, §6
- [ ] T030 [P] [US2] Test `--web-disable=ui` (REST API served, no provider needed) and `--web-disable=all` (`Init` returns `nil, nil`) — CON-002, AC-006

### Implementation for User Story 2

- [ ] T031 [US2] Widen `prepareIndexHTML` template data to `map[string]any` and merge `Provider.IndexData()` into it, with `BasePath` written last — REQ-007
- [ ] T032 [US2] Replace the hardcoded route slice in `handleStatic` (`webserver.go:133`) with `Provider.SPARoutes()`, treating a single `"*"` entry as "every path without a file extension" — REQ-008
- [ ] T033 [US2] Ensure the `WebDisableUI` branch skips the provider entirely so `Init` succeeds with none configured — CON-002

**Checkpoint**: US1 and US2 both work; the default provider reproduces today's UI behaviour exactly

---

## Phase 5: User Story 3 - Route extension hook and CORS origin (Priority: P3)

**Goal**: an embedder registers extra HTTP routes under the same base path, behind the same JWT
check, and can point CORS at its own dev origin.

**Independent Test**: a hook-registered `/hello` returns 401 without a token and 200 with a valid
one; `--web-cors-origin=https://example.test` is reflected in `Access-Control-Allow-Origin`.

### Tests for User Story 3

- [ ] T034 [P] [US3] Test a hook-registered route wrapped by `auth`: 401 without a token, 200 with a valid JWT — REQ-010, §6
- [ ] T035 [P] [US3] Test that hook routes are mounted under `basePath` and cannot shadow built-in routes (registering `source` from the hook leaves the built-in handler in place) — REQ-009
- [ ] T036 [P] [US3] Test `WithCORSOrigin` and the `--web-cors-origin` flag, asserting the default is still `http://localhost:4000` — REQ-011

### Implementation for User Story 3

- [ ] T037 [US3] Add `WithRoutes(fn func(mux *http.ServeMux, basePath string, auth func(http.HandlerFunc) http.Handler)) Option`, invoked after the built-in routes (`webserver.go:77-87`) and before `mux.HandleFunc(s.basePath, s.handleStatic)` — REQ-009, §4.2
- [ ] T038 [US3] Pass `NewEnsureAuth` (`pkg/webserver/jwt.go:71`) as the hook's `auth` argument so embedder routes share login/token/expiry semantics — REQ-010
- [ ] T039 [US3] Add `WithCORSOrigin(origin string) Option` and make `corsMiddleware` (`webserver.go:213`) use the configured origin instead of the hardcoded literal — REQ-011
- [ ] T040 [US3] Add `--web-cors-origin` / `PW_WEBCORSORIGIN` to `pkg/webserver/cmdopts.go` with default `http://localhost:4000`, and wire it into `Init` — REQ-011

**Checkpoint**: US1–US3 independently functional; `pgwatch --help` gains exactly one flag

---

## Phase 6: User Story 4 - `pkg/app` bootstrap (Priority: P4)

**Goal**: embedders get pgwatch's whole wiring sequence — logger, config readers, sink writer,
schema check, reaper, web server, `Reap` — including signal handling and exit codes.

**Independent Test**: a `main` calling `app.New(ctx, opts, app.WithUI(p))` then `a.Run(ctx)` starts a
collector and returns the documented exit codes.

### Tests for User Story 4

- [ ] T041 [P] [US4] Test that `Run` returns the same exit codes as today for each failure path: `ExitCodeOK`, `ExitCodeConfigError`, `ExitCodeUpgradeError`, `ExitCodeWebUIError`, `ExitCodeUserCancel`, `ExitCodeFatalError` — §4.3

### Implementation for User Story 4

- [ ] T042 [US4] Create `pkg/app` with `App`, `Option`, `New(ctx, opts, ...Option)`, `Run(ctx) int`, `Options()`, `Logger()`, `Ready()` per §4.3, wrapping `cmd/pgwatch/main.go:59-113` — REQ-016
- [ ] T043 [US4] Move signal handling (`setupCloseHandler`), panic recovery and exit-code mapping from `cmd/pgwatch/main.go:20-55` into `pkg/app` — GUD-002
- [ ] T044 [US4] Add `app.WithUI(ui.Provider)` and `app.WithRoutes(...)`, forwarded to `webserver.Init` — REQ-016
- [ ] T045 [US4] Reduce `cmd/pgwatch/main.go` to the §4.6 sketch, keeping `printVersion` and subcommand handling in `cmd/pgwatch`

**Checkpoint**: the binary and any embedder share one wiring sequence

---

## Phase 7: User Story 5 - `cmdopts` extensions (Priority: P5)

**Goal**: embedders add their own flag groups and subcommands before parsing, without forking
`cmdopts`.

**Independent Test**: an extension registering a `--ee-*` group and an `ee` subcommand is parsed;
`pgwatch --help` (no extension) is byte-identical to today.

### Tests for User Story 5

- [ ] T046 [P] [US5] Test that an `Extension` adding a `go-flags` group and a subcommand is parsed, and that `New(out)` with no extension produces unchanged `--help` output — REQ-018, AC-002

### Implementation for User Story 5

- [ ] T047 [US5] Add the `Extension` interface (`Register(parser *flags.Parser, opts *Options) error`) and change `New` to `New(out io.Writer, exts ...Extension)` in `pkg/cmdopts/cmdoptions.go:70-76`, calling `Register` after `addCommands` and before `parser.Parse()` — REQ-018, §4.5
- [ ] T048 [US5] Verify every `cmdopts.New` caller (`cmd/pgwatch`, `pkg/app`, tests) compiles unchanged

**Checkpoint**: all user stories independently functional

---

## Phase 8: Polish & Cross-Cutting Concerns

- [ ] T049 [P] Write the example embedder `docs/howto/embedding/main.go`: a trivial `ui.Provider` plus one `/hello` route registered through `WithRoutes` behind login — AC-003
- [ ] T050 [P] Add a CI job that builds the example embedder against the module zip with `GOFLAGS=-mod=mod GOWORK=off` — AC-003, §6
- [ ] T051 [P] Add a golden CI comparison of `pgwatch --help` and the REST endpoint list before/after the change — AC-002
- [ ] T052 [P] Write the embedding guide in `docs/developer/` (provider, route hook, `pkg/app`, `cmdopts` extensions) and add it to `mkdocs.yml` nav
- [ ] T053 Run `task lint`, `go mod tidy`, `task test`; confirm AC-001 through AC-007 all hold

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: no dependencies — can start immediately
- **Foundational (Phase 2)**: depends on T002/T004; BLOCKS all user stories. The embed removal is
  what makes `webserver` publishable (REQ-013) and gives the UI and route extension points a home
- **US1 (Phase 3)**: depends on Phase 2 — the relocation must not carry `//go:embed build` into `pkg/`
- **US2 (Phase 4)**: depends on Phase 2 (the `Provider` must exist); merges cleanly after US1
- **US3 (Phase 5)**: depends on Phase 2 (`Option` type from T008)
- **US4 (Phase 6)**: depends on US1 (imports `pkg/*`) and US3 (forwards `WithRoutes`)
- **US5 (Phase 7)**: depends on US1 (`pkg/cmdopts` must exist)
- **Polish (Phase 8)**: T049/T050 depend on US1 + US2 + US3; T051 can run any time after Phase 2

### Within Each User Story

- Tests are written first and must fail before implementation
- Interface before consumers (`pkg/ui` → `webserver` → `cmd/pgwatch`)
- Relocation before anything that adds new exported API to a relocated package
- Story complete before moving to the next priority

### Parallel Opportunities

- T001, T002, T003 in parallel
- T012, T013 in parallel; the relocation tasks T015–T021 are **sequential** — each one rewrites
  imports across the whole tree and they conflict on the same files
- All `[P]` test tasks within a story in parallel
- Once Phase 3 lands, US2 / US3 / US5 can be worked in parallel by different people; US4 waits on US3

---

## Parallel Example: User Story 2

```bash
# Launch all tests for User Story 2 together:
Task: "SPARoutes fallback + \"*\" wildcard table test in pkg/webserver/webserver_test.go"
Task: "IndexData merge / BasePath-not-overridable test in pkg/webserver/webserver_test.go"
Task: "--web-disable=ui and --web-disable=all test in pkg/webserver/server_test.go"
```

---

## Implementation Strategy

### MVP First (US1 only)

1. Phase 1: Setup
2. Phase 2: Foundational (CRITICAL — blocks all stories)
3. Phase 3: US1 — published packages
4. **STOP and VALIDATE**: AC-001, AC-004, AC-005, AC-007; tag and confirm `go build …@<tag>` from an
   empty directory
5. Ship — embedders can already import the engine and bring their own provider

### Incremental Delivery

1. Setup + Foundational → the React embed lives behind a provider
2. + US1 → engine importable from the module proxy (MVP)
3. + US2 → embedders control SPA routes and index data
4. + US3 → embedders add authenticated routes and set the CORS origin
5. + US4 → embedders reuse the whole bootstrap
6. + US5 → embedders add their own flags and subcommands

Every increment leaves `pgwatch` behaviour untouched (AC-002).

---

## Notes

- `[P]` = different files, no dependencies
- The relocation (T015–T021) is a pure move + import rewrite; **no behaviour or identifier changes**
  belong in those commits (REQ-002). T023 is the one exception and is a separate task on purpose
- Commit after each task or logical group; keep the relocation commits mechanical so review is cheap
- Verify tests fail before implementing
- `internal/webui` (React sources) stays internal — CON-001
- Stop at any checkpoint to validate a story independently
