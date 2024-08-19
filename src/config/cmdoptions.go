package config

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"time"

	"github.com/cybertec-postgresql/pgwatch3/db"
	"github.com/cybertec-postgresql/pgwatch3/log"
	"github.com/cybertec-postgresql/pgwatch3/metrics"
	"github.com/cybertec-postgresql/pgwatch3/sinks"
	"github.com/cybertec-postgresql/pgwatch3/sources"
	"github.com/cybertec-postgresql/pgwatch3/webserver"
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

type Options struct {
	Sources sources.SourceCmdOpts  `group:"Sources"`
	Metrics metrics.MetricCmdOpts  `group:"Metrics"`
	Sinks   sinks.SinkCmdOpts      `group:"Sinks"`
	Logging log.LoggingCmdOpts     `group:"Logging"`
	WebUI   webserver.WebUICmdOpts `group:"WebUI"`
	Init    bool                   `long:"init" description:"Initialize configurations schemas to the latest version and exit. Can be used with --upgrade"`
	Upgrade bool                   `long:"upgrade" description:"Upgrade configurations to the latest version"`
	Help    bool

	// sourcesReaderWriter reads/writes the monitored sources (databases, patroni clusters, pgpools, etc.) information
	SourcesReaderWriter sources.ReaderWriter
	// metricsReaderWriter reads/writes the metric and preset definitions
	MetricsReaderWriter metrics.ReaderWriter
	ExitCode            int32
	CommandCompleted    bool
}

func addCommands(parser *flags.Parser, opts *Options) {
	_, _ = parser.AddCommand("metric",
		"Manage metrics",
		"Commands to manage metrics",
		NewMetricCommand(opts))
	_, _ = parser.AddCommand("source",
		"Manage sources",
		"Commands to manage sources",
		NewSourceCommand(opts))
}

// New returns a new instance of CmdOptions
func New(writer io.Writer) (*Options, error) {
	cmdOpts := new(Options)
	parser := flags.NewParser(cmdOpts, flags.HelpFlag)
	parser.SubcommandsOptional = true // if not command specified, start monitoring
	addCommands(parser, cmdOpts)
	nonParsedArgs, err := parser.Parse()
	if err != nil { //subcommands executed as part of parsing
		if flagsErr, ok := err.(*flags.Error); ok && flagsErr.Type == flags.ErrHelp {
			cmdOpts.Help = true
		}
		if !flags.WroteHelp(err) {
			parser.WriteHelp(writer)
		}
	} else {
		if !cmdOpts.CommandCompleted && len(nonParsedArgs) > 0 {
			err = fmt.Errorf("unknown argument(s): %v", nonParsedArgs)
		}
	}
	if err == nil {
		err = validateConfig(cmdOpts)
	}
	return cmdOpts, err
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
	if _, err := pgx.ParseConfig(arg); err == nil {
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

// InitMetricReader creates a new source reader based on the configuration kind from the options.
func (c *Options) InitMetricReader(ctx context.Context) (err error) {
	var configKind Kind
	if c.Metrics.Metrics == "" { //if config database is configured, use it for metrics as well
		if k, err := c.GetConfigKind(c.Sources.Sources); err == nil && k == ConfigPgURL {
			c.Metrics.Metrics = c.Sources.Sources
		} else { // otherwise use built-in metrics
			c.MetricsReaderWriter, err = metrics.NewYAMLMetricReaderWriter(ctx, "")
			return err
		}
	}
	if configKind, err = c.GetConfigKind(c.Metrics.Metrics); err != nil {
		return
	}
	switch configKind {
	case ConfigPgURL:
		var configDb db.PgxPoolIface
		if configDb, err = db.New(ctx, c.Sources.Sources); err != nil {
			return err
		}
		c.MetricsReaderWriter, err = metrics.NewPostgresMetricReaderWriter(ctx, configDb)
	default:
		c.MetricsReaderWriter, err = metrics.NewYAMLMetricReaderWriter(ctx, c.Metrics.Metrics)
	}
	return err
}

// InitSourceReader creates a new source reader based on the configuration kind from the options.
func (c *Options) InitSourceReader(ctx context.Context) (err error) {
	var configKind Kind
	if configKind, err = c.GetConfigKind(c.Sources.Sources); err != nil {
		return
	}
	switch configKind {
	case ConfigPgURL:
		var configDb db.PgxPoolIface
		if configDb, err = db.New(ctx, c.Sources.Sources); err != nil {
			return err
		}
		c.SourcesReaderWriter, err = sources.NewPostgresSourcesReaderWriter(ctx, configDb)
	default:
		c.SourcesReaderWriter, err = sources.NewYAMLSourcesReaderWriter(ctx, c.Sources.Sources)
	}
	return err
}

// InitConfigReaders creates the configuration readers based on the configuration kind from the options.
func (c *Options) InitConfigReaders(ctx context.Context) error {
	return errors.Join(c.InitMetricReader(ctx), c.InitSourceReader(ctx))
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
