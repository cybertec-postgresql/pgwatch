---
title: Public API Stability
---

# Public API stability

pgwatch is an application first, but its engine is importable. Everything under
`pkg/` is public API that other Go modules may build on; everything under
`internal/` is private and may change in any release.

## What is public

| Package | Purpose |
|---|---|
| `pkg/cmdopts` | command-line options, config readers, schema constants |
| `pkg/db` | database helpers shared by the engine |
| `pkg/log` | logging setup and context plumbing |
| `pkg/metrics` | metric definitions and their reader/writer |
| `pkg/reaper` | the measurement gathering loop |
| `pkg/sinks` | measurement sinks |
| `pkg/sources` | monitored source definitions and their reader/writer |
| `pkg/ui` | the `Provider` interface for pluggable web UIs |
| `pkg/webserver` | the REST API and static UI server |

Nothing else is public. In particular `internal/webui` (the React sources) and
`internal/webui/embed` (the default UI provider) are internal on purpose: an
embedder brings its own `ui.Provider` and never compiles the React assets.

## The promise

Within a major version, the exported API of `pkg/*` follows the
[Go compatibility promise](https://go.dev/doc/go1compat): code that compiles
against `v6.x` keeps compiling against every later `v6.y`. Concretely, within a
major version we do not

- remove or rename an exported identifier,
- add a method to an exported interface,
- change an exported function's signature, or
- change the meaning of an exported constant.

We may add packages, add exported identifiers, add struct fields, and relax (but
not tighten) accepted inputs.

Breaking any of the above requires a new major version and a new module path
(`.../pgwatch/v7/...`), so an upgrade is never silent.

## Experimental symbols

An API that is published but not yet settled carries an `// Experimental:` line
in its doc comment:

```go
// WithRoutes registers additional HTTP routes on the server's mux.
//
// Experimental: this signature may change in a minor release.
func WithRoutes(fn func(mux *http.ServeMux, basePath string, auth func(http.HandlerFunc) http.Handler)) Option
```

Experimental symbols are exempt from the promise above. They are the exception,
not the default: use them knowing a minor release may change them, and expect
either stabilisation or removal within one major version.

## Adding to the public API

Publishing a package or symbol is a commitment for the rest of the major
version, so:

- keep `pkg/` limited to the engine; helpers that exist only for tests belong in
  `internal/testutil`,
- do not export a type merely because a test needs it,
- mark anything you are unsure about `// Experimental:` on the way in — it is far
  cheaper than a major version bump later.
