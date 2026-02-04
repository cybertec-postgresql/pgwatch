package cmdopts

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/db"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/log"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/sinks"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/sources"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/webserver"
	flags "github.com/jessevdk/go-flags"
)

const (
	ExitCodeOK int32 = iota
	ExitCodeConfigError
	ExitCodeCmdError
	ExitCodeWebUIError
	ExitCodeUpgradeError
	ExitCodeUserCancel
	ExitCodeShutdownCommand
	ExitCodeFatalError
)

type Kind int

const (
	ConfigPgURL Kind = iota
	ConfigFile
	ConfigFolder
	ConfigError
)

// Options contains the command line options.
type Options struct {
	Sources sources.CmdOpts   `group:"Sources"`
	Metrics metrics.CmdOpts   `group:"Metrics"`
	Sinks   sinks.CmdOpts     `group:"Sinks"`
	Logging log.CmdOpts       `group:"Logging"`
	WebUI   webserver.CmdOpts `group:"WebUI"`
	Help    bool

	SourcesReaderWriter sources.ReaderWriter
	MetricsReaderWriter metrics.ReaderWriter
	SinksWriter         sinks.Writer

	ExitCode         int32
	CommandCompleted bool

	OutputWriter io.Writer
}

func addCommands(parser *flags.Parser, opts *Options) {
	_, _ = parser.AddCommand("metric", "Manage metrics", "", NewMetricCommand(opts))
	_, _ = parser.AddCommand("source", "Manage sources", "", NewSourceCommand(opts))
	_, _ = parser.AddCommand("config", "Manage configurations", "", NewConfigCommand(opts))
}

// New returns a new instance of Options and immediately executes the subcommand if specified.
// Subcommands are responsible for setting exit code.
// Function prints help message only if options are incorrect. If subcommand is executed
// but fails, function outputs the error message only, indicating that some argument
// values might be incorrect, e.g. wrong file name, lack of privileges, etc.
func New(writer io.Writer) (cmdOpts *Options, err error) {
	cmdOpts = new(Options)
	parser := flags.NewParser(cmdOpts, flags.HelpFlag)
	parser.SubcommandsOptional = true // if not command specified, start monitoring
	cmdOpts.OutputWriter = writer
	addCommands(parser, cmdOpts)
	nonParsedArgs, err := parser.Parse() // parse and execute subcommand if any
	if err != nil {
		if flagsErr, ok := err.(*flags.Error); ok && flagsErr.Type == flags.ErrHelp {
			cmdOpts.Help = true
		}
		if !flags.WroteHelp(err) && !cmdOpts.CommandCompleted {
			parser.WriteHelp(writer)
		}
		return cmdOpts, err
	}
	if cmdOpts.CommandCompleted { // subcommand executed, nothing to do more
		return
	}
	if len(nonParsedArgs) > 0 { // we don't expect any non-parsed arguments
		return cmdOpts, fmt.Errorf("unknown argument(s): %v", nonParsedArgs)
	}
	err = cmdOpts.ValidateConfig()
	return
}

func (c *Options) CompleteCommand(code int32) {
	c.CommandCompleted = true
	c.ExitCode = code
}

// Verbose returns true if the debug log is enabled
func (c *Options) Verbose() bool {
	return c.Logging.LogLevel == "debug"
}

func (c *Options) GetConfigKind(arg string) (_ Kind, err error) {
	if arg == "" {
		return Kind(ConfigError), errors.New("no configuration provided")
	}
	if c.IsPgConnStr(arg) {
		return Kind(ConfigPgURL), nil
	}
	var fi os.FileInfo
	if fi, err = os.Stat(arg); err == nil {
		if fi.IsDir() {
			return Kind(ConfigFolder), nil
		}
		return Kind(ConfigFile), nil
	}
	return Kind(ConfigError), err
}

func (c *Options) IsPgConnStr(arg string) bool {
	return db.IsPgConnStr(arg)
}

// InitMetricReader creates a new source reader based on the configuration kind from the options.
func (c *Options) InitMetricReader(ctx context.Context) (err error) {
	if c.Metrics.Metrics == "" { // use built-in metrics
		c.MetricsReaderWriter, err = metrics.NewYAMLMetricReaderWriter(ctx, "")
		return
	}
	if c.IsPgConnStr(c.Metrics.Metrics) {
		c.MetricsReaderWriter, err = metrics.NewPostgresMetricReaderWriter(ctx, c.Metrics.Metrics)
	} else {
		c.MetricsReaderWriter, err = metrics.NewYAMLMetricReaderWriter(ctx, c.Metrics.Metrics)
	}
	return
}

// InitSourceReader creates a new source reader based on the configuration kind from the options.
func (c *Options) InitSourceReader(ctx context.Context) (err error) {
	var configKind Kind
	if configKind, err = c.GetConfigKind(c.Sources.Sources); err != nil {
		return
	}
	switch configKind {
	case ConfigPgURL:
		c.SourcesReaderWriter, err = sources.NewPostgresSourcesReaderWriter(ctx, c.Sources.Sources)
	default:
		c.SourcesReaderWriter, err = sources.NewYAMLSourcesReaderWriter(ctx, c.Sources.Sources)
	}
	return
}

// InitConfigReaders creates the configuration readers based on the configuration kind from the options.
func (c *Options) InitConfigReaders(ctx context.Context) error {
	err := errors.Join(c.InitMetricReader(ctx), c.InitSourceReader(ctx))
	if err != nil {
		return err
	}
	return db.NeedsMigration(c.MetricsReaderWriter, metrics.ErrNeedsMigration)
}

// InitSinkWriter creates a new MultiWriter instance if needed.
func (c *Options) InitSinkWriter(ctx context.Context) (err error) {
	c.SinksWriter, err = sinks.NewSinkWriter(ctx, &c.Sinks)
	if err != nil {
		return err
	}
	return db.NeedsMigration(c.SinksWriter, sinks.ErrNeedsMigration)
}

// NeedsSchemaUpgrade checks if the configuration database schema needs an upgrade.
func (c *Options) NeedsSchemaUpgrade() (upgrade bool, err error) {
	if m, ok := c.SourcesReaderWriter.(db.Migrator); ok {
		upgrade, err = m.NeedsMigration()
	}
	if upgrade || err != nil {
		return
	}
	if m, ok := c.MetricsReaderWriter.(db.Migrator); ok {
		upgrade, err = m.NeedsMigration()
	}
	if upgrade || err != nil {
		return
	}
	if m, ok := c.SinksWriter.(db.Migrator); ok {
		return m.NeedsMigration()
	}
	return
}

// ValidateConfig checks if the configuration is valid.
// Configuration database can be specified for one of the --sources or --metrics.
// If one is specified, the other one is set to the same value.
func (c *Options) ValidateConfig() error {
	if len(c.Sources.Sources)+len(c.Metrics.Metrics) == 0 {
		return errors.New("both --sources and --metrics are empty")
	}
	switch { // if specified configuration database, use it for both sources and metrics
	case c.Sources.Sources == "" && c.IsPgConnStr(c.Metrics.Metrics):
		c.Sources.Sources = c.Metrics.Metrics
	case c.Metrics.Metrics == "" && c.IsPgConnStr(c.Sources.Sources):
		c.Metrics.Metrics = c.Sources.Sources
	}
	if c.Sources.Refresh <= 1 {
		return errors.New("--refresh must be greater than 1")
	}
	if c.Sources.MaxParallelConnectionsPerDb < 1 {
		return errors.New("--max-parallel-connections-per-db must be >= 1")
	}

	// validate that input is boolean is set
	if c.Sinks.BatchingDelay <= 0 || c.Sinks.BatchingDelay > time.Hour {
		return errors.New("--batching-delay must be between 0 and 1h")
	}

	return nil
}
