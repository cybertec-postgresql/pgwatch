---
title: Public extension API: published engine packages and a pluggable web UI
version: 1.0
date_created: 2026-09-07
date_updated: 2026-09-07
owner: pgwatch maintainers
status: draft
tags: [architecture, design, api, webui, packaging]
---

# Introduction

pgwatch is built as one binary whose engine lives entirely under `internal/`. That is the right
default for an application, but it prevents a growing class of legitimate users from building
on pgwatch as a library: distributions that ship a different web UI, operators who want to add
their own HTTP endpoints next to the REST API, and embedders who want to start the collector from
their own `main` with extra wiring. Today each of them has to fork.

This specification makes pgwatch embeddable without changing its behaviour: the engine packages
become importable, the web UI becomes a pluggable component, the web server gains a route
extension point, and the module becomes buildable straight from the module proxy. The `pgwatch`
binary itself stays byte-for-byte equivalent in behaviour.

Code references are against `master` at `v6.0.0-beta-56-gae677c8028`. Line numbers are
indicative, not contractual.

---

## 1. Purpose & Scope

**Purpose**: allow an external Go module to import the pgwatch engine, inject a web UI, register
additional authenticated routes and run the collector, using only public API.

**In scope**:

- Publishing the engine packages `cmdopts`, `db`, `log`, `metrics`, `reaper`, `sinks`,
  `sources` and `webserver` under public import paths, with a stability policy.
- A `ui.Provider` interface and a way to register a provider with the web server.
- A route extension hook that reuses the existing JWT middleware.
- Configurable SPA fallback routes and index template data.
- Making the module consumable from the Go module proxy (generated protobuf code, UI embed).
- A configurable CORS origin.
- Exporting the config and sink schema constants.
- An optional `app` package that wraps the `main` wiring sequence.
- An options extension so embedders can add their own flag groups and subcommands.

**Out of scope**:

- Any change in behaviour of the `pgwatch` binary, its flags, its REST API or its metrics.
- New sinks, sources or metrics.
- A metrics-read API.
- Changes to the React web UI.

**Audience**: pgwatch maintainers. Also intended for direct consumption by AI coding assistants.

---

## 2. Definitions

| Term | Definition |
|---|---|
| **Engine packages** | `internal/cmdopts`, `internal/db`, `internal/log`, `internal/metrics`, `internal/reaper`, `internal/sinks`, `internal/sources`, `internal/webserver`. |
| **Embedder** | An external Go module that imports pgwatch packages and provides its own `main`. |
| **Provider** | An implementation of `ui.Provider` supplying the static assets and routes of a web UI. |
| **Default provider** | The provider serving the React UI built from `internal/webui`. |
| **Route hook** | A callback invoked during web server initialisation with the mux, the base path and the authentication wrapper. |

---

## 3. Requirements, Constraints & Guidelines

### Package publication

- **REQ-001**: The engine packages MUST be importable from another module. The recommended
  layout is `pkg/<name>` (for example `github.com/cybertec-postgresql/pgwatch/v6/pkg/reaper`),
  keeping `internal/` for `testutil`, the React UI sources and the default provider's embed.
- **REQ-002**: Package names, exported identifiers and behaviour MUST NOT change as part of the
  move; the change is a relocation plus an import-path rewrite.
- **REQ-003**: A stability policy MUST be documented in `docs/developer/`: within a major
  version, exported API of `pkg/*` follows the Go compatibility promise; packages or symbols
  that are not yet stable carry an `// Experimental:` doc comment.
- **CON-001**: `internal/webui` (React sources) MUST stay internal.
- **GUD-001**: Keep `pkg/` limited to the engine packages listed; do not publish helpers that
  exist only for tests.

### Web UI provider

- **REQ-004**: A package `pkg/ui` MUST define:

  ```go
  type Provider interface {
      FS() fs.FS                   // static assets; index.html at the root is a Go html/template
      SPARoutes() []string         // client-side routes that must be answered with index.html
      IndexData() map[string]any   // extra template data merged with {"BasePath": ...}
  }
  ```

- **REQ-005**: The web server MUST obtain its UI from a `Provider` instead of the package-level
  `uiFS` (`internal/webserver/webserver.go:26-33`). If no provider is configured and the UI is
  not disabled, initialisation MUST fail with a clear error.
- **REQ-006**: The default provider MUST live in a package that only `cmd/pgwatch` imports (for
  example `internal/webui/embed`), so embedders never compile the React assets.
- **REQ-007**: `prepareIndexHTML` (`webserver.go:105-120`) MUST merge `Provider.IndexData()`
  into the template data; keys supplied by the provider MUST NOT override `BasePath`.
- **REQ-008**: `handleStatic` (`webserver.go:122-165`) MUST take the SPA fallback list from
  `Provider.SPARoutes()` instead of the hardcoded slice at line 133. A provider MAY return a
  single `"*"` entry meaning "every path without a file extension".
- **CON-002**: `--web-disable=ui` MUST keep its meaning: the provider is ignored and only the
  REST API is served; `--web-disable=all` returns no server, as today (`webserver.go:52-54`).

### Route extension hook

- **REQ-009**: `webserver.Init` MUST accept functional options. One option MUST be

  ```go
  func WithRoutes(fn func(mux *http.ServeMux, basePath string, auth func(http.HandlerFunc) http.Handler)) Option
  ```

  invoked after the built-in routes are registered (`webserver.go:77-87`) and before the static
  handler is mounted, so embedder routes can never shadow built-in ones and always sit under the
  same base path.
- **REQ-010**: The `auth` argument MUST wrap a handler with the existing JWT check
  (`internal/webserver/jwt.go:71`, `NewEnsureAuth`) so embedder routes share login, token and
  expiry semantics with the REST API.
- **REQ-011**: A second option `WithCORSOrigin(origin string)` MUST replace the hardcoded
  `http://localhost:4000` (`webserver.go:213`), and the same value MUST be settable through
  `--web-cors-origin` / `PW_WEBCORSORIGIN` (default: current behaviour).

### Module consumability

- **REQ-012**: Generated protobuf files `api/pb/*.pb.go` MUST be committed. A CI step MUST run
  `go generate ./api/pb/` and fail if the working tree changes.
- **REQ-013**: No package that an embedder imports may `//go:embed` a gitignored directory. The
  React build output MUST be embedded only by the default provider package (REQ-006), and that
  package MUST be excluded from the module's public API surface by living under `internal/`.
- **REQ-014**: `go build github.com/cybertec-postgresql/pgwatch/v6/cmd/pgwatch@<tag>` from an
  empty directory MUST succeed once a tag containing this change exists.

### Schema constants and bootstrap

- **REQ-015**: `configSchema` and `sinkSchema` (`cmd/pgwatch/version.go:10-11`) MUST move to a
  published package as exported constants (recommended: `pkg/cmdopts.ConfigSchema` and
  `pkg/cmdopts.SinkSchema`, next to `NeedsSchemaUpgrade`), and `printVersion` MUST use them.
- **REQ-016**: A package `pkg/app` SHOULD wrap the sequence of `cmd/pgwatch/main.go:59-113`
  (options → logger → config readers → sink writer → schema check → reaper → web server →
  `Reap`) behind `New(ctx, opts, ...Option) (*App, error)` and `Run(ctx) int`, with options
  `WithUI(ui.Provider)` and `WithRoutes(...)`. `cmd/pgwatch/main.go` SHOULD become a thin caller
  of it so that the binary and any embedder share one wiring sequence.
- **GUD-002**: Keep signal handling, panic recovery and exit-code mapping inside `pkg/app` so
  embedders inherit them.
- **REQ-018**: `cmdopts.New` (`internal/cmdopts/cmdoptions.go:70-76`) MUST accept embedder
  extensions registered before parsing: additional `go-flags` option groups (for example an
  embedder-specific group of `--ee-*` flags read from the environment with the same `PW_` prefix
  conventions) and additional subcommands. The `pgwatch` binary passes no extension, so its
  `--help` output is unchanged.

### Metadata

- **REQ-017**: Package metadata MUST state the actual license: `.goreleaser.yml` nfpm
  `license` and `internal/webui/package.json` `license` currently say MIT while `LICENSE` is
  BSD-3-Clause.

---

## 4. Interfaces & Data Contracts

### 4.1 Package layout (proposed)

| Today | After | Note |
|---|---|---|
| `internal/cmdopts` | `pkg/cmdopts` | gains `ConfigSchema`, `SinkSchema` |
| `internal/db` | `pkg/db` | |
| `internal/log` | `pkg/log` | |
| `internal/metrics` | `pkg/metrics` | embedded `metrics.yaml` moves with it |
| `internal/reaper` | `pkg/reaper` | |
| `internal/sinks` | `pkg/sinks` | embedded `sql/*.sql` moves with it |
| `internal/sources` | `pkg/sources` | |
| `internal/webserver` | `pkg/webserver` | loses the embed; gains options |
| — | `pkg/ui` | `Provider` interface |
| — | `pkg/app` | optional bootstrap |
| `internal/webui` | `internal/webui` | unchanged React sources |
| — | `internal/webui/embed` | default provider, `//go:embed build` |
| `internal/testutil` | `internal/testutil` | unchanged |

### 4.2 `webserver.Init` signature

```go
type Option func(*WebUIServer)

func WithUI(p ui.Provider) Option
func WithRoutes(fn func(mux *http.ServeMux, basePath string, auth func(http.HandlerFunc) http.Handler)) Option
func WithCORSOrigin(origin string) Option

func Init(ctx context.Context, opts CmdOpts, mrw metrics.ReaderWriter, srw sources.ReaderWriter,
    rc Readier, options ...Option) (*WebUIServer, error)
```

### 4.3 `pkg/app`

```go
type App struct { /* unexported */ }
type Option func(*App)

func WithUI(p ui.Provider) Option
func WithRoutes(fn func(mux *http.ServeMux, basePath string, auth func(http.HandlerFunc) http.Handler)) Option

func New(ctx context.Context, opts *cmdopts.Options, options ...Option) (*App, error)
func (a *App) Run(ctx context.Context) int
func (a *App) Options() *cmdopts.Options
func (a *App) Logger() log.Logger
func (a *App) Ready() bool
```

`Run` returns the same exit codes `cmd/pgwatch/main.go` produces today
(`cmdopts.ExitCodeOK`, `ExitCodeConfigError`, `ExitCodeUpgradeError`, `ExitCodeWebUIError`,
`ExitCodeUserCancel`, `ExitCodeFatalError`).

### 4.4 Default provider

```go
package embed // internal/webui/embed

//go:embed build
var buildFS embed.FS

func Provider() ui.Provider  // FS: fs.Sub(buildFS, "build"); SPARoutes: "/", "/sources", "/metrics", "/presets", "/logs"; IndexData: nil
```

### 4.5 `cmdopts` extension

```go
type Extension interface {
    // Register adds groups or commands to the parser before it parses os.Args.
    Register(parser *flags.Parser, opts *Options) error
}

func New(out io.Writer, exts ...Extension) (*Options, error)
```

### 4.6 `cmd/pgwatch/main.go` after the change (sketch)

```go
func main() {
    opts, err := cmdopts.New(os.Stdout)
    // ... existing error and subcommand handling ...
    a, err := app.New(ctx, opts, app.WithUI(embed.Provider()))
    // ... existing error handling ...
    Exit(a.Run(ctx))
}
```

---

## 5. Acceptance Criteria

- **AC-001**: `go test ./...` passes unchanged after the relocation.
- **AC-002**: `pgwatch --help`, every REST endpoint and the served UI are identical before and
  after the change (golden comparison in CI).
- **AC-003**: An example embedder under `docs/howto/embedding/` (a `main.go` that registers a
  trivial provider and one route) builds and serves `/hello` behind login.
- **AC-004**: `git status` is clean after `go generate ./api/pb/` in CI.
- **AC-005**: `go build github.com/cybertec-postgresql/pgwatch/v6/cmd/pgwatch@<tag>` succeeds
  from an empty directory.
- **AC-006**: `--web-disable=ui` serves the REST API without any provider; `--web-disable=all`
  serves nothing.
- **AC-007**: `pgwatch --version` prints the schema constants from the published package.

---

## 6. Test Automation Strategy

- Relocation: existing unit and integration tests move with their packages; no new tests needed
  beyond compile.
- Provider and route hook: table-driven tests in `pkg/webserver` covering a fake provider,
  `SPARoutes` fallback, `IndexData` merge without `BasePath` override, a hook-registered route
  behind `auth` (401 without token, 200 with), CORS origin option.
- Consumability: a CI job that builds the example embedder against the module zip with
  `GOFLAGS=-mod=mod` and `GOWORK=off`.
- Generated code freshness: CI diff after `go generate ./api/pb/`.

---

## 7. Rationale & Context

### Why publish the packages instead of a narrow facade?

A facade must grow with every embedder need and still cannot hide the types an embedder has to
name (`cmdopts.Options`, `metrics.ReaderWriter`, `sources.ReaderWriter`). The engine packages
are already pgwatch's architecture; publishing them costs one relocation and a stability policy.

### Why a provider interface and not a build-time asset swap?

A build-time swap forces embedders to patch the pgwatch tree in CI and gives them no way to add
routes or template data. An interface is a few lines and keeps the default binary unchanged.

### Why commit generated protobuf code?

The Go module proxy serves the git tree; a package that imports `api/pb` cannot compile from the
proxy while the generated files are gitignored. Committing them with a freshness check is the
standard solution.

### Why keep the React embed out of the published packages?

`//go:embed build` in a published package makes every embedder's build depend on a directory
that is not in git. Isolating it in the default provider keeps the module buildable and lets
embedders bring their own assets.

---

## 8. Dependencies & External Integrations

- Go 1.26 module system (`internal/` rule, module proxy).
- Existing JWT middleware (`internal/webserver/jwt.go`) and CORS middleware.
- GoReleaser and Docker builds: the build of the default provider's assets stays in
  `cmd/pgwatch` builds only.

---

**Origin**: drafted in the pgwatch Enterprise Edition repository as an upstream proposal.
Assisted by Claude Code.
