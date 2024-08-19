package config

import (
	"context"
	"fmt"
	"math"
	"slices"

	"golang.org/x/exp/maps"
)

type MetricCommand struct {
	owner     *Options
	PrintInit MetricPrintInitCommand `command:"print-init" description:"Get and print init SQL for a given metric or preset"`
	PrintSQL  MetricPrintSQLCommand  `command:"print-sql" description:"Get and print SQL for a given metric"`
}

func NewMetricCommand(owner *Options) *MetricCommand {
	return &MetricCommand{
		owner:     owner,
		PrintInit: MetricPrintInitCommand{owner: owner},
		PrintSQL:  MetricPrintSQLCommand{owner: owner},
	}
}

type MetricPrintInitCommand struct {
	owner *Options
}

func (cmd *MetricPrintInitCommand) Execute(args []string) error {
	err := cmd.owner.InitMetricReader(context.Background())
	if err != nil {
		return err
	}
	metrics, err := cmd.owner.MetricsReaderWriter.GetMetrics()
	if err != nil {
		return err
	}
	for _, name := range args {
		if preset, ok := metrics.PresetDefs[name]; ok {
			args = append(args, maps.Keys(preset.Metrics)...)
		}
	}
	slices.Sort(args)
	args = slices.Compact(args)
	for _, mname := range args {
		if m, ok := metrics.MetricDefs[mname]; ok && m.InitSQL != "" {
			fmt.Println("-- ", mname)
			fmt.Println(m.InitSQL)
		}
	}
	cmd.owner.CompleteCommand(ExitCodeOK)
	return nil
}

type MetricPrintSQLCommand struct {
	owner   *Options
	Version int `short:"v" long:"version" description:"PostgreSQL version to get SQL for"`
}

func (cmd *MetricPrintSQLCommand) Execute(args []string) error {
	err := cmd.owner.InitMetricReader(context.Background())
	if err != nil {
		return err
	}
	metrics, err := cmd.owner.MetricsReaderWriter.GetMetrics()
	if err != nil {
		return err
	}
	if cmd.Version == 0 {
		cmd.Version = math.MaxInt32
	}
	for _, name := range args {
		if m, ok := metrics.MetricDefs[name]; ok {
			fmt.Println("-- ", name)
			fmt.Println(m.GetSQL(cmd.Version))
		}
	}
	cmd.owner.CompleteCommand(ExitCodeOK)
	return nil
}
