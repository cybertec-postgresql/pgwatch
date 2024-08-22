package cmdopts

import (
	"context"
	"errors"
	"fmt"

	"github.com/cybertec-postgresql/pgwatch3/metrics"
	"github.com/cybertec-postgresql/pgwatch3/sources"
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

func (cmd *ConfigInitCommand) Execute([]string) error {
	if len(cmd.owner.Sources.Sources)+len(cmd.owner.Metrics.Metrics) == 0 {
		return errors.New("both --sources and --metrics are empty")
	}
	if cmd.owner.Metrics.Metrics > "" {
		cmd.InitMetrics()
	}
	if cmd.owner.Sources.Sources > "" && cmd.owner.Metrics.Metrics != cmd.owner.Sources.Sources {
		cmd.InitSources()
	}
	cmd.owner.CommandCompleted = true
	return nil
}

func (cmd *ConfigInitCommand) InitSources() {
	ctx := context.Background()
	kind, err := cmd.owner.GetConfigKind(cmd.owner.Sources.Sources)
	if err != nil {
		cmd.owner.CompleteCommand(ExitCodeConfigError)
		return
	}
	switch kind {
	case ConfigPgURL:
		if err = cmd.owner.InitSourceReader(ctx); err != nil {
			cmd.owner.CompleteCommand(ExitCodeConfigError)
			return
		}
	case ConfigFile:
		writer, _ := sources.NewYAMLSourcesReaderWriter(ctx, cmd.owner.Sources.Sources)
		if err = writer.WriteSources(sources.Sources{sources.Source{}}); err != nil {
			fmt.Printf("cannot init sources: %s", err)
			cmd.owner.CompleteCommand(ExitCodeConfigError)
			return
		}
	}
	fmt.Println("Sources initialized")
}

func (cmd *ConfigInitCommand) InitMetrics() {
	ctx := context.Background()
	kind, err := cmd.owner.GetConfigKind(cmd.owner.Metrics.Metrics)
	if err != nil {
		cmd.owner.CompleteCommand(ExitCodeConfigError)
		return
	}
	switch kind {
	case ConfigPgURL:
		if err = cmd.owner.InitMetricReader(ctx); err != nil {
			cmd.owner.CompleteCommand(ExitCodeConfigError)
			return
		}
	case ConfigFile:
		reader, _ := metrics.NewYAMLMetricReaderWriter(ctx, "")
		writer, _ := metrics.NewYAMLMetricReaderWriter(ctx, cmd.owner.Metrics.Metrics)
		defMetrics, _ := reader.GetMetrics()
		if err = writer.WriteMetrics(defMetrics); err != nil {
			fmt.Printf("cannot init metrics: %s", err)
			cmd.owner.CompleteCommand(ExitCodeConfigError)
			return
		}
	}
	fmt.Println("Metrics initialized")
}

type ConfigUpgradeCommand struct {
	owner *Options
}

func (cmd *ConfigUpgradeCommand) Execute([]string) (err error) {
	err = cmd.owner.InitConfigReaders(context.Background())
	if err != nil {
		cmd.owner.CompleteCommand(ExitCodeConfigError)
		fmt.Println(err)
		return
	}
	if m, ok := cmd.owner.MetricsReaderWriter.(metrics.Migrator); ok {
		if err = m.Migrate(); err != nil {
			cmd.owner.CompleteCommand(ExitCodeUpgradeError)
			fmt.Printf("cannot upgrade metrics: %s", err)
			return
		}
		cmd.owner.CompleteCommand(ExitCodeOK)
		fmt.Println("Configuration schema upgraded")
		return
	}
	// if s, ok := cmd.owner.SourcesReaderWriter.(sources.Migrator); ok {
	// 	if err = cmd.UpgradeConfiguration(s); err != nil {
	// 		cmd.owner.CompleteCommand(ExitCodeUpgradeError)
	// 		fmt.Printf("cannot upgrade sources: %s", err)
	// 		return
	// 	}
	// }
	return errors.New("Configuration storage does not support upgrade")
}
