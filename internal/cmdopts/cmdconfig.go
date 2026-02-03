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
	var upgraded bool // track if any component was upgraded

	// Upgrade metrics/sources configuration if it's postgres
	if opts.IsPgConnStr(opts.Metrics.Metrics) && opts.IsPgConnStr(opts.Sources.Sources) {
		if opts.MetricsReaderWriter == nil {
			err = opts.InitMetricReader(ctx)
			if err != nil {
				opts.CompleteCommand(ExitCodeConfigError)
				return
			}
		}
		if m, ok := opts.MetricsReaderWriter.(db.Migrator); ok {
			err = m.Migrate()
			if err != nil {
				opts.CompleteCommand(ExitCodeConfigError)
				return
			}
			upgraded = true
		} else {
			fmt.Fprintln(opts.OutputWriter, "[WARN] configuration storage does not support upgrade, skipping")
		}
	} else if opts.Metrics.Metrics > "" || opts.Sources.Sources > "" {
		// At least one is specified but not postgres connection - log warning
		fmt.Fprintln(opts.OutputWriter, "[WARN] configuration storage does not support upgrade, skipping")
	}

	// Upgrade sinks configuration if it's postgres
	if len(opts.Sinks.Sinks) > 0 {
		if opts.SinksWriter == nil {
			opts.SinksWriter, err = sinks.NewSinkWriter(ctx, &opts.Sinks)
			if err != nil {
				opts.CompleteCommand(ExitCodeConfigError)
				return
			}
		}
		if m, ok := opts.SinksWriter.(db.Migrator); ok {
			err = m.Migrate()
			if err != nil {
				opts.CompleteCommand(ExitCodeConfigError)
				return
			}
			upgraded = true
		} else {
			fmt.Fprintln(opts.OutputWriter, "[WARN] sink storage does not support upgrade, skipping")
		}
	}

	// Only fail if nothing was upgraded
	if !upgraded {
		opts.CompleteCommand(ExitCodeConfigError)
		return errors.New("no components support upgrade")
	}

	opts.CompleteCommand(ExitCodeOK)
	return
}
