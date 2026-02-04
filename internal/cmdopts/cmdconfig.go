package cmdopts

import (
	"context"
	"errors"
	"fmt"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/db"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/sinks"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/sources"
)

type ConfigCommand struct {
	owner   *Options
	Init    ConfigInitCommand    `command:"init" description:"Initialize configuration"`
	Upgrade ConfigUpgradeCommand `command:"upgrade" description:"Upgrade configuration schema"`
}

func NewConfigCommand(owner *Options) *ConfigCommand {
	return &ConfigCommand{
		owner:   owner,
		Init:    ConfigInitCommand{owner: owner},
		Upgrade: ConfigUpgradeCommand{owner: owner},
	}
}

type ConfigInitCommand struct {
	owner *Options
}

// Execute initializes the configuration.
func (cmd *ConfigInitCommand) Execute([]string) (err error) {
	if err = cmd.owner.ValidateConfig(); err != nil {
		return
	}
	if cmd.owner.Metrics.Metrics > "" {
		err = cmd.InitMetrics()
	}
	if cmd.owner.Sources.Sources > "" && cmd.owner.Metrics.Metrics != cmd.owner.Sources.Sources {
		err = errors.Join(err, cmd.InitSources())
	}
	if len(cmd.owner.Sinks.Sinks) > 0 {
		err = errors.Join(err, cmd.InitSinks())
	}
	cmd.owner.CompleteCommand(map[bool]int32{
		true:  ExitCodeOK,
		false: ExitCodeConfigError,
	}[err == nil])
	return
}

// InitSources initializes the sources configuration.
func (cmd *ConfigInitCommand) InitSources() (err error) {
	ctx := context.Background()
	opts := cmd.owner
	if opts.IsPgConnStr(opts.Sources.Sources) {
		return opts.InitSourceReader(ctx)
	}
	rw, _ := sources.NewYAMLSourcesReaderWriter(ctx, opts.Sources.Sources)
	return rw.WriteSources(sources.Sources{sources.Source{}})
}

// InitMetrics initializes the metrics configuration.
func (cmd *ConfigInitCommand) InitMetrics() (err error) {
	ctx := context.Background()
	opts := cmd.owner
	err = opts.InitMetricReader(ctx)
	if err != nil || opts.IsPgConnStr(opts.Metrics.Metrics) {
		return // nothing to do, database initialized automatically
	}
	reader, _ := metrics.NewYAMLMetricReaderWriter(ctx, "")
	defMetrics, _ := reader.GetMetrics()
	return opts.MetricsReaderWriter.WriteMetrics(defMetrics)
}

// InitSinks initializes the sinks configuration.
func (cmd *ConfigInitCommand) InitSinks() (err error) {
	ctx := context.Background()
	opts := cmd.owner
	return opts.InitSinkWriter(ctx)
}

type ConfigUpgradeCommand struct {
	owner *Options
}

// Execute upgrades the configuration schema.
func (cmd *ConfigUpgradeCommand) Execute([]string) (err error) {
	opts := cmd.owner
	// For upgrade command, validate that at least one component is specified
	if len(opts.Sources.Sources)+len(opts.Metrics.Metrics)+len(opts.Sinks.Sinks) == 0 {
		opts.CompleteCommand(ExitCodeConfigError)
		return errors.New("at least one of --sources, --metrics, or --sink must be specified")
	}

	ctx := context.Background()

	f := func(uri string, newMigratorFunc func() (any, error)) error {
		if uri == "" {
			return nil
		}
		if !opts.IsPgConnStr(uri) {
			return fmt.Errorf("cannot upgrade storage %s: %w", uri, errors.ErrUnsupported)
		}
		m, initErr := newMigratorFunc()
		if initErr != nil {
			return initErr
		}
		return m.(db.Migrator).Migrate()

	}

	err = f(opts.Sources.Sources, func() (any, error) {
		return sources.NewPostgresSourcesReaderWriter(ctx, opts.Sources.Sources)
	})

	err = errors.Join(err, f(opts.Metrics.Metrics, func() (any, error) {
		return metrics.NewPostgresMetricReaderWriter(ctx, opts.Metrics.Metrics)
	}))

	for _, uri := range opts.Sinks.Sinks {
		err = errors.Join(err, f(uri, func() (any, error) {
			return sinks.NewPostgresSinkMigrator(ctx, uri)
		}))
	}

	if err == nil {
		opts.CompleteCommand(ExitCodeOK)
		return nil
	}

	// Check if all errors are ErrUnsupported
	allUnsupported := true
	for _, e := range err.(interface{ Unwrap() []error }).Unwrap() {
		if !errors.Is(e, errors.ErrUnsupported) {
			allUnsupported = false
			break
		}
		fmt.Fprintln(opts.OutputWriter, e)
	}

	if allUnsupported {
		opts.CompleteCommand(ExitCodeOK)
		return nil
	}
	opts.CompleteCommand(ExitCodeConfigError)
	return err
}
