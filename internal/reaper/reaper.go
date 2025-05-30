package reaper

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"sync/atomic"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/cmdopts"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/log"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/sources"
)

var hostLastKnownStatusInRecovery = make(map[string]bool) // isInRecovery
var metricsConfig map[string]float64                      // set to host.Metrics or host.MetricsStandby (in case optional config defined and in recovery state
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
	cancelFuncs          map[string]context.CancelFunc
}

// NewReaper creates a new Reaper instance
func NewReaper(ctx context.Context, opts *cmdopts.Options) (r *Reaper) {
	return &Reaper{
		Options:              opts,
		measurementCh:        make(chan metrics.MeasurementEnvelope, 10000),
		measurementCache:     NewInstanceMetricCache(),
		logger:               log.GetLogger(ctx),
		monitoredSources:     make(sources.SourceConns, 0),
		prevLoopMonitoredDBs: make(sources.SourceConns, 0),
		cancelFuncs:          make(map[string]context.CancelFunc), // [db1+metric1]cancel()
	}
}

// Ready() returns true if the service is healthy and operating correctly
func (r *Reaper) Ready() bool {
	return r.ready.Load()
}

// Reap() starts the main monitoring loop. It is responsible for fetching metrics measurements
// from the sources and storing them to the sinks. It also manages the lifecycle of
// the metric gatherers. In case of a source or metric definition change, it will
// start or stop the gatherers accordingly.
func (r *Reaper) Reap(ctx context.Context) {
	var err error
	logger := r.logger

	go r.WriteMeasurements(ctx)
	go r.WriteMonitoredSources(ctx)

	r.ready.Store(true)

	for { //main loop
		if err = r.LoadSources(); err != nil {
			logger.WithError(err).Error("could not refresh active sources, using last valid cache")
		}
		if err = r.LoadMetrics(); err != nil {
			logger.WithError(err).Error("could not refresh metric definitions, using last valid cache")
		}

		UpdateMonitoredDBCache(r.monitoredSources)
		hostsToShutDownDueToRoleChange := make(map[string]bool) // hosts went from master to standby and have "only if master" set
		for _, monitoredSource := range r.monitoredSources {
			srcL := logger.WithField("source", monitoredSource.Name)

			if monitoredSource.Connect(ctx, r.Sources) != nil {
				srcL.WithError(err).Warning("could not init connection, retrying on next iteration")
				continue
			}

			if err = monitoredSource.FetchRuntimeInfo(ctx, true); err != nil {
				srcL.WithError(err).Error("could not start metric gathering")
				continue
			}
			srcL.WithField("recovery", monitoredSource.IsInRecovery).Infof("Connect OK. Version: %s", monitoredSource.VersionStr)
			if monitoredSource.IsInRecovery && monitoredSource.OnlyIfMaster {
				srcL.Info("not added to monitoring due to 'master only' property")
				continue
			}

			if monitoredSource.IsInRecovery && len(monitoredSource.MetricsStandby) > 0 {
				metricsConfig = monitoredSource.MetricsStandby
			} else {
				metricsConfig = monitoredSource.Metrics
			}
			hostLastKnownStatusInRecovery[monitoredSource.Name] = monitoredSource.IsInRecovery

			r.CreateSourceHelpers(ctx, srcL, monitoredSource)

			if monitoredSource.IsPostgresSource() {
				if r.Sources.MinDbSizeMB >= 8 { // an empty DB is a bit less than 8MB
					DBSizeMB := monitoredSource.ApproxDbSize / 1048576 // only remove from monitoring when we're certain it's under the threshold
					if DBSizeMB != 0 && DBSizeMB < r.Sources.MinDbSizeMB {
						srcL.Infof("ignored due to the --min-db-size-mb filter, current size %d MB", DBSizeMB)
						hostsToShutDownDueToRoleChange[monitoredSource.Name] = true // for the case when DB size was previosly above the threshold
						continue
					}
				}

				lastKnownStatusInRecovery := hostLastKnownStatusInRecovery[monitoredSource.Name]
				if monitoredSource.IsInRecovery && monitoredSource.OnlyIfMaster {
					srcL.Info("to be removed from monitoring due to 'master only' property and status change")
					hostsToShutDownDueToRoleChange[monitoredSource.Name] = true
					continue
				} else if lastKnownStatusInRecovery != monitoredSource.IsInRecovery {
					if monitoredSource.IsInRecovery && len(monitoredSource.MetricsStandby) > 0 {
						srcL.Warning("Switching metrics collection to standby config...")
						metricsConfig = monitoredSource.MetricsStandby
						hostLastKnownStatusInRecovery[monitoredSource.Name] = true
					} else {
						srcL.Warning("Switching metrics collection to primary config...")
						metricsConfig = monitoredSource.Metrics
						hostLastKnownStatusInRecovery[monitoredSource.Name] = false
					}
				}

			}

			for metricName, interval := range metricsConfig {
				metric := metricName
				metricDefExists := false
				var mvp metrics.Metric

				if strings.HasPrefix(metric, recoPrefix) {
					metric = recoMetricName
					metricDefExists = true
				} else {
					mvp, metricDefExists = metricDefs.GetMetricDef(metric)
				}

				dbMetric := monitoredSource.Name + dbMetricJoinStr + metric
				_, cancelFuncExists := r.cancelFuncs[dbMetric]

				if metricDefExists && !cancelFuncExists { // initialize a new per db/per metric control channel
					if interval > 0 {
						srcL.WithField("metric", metric).WithField("interval", interval).Info("starting gatherer")
						metricCtx, cancelFunc := context.WithCancel(ctx)
						r.cancelFuncs[dbMetric] = cancelFunc

						metricNameForStorage := metricName
						if _, isSpecialMetric := specialMetrics[metricName]; !isSpecialMetric && mvp.StorageName > "" {
							metricNameForStorage = mvp.StorageName
						}

						if err := r.SinksWriter.SyncMetric(monitoredSource.Name, metricNameForStorage, "add"); err != nil {
							srcL.Error(err)
						}

						go r.reapMetricMeasurements(metricCtx, monitoredSource, metric)
					}
				} else if (!metricDefExists && cancelFuncExists) || interval <= 0 {
					// metric definition files were recently removed or interval set to zero
					if cancelFunc, isOk := r.cancelFuncs[dbMetric]; isOk {
						cancelFunc()
					}
					srcL.WithField("metric", metric).Warning("shutting down gatherer...")
					delete(r.cancelFuncs, dbMetric)
				} else if !metricDefExists {
					epoch, ok := lastSQLFetchError.Load(metric)
					if !ok || ((time.Now().Unix() - epoch.(int64)) > 3600) { // complain only 1x per hour
						srcL.WithField("metric", metric).Warning("metric definition not found")
						lastSQLFetchError.Store(metric, time.Now().Unix())
					}
				}
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
		extsCreated := TryCreateMissingExtensions(ctx, monitoredSource.Name, extsToCreate, monitoredSource.Extensions)
		srcL.Infof("%d/%d extensions created based on --try-create-listed-exts-if-missing input %v", len(extsCreated), len(extsToCreate), extsCreated)
	}

	if r.Sources.CreateHelpers {
		srcL.Info("trying to create helper objects if missing")
		if err := TryCreateMetricsFetchingHelpers(ctx, monitoredSource); err != nil {
			srcL.WithError(err).Warning("failed to create helper functions")
		}
	}

}

func (r *Reaper) ShutdownOldWorkers(ctx context.Context, hostsToShutDownDueToRoleChange map[string]bool) {
	logger := r.logger
	// loop over existing channels and stop workers if DB or metric removed from config
	// or state change makes it uninteresting
	logger.Debug("checking if any workers need to be shut down...")
	for dbMetric, cancelFunc := range r.cancelFuncs {
		var currentMetricConfig map[string]float64
		var md *sources.SourceConn
		var ok, dbRemovedFromConfig bool
		singleMetricDisabled := false
		splits := strings.Split(dbMetric, dbMetricJoinStr)
		db := splits[0]
		metric := splits[1]

		_, wholeDbShutDownDueToRoleChange := hostsToShutDownDueToRoleChange[db]
		if !wholeDbShutDownDueToRoleChange {
			monitoredDbCacheLock.RLock()
			md, ok = monitoredDbCache[db]
			monitoredDbCacheLock.RUnlock()
			if !ok { // normal removing of DB from config
				dbRemovedFromConfig = true
				logger.Debugf("DB %s removed from config, shutting down all metric worker processes...", db)
			}
		}

		if !(wholeDbShutDownDueToRoleChange || dbRemovedFromConfig) { // maybe some single metric was disabled
			if md.IsInRecovery && len(md.MetricsStandby) > 0 {
				currentMetricConfig = md.MetricsStandby
			} else {
				currentMetricConfig = md.Metrics
			}
			interval, isMetricActive := currentMetricConfig[metric]
			singleMetricDisabled = !isMetricActive || interval <= 0
		}

		if ctx.Err() != nil || wholeDbShutDownDueToRoleChange || dbRemovedFromConfig || singleMetricDisabled {
			logger.WithField("source", db).WithField("metric", metric).Info("stopping gatherer...")
			cancelFunc()
			delete(r.cancelFuncs, dbMetric)
			if err := r.SinksWriter.SyncMetric(db, metric, "remove"); err != nil {
				logger.Error(err)
			}
		}
	}

	// Destroy conn pools and metric writers
	r.CloseResourcesForRemovedMonitoredDBs(hostsToShutDownDueToRoleChange)
}

func (r *Reaper) reapMetricMeasurements(ctx context.Context, md *sources.SourceConn, metricName string) {
	hostState := make(map[string]map[string]string)
	var lastUptimeS int64 = -1 // used for "server restarted" event detection
	var lastErrorNotificationTime time.Time
	var err error
	var ok bool

	failedFetches := 0
	lastDBVersionFetchTime := time.Unix(0, 0) // check DB ver. ev. 5 min

	l := r.logger.WithField("source", md.Name).WithField("metric", metricName)
	if metricName == specialMetricServerLogEventCounts {
		metrics.ParseLogs(ctx, md, md.RealDbname, md.GetMetricInterval(metricName), r.measurementCh) // no return
		return
	}

	for {
		interval := md.GetMetricInterval(metricName)
		if lastDBVersionFetchTime.Add(time.Minute * time.Duration(5)).Before(time.Now()) {
			// in case of errors just ignore metric "disabled" time ranges
			if err = md.FetchRuntimeInfo(ctx, false); err != nil {
				lastDBVersionFetchTime = time.Now()
			}

			if _, ok = metricDefs.GetMetricDef(metricName); !ok {
				l.Error("Could not get metric version properties")
				return
			}
		}

		var metricStoreMessages *metrics.MeasurementEnvelope

		// 1st try local overrides for some metrics if operating in push mode
		if r.Metrics.DirectOSStats && IsDirectlyFetchableMetric(metricName) {
			metricStoreMessages, err = r.FetchStatsDirectlyFromOS(ctx, md, metricName)
			if err != nil {
				l.WithError(err).Errorf("Could not reader metric directly from OS")
			}
		}
		t1 := time.Now()
		if metricStoreMessages == nil {
			metricStoreMessages, err = r.FetchMetric(ctx, md, metricName, hostState)
		}

		if time.Since(t1) > (time.Second * time.Duration(interval)) {
			l.Warningf("Total fetching time of %v bigger than %vs interval", time.Since(t1), interval)
		}

		if err != nil {
			failedFetches++
			// complain only 1x per 10min per host/metric...
			if time.Since(lastErrorNotificationTime) > time.Minute*10 {
				l.WithError(err).WithField("count", failedFetches).Error("failed to fetch metric data")
				lastErrorNotificationTime = time.Now()
			}
		} else if metricStoreMessages != nil && len(metricStoreMessages.Data) > 0 {
			r.measurementCh <- *metricStoreMessages
			// pick up "server restarted" events here to avoid doing extra selects from CheckForPGObjectChangesAndStore code
			if metricName == "db_stats" {
				postmasterUptimeS, ok := (metricStoreMessages.Data)[0]["postmaster_uptime_s"]
				if ok {
					if lastUptimeS != -1 {
						if postmasterUptimeS.(int64) < lastUptimeS { // restart (or possibly also failover when host is routed) happened
							message := "Detected server restart (or failover)"
							l.Warning(message)
							detectedChangesSummary := make(metrics.Measurements, 0)
							entry := metrics.NewMeasurement(metricStoreMessages.Data.GetEpoch())
							entry["details"] = message
							detectedChangesSummary = append(detectedChangesSummary, entry)
							r.measurementCh <- metrics.MeasurementEnvelope{
								DBName:     md.Name,
								SourceType: string(md.Kind),
								MetricName: "object_changes",
								Data:       detectedChangesSummary,
								CustomTags: metricStoreMessages.CustomTags,
							}
						}
					}
					lastUptimeS = postmasterUptimeS.(int64)
				}
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second * time.Duration(interval)):
			// continue
		}
	}
}

// LoadSources loads sources from the reader
func (r *Reaper) LoadSources() (err error) {
	if DoesEmergencyTriggerfileExist(r.Metrics.EmergencyPauseTriggerfile) {
		r.logger.Warningf("Emergency pause triggerfile detected at %s, ignoring currently configured DBs", r.Metrics.EmergencyPauseTriggerfile)
		r.monitoredSources = make([]*sources.SourceConn, 0)
		return nil
	}
	if r.monitoredSources, err = r.monitoredSources.SyncFromReader(r.SourcesReaderWriter); err != nil {
		return err
	}
	r.logger.WithField("sources", len(r.monitoredSources)).Info("sources refreshed")
	return nil
}

// WriteMonitoredSources writes actively monitored DBs listing to sinks
// every monitoredDbsDatastoreSyncIntervalSeconds (default 10min)
func (r *Reaper) WriteMonitoredSources(ctx context.Context) {
	for {
		if len(monitoredDbCache) > 0 {
			now := time.Now().UnixNano()
			monitoredDbCacheLock.RLock()
			for _, mdb := range monitoredDbCache {
				db := metrics.NewMeasurement(now)
				db["tag_group"] = mdb.Group
				db["master_only"] = mdb.OnlyIfMaster
				for k, v := range mdb.CustomTags {
					db[metrics.TagPrefix+k] = v
				}
				r.measurementCh <- metrics.MeasurementEnvelope{
					DBName:     mdb.Name,
					MetricName: monitoredDbsDatastoreSyncMetricName,
					Data:       metrics.Measurements{db},
				}
			}
			monitoredDbCacheLock.RUnlock()
		}
		select {
		case <-time.After(time.Second * monitoredDbsDatastoreSyncIntervalSeconds):
			// continue
		case <-ctx.Done():
			return
		}
	}
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

func (r *Reaper) FetchMetric(ctx context.Context, md *sources.SourceConn, metricName string, hostState map[string]map[string]string) (_ *metrics.MeasurementEnvelope, err error) {
	var sql string
	var data metrics.Measurements
	var metric metrics.Metric
	var fromCache bool
	var cacheKey string
	var ok bool

	if metric, ok = metricDefs.GetMetricDef(metricName); !ok {
		return nil, metrics.ErrMetricNotFound
	}
	l := r.logger.WithField("source", md.Name).WithField("metric", metricName)

	if metric.IsInstanceLevel && r.Metrics.InstanceLevelCacheMaxSeconds > 0 && time.Second*time.Duration(md.GetMetricInterval(metricName)) < r.Metrics.CacheAge() {
		cacheKey = fmt.Sprintf("%s:%s", md.GetClusterIdentifier(), metricName)
	}
	if data = r.measurementCache.Get(cacheKey, r.Metrics.CacheAge()); len(data) > 0 {
		fromCache = true
		goto send_to_storageChannel
	}

	sql = metric.GetSQL(md.Version)
	if sql == "" && !(metricName == specialMetricChangeEvents || metricName == recoMetricName) {
		l.Warning("Ignoring fetching because of empty SQL")
		return nil, nil
	}

	if (metric.PrimaryOnly() && md.IsInRecovery) || (metric.StandbyOnly() && !md.IsInRecovery) {
		l.Debug("Skipping fetching of as server in wrong IsInRecovery: ", md.IsInRecovery)
		return nil, nil
	}

	switch metricName {
	case specialMetricChangeEvents:
		r.CheckForPGObjectChangesAndStore(ctx, md.Name, md, hostState) // TODO no hostState for Prometheus currently
		return nil, nil
	case recoMetricName:
		if data, err = GetRecommendations(ctx, md.Name, md); err != nil {
			return nil, err
		}
	default:
		if data, err = QueryMeasurements(ctx, md.Name, sql); err != nil {
			// let's soften errors to "info" from functions that expect the server to be a primary to reduce noise
			if strings.Contains(err.Error(), "recovery is in progress") && md.IsInRecovery {
				l.Debugf("[%s:%s] failed to fetch metrics: %s", md.Name, metricName, err)
				return nil, err
			}
			if metricName == specialMetricInstanceUp {
				l.WithError(err).Debugf("[%s:%s] failed to fetch metrics. marking instance as not up", md.Name, metricName)
				data = make(metrics.Measurements, 1)
				data[0] = metrics.NewMeasurement(time.Now().UnixNano())
				data[0]["is_up"] = 0 // should be updated if the "instance_up" metric definition is changed
				goto send_to_storageChannel
			}
			l.
				WithFields(map[string]any{"source": md.Name, "metric": metricName}).
				WithError(err).Error("failed to fetch metrics")

			return nil, err
		}
	}
	r.measurementCache.Put(cacheKey, data)

send_to_storageChannel:
	r.AddSysinfoToMeasurements(data, md)
	l.WithField("cache", fromCache).WithField("rows", len(data)).Info("measurements fetched")
	return &metrics.MeasurementEnvelope{
		DBName:           md.Name,
		MetricName:       cmp.Or(metric.StorageName, metricName),
		Data:             data,
		CustomTags:       md.CustomTags,
		MetricDef:        metric,
		RealDbname:       md.RealDbname,
		SystemIdentifier: md.SystemIdentifier}, nil
}
