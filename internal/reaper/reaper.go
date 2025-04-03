package reaper

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"sync/atomic"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/cmdopts"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/log"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/sources"
)

var monitoredSources = make(sources.SourceConns, 0)
var hostLastKnownStatusInRecovery = make(map[string]bool) // isInRecovery
var metricConfig map[string]float64                       // set to host.Metrics or host.MetricsStandby (in case optional config defined and in recovery state
var metricDefs = NewConcurrentMetricDefs()

// Reaper is the struct that responsible for fetching metrics measurements from the sources and storing them to the sinks
type Reaper struct {
	*cmdopts.Options
	ready         atomic.Bool
	measurementCh chan []metrics.MeasurementEnvelope
	logger        log.LoggerIface
}

// NewReaper creates a new Reaper instance
func NewReaper(ctx context.Context, opts *cmdopts.Options) (r *Reaper, err error) {
	r = &Reaper{
		Options:       opts,
		measurementCh: make(chan []metrics.MeasurementEnvelope, 10000),
		logger:        log.GetLogger(ctx),
	}
	return r, nil
}

// Ready() returns true if the service is healthy and operating correctly
func (r *Reaper) Ready() bool {
	return r.ready.Load()
}

// Reap() starts the main monitoring loop. It is responsible for fetching metrics measurements
// from the sources and storing them to the sinks. It also manages the lifecycle of
// the metric gatherers. In case of a source or metric definition change, it will
// start or stop the gatherers accordingly.
func (r *Reaper) Reap(ctx context.Context) (err error) {
	cancelFuncs := make(map[string]context.CancelFunc) // [db1+metric1]=chan

	mainLoopCount := 0
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

		UpdateMonitoredDBCache(monitoredSources)
		hostsToShutDownDueToRoleChange := make(map[string]bool) // hosts went from master to standby and have "only if master" set
		for _, monitoredSource := range monitoredSources {
			srcL := logger.WithField("source", monitoredSource.Name)

			if monitoredSource.Connect(ctx, r.Sources) != nil {
				srcL.WithError(err).Warning("could not init connection, retrying on next iteration")
				continue
			}

			InitPGVersionInfoFetchingLockIfNil(monitoredSource)

			var dbSettings MonitoredDatabaseSettings

			dbSettings, err = GetMonitoredDatabaseSettings(ctx, monitoredSource, true)
			if err != nil {
				srcL.WithError(err).Error("could not start metric gathering")
				continue
			}
			srcL.WithField("recovery", dbSettings.IsInRecovery).Infof("Connect OK. Version: %s", dbSettings.VersionStr)
			if dbSettings.IsInRecovery && monitoredSource.OnlyIfMaster {
				srcL.Info("not added to monitoring due to 'master only' property")
				continue
			}
			metricConfig = func() map[string]float64 {
				if len(monitoredSource.Metrics) > 0 {
					return monitoredSource.Metrics
				}
				if monitoredSource.PresetMetrics > "" {
					return metricDefs.GetPresetMetrics(monitoredSource.PresetMetrics)
				}
				return nil
			}()
			hostLastKnownStatusInRecovery[monitoredSource.Name] = dbSettings.IsInRecovery
			if dbSettings.IsInRecovery {
				metricConfig = func() map[string]float64 {
					if len(monitoredSource.MetricsStandby) > 0 {
						return monitoredSource.MetricsStandby
					}
					if monitoredSource.PresetMetricsStandby > "" {
						return metricDefs.GetPresetMetrics(monitoredSource.PresetMetricsStandby)
					}
					return nil
				}()
			}

			if monitoredSource.IsPostgresSource() && !dbSettings.IsInRecovery && r.Metrics.CreateHelpers {
				srcL.Info("trying to create helper objects if missing")
				if err = TryCreateMetricsFetchingHelpers(ctx, monitoredSource); err != nil {
					srcL.WithError(err).Warning("failed to create helper functions")
				}
			}

			if monitoredSource.IsPostgresSource() {
				var DBSizeMB int64

				if r.Sources.MinDbSizeMB >= 8 { // an empty DB is a bit less than 8MB
					DBSizeMB, _ = DBGetSizeMB(ctx, monitoredSource.Name) // ignore errors, i.e. only remove from monitoring when we're certain it's under the threshold
					if DBSizeMB != 0 {
						if DBSizeMB < r.Sources.MinDbSizeMB {
							srcL.Infof("ignored due to the --min-db-size-mb filter, current size %d MB", DBSizeMB)
							hostsToShutDownDueToRoleChange[monitoredSource.Name] = true // for the case when DB size was previosly above the threshold
							continue
						}
					}
				}
				ver, err := GetMonitoredDatabaseSettings(ctx, monitoredSource, false)
				if err == nil { // ok to ignore error, re-tried on next loop
					lastKnownStatusInRecovery := hostLastKnownStatusInRecovery[monitoredSource.Name]
					if ver.IsInRecovery && monitoredSource.OnlyIfMaster {
						srcL.Info("to be removed from monitoring due to 'master only' property and status change")
						hostsToShutDownDueToRoleChange[monitoredSource.Name] = true
						continue
					} else if lastKnownStatusInRecovery != ver.IsInRecovery {
						if ver.IsInRecovery && len(monitoredSource.MetricsStandby) > 0 {
							srcL.Warning("Switching metrics collection to standby config...")
							metricConfig = monitoredSource.MetricsStandby
							hostLastKnownStatusInRecovery[monitoredSource.Name] = true
						} else {
							srcL.Warning("Switching metrics collection to primary config...")
							metricConfig = monitoredSource.Metrics
							hostLastKnownStatusInRecovery[monitoredSource.Name] = false
						}
					}
				}

				if mainLoopCount == 0 && r.Sources.TryCreateListedExtsIfMissing != "" && !ver.IsInRecovery {
					extsToCreate := strings.Split(r.Sources.TryCreateListedExtsIfMissing, ",")
					extsCreated := TryCreateMissingExtensions(ctx, monitoredSource.Name, extsToCreate, ver.Extensions)
					srcL.Infof("%d/%d extensions created based on --try-create-listed-exts-if-missing input %v", len(extsCreated), len(extsToCreate), extsCreated)
				}
			}

			for metricName, interval := range metricConfig {
				metric := metricName
				metricDefOk := false
				var mvp metrics.Metric

				if strings.HasPrefix(metric, recoPrefix) {
					metric = recoMetricName
					metricDefOk = true
				} else {
					mvp, metricDefOk = metricDefs.GetMetricDef(metric)
				}

				dbMetric := monitoredSource.Name + dbMetricJoinStr + metric
				_, chOk := cancelFuncs[dbMetric]

				if metricDefOk && !chOk { // initialize a new per db/per metric control channel
					if interval > 0 {
						srcL.WithField("metric", metric).WithField("interval", interval).Info("starting gatherer")
						metricCtx, cancelFunc := context.WithCancel(ctx)
						cancelFuncs[dbMetric] = cancelFunc

						metricNameForStorage := metricName
						if _, isSpecialMetric := specialMetrics[metricName]; !isSpecialMetric {
							metricNameForStorage = mvp.StorageName
						}

						if err := r.SinksWriter.SyncMetric(monitoredSource.Name, metricNameForStorage, "add"); err != nil {
							srcL.Error(err)
						}

						go r.reapMetricMeasurements(metricCtx, monitoredSource, metric, metricConfig[metric])
					}
				} else if (!metricDefOk && chOk) || interval <= 0 {
					// metric definition files were recently removed or interval set to zero
					if cancelFunc, isOk := cancelFuncs[dbMetric]; isOk {
						cancelFunc()
					}
					srcL.WithField("metric", metric).Warning("shutting down gatherer...")
					delete(cancelFuncs, dbMetric)
				} else if !metricDefOk {
					epoch, ok := lastSQLFetchError.Load(metric)
					if !ok || ((time.Now().Unix() - epoch.(int64)) > 3600) { // complain only 1x per hour
						srcL.WithField("metric", metric).Warning("metric definition not found")
						lastSQLFetchError.Store(metric, time.Now().Unix())
					}
				}
			}
		}

		if mainLoopCount == 0 {
			goto MainLoopSleep
		}

		// loop over existing channels and stop workers if DB or metric removed from config
		// or state change makes it uninteresting
		logger.Debug("checking if any workers need to be shut down...")
		for dbMetric, cancelFunc := range cancelFuncs {
			var currentMetricConfig map[string]float64
			var dbInfo *sources.SourceConn
			var ok, dbRemovedFromConfig bool
			singleMetricDisabled := false
			splits := strings.Split(dbMetric, dbMetricJoinStr)
			db := splits[0]
			metric := splits[1]

			_, wholeDbShutDownDueToRoleChange := hostsToShutDownDueToRoleChange[db]
			if !wholeDbShutDownDueToRoleChange {
				monitoredDbCacheLock.RLock()
				dbInfo, ok = monitoredDbCache[db]
				monitoredDbCacheLock.RUnlock()
				if !ok { // normal removing of DB from config
					dbRemovedFromConfig = true
					logger.Debugf("DB %s removed from config, shutting down all metric worker processes...", db)
				}
			}

			if !(wholeDbShutDownDueToRoleChange || dbRemovedFromConfig) { // maybe some single metric was disabled
				MonitoredDatabasesSettingsLock.RLock()
				verInfo, ok := MonitoredDatabasesSettings[db]
				MonitoredDatabasesSettingsLock.RUnlock()
				if !ok {
					logger.Warningf("Could not find PG version info for DB %s, skipping shutdown check of metric worker process for %s", db, metric)
					continue
				}
				if verInfo.IsInRecovery && dbInfo.PresetMetricsStandby > "" || !verInfo.IsInRecovery && dbInfo.PresetMetrics > "" {
					continue // no need to check presets for single metric disabling
				}
				if verInfo.IsInRecovery && len(dbInfo.MetricsStandby) > 0 {
					currentMetricConfig = dbInfo.MetricsStandby
				} else {
					currentMetricConfig = dbInfo.Metrics
				}

				interval, isMetricActive := currentMetricConfig[metric]
				if !isMetricActive || interval <= 0 {
					singleMetricDisabled = true
				}
			}

			if ctx.Err() != nil || wholeDbShutDownDueToRoleChange || dbRemovedFromConfig || singleMetricDisabled {
				logger.WithField("source", db).WithField("metric", metric).Info("stoppin gatherer...")
				cancelFunc()
				delete(cancelFuncs, dbMetric)
				if err := r.SinksWriter.SyncMetric(db, metric, "remove"); err != nil {
					logger.Error(err)
				}
			}
		}

		// Destroy conn pools and metric writers
		CloseResourcesForRemovedMonitoredDBs(r.SinksWriter, monitoredSources, prevLoopMonitoredDBs, hostsToShutDownDueToRoleChange)

	MainLoopSleep:
		mainLoopCount++
		prevLoopMonitoredDBs = slices.Clone(monitoredSources)

		logger.Debugf("main sleeping %ds...", r.Sources.Refresh)
		select {
		case <-time.After(time.Second * time.Duration(r.Sources.Refresh)):
			// pass
		case <-ctx.Done():
			return
		}
	}
}

// metrics.ControlMessage notifies of shutdown + interval change
func (r *Reaper) reapMetricMeasurements(ctx context.Context, mdb *sources.SourceConn, metricName string, interval float64) {
	hostState := make(map[string]map[string]string)
	var lastUptimeS int64 = -1 // used for "server restarted" event detection
	var lastErrorNotificationTime time.Time
	var vme MonitoredDatabaseSettings
	var mvp metrics.Metric
	var err error
	var ok bool

	failedFetches := 0
	lastDBVersionFetchTime := time.Unix(0, 0) // check DB ver. ev. 5 min

	l := r.logger.WithField("source", mdb.Name).WithField("metric", metricName)
	if metricName == specialMetricServerLogEventCounts {
		MonitoredDatabasesSettingsLock.RLock()
		realDbname := MonitoredDatabasesSettings[mdb.Name].RealDbname // to manage 2 sets of event counts - monitored DB + global
		MonitoredDatabasesSettingsLock.RUnlock()
		metrics.ParseLogs(ctx, mdb, realDbname, interval, r.measurementCh) // no return
		return
	}

	for {
		if lastDBVersionFetchTime.Add(time.Minute * time.Duration(5)).Before(time.Now()) {
			vme, err = GetMonitoredDatabaseSettings(ctx, mdb, false) // in case of errors just ignore metric "disabled" time ranges
			if err != nil {
				lastDBVersionFetchTime = time.Now()
			}

			mvp, ok = metricDefs.GetMetricDef(metricName)
			if !ok {
				l.Errorf("Could not get metric version properties: %s", metricName)
				return
			}
		}

		var metricStoreMessages []metrics.MeasurementEnvelope
		mfm := MetricFetchConfig{
			DBUniqueName:        mdb.Name,
			DBUniqueNameOrig:    mdb.GetDatabaseName(),
			MetricName:          metricName,
			Source:              mdb.Kind,
			Interval:            time.Second * time.Duration(interval),
			StmtTimeoutOverride: 0,
		}

		// 1st try local overrides for some metrics if operating in push mode
		if r.Metrics.DirectOSStats && IsDirectlyFetchableMetric(metricName) {
			metricStoreMessages, err = FetchStatsDirectlyFromOS(ctx, mfm, vme, mvp)
			if err != nil {
				l.WithError(err).Errorf("Could not reader metric directly from OS")
			}
		}
		t1 := time.Now()
		if metricStoreMessages == nil {
			metricStoreMessages, err = r.FetchMetrics(ctx, mfm, hostState)
		}
		t2 := time.Now()

		if t2.Sub(t1) > (time.Second * time.Duration(interval)) {
			l.Warningf("Total fetching time of %vs bigger than %vs interval", t2.Sub(t1).Truncate(time.Millisecond*100).Seconds(), interval)
		}

		if err != nil {
			failedFetches++
			// complain only 1x per 10min per host/metric...
			if lastErrorNotificationTime.IsZero() || lastErrorNotificationTime.Add(time.Second*time.Duration(600)).Before(time.Now()) {
				l.WithError(err).Error("failed to fetch metric data")
				if failedFetches > 1 {
					l.Errorf("Total failed fetches: %d", failedFetches)
				}
				lastErrorNotificationTime = time.Now()
			}
		} else if metricStoreMessages != nil {
			if len(metricStoreMessages[0].Data) > 0 {

				// pick up "server restarted" events here to avoid doing extra selects from CheckForPGObjectChangesAndStore code
				if metricName == "db_stats" {
					postmasterUptimeS, ok := (metricStoreMessages[0].Data)[0]["postmaster_uptime_s"]
					if ok {
						if lastUptimeS != -1 {
							if postmasterUptimeS.(int64) < lastUptimeS { // restart (or possibly also failover when host is routed) happened
								message := "Detected server restart (or failover) of \"" + mdb.Name + "\""
								l.Warning(message)
								detectedChangesSummary := make(metrics.Measurements, 0)
								entry := metrics.Measurement{"details": message, "epoch_ns": (metricStoreMessages[0].Data)[0]["epoch_ns"]}
								detectedChangesSummary = append(detectedChangesSummary, entry)
								metricStoreMessages = append(metricStoreMessages,
									metrics.MeasurementEnvelope{
										DBName:     mdb.Name,
										SourceType: string(mdb.Kind),
										MetricName: "object_changes",
										Data:       detectedChangesSummary,
										CustomTags: metricStoreMessages[0].CustomTags,
									})
							}
						}
						lastUptimeS = postmasterUptimeS.(int64)
					}
				}
				r.measurementCh <- metricStoreMessages
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

// WriteMonitoredSources writes actively monitored DBs listing to sinks
// every monitoredDbsDatastoreSyncIntervalSeconds (default 10min)
func (r *Reaper) WriteMonitoredSources(ctx context.Context) {
	if len(monitoredDbCache) == 0 {
		return
	}
	msms := make([]metrics.MeasurementEnvelope, len(monitoredDbCache))
	now := time.Now().UnixNano()

	monitoredDbCacheLock.RLock()
	for _, mdb := range monitoredDbCache {
		db := metrics.Measurement{
			"tag_group":   mdb.Group,
			"master_only": mdb.OnlyIfMaster,
			"epoch_ns":    now,
		}
		for k, v := range mdb.CustomTags {
			db[tagPrefix+k] = v
		}
		msms = append(msms, metrics.MeasurementEnvelope{
			DBName:     mdb.Name,
			MetricName: monitoredDbsDatastoreSyncMetricName,
			Data:       metrics.Measurements{db},
		})
	}
	monitoredDbCacheLock.RUnlock()

	select {
	case <-time.After(time.Second * monitoredDbsDatastoreSyncIntervalSeconds):
		// continue
	case r.measurementCh <- msms:
		//continue
	case <-ctx.Done():
		return
	}
}

func (r *Reaper) AddSysinfoToMeasurements(data metrics.Measurements, ver MonitoredDatabaseSettings) metrics.Measurements {
	enrichedData := make(metrics.Measurements, 0)
	for _, dr := range data {
		if r.Sinks.RealDbnameField > "" && ver.RealDbname > "" {
			old, ok := dr[r.Sinks.RealDbnameField]
			if !ok || old == "" {
				dr[r.Sinks.RealDbnameField] = ver.RealDbname
			}
		}
		if r.Sinks.SystemIdentifierField > "" && ver.SystemIdentifier > "" {
			old, ok := dr[r.Sinks.SystemIdentifierField]
			if !ok || old == "" {
				dr[r.Sinks.SystemIdentifierField] = ver.SystemIdentifier
			}
		}
		enrichedData = append(enrichedData, dr)
	}
	return enrichedData
}

var instanceMetricCache = make(map[string](metrics.Measurements)) // [dbUnique+metric]lastly_fetched_data
var instanceMetricCacheLock = sync.RWMutex{}
var instanceMetricCacheTimestamp = make(map[string]time.Time) // [dbUnique+metric]last_fetch_time
var instanceMetricCacheTimestampLock = sync.RWMutex{}

func IsCacheableMetric(msg MetricFetchConfig, mvp metrics.Metric) bool {
	if !(msg.Source == sources.SourcePostgresContinuous || msg.Source == sources.SourcePatroniContinuous) {
		return false
	}
	return mvp.IsInstanceLevel
}

func PutToInstanceCache(msg MetricFetchConfig, data metrics.Measurements) {
	if len(data) == 0 {
		return
	}
	dataCopy := deepCopyMetricData(data)
	instanceMetricCacheLock.Lock()
	instanceMetricCache[msg.DBUniqueNameOrig+msg.MetricName] = dataCopy
	instanceMetricCacheLock.Unlock()

	instanceMetricCacheTimestampLock.Lock()
	instanceMetricCacheTimestamp[msg.DBUniqueNameOrig+msg.MetricName] = time.Now()
	instanceMetricCacheTimestampLock.Unlock()
}

func deepCopyMetricData(data metrics.Measurements) metrics.Measurements {
	newData := make(metrics.Measurements, len(data))
	for i, dr := range data {
		newData[i] = maps.Clone(dr)
	}
	return newData
}

func (r *Reaper) FetchMetrics(ctx context.Context,
	msg MetricFetchConfig,
	hostState map[string]map[string]string) ([]metrics.MeasurementEnvelope, error) {

	var dbSettings MonitoredDatabaseSettings
	var dbVersion int
	var err error
	var sql string
	var data, cachedData metrics.Measurements
	var md *sources.SourceConn
	var fromCache, isCacheable bool

	md, err = GetMonitoredDatabaseByUniqueName(msg.DBUniqueName)
	if err != nil {
		log.GetLogger(ctx).Errorf("[%s:%s] could not get monitored DB details", msg.DBUniqueName, err)
		return nil, err
	}

	dbSettings, err = GetMonitoredDatabaseSettings(ctx, md, false)
	if err != nil {
		log.GetLogger(ctx).Error("failed to fetch pg version for ", msg.DBUniqueName, msg.MetricName, err)
		return nil, err
	}
	if msg.MetricName == specialMetricDbSize || msg.MetricName == specialMetricTableStats {
		if dbSettings.ExecEnv == execEnvAzureSingle && dbSettings.ApproxDBSizeB > 1e12 { // 1TB
			subsMetricName := msg.MetricName + "_approx"
			mvpApprox, ok := metricDefs.GetMetricDef(subsMetricName)
			if ok && mvpApprox.StorageName == msg.MetricName {
				log.GetLogger(ctx).Infof("[%s:%s] Transparently swapping metric to %s due to hard-coded rules...", msg.DBUniqueName, msg.MetricName, subsMetricName)
				msg.MetricName = subsMetricName
			}
		}
	}
	dbVersion = dbSettings.Version

	if msg.Source == sources.SourcePgBouncer {
		dbVersion = 0 // version is 0.0 for all pgbouncer sql per convention
	}

	mvp, ok := metricDefs.GetMetricDef(msg.MetricName)
	if !ok && msg.MetricName != recoMetricName {
		epoch, ok := lastSQLFetchError.Load(msg.MetricName + dbMetricJoinStr + fmt.Sprintf("%v", dbVersion))
		if !ok || ((time.Now().Unix() - epoch.(int64)) > 3600) { // complain only 1x per hour
			log.GetLogger(ctx).Infof("Failed to get SQL for metric '%s', version '%s': %v", msg.MetricName, dbSettings.VersionStr, err)
			lastSQLFetchError.Store(msg.MetricName+dbMetricJoinStr+fmt.Sprintf("%v", dbVersion), time.Now().Unix())
		}
		return nil, metrics.ErrMetricNotFound
	}

	isCacheable = IsCacheableMetric(msg, mvp)
	if isCacheable && r.Metrics.InstanceLevelCacheMaxSeconds > 0 && msg.Interval.Seconds() > float64(r.Metrics.InstanceLevelCacheMaxSeconds) {
		cachedData = GetFromInstanceCacheIfNotOlderThanSeconds(msg, r.Metrics.InstanceLevelCacheMaxSeconds)
		if len(cachedData) > 0 {
			fromCache = true
			goto send_to_storageChannel
		}
	}

	sql = mvp.GetSQL(dbVersion)

	if sql == "" && !(msg.MetricName == specialMetricChangeEvents || msg.MetricName == recoMetricName) {
		// let's ignore dummy SQLs
		log.GetLogger(ctx).Debugf("[%s:%s] Ignoring fetch message - got an empty/dummy SQL string", msg.DBUniqueName, msg.MetricName)
		return nil, nil
	}

	if (mvp.PrimaryOnly() && dbSettings.IsInRecovery) || (mvp.StandbyOnly() && !dbSettings.IsInRecovery) {
		log.GetLogger(ctx).Debugf("[%s:%s] Skipping fetching of  as server not in wanted state (IsInRecovery=%v)", msg.DBUniqueName, msg.MetricName, dbSettings.IsInRecovery)
		return nil, nil
	}

	if msg.MetricName == specialMetricChangeEvents { // special handling, multiple queries + stateful
		r.CheckForPGObjectChangesAndStore(ctx, msg.DBUniqueName, dbSettings, hostState) // TODO no hostState for Prometheus currently
	} else if msg.MetricName == recoMetricName {
		if data, err = GetRecommendations(ctx, msg.DBUniqueName, dbSettings); err != nil {
			return nil, err
		}
	} else if msg.Source == sources.SourcePgPool {
		if data, err = FetchMetricsPgpool(ctx, msg, dbSettings, mvp); err != nil {
			return nil, err
		}
	} else {
		data, err = QueryMeasurements(ctx, msg.DBUniqueName, sql)

		if err != nil {
			// let's soften errors to "info" from functions that expect the server to be a primary to reduce noise
			if strings.Contains(err.Error(), "recovery is in progress") {
				MonitoredDatabasesSettingsLock.RLock()
				ver := MonitoredDatabasesSettings[msg.DBUniqueName]
				MonitoredDatabasesSettingsLock.RUnlock()
				if ver.IsInRecovery {
					log.GetLogger(ctx).Debugf("[%s:%s] failed to fetch metrics: %s", msg.DBUniqueName, msg.MetricName, err)
					return nil, err
				}
			}

			if msg.MetricName == specialMetricInstanceUp {
				log.GetLogger(ctx).WithError(err).Debugf("[%s:%s] failed to fetch metrics. marking instance as not up", msg.DBUniqueName, msg.MetricName)
				data = make(metrics.Measurements, 1)
				data[0] = metrics.Measurement{"epoch_ns": time.Now().UnixNano(), "is_up": 0} // should be updated if the "instance_up" metric definition is changed
				goto send_to_storageChannel
			}

			log.GetLogger(ctx).
				WithFields(map[string]any{"source": msg.DBUniqueName, "metric": msg.MetricName}).
				WithError(err).Error("failed to fetch metrics")

			return nil, err
		}

		log.GetLogger(ctx).WithFields(map[string]any{"source": msg.DBUniqueName, "metric": msg.MetricName, "rows": len(data)}).Info("measurements fetched")
	}

	if isCacheable && r.Metrics.InstanceLevelCacheMaxSeconds > 0 && msg.Interval.Seconds() > float64(r.Metrics.InstanceLevelCacheMaxSeconds) {
		PutToInstanceCache(msg, data)
	}

send_to_storageChannel:

	if (r.Sinks.RealDbnameField > "" || r.Sinks.SystemIdentifierField > "") && msg.Source == sources.SourcePostgres {
		MonitoredDatabasesSettingsLock.RLock()
		ver := MonitoredDatabasesSettings[msg.DBUniqueName]
		MonitoredDatabasesSettingsLock.RUnlock()
		data = r.AddSysinfoToMeasurements(data, ver)
	}

	if mvp.StorageName != "" {
		log.GetLogger(ctx).Debugf("[%s] rerouting metric %s data to %s based on metric attributes", msg.DBUniqueName, msg.MetricName, mvp.StorageName)
		msg.MetricName = mvp.StorageName
	}
	if fromCache {
		log.GetLogger(ctx).Infof("[%s:%s] loaded %d rows from the instance cache", msg.DBUniqueName, msg.MetricName, len(cachedData))
		return []metrics.MeasurementEnvelope{{DBName: msg.DBUniqueName, MetricName: msg.MetricName, Data: cachedData, CustomTags: md.CustomTags,
			MetricDef: mvp, RealDbname: dbSettings.RealDbname, SystemIdentifier: dbSettings.SystemIdentifier}}, nil
	}
	return []metrics.MeasurementEnvelope{{DBName: msg.DBUniqueName, MetricName: msg.MetricName, Data: data, CustomTags: md.CustomTags,
		MetricDef: mvp, RealDbname: dbSettings.RealDbname, SystemIdentifier: dbSettings.SystemIdentifier}}, nil

}
