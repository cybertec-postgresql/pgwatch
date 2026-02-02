package cmdopts

import (
	"context"
	"fmt"
	"io"
	"math"
	"slices"
)

type MetricCommand struct {
	owner     *Options
	PrintInit MetricPrintInitCommand `command:"print-init" description:"Get and print init SQL for a given metric or preset"`
	PrintSQL  MetricPrintSQLCommand  `command:"print-sql" description:"Get and print SQL for a given metric"`
	List      MetricListCommand      `command:"list" description:"Get and print the list of available metrics"`
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
type MetricPrintInitResult struct {
	Name    string `json:"name"`
	InitSQL string `json:"init_sql"`
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
			for k := range preset.Metrics {
				args = append(args, k)
			}
		}
	}
	slices.Sort(args)
	args = slices.Compact(args)
	results := make([]MetricPrintInitResult, 0, len(args))
	for _, mname := range args {
		if m, ok := metrics.MetricDefs[mname]; ok && m.InitSQL != "" {
			//
			// fmt.Fprintln(w, "--", mname)
			// fmt.Fprintln(w, m.InitSQL)
			results = append(results, MetricPrintInitResult{Name: mname, InitSQL: m.InitSQL})
		}
	}
	cmd.owner.Print(results)
	cmd.owner.CompleteCommand(ExitCodeOK)
	return nil
}

type MetricPrintSQLCommand struct {
	owner   *Options
	Version int `short:"v" long:"version" description:"PostgreSQL version to get SQL for"`
}

type MetricInitSQLResult struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}
type MetricInitSQLResults []MetricInitSQLResult

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
	results := make(MetricInitSQLResults, 0, len(args))
	for _, name := range args {
		if m, ok := metrics.MetricDefs[name]; ok {
			if v := m.GetSQL(cmd.Version); v > "" {
				// fmt.Fprintln(w, "--", name)
				// fmt.Fprintln(w, v)
				results = append(results, MetricInitSQLResult{Name: name, Version: fmt.Sprintf("%d", cmd.Version)})
			}
		}
	}
	cmd.owner.Print(results)
	cmd.owner.CompleteCommand(ExitCodeOK)
	return nil
}

type MetricListResult struct {
	Metrics []string `json:"metrics"`
	Presets []string `json:"presets"`
}

func (r MetricListResult) PrintText(w io.Writer) error {
	fmt.Fprintln(w, "Metrics:")
	for _, k := range r.Metrics {
		fmt.Fprintln(w, k)
	}
	fmt.Fprintln(w, "Presets:")
	for _, k := range r.Presets {
		fmt.Fprintln(w, k)
	}
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
	metrics, err := cmd.owner.MetricsReaderWriter.GetMetrics()
	if err != nil {
		return err
	}
	var result MetricListResult
	for k := range metrics.MetricDefs {
		result.Metrics = append(result.Metrics, k)
	}
	slices.Sort(result.Metrics)
	for k := range metrics.PresetDefs {
		result.Presets = append(result.Presets, k)
	}
	slices.Sort(result.Presets)
	cmd.owner.Print(result)
	cmd.owner.CompleteCommand(ExitCodeOK)
	return nil
}
