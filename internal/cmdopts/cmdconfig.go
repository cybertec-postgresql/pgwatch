package cmdopts

import (
	"context"
	"errors"

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
	if err = opts.ValidateConfig(); err != nil {
		return
	}
	ctx := context.Background()
	// Upgrade metrics/sources configuration if it's postgres
	if opts.IsPgConnStr(opts.Metrics.Metrics) && opts.IsPgConnStr(opts.Sources.Sources) {
		var m metrics.Migrator
		m, err = metrics.NewPostgresMetricMigrator(ctx, opts.Metrics.Metrics)
		if err != nil {
			opts.CompleteCommand(ExitCodeConfigError)
			return
		}
		if err = m.Migrate(); err != nil {
			opts.CompleteCommand(ExitCodeConfigError)
			return
		}
	} else {
		opts.CompleteCommand(ExitCodeConfigError)
		return errors.New("configuration storage does not support upgrade")
	}
	// Upgrade sinks configuration if it's postgres
	if len(opts.Sinks.Sinks) > 0 {
		var m metrics.Migrator
		m, err = sinks.NewPostgresSinkMigrator(ctx, opts.Sinks.Sinks[0], &opts.Sinks)
		if err != nil {
			opts.CompleteCommand(ExitCodeConfigError)
			return
		}
		if err = m.Migrate(); err != nil {
			opts.CompleteCommand(ExitCodeConfigError)
			return
		}
	}
	opts.CompleteCommand(ExitCodeOK)
	return
}
