package reaper

import (
	"context"
	"os"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"
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
	sqlPgDirs = `select 
current_setting('data_directory') as dd, 
current_setting('log_directory') as ld, 
current_setting('server_version_num')::int as pgver`
	sqlTsDirs = `select 
spcname::text as name, 
pg_catalog.pg_tablespace_location(oid) as location 
from pg_catalog.pg_tablespace 
where not spcname like any(array[E'pg\\_%'])`
)

var directlyFetchableOSMetrics = map[string]bool{metricPsutilCPU: true, metricPsutilDisk: true, metricPsutilDiskIoTotal: true, metricPsutilMem: true, metricCPULoad: true}

func IsDirectlyFetchableMetric(metric string) bool {
	_, ok := directlyFetchableOSMetrics[metric]
	return ok
}

func FetchStatsDirectlyFromOS(ctx context.Context, msg MetricFetchConfig, vme MonitoredDatabaseSettings, mvp metrics.Metric) (*metrics.MeasurementEnvelope, error) {
	var data, dataDirs, dataTblspDirs metrics.Measurements
	var err error

	switch msg.MetricName {
	case metricCPULoad:
		data, err = GetLoadAvgLocal()
	case metricPsutilCPU:
		data, err = GetGoPsutilCPU(msg.Interval)
	case metricPsutilDisk:
		if dataDirs, err = QueryMeasurements(ctx, msg.DBUniqueName, sqlPgDirs); err != nil {
			return nil, err
		}
		if dataTblspDirs, err = QueryMeasurements(ctx, msg.DBUniqueName, sqlTsDirs); err != nil {
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

	msm, err := DataRowsToMeasurementEnvelope(data, msg, vme, mvp)
	if err != nil {
		return nil, err
	}
	return &msm, nil
}

// data + custom tags + counters
func DataRowsToMeasurementEnvelope(data metrics.Measurements, msg MetricFetchConfig, vme MonitoredDatabaseSettings, mvp metrics.Metric) (metrics.MeasurementEnvelope, error) {
	md, err := GetMonitoredDatabaseByUniqueName(msg.DBUniqueName)
	if err != nil {
		return metrics.MeasurementEnvelope{}, err
	}
	return metrics.MeasurementEnvelope{
		DBName:           msg.DBUniqueName,
		SourceType:       string(msg.Source),
		MetricName:       msg.MetricName,
		CustomTags:       md.CustomTags,
		Data:             data,
		MetricDef:        mvp,
		RealDbname:       vme.RealDbname,
		SystemIdentifier: vme.SystemIdentifier,
	}, nil
}
