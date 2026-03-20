package reaper

import (
	"context"
	"runtime"
	"slices"
	"strings"
	"time"

	"sync/atomic"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/cmdopts"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/log"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/sinks"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/sources"
)

const (
	specialMetricChangeEvents         = "change_events"
	specialMetricServerLogEventCounts = "server_log_event_counts"
	specialMetricInstanceUp           = "instance_up"
)

var specialMetrics = map[string]bool{specialMetricChangeEvents: true, specialMetricServerLogEventCounts: true}

var hostLastKnownStatusInRecovery = make(map[string]bool) // isInRecovery
var metricsConfig metrics.MetricIntervals                 // set to host.Metrics or host.MetricsStandby (in case optional config defined and in recovery state
var metricDefs = NewConcurrentMetricDefs()

// Reaper is the struct that responsible for fetching metrics measurements from the sources and storing them to the sinks
type Reaper struct {
	*cmdopts.Options
	ready                atomic.Bool
	measurementCh        chan metrics.MeasurementEnvelope
	measurementCache     *InstanceMetricCache
	logger               log.Logger
	monitoredSources     sources.SourceConns
	prevLoopMonitoredDBs sources.SourceConns
	cancelFuncs          map[string]context.CancelFunc // [sourceName]cancel() — one per source
	sourceReapers        map[string]*SourceReaper      // [sourceName] — active SourceReaper instances
}

// NewReaper creates a new Reaper instance
func NewReaper(ctx context.Context, opts *cmdopts.Options) (r *Reaper) {
	return &Reaper{
		Options:              opts,
		measurementCh:        make(chan metrics.MeasurementEnvelope, 256),
		measurementCache:     NewInstanceMetricCache(),
		logger:               log.GetLogger(ctx),
		monitoredSources:     make(sources.SourceConns, 0),
		prevLoopMonitoredDBs: make(sources.SourceConns, 0),
		cancelFuncs:          make(map[string]context.CancelFunc), // [sourceName]cancel()
		sourceReapers:        make(map[string]*SourceReaper),
	}
}

// Ready() returns true if the service is healthy and operating correctly
func (r *Reaper) Ready() bool {
	return r.ready.Load()
}

func (r *Reaper) PrintMemStats() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	bToKb := func(b uint64) uint64 {
		return b / 1024
	}
	r.logger.Debugf("Alloc: %d Kb, TotalAlloc: %d Kb, Sys: %d Kb, NumGC: %d, HeapAlloc: %d Kb, HeapSys: %d Kb",
		bToKb(m.Alloc), bToKb(m.TotalAlloc), bToKb(m.Sys), m.NumGC, bToKb(m.HeapAlloc), bToKb(m.HeapSys))
}

// Reap() starts the main monitoring loop. It is responsible for fetching metrics measurements
// from the sources and storing them to the sinks. It also manages the lifecycle of
// the metric gatherers. In case of a source or metric definition change, it will
// start or stop the gatherers accordingly.
func (r *Reaper) Reap(ctx context.Context) {
	var err error
	logger := r.logger

	go r.WriteMeasurements(ctx)

	r.ready.Store(true)

	for { //main loop
		if r.Logging.LogLevel == "debug" {
			r.PrintMemStats()
		}
		if err = r.LoadSources(ctx); err != nil {
			logger.WithError(err).Error("could not refresh active sources, using last valid cache")
		}
		if err = r.LoadMetrics(); err != nil {
			logger.WithError(err).Error("could not refresh metric definitions, using last valid cache")
		}

		// UpdateMonitoredDBCache(r.monitoredSources)
		hostsToShutDownDueToRoleChange := make(map[string]bool) // hosts went from master to standby and have "only if master" set
		for _, monitoredSource := range r.monitoredSources {
			srcL := logger.WithField("source", monitoredSource.Name)
			ctx = log.WithLogger(ctx, srcL)

			if monitoredSource.Connect(ctx, r.Sources) != nil {
				r.WriteInstanceDown(monitoredSource)
				srcL.Warning("could not init connection, retrying on next iteration")
				continue
			}

			if err = monitoredSource.FetchRuntimeInfo(ctx, true); err != nil {
				srcL.WithError(err).Error("could not start metric gathering")
				continue
			}
			srcL.WithField("recovery", monitoredSource.IsInRecovery).Infof("Connect OK. Version: %s", monitoredSource.VersionStr)
			if monitoredSource.IsInRecovery && monitoredSource.OnlyIfMaster {
				srcL.Info("not added to monitoring due to 'master only' property")
				if monitoredSource.IsPostgresSource() {
					srcL.Info("to be removed from monitoring due to 'master only' property and status change")
					hostsToShutDownDueToRoleChange[monitoredSource.Name] = true
				}
				continue
			}

			if monitoredSource.IsInRecovery && len(monitoredSource.MetricsStandby) > 0 {
				metricsConfig = monitoredSource.MetricsStandby
			} else {
				metricsConfig = monitoredSource.Metrics
			}

			r.CreateSourceHelpers(ctx, srcL, monitoredSource)

			if monitoredSource.IsPostgresSource() {
				DBSizeMB := monitoredSource.ApproxDbSize / 1048576 // only remove from monitoring when we're certain it's under the threshold
				if DBSizeMB != 0 && DBSizeMB < r.Sources.MinDbSizeMB {
					srcL.Infof("ignored due to the --min-db-size-mb filter, current size %d MB", DBSizeMB)
					hostsToShutDownDueToRoleChange[monitoredSource.Name] = true // for the case when DB size was previosly above the threshold
					continue
				}

				lastKnownStatusInRecovery := hostLastKnownStatusInRecovery[monitoredSource.Name]
				if lastKnownStatusInRecovery != monitoredSource.IsInRecovery {
					if monitoredSource.IsInRecovery && len(monitoredSource.MetricsStandby) > 0 {
						srcL.Warning("Switching metrics collection to standby config...")
						metricsConfig = monitoredSource.MetricsStandby
					} else if !monitoredSource.IsInRecovery {
						srcL.Warning("Switching metrics collection to primary config...")
						metricsConfig = monitoredSource.Metrics
					}
					// else: it already has primary config do nothing + no warn
				}
			}
			hostLastKnownStatusInRecovery[monitoredSource.Name] = monitoredSource.IsInRecovery

			// Sync metric names with sinks for the active config
			for metricName := range metricsConfig {
				mvp, metricDefExists := metricDefs.GetMetricDef(metricName)
				if !metricDefExists {
					epoch, ok := lastSQLFetchError.Load(metricName)
					if !ok || ((time.Now().Unix() - epoch.(int64)) > 3600) {
						srcL.WithField("metric", metricName).Warning("metric definition not found")
						lastSQLFetchError.Store(metricName, time.Now().Unix())
					}
					continue
				}
				metricNameForStorage := metricName
				if _, isSpecialMetric := specialMetrics[metricName]; !isSpecialMetric && mvp.StorageName > "" {
					metricNameForStorage = mvp.StorageName
				}
				if err := r.SinksWriter.SyncMetric(monitoredSource.Name, metricNameForStorage, sinks.AddOp); err != nil {
					srcL.Error(err)
				}
			}

			// Start SourceReaper for this source if not already running
			if _, exists := r.sourceReapers[monitoredSource.Name]; !exists {
				srcL.Info("starting source reaper")
				sr := NewSourceReaper(r, monitoredSource)
				sourceCtx, cancelFunc := context.WithCancel(ctx)
				r.cancelFuncs[monitoredSource.Name] = cancelFunc
				r.sourceReapers[monitoredSource.Name] = sr
				go sr.Run(sourceCtx)
			}
		}

		r.ShutdownOldWorkers(ctx, hostsToShutDownDueToRoleChange)

		r.prevLoopMonitoredDBs = slices.Clone(r.monitoredSources)
		select {
		case <-time.After(time.Second * time.Duration(r.Sources.Refresh)):
			logger.Debugf("wake up after %d seconds", r.Sources.Refresh)
		case <-ctx.Done():
			return
		}
	}
}

// CreateSourceHelpers creates the extensions and metric helpers for the monitored source
func (r *Reaper) CreateSourceHelpers(ctx context.Context, srcL log.Logger, monitoredSource *sources.SourceConn) {
	if r.prevLoopMonitoredDBs.GetMonitoredDatabase(monitoredSource.Name) != nil {
		return // already created
	}
	if !monitoredSource.IsPostgresSource() || monitoredSource.IsInRecovery {
		return // no need to create anything for non-postgres sources
	}

	if r.Sources.TryCreateListedExtsIfMissing > "" {
		srcL.Info("trying to create extensions if missing")
		extsToCreate := strings.Split(r.Sources.TryCreateListedExtsIfMissing, ",")
		extsCreated, err := monitoredSource.TryCreateMissingExtensions(ctx, extsToCreate)
		if err != nil {
			srcL.Warning(err)
		}
		if extsCreated != "" {
			srcL.Infof("%d/%d extensions created: %s", len(extsCreated), len(extsToCreate), extsCreated)
		}
	}

	if r.Sources.CreateHelpers {
		srcL.Info("trying to create helper objects if missing")
		if err := monitoredSource.TryCreateMetricsHelpers(ctx, func(metric string) string {
			if m, ok := metricDefs.GetMetricDef(metric); ok {
				return m.InitSQL
			}
			return ""
		}); err != nil {
			srcL.Warning(err)
		}
	}

}

func (r *Reaper) ShutdownOldWorkers(ctx context.Context, hostsToShutDown map[string]bool) {
	logger := r.logger
	// loop over existing source reapers and stop if DB removed from config
	// or state change makes it uninteresting
	logger.Debug("checking if any workers need to be shut down...")
	for sourceName, cancelFunc := range r.cancelFuncs {
		var dbRemovedFromConfig bool

		_, wholeDbShutDown := hostsToShutDown[sourceName]
		if !wholeDbShutDown {
			md := r.monitoredSources.GetMonitoredDatabase(sourceName)
			if md == nil { // normal removing of DB from config
				dbRemovedFromConfig = true
				logger.Debugf("DB %s removed from config, shutting down source reaper...", sourceName)
			}
		}

		if ctx.Err() != nil || wholeDbShutDown || dbRemovedFromConfig {
			logger.WithField("source", sourceName).Info("stopping source reaper...")
			cancelFunc()
			delete(r.cancelFuncs, sourceName)
			delete(r.sourceReapers, sourceName)
			if err := r.SinksWriter.SyncMetric(sourceName, "", sinks.DeleteOp); err != nil {
				logger.Error(err)
			}
		}
	}

	// Destroy conn pools and metric writers
	r.CloseResourcesForRemovedMonitoredDBs(hostsToShutDown)
}

// LoadSources loads sources from the reader
func (r *Reaper) LoadSources(ctx context.Context) (err error) {
	if DoesEmergencyTriggerfileExist(r.Metrics.EmergencyPauseTriggerfile) {
		r.logger.Warningf("Emergency pause triggerfile detected at %s, ignoring currently configured DBs", r.Metrics.EmergencyPauseTriggerfile)
		r.monitoredSources = make(sources.SourceConns, 0)
		return nil
	}

	var newSrcs sources.SourceConns
	srcs, err := r.SourcesReaderWriter.GetSources()
	if err != nil {
		return err
	}
	srcs = slices.DeleteFunc(srcs, func(s sources.Source) bool {
		return !s.IsEnabled || len(r.Sources.Groups) > 0 && !slices.Contains(r.Sources.Groups, s.Group)
	})
	if newSrcs, err = srcs.ResolveDatabases(); err != nil {
		r.logger.WithError(err).Error("could not resolve databases from sources")
	}

	for i, newMD := range newSrcs {
		md := r.monitoredSources.GetMonitoredDatabase(newMD.Name)
		if md == nil {
			continue
		}
		if md.Equal(newMD.Source) {
			// replace with the existing connection if the source is the same
			newSrcs[i] = md
			continue
		}
		// Source configs changed, stop all running gatherers to trigger a restart
		// TODO: Optimize this for single metric addition/deletion/interval-change cases to not do a full restart
		r.logger.WithField("source", md.Name).Info("Source configs changed, restarting all gatherers...")
		r.ShutdownOldWorkers(ctx, map[string]bool{md.Name: true})
	}
	r.monitoredSources = newSrcs
	r.logger.WithField("sources", len(r.monitoredSources)).Info("sources refreshed")
	return nil
}

// WriteInstanceDown writes instance_up = 0 metric to sinks for the given source
func (r *Reaper) WriteInstanceDown(md *sources.SourceConn) {
	r.measurementCh <- metrics.MeasurementEnvelope{
		DBName:     md.Name,
		MetricName: specialMetricInstanceUp,
		Data: metrics.Measurements{metrics.Measurement{
			metrics.EpochColumnName: time.Now().UnixNano(),
			specialMetricInstanceUp: 0},
		},
	}
}

// GetMeasurementCache returns the instance-level metric cache
func (r *Reaper) GetMeasurementCache(key string) metrics.Measurements {
	return r.measurementCache.Get(key, r.Metrics.CacheAge())
}

// WriteMeasurements() writes the metrics to the sinks
func (r *Reaper) WriteMeasurements(ctx context.Context) {
	var err error
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-r.measurementCh:
			if err = r.SinksWriter.Write(msg); err != nil {
				r.logger.Error(err)
			}
		}
	}
}

func (r *Reaper) AddSysinfoToMeasurements(data metrics.Measurements, md *sources.SourceConn) {
	for _, dr := range data {
		if r.Sinks.RealDbnameField > "" && md.RealDbname > "" {
			dr[r.Sinks.RealDbnameField] = md.RealDbname
		}
		if r.Sinks.SystemIdentifierField > "" && md.SystemIdentifier > "" {
			dr[r.Sinks.SystemIdentifierField] = md.SystemIdentifier
		}
	}
}
