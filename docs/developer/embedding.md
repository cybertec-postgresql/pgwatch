---
title: Embedding pgwatch
---

# Embedding pgwatch

pgwatch is an application, but its engine is a set of ordinary Go packages. An
external module can import them and run the collector from its own `main`,
serving its own web UI and its own HTTP endpoints, without forking pgwatch.

Everything on this page is public API covered by the
[stability policy](api_stability.md).

```console
$ go get github.com/cybertec-postgresql/pgwatch/v7@latest
```

There are four extension points, and you can use any subset of them:

| Extension point | What it gives you |
|---|---|
| [`ui.Provider`](#a-web-ui-of-your-own) | your own web UI instead of the bundled React one |
| [`app.WithRoutes`](#extra-http-endpoints) | extra HTTP endpoints behind the same login |
| [`cmdopts.Extension`](#your-own-flags-and-subcommands) | your own flag groups and subcommands |
| [`pkg/app`](#running-the-collector) | the whole pgwatch startup sequence, exit codes included |

A complete, compiling example lives in
[`docs/howto/embedding/`](https://github.com/cybertec-postgresql/pgwatch/tree/master/docs/howto/embedding);
the excerpts below are taken from it.

## A web UI of your own

`pkg/webserver` knows nothing about React. It asks a `ui.Provider` for the
assets to serve, so a UI is three methods over an `fs.FS`:

```go
type Provider interface {
    FS() fs.FS                  // assets, with index.html at the root
    SPARoutes() []string        // paths answered with index.html
    IndexData() map[string]any  // extra data for the index.html template
}
```

`index.html` is parsed as a Go [`html/template`](https://pkg.go.dev/html/template).
The server merges `IndexData()` with its own template data and writes its keys
last, so `BasePath` always reflects `--web-base-path` no matter what the
provider returns.

`SPARoutes()` lists the client-side routes of your application — paths that
must be answered with `index.html` rather than looked up as files. A single
`"*"` entry means "every path that does not look like a file", which is what
most client-side routers want:

```go
func (p provider) SPARoutes() []string { return []string{"*"} }
```

!!! warning "Embed committed assets"
    Embed assets that are **checked into git**. `//go:embed` of a directory
    produced by your build only works from a checkout — the Go module proxy
    serves the git tree, so a generated directory is simply not there. This is
    the same reason the `pgwatch` binary itself is not proxy-buildable; see
    the [stability policy](api_stability.md#what-is-public).

## Extra HTTP endpoints

`app.WithRoutes` hands you a mux, the computed base path, and the REST API's
own auth wrapper. Routes registered here sit under the same base path as the
REST API and can never shadow a built-in route:

```go
func (e *acmeExtension) routes(mux *http.ServeMux, basePath string, auth func(http.HandlerFunc) http.Handler) {
    mux.Handle(basePath+"hello", auth(func(w http.ResponseWriter, _ *http.Request) {
        fmt.Fprintln(w, e.opts.Greeting)
    }))
}
```

Wrapping a handler in `auth` puts it behind the same JWT check the REST API
uses, with the same login, token and expiry semantics. Leave it unwrapped for a
public endpoint.

Callers authenticate exactly as they do against the REST API — `POST /login`,
then the token in a `Token` header:

```console
$ TOKEN=$(curl -s -XPOST -d '{"user":"admin","password":"secret"}' http://localhost:8080/login)
$ curl -H "Token: $TOKEN" http://localhost:8080/hello
hi there
```

For a browser UI served from a different origin during development, point CORS
at it with `--web-cors-origin` (or `webserver.WithCORSOrigin`); it defaults to
`http://localhost:4000`.

## Your own flags and subcommands

`cmdopts.New` takes optional extensions, registered after pgwatch's own
subcommands and before the parser looks at `os.Args`. An extension can add
[go-flags](https://pkg.go.dev/github.com/jessevdk/go-flags) option groups,
subcommands, or both:

```go
type acmeOptions struct {
    Greeting string `long:"greeting" description:"What /hello says" env:"PW_GREETING" default:"hello"`
}

func (e *acmeExtension) Register(parser *flags.Parser, _ *cmdopts.Options) error {
    _, err := parser.AddGroup("Acme", "Acme-specific options", &e.opts)
    return err
}
```

Your flags then appear in `--help` next to the built-in ones and follow the
same conventions — give each one an `env:"PW_…"` tag so it can be set from the
environment like every pgwatch flag. Use `cmdopts.ExtensionFunc` to register
something small without declaring a type.

The `pgwatch` binary passes no extension, so its own `--help` is unaffected.

## Running the collector

`pkg/app` is the startup sequence the `pgwatch` binary itself runs: logger,
configuration readers, sink writer, schema check, reaper, web server, and then
the measurement gathering loop. `Run` also owns signal handling, panic recovery
and the exit-code mapping, so an embedder inherits all of it:

```go
opts, err := cmdopts.New(os.Stdout, ext)
// ... error and subcommand handling ...

a, err := app.New(ctx, opts,
    app.WithUI(provider{fsys: mustSub(uiFS, "ui")}),
    app.WithRoutes(ext.routes),
)
// ... error handling ...

os.Exit(a.Run(ctx))
```

`Run` returns the process exit code: `cmdopts.ExitCodeOK` on a clean stop,
`ExitCodeConfigError`, `ExitCodeUpgradeError` or `ExitCodeWebUIError` when a
startup step fails, `ExitCodeUserCancel` when the OS interrupted it, and
`ExitCodeFatalError` on a panic, which `Run` recovers and logs with its
callstack.

`App` also exposes `Options()`, `Logger()` and `Ready()` if you need to reach
into the running instance.

## Skipping the UI entirely

If your embedder only wants the REST API, pass `--web-disable=ui` and no
provider is needed. `--web-disable=all` starts the collector with no web server
at all.

## The complete example

```go title="docs/howto/embedding/main.go"
--8<-- "docs/howto/embedding/main.go"
```

Run it like `pgwatch` itself:

```console
$ go run ./docs/howto/embedding --sources=postgresql://user@host/db \
    --sink=postgresql://user@host/measurements \
    --web-user=admin --web-password=secret --greeting="hi there"
```
