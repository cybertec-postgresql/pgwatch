package cmdopts

import (
	"context"
	"errors"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/sources"
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
	cmd.owner.CompleteCommand(map[bool]int32{
		true:  ExitCodeOK,
		false: ExitCodeConfigError,
	}[err == nil])
	return
}

// InitSources initializes the sources configuration.
func (cmd *ConfigInitCommand) InitSources() error {
	ctx := context.Background()
	if cmd.owner.IsPgConnStr(cmd.owner.Sources.Sources) {
		return cmd.owner.InitSourceReader(ctx)
	}
	writer, _ := sources.NewYAMLSourcesReaderWriter(ctx, cmd.owner.Sources.Sources)
	return writer.WriteSources(sources.Sources{sources.Source{}})
}

// InitMetrics initializes the metrics configuration.
func (cmd *ConfigInitCommand) InitMetrics() (err error) {
	ctx := context.Background()
	if cmd.owner.IsPgConnStr(cmd.owner.Metrics.Metrics) {
		return cmd.owner.InitMetricReader(ctx)
	}
	reader, _ := metrics.NewYAMLMetricReaderWriter(ctx, "")
	writer, _ := metrics.NewYAMLMetricReaderWriter(ctx, cmd.owner.Metrics.Metrics)
	defMetrics, _ := reader.GetMetrics()
	return writer.WriteMetrics(defMetrics)
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
	// for now only postgres configuration is upgradable
	if opts.IsPgConnStr(opts.Metrics.Metrics) && opts.IsPgConnStr(opts.Sources.Sources) {
		err = opts.InitMetricReader(context.Background())
		if err != nil {
			opts.CompleteCommand(ExitCodeConfigError)
			return
		}
		if m, ok := opts.MetricsReaderWriter.(metrics.Migrator); ok {
			err = m.Migrate()
			opts.CompleteCommand(map[bool]int32{
				true:  ExitCodeOK,
				false: ExitCodeConfigError,
			}[err == nil])
			return
		}
	}
	opts.CompleteCommand(ExitCodeConfigError)
	return errors.New("configuration storage does not support upgrade")
}
