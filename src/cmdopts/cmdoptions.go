package cmdopts

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"time"

	"github.com/cybertec-postgresql/pgwatch/log"
	"github.com/cybertec-postgresql/pgwatch/metrics"
	"github.com/cybertec-postgresql/pgwatch/sinks"
	"github.com/cybertec-postgresql/pgwatch/sources"
	"github.com/cybertec-postgresql/pgwatch/webserver"
	"github.com/jackc/pgx/v5"
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

	// sourcesReaderWriter reads/writes the monitored sources (databases, patroni clusters, pgpools, etc.) information
	SourcesReaderWriter sources.ReaderWriter
	// metricsReaderWriter reads/writes the metric and preset definitions
	MetricsReaderWriter metrics.ReaderWriter
	ExitCode            int32
	CommandCompleted    bool
}

func addCommands(parser *flags.Parser, opts *Options) {
	_, _ = parser.AddCommand("metric", "Manage metrics", "", NewMetricCommand(opts))
	_, _ = parser.AddCommand("source", "Manage sources", "", NewSourceCommand(opts))
	_, _ = parser.AddCommand("config", "Manage configurations", "", NewConfigCommand(opts))
}

// New returns a new instance of Options and immediately executes the subcommand if specified.
// Errors are returned for parsing only, if the command line arguments are invalid.
// Subcommands execution errors (if any) do not affect error returned.
// Subcommands are responsible for setting exit code only.
func New(writer io.Writer) (cmdOpts *Options, err error) {
	cmdOpts = new(Options)
	parser := flags.NewParser(cmdOpts, flags.HelpFlag)
	parser.SubcommandsOptional = true // if not command specified, start monitoring
	addCommands(parser, cmdOpts)
	nonParsedArgs, err := parser.Parse() // parse and execute subcommand if any
	if err != nil {
		if flagsErr, ok := err.(*flags.Error); ok && flagsErr.Type == flags.ErrHelp {
			cmdOpts.Help = true
		}
		if !flags.WroteHelp(err) {
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
	err = validateConfig(cmdOpts)
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
	_, err := pgx.ParseConfig(arg)
	return err == nil
}

// InitMetricReader creates a new source reader based on the configuration kind from the options.
func (c *Options) InitMetricReader(ctx context.Context) (err error) {
	if c.Metrics.Metrics == "" { //if config database is configured, use it for metrics as well
		if c.IsPgConnStr(c.Sources.Sources) {
			c.Metrics.Metrics = c.Sources.Sources
		} else { // otherwise use built-in metrics
			c.MetricsReaderWriter, err = metrics.NewYAMLMetricReaderWriter(ctx, "")
			return err
		}
	}
	if c.IsPgConnStr(c.Metrics.Metrics) {
		c.MetricsReaderWriter, err = metrics.NewPostgresMetricReaderWriter(ctx, c.Metrics.Metrics)
	} else {
		c.MetricsReaderWriter, err = metrics.NewYAMLMetricReaderWriter(ctx, c.Metrics.Metrics)
	}
	return err
}

// InitSourceReader creates a new source reader based on the configuration kind from the options.
func (c *Options) InitSourceReader(ctx context.Context) (err error) {
	var configKind Kind
	if c.Sources.Sources == "" { //if config database is configured, use it for sources as well
		if c.IsPgConnStr(c.Metrics.Metrics) {
			c.Sources.Sources = c.Metrics.Metrics
		}
	}
	if configKind, err = c.GetConfigKind(c.Sources.Sources); err != nil {
		return
	}
	switch configKind {
	case ConfigPgURL:
		c.SourcesReaderWriter, err = sources.NewPostgresSourcesReaderWriter(ctx, c.Sources.Sources)
	default:
		c.SourcesReaderWriter, err = sources.NewYAMLSourcesReaderWriter(ctx, c.Sources.Sources)
	}
	return err
}

// InitConfigReaders creates the configuration readers based on the configuration kind from the options.
func (c *Options) InitConfigReaders(ctx context.Context) error {
	return errors.Join(c.InitMetricReader(ctx), c.InitSourceReader(ctx))
}

// NeedsSchemaUpgrade checks if the configuration database schema needs an upgrade.
func (c *Options) NeedsSchemaUpgrade() (upgrade bool, err error) {
	if m, ok := c.SourcesReaderWriter.(metrics.Migrator); ok {
		upgrade, err = m.NeedsMigration()
	}
	if upgrade || err != nil {
		return
	}
	if m, ok := c.MetricsReaderWriter.(metrics.Migrator); ok {
		return m.NeedsMigration()
	}
	return
}

// InitWebUI initializes the web UI server
func (c *Options) InitWebUI(fs fs.FS, logger log.LoggerIface) error {
	if webserver.Init(c.WebUI, fs, c.MetricsReaderWriter, c.SourcesReaderWriter, logger) == nil {
		return errors.New("failed to initialize web UI")
	}
	return nil
}

func validateConfig(c *Options) error {
	if len(c.Sources.Sources)+len(c.Metrics.Metrics) == 0 {
		return errors.New("both --sources and --metrics are empty")
	}
	if c.Sources.Refresh <= 1 {
		return errors.New("--servers-refresh-loop-seconds must be greater than 1")
	}
	if c.Sources.MaxParallelConnectionsPerDb < 1 {
		return errors.New("--max-parallel-connections-per-db must be >= 1")
	}

	// validate that input is boolean is set
	if c.Sinks.BatchingDelay <= 0 || c.Sinks.BatchingDelay > time.Hour {
		return errors.New("--batching-delay-ms must be between 0 and 3600000")
	}

	return nil
}