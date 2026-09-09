// Command embedding is a minimal pgwatch embedder: it runs the real collector
// and REST API, serves its own web UI instead of the React one, adds an
// authenticated /hello endpoint next to the REST API, and contributes its own
// command-line flag group.
//
// Run it exactly like pgwatch itself, for example:
//
//	go run ./docs/howto/embedding --sources=postgresql://user@host/db \
//	    --web-user=admin --web-password=secret --greeting="hi there"
//
// Log in and call the extra endpoint with the token you get back — the same
// JWT the REST API uses, in the same Token header:
//
//	TOKEN=$(curl -s -XPOST -d '{"user":"admin","password":"secret"}' \
//	    http://localhost:8080/login)
//	curl -H "Token: $TOKEN" http://localhost:8080/hello
package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"os"

	"github.com/cybertec-postgresql/pgwatch/v6/pkg/app"
	"github.com/cybertec-postgresql/pgwatch/v6/pkg/cmdopts"
	"github.com/cybertec-postgresql/pgwatch/v6/pkg/ui"
	flags "github.com/jessevdk/go-flags"
)

// The embedder's own assets. Unlike pgwatch's React build these are checked
// into git, which is what keeps this command buildable from the module proxy.
//
//go:embed ui
var uiFS embed.FS

// provider is a whole ui.Provider: three methods over an fs.FS whose root
// holds an index.html.
type provider struct {
	fsys fs.FS
}

var _ ui.Provider = provider{}

// FS returns the assets to serve.
func (p provider) FS() fs.FS { return p.fsys }

// SPARoutes lists the paths answered with index.html rather than looked up as
// files. "*" means every path that does not look like a file, which is what a
// client-side router usually wants.
func (p provider) SPARoutes() []string { return []string{"*"} }

// IndexData is merged into the index.html template data. The server's own
// keys win, so BasePath below is ignored in favour of the real base path.
func (p provider) IndexData() map[string]any {
	return map[string]any{
		"Title":    "Acme Monitoring",
		"Flavour":  "embedded",
		"BasePath": "never wins over the server's value",
	}
}

// acmeOptions is the embedder's flag group. It follows pgwatch's conventions:
// long flags and a PW_ environment variable for each.
type acmeOptions struct {
	Greeting string `long:"greeting" description:"What /hello says" env:"PW_GREETING" default:"hello"`
}

// acmeExtension registers that group with the pgwatch option parser before it
// parses os.Args, so --greeting shows up in --help alongside the built-in
// flags. An extension can add subcommands here too.
type acmeExtension struct {
	opts acmeOptions
}

func (e *acmeExtension) Register(parser *flags.Parser, _ *cmdopts.Options) error {
	_, err := parser.AddGroup("Acme", "Acme-specific options", &e.opts)
	return err
}

// routes registers the embedder's HTTP handlers. They are mounted under the
// same base path as the REST API and can never shadow a built-in route;
// wrapping a handler in auth puts it behind the same JWT check, so callers
// need a token from /login.
func (e *acmeExtension) routes(mux *http.ServeMux, basePath string, auth func(http.HandlerFunc) http.Handler) {
	mux.Handle(basePath+"hello", auth(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, e.opts.Greeting)
	}))
}

func main() {
	ext := new(acmeExtension)

	opts, err := cmdopts.New(os.Stdout, ext)
	if err != nil {
		fmt.Println(err)
		if opts.Help {
			os.Exit(int(cmdopts.ExitCodeOK))
		}
		os.Exit(int(cmdopts.ExitCodeConfigError))
	}
	if opts.CommandCompleted { // a subcommand ran, nothing left to do
		os.Exit(int(opts.ExitCode))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// app.New wires the same startup sequence the pgwatch binary runs, and
	// app.Run owns signal handling, panic recovery and the exit codes.
	a, err := app.New(ctx, opts,
		app.WithUI(provider{fsys: mustSub(uiFS, "ui")}),
		app.WithRoutes(ext.routes),
	)
	if err != nil {
		fmt.Println(err)
		os.Exit(int(cmdopts.ExitCodeConfigError))
	}

	os.Exit(a.Run(ctx))
}

func mustSub(fsys fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err)
	}
	return sub
}
