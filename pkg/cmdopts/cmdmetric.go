package cmdopts

import (
	"context"
	"fmt"
	"math"

	"gopkg.in/yaml.v3"
)

type MetricCommand struct {
	owner     *Options
	PrintInit MetricPrintInitCommand `command:"print-init" description:"Get and print init SQL for a given metric or preset"`
	PrintSQL  MetricPrintSQLCommand  `command:"print-sql" description:"Get and print SQL for a given metric"`
	List      MetricListCommand      `command:"list" description:"List available metrics and presets"`
}

func NewMetricCommand(owner *Options) *MetricCommand {
	return &MetricCommand{
		owner:     owner,
		PrintInit: MetricPrintInitCommand{owner: owner},
		PrintSQL:  MetricPrintSQLCommand{owner: owner},
		List:      MetricListCommand{owner: owner},
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
	metrics, err = metrics.FilterByNames(args)
	if err != nil {
		return err
	}
	w := cmd.owner.OutputWriter
	for mname, mdef := range metrics.MetricDefs {
		if mdef.InitSQL > "" {
			fmt.Fprintln(w, "--", mname)
			fmt.Fprintln(w, mdef.InitSQL)
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
	metrics, err = metrics.FilterByNames(args)
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
				fmt.Fprintln(w, "--", name)
				fmt.Fprintln(w, v)
			}
		}
	}
	cmd.owner.CompleteCommand(ExitCodeOK)
	return nil
}

type MetricListCommand struct {
	owner *Options
}

func (cmd *MetricListCommand) Execute(args []string) error {
	err := cmd.owner.InitMetricReader(context.Background())
	if err != nil {
		return err
	}
	allMetrics, err := cmd.owner.MetricsReaderWriter.GetMetrics()
	if err != nil {
		return err
	}
	result, err := allMetrics.FilterByNames(args)
	if err != nil {
		return err
	}
	w := cmd.owner.OutputWriter

	yamlData, _ := yaml.Marshal(result)
	fmt.Fprint(w, string(yamlData))
	cmd.owner.CompleteCommand(ExitCodeOK)
	return nil
}
