package reaper

import (
	"context"
	"os"

	"github.com/cybertec-postgresql/pgwatch/metrics"
	"github.com/cybertec-postgresql/pgwatch/metrics/psutil"
)

func DoesEmergencyTriggerfileExist(fname string) bool {
	// Main idea of the feature is to be able to quickly free monitored DBs / network of any extra "monitoring effect" load.
	// In highly automated K8s / IaC environments such a temporary change might involve pull requests, peer reviews, CI/CD etc
	// which can all take too long vs "exec -it pgwatch3-pod -- touch /tmp/pgwatch3-emergency-pause".
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

var directlyFetchableOSMetrics = map[string]bool{metricPsutilCPU: true, metricPsutilDisk: true, metricPsutilDiskIoTotal: true, metricPsutilMem: true, metricCPULoad: true}

func IsDirectlyFetchableMetric(metric string) bool {
	_, ok := directlyFetchableOSMetrics[metric]
	return ok
}

func FetchStatsDirectlyFromOS(ctx context.Context, msg MetricFetchConfig, vme MonitoredDatabaseSettings, mvp metrics.Metric) ([]metrics.MeasurementMessage, error) {
	var data []map[string]any
	var err error

	if msg.MetricName == metricCPULoad { // could function pointers work here?
		data, err = psutil.GetLoadAvgLocal()
	} else if msg.MetricName == metricPsutilCPU {
		data, err = psutil.GetGoPsutilCPU(msg.Interval)
	} else if msg.MetricName == metricPsutilDisk {
		data, err = GetGoPsutilDiskPG(ctx, msg.DBUniqueName)
	} else if msg.MetricName == metricPsutilDiskIoTotal {
		data, err = psutil.GetGoPsutilDiskTotals()
	} else if msg.MetricName == metricPsutilMem {
		data, err = psutil.GetGoPsutilMem()
	}
	if err != nil {
		return nil, err
	}

	msm, err := DatarowsToMetricstoreMessage(data, msg, vme, mvp)
	if err != nil {
		return nil, err
	}
	return []metrics.MeasurementMessage{msm}, nil
}

// data + custom tags + counters
func DatarowsToMetricstoreMessage(data metrics.Measurements, msg MetricFetchConfig, vme MonitoredDatabaseSettings, mvp metrics.Metric) (metrics.MeasurementMessage, error) {
	md, err := GetMonitoredDatabaseByUniqueName(msg.DBUniqueName)
	if err != nil {
		return metrics.MeasurementMessage{}, err
	}
	return metrics.MeasurementMessage{
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
