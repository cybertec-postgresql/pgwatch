package reaper

import (
	"context"
	"os"

	"slices"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/sources"
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

const sqlPgDirs = `select name, path from 
(values 
	('data_directory', current_setting('data_directory')),
	('pg_wal', current_setting('data_directory')||'/pg_wal'),
	('log_directory', case 
        when current_setting('log_directory') ~ '/.+' then current_setting('log_directory') 
        else current_setting('data_directory') || '/' || current_setting('log_directory') 
    end)) as d(name, path)
union all
select spcname::text, pg_catalog.pg_tablespace_location(oid)
from pg_catalog.pg_tablespace where spcname !~ 'pg_.+'`

var directlyFetchableOSMetrics = []string{metricPsutilCPU, metricPsutilDisk, metricPsutilDiskIoTotal, metricPsutilMem, metricCPULoad}

func IsDirectlyFetchableMetric(md *sources.SourceConn, metric string) bool {
	return slices.Contains(directlyFetchableOSMetrics, metric) && md.IsClientOnSameHost()
}

func (r *Reaper) FetchStatsDirectlyFromOS(ctx context.Context, md *sources.SourceConn, metricName string) (*metrics.MeasurementEnvelope, error) {
	var data, pgDirs metrics.Measurements
	var err error

	switch metricName {
	case metricCPULoad:
		data, err = GetLoadAvgLocal()
	case metricPsutilCPU:
		data, err = GetGoPsutilCPU(md.GetMetricInterval(metricName))
	case metricPsutilDisk:
		if pgDirs, err = QueryMeasurements(ctx, md, sqlPgDirs); err != nil {
			return nil, err
		}
		data, err = GetGoPsutilDiskPG(pgDirs)
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
