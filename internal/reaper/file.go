package reaper

import (
	"context"
	"os"

	"slices"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/sources"
)

func DoesEmergencyTriggerfileExist(fname string) bool {
	// Main idea of the feature is to be able to quickly free monitored DBs / network of any extra "monitoring effect" load.
	// In highly automated K8s / IaC environments such a temporary change might involve pull requests, peer reviews, CI/CD etc
	// which can all take too long vs "exec -it pgwatch-pod -- touch /tmp/pgwatch-emergency-pause".
	// After creating the file it can still take up to --servers-refresh-loop-seconds (2min def.) for change to take effect!
	if fname == "" {
		return false
	}
	_, err := os.Stat(fname)
	return err == nil
}

const (
	metricCPULoad           = "cpu_load"
	metricPsutilCPU         = "psutil_cpu"
	metricPsutilDisk        = "psutil_disk"
	metricPsutilDiskIoTotal = "psutil_disk_io_total"
	metricPsutilMem         = "psutil_mem"
)

const (
	sqlPgDirs = `select current_setting('data_directory') as dd, current_setting('log_directory') as ld`
	sqlTsDirs = `select spcname::text as name, pg_catalog.pg_tablespace_location(oid) as location from pg_catalog.pg_tablespace where not spcname like any(array[E'pg\\_%'])`
)

var directlyFetchableOSMetrics = []string{metricPsutilCPU, metricPsutilDisk, metricPsutilDiskIoTotal, metricPsutilMem, metricCPULoad}

func IsDirectlyFetchableMetric(metric string) bool {
	return slices.Contains(directlyFetchableOSMetrics, metric)
}

func (r *Reaper) FetchStatsDirectlyFromOS(ctx context.Context, md *sources.SourceConn, metricName string) (*metrics.MeasurementEnvelope, error) {
	var data, dataDirs, dataTblspDirs metrics.Measurements
	var err error

	switch metricName {
	case metricCPULoad:
		data, err = GetLoadAvgLocal()
	case metricPsutilCPU:
		data, err = GetGoPsutilCPU(md.GetMetricInterval(metricName))
	case metricPsutilDisk:
		if dataDirs, err = QueryMeasurements(ctx, md, sqlPgDirs); err != nil {
			return nil, err
		}
		if dataTblspDirs, err = QueryMeasurements(ctx, md, sqlTsDirs); err != nil {
			return nil, err
		}
		data, err = GetGoPsutilDiskPG(dataDirs, dataTblspDirs)
	case metricPsutilDiskIoTotal:
		data, err = GetGoPsutilDiskTotals()
	case metricPsutilMem:
		data, err = GetGoPsutilMem()
	}
	if err != nil {
		return nil, err
	}
	return &metrics.MeasurementEnvelope{
		DBName:     md.Name,
		MetricName: metricName,
		CustomTags: md.CustomTags,
		Data:       data,
	}, nil
}
