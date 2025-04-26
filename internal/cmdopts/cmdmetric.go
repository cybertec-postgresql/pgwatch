package cmdopts

import (
	"context"
	"fmt"
	"math"
	"slices"
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
	w := cmd.owner.OutputWriter
	for _, name := range args {
		if preset, ok := metrics.PresetDefs[name]; ok {
			for k := range preset.Metrics {
				args = append(args, k)
			}
		}
	}
	slices.Sort(args)
	args = slices.Compact(args)
	for _, mname := range args {
		if m, ok := metrics.MetricDefs[mname]; ok && m.InitSQL != "" {

			fmt.Fprintln(w, "--", mname)
			fmt.Fprintln(w, m.InitSQL)
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
	w := cmd.owner.OutputWriter
	if cmd.Version == 0 {
		cmd.Version = math.MaxInt32
	}
	for _, name := range args {
		if m, ok := metrics.MetricDefs[name]; ok {
			if v := m.GetSQL(cmd.Version); v > "" {
				fmt.Fprintln(w, "--", name, v)
			}
		}
	}
	cmd.owner.CompleteCommand(ExitCodeOK)
	return nil
}
