package reaper

import (
	"context"
	"fmt"
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

var monitoredSources = make(sources.MonitoredDatabases, 0)
var hostLastKnownStatusInRecovery = make(map[string]bool) // isInRecovery
var metricConfig map[string]float64                       // set to host.Metrics or host.MetricsStandby (in case optional config defined and in recovery state
var metricDefinitionMap *metrics.Metrics = &metrics.Metrics{}
var metricDefMapLock = sync.RWMutex{}

// Reaper is the struct that responsible for fetching metrics measurements from the sources and storing them to the sinks
type Reaper struct {
	*cmdopts.Options
	ready         atomic.Bool
	measurementCh chan []metrics.MeasurementEnvelope
	logger        log.LoggerIface
}

// NewReaper creates a new Reaper instance
func NewReaper(mainContext context.Context, opts *cmdopts.Options) (r *Reaper, err error) {
	r = &Reaper{
		Options:       opts,
		measurementCh: make(chan []metrics.MeasurementEnvelope, 10000),
		logger:        log.GetLogger(mainContext),
	}
	if monitoredSources, err = monitoredSources.SyncFromReader(r.SourcesReaderWriter); err != nil {
		return nil, err
	}
	go r.ReadMetrics(mainContext)
	go r.WriteMeasurements(mainContext)
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
func (r *Reaper) Reap(mainContext context.Context) (err error) {
	cancelFuncs := make(map[string]context.CancelFunc) // [db1+metric1]=chan

	mainLoopCount := 0
	logger := r.logger

	r.ready.Store(true)

	for { //main loop
		r.logger.WithField("sources", len(monitoredSources)).Info("sourcers refreshed")
		hostsToShutDownDueToRoleChange := make(map[string]bool) // hosts went from master to standby and have "only if master" set
		gatherersShutDown := 0

		if DoesEmergencyTriggerfileExist(r.Metrics.EmergencyPauseTriggerfile) {
			logger.Warningf("Emergency pause triggerfile detected at %s, ignoring currently configured DBs", r.Metrics.EmergencyPauseTriggerfile)
			monitoredSources = make([]*sources.MonitoredDatabase, 0)
		}

		UpdateMonitoredDBCache(monitoredSources)

		if lastMonitoredDBsUpdate.IsZero() || lastMonitoredDBsUpdate.Before(time.Now().Add(-1*time.Second*monitoredDbsDatastoreSyncIntervalSeconds)) {
			go SyncMonitoredDBsToDatastore(mainContext, monitoredSources, r.measurementCh)
			lastMonitoredDBsUpdate = time.Now()
		}

		for _, monitoredSource := range monitoredSources {
			logger.WithField("source", monitoredSource).Debug()

			srcType := monitoredSource.Kind

			if monitoredSource.Connect(mainContext, r.Sources) != nil {
				logger.Warningf("could not init connection, retrying on next iteration: %w", err)
				continue
			}

			InitPGVersionInfoFetchingLockIfNil(monitoredSource)

			var ver MonitoredDatabaseSettings

			ver, err = GetMonitoredDatabaseSettings(mainContext, monitoredSource.Name, srcType, true)
			if err != nil {
				logger.WithError(err).WithField("source", monitoredSource.Name).
					Error("could not start metric gathering")
				continue
			}
			logger.WithField("source", monitoredSource.Name).WithField("recovery", ver.IsInRecovery).Infof("Connect OK. Version: %s", ver.VersionStr)
			if ver.IsInRecovery && monitoredSource.OnlyIfMaster {
				logger.Infof("not added to monitoring due to 'master only' property")
				continue
			}
			metricConfig = func() map[string]float64 {
				if len(monitoredSource.Metrics) > 0 {
					return monitoredSource.Metrics
				}
				if monitoredSource.PresetMetrics > "" {
					return metricDefinitionMap.PresetDefs[monitoredSource.PresetMetrics].Metrics
				}
				return nil
			}()
			hostLastKnownStatusInRecovery[monitoredSource.Name] = ver.IsInRecovery
			if ver.IsInRecovery {
				metricConfig = func() map[string]float64 {
					if len(monitoredSource.MetricsStandby) > 0 {
						return monitoredSource.MetricsStandby
					}
					if monitoredSource.PresetMetricsStandby > "" {
						return metricDefinitionMap.PresetDefs[monitoredSource.PresetMetricsStandby].Metrics
					}
					return nil
				}()
			}

			if monitoredSource.IsPostgresSource() && !ver.IsInRecovery && r.Metrics.CreateHelpers {
				ls := logger.WithField("source", monitoredSource.Name)
				ls.Info("trying to create helper objects if missing")
				if err = TryCreateMetricsFetchingHelpers(mainContext, monitoredSource); err != nil {
					ls.Warning("failed to create helper functions: %w", err)
				}
			}

			if monitoredSource.IsPostgresSource() {
				var DBSizeMB int64

				if r.Sources.MinDbSizeMB >= 8 { // an empty DB is a bit less than 8MB
					DBSizeMB, _ = DBGetSizeMB(mainContext, monitoredSource.Name) // ignore errors, i.e. only remove from monitoring when we're certain it's under the threshold
					if DBSizeMB != 0 {
						if DBSizeMB < r.Sources.MinDbSizeMB {
							logger.Infof("[%s] DB will be ignored due to the --min-db-size-mb filter. Current (up to %v cached) DB size = %d MB", monitoredSource.Name, dbSizeCachingInterval, DBSizeMB)
							hostsToShutDownDueToRoleChange[monitoredSource.Name] = true // for the case when DB size was previosly above the threshold
							SetUndersizedDBState(monitoredSource.Name, true)
							continue
						}
						SetUndersizedDBState(monitoredSource.Name, false)
					}
				}
				ver, err := GetMonitoredDatabaseSettings(mainContext, monitoredSource.Name, monitoredSource.Kind, false)
				if err == nil { // ok to ignore error, re-tried on next loop
					lastKnownStatusInRecovery := hostLastKnownStatusInRecovery[monitoredSource.Name]
					if ver.IsInRecovery && monitoredSource.OnlyIfMaster {
						logger.Infof("[%s] to be removed from monitoring due to 'master only' property and status change", monitoredSource.Name)
						hostsToShutDownDueToRoleChange[monitoredSource.Name] = true
						SetRecoveryIgnoredDBState(monitoredSource.Name, true)
						continue
					} else if lastKnownStatusInRecovery != ver.IsInRecovery {
						if ver.IsInRecovery && len(monitoredSource.MetricsStandby) > 0 {
							logger.Warningf("Switching metrics collection for \"%s\" to standby config...", monitoredSource.Name)
							metricConfig = monitoredSource.MetricsStandby
							hostLastKnownStatusInRecovery[monitoredSource.Name] = true
						} else {
							logger.Warningf("Switching metrics collection for \"%s\" to primary config...", monitoredSource.Name)
							metricConfig = monitoredSource.Metrics
							hostLastKnownStatusInRecovery[monitoredSource.Name] = false
							SetRecoveryIgnoredDBState(monitoredSource.Name, false)
						}
					}
				}

				if mainLoopCount == 0 && r.Sources.TryCreateListedExtsIfMissing != "" && !ver.IsInRecovery {
					extsToCreate := strings.Split(r.Sources.TryCreateListedExtsIfMissing, ",")
					extsCreated := TryCreateMissingExtensions(mainContext, monitoredSource.Name, extsToCreate, ver.Extensions)
					logger.Infof("[%s] %d/%d extensions created based on --try-create-listed-exts-if-missing input %v", monitoredSource.Name, len(extsCreated), len(extsToCreate), extsCreated)
				}
			}

			for metricName, interval := range metricConfig {
				metric := metricName
				metricDefOk := false

				if strings.HasPrefix(metric, recoPrefix) {
					metric = recoMetricName
					metricDefOk = true
				} else {
					metricDefMapLock.RLock()
					_, metricDefOk = metricDefinitionMap.MetricDefs[metric]
					metricDefMapLock.RUnlock()
				}

				dbMetric := monitoredSource.Name + dbMetricJoinStr + metric
				_, chOk := cancelFuncs[dbMetric]

				if metricDefOk && !chOk { // initialize a new per db/per metric control channel
					if interval > 0 {
						hostMetricIntervalMap[dbMetric] = interval
						logger.WithField("source", monitoredSource.Name).WithField("metric", metric).WithField("interval", interval).Info("starting gatherer")
						metricCtx, cancelFunc := context.WithCancel(mainContext)
						cancelFuncs[dbMetric] = cancelFunc

						metricNameForStorage := metricName
						if _, isSpecialMetric := specialMetrics[metricName]; !isSpecialMetric {
							vme, err := GetMonitoredDatabaseSettings(mainContext, monitoredSource.Name, srcType, false)
							if err != nil {
								logger.Warning("Failed to determine possible re-routing name, Grafana dashboards with re-routed metrics might not show all hosts")
							} else {
								mvp, err := GetMetricVersionProperties(metricName, vme, nil)
								if err != nil && !strings.Contains(err.Error(), "too old") {
									logger.Warning("Failed to determine possible re-routing name, Grafana dashboards with re-routed metrics might not show all hosts")
								} else if mvp.StorageName != "" {
									metricNameForStorage = mvp.StorageName
								}
							}
						}

						if err := r.SinksWriter.SyncMetric(monitoredSource.Name, metricNameForStorage, "add"); err != nil {
							logger.Error(err)
						}

						go r.reapMetricMeasurementsFromSource(metricCtx,
							monitoredSource.Name,
							monitoredSource.GetDatabaseName(),
							srcType,
							metric,
							metricConfig)
					}
				} else if (!metricDefOk && chOk) || interval <= 0 {
					// metric definition files were recently removed or interval set to zero
					if cancelFunc, isOk := cancelFuncs[dbMetric]; isOk {
						cancelFunc()
					}
					logger.Warning("shutting down metric", metric, "for", monitoredSource.Name)
					delete(cancelFuncs, dbMetric)
				} else if !metricDefOk {
					epoch, ok := lastSQLFetchError.Load(metric)
					if !ok || ((time.Now().Unix() - epoch.(int64)) > 3600) { // complain only 1x per hour
						logger.Warningf("metric definition \"%s\" not found for \"%s\"", metric, monitoredSource.Name)
						lastSQLFetchError.Store(metric, time.Now().Unix())
					}
				} else {
					// check if interval has changed
					if hostMetricIntervalMap[dbMetric] != interval {
						logger.Warning("updating interval update for", monitoredSource.Name, metric)
						hostMetricIntervalMap[dbMetric] = interval
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
			var dbInfo *sources.MonitoredDatabase
			var ok, dbRemovedFromConfig bool
			singleMetricDisabled := false
			splits := strings.Split(dbMetric, dbMetricJoinStr)
			db := splits[0]
			metric := splits[1]
			//log.Debugf("Checking if need to shut down worker for [%s:%s]...", db, metric)

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

			if mainContext.Err() != nil || wholeDbShutDownDueToRoleChange || dbRemovedFromConfig || singleMetricDisabled {
				logger.WithField("source", db).WithField("metric", metric).Infof("stoppin gatherer...")
				cancelFunc()
				delete(cancelFuncs, dbMetric)
				logger.Debugf("cancel function for [%s:%s] deleted", db, metric)
				gatherersShutDown++
				ClearDBUnreachableStateIfAny(db)
				if err := r.SinksWriter.SyncMetric(db, metric, "remove"); err != nil {
					logger.Error(err)
				}
			}
		}

		if gatherersShutDown > 0 {
			logger.Warningf("sent STOP message to %d gatherers (it might take some time for them to stop though)", gatherersShutDown)
		}

		// Destroy conn pools and metric writers
		CloseResourcesForRemovedMonitoredDBs(r.SinksWriter, monitoredSources, prevLoopMonitoredDBs, hostsToShutDownDueToRoleChange)

	MainLoopSleep:
		mainLoopCount++
		prevLoopMonitoredDBs = slices.Clone(monitoredSources)

		logger.Debugf("main sleeping %ds...", r.Sources.Refresh)
		select {
		case <-time.After(time.Second * time.Duration(r.Sources.Refresh)):
			if monitoredSources, err = monitoredSources.SyncFromReader(r.SourcesReaderWriter); err != nil {
				logger.Error("could not fetch active hosts, using last valid config data:", err)
			}
		case <-mainContext.Done():
			return
		}
	}
}

// metrics.ControlMessage notifies of shutdown + interval change
func (r *Reaper) reapMetricMeasurementsFromSource(ctx context.Context,
	dbUniqueName, dbUniqueNameOrig string,
	srcType sources.Kind,
	metricName string,
	configMap map[string]float64) {

	hostState := make(map[string]map[string]string)
	var lastUptimeS int64 = -1 // used for "server restarted" event detection
	var lastErrorNotificationTime time.Time
	var vme MonitoredDatabaseSettings
	var mvp metrics.Metric
	var err error
	failedFetches := 0
	lastDBVersionFetchTime := time.Unix(0, 0) // check DB ver. ev. 5 min

	l := log.GetLogger(ctx).WithField("source", dbUniqueName).WithField("metric", metricName)
	if metricName == specialMetricServerLogEventCounts {
		mdb, err := GetMonitoredDatabaseByUniqueName(dbUniqueName)
		if err != nil {
			return
		}
		MonitoredDatabasesSettingsLock.RLock()
		realDbname := MonitoredDatabasesSettings[dbUniqueName].RealDbname // to manage 2 sets of event counts - monitored DB + global
		MonitoredDatabasesSettingsLock.RUnlock()
		conn := mdb.Conn
		metrics.ParseLogs(ctx, conn, mdb, realDbname, metricName, configMap, r.measurementCh) // no return
		return
	}

	for {
		interval := configMap[metricName]
		if lastDBVersionFetchTime.Add(time.Minute * time.Duration(5)).Before(time.Now()) {
			vme, err = GetMonitoredDatabaseSettings(ctx, dbUniqueName, srcType, false) // in case of errors just ignore metric "disabled" time ranges
			if err != nil {
				lastDBVersionFetchTime = time.Now()
			}

			mvp, err = GetMetricVersionProperties(metricName, vme, nil)
			if err != nil {
				l.Errorf("Could not get metric version properties: %v", err)
				return
			}
		}

		var metricStoreMessages []metrics.MeasurementEnvelope
		var err error
		mfm := MetricFetchConfig{
			DBUniqueName:        dbUniqueName,
			DBUniqueNameOrig:    dbUniqueNameOrig,
			MetricName:          metricName,
			Source:              srcType,
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
			metricStoreMessages, err = FetchMetrics(ctx, mfm, hostState, r.measurementCh, "", r.Options)
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
								message := "Detected server restart (or failover) of \"" + dbUniqueName + "\""
								l.Warning(message)
								detectedChangesSummary := make(metrics.Measurements, 0)
								entry := metrics.Measurement{"details": message, "epoch_ns": (metricStoreMessages[0].Data)[0]["epoch_ns"]}
								detectedChangesSummary = append(detectedChangesSummary, entry)
								metricStoreMessages = append(metricStoreMessages,
									metrics.MeasurementEnvelope{
										DBName:     dbUniqueName,
										SourceType: string(srcType),
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
			l.Debugf("MetricGathererLoop slept for %s", time.Second*time.Duration(interval))
		}
	}
}

func StoreMetrics(metrics []metrics.MeasurementEnvelope, storageCh chan<- []metrics.MeasurementEnvelope) (int, error) {
	if len(metrics) > 0 {
		storageCh <- metrics
		return len(metrics), nil
	}
	return 0, nil
}

func SyncMonitoredDBsToDatastore(ctx context.Context, monitoredDbs []*sources.MonitoredDatabase, persistenceChannel chan []metrics.MeasurementEnvelope) {
	if len(monitoredDbs) > 0 {
		msms := make([]metrics.MeasurementEnvelope, len(monitoredDbs))
		now := time.Now()

		for _, mdb := range monitoredDbs {
			db := metrics.Measurement{
				"tag_group":                   mdb.Group,
				"master_only":                 mdb.OnlyIfMaster,
				"epoch_ns":                    now.UnixNano(),
				"continuous_discovery_prefix": mdb.GetDatabaseName(),
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
		select {
		case persistenceChannel <- msms:
			//continue
		case <-ctx.Done():
			return
		}
	}
}

func AddDbnameSysinfoIfNotExistsToQueryResultData(data metrics.Measurements, ver MonitoredDatabaseSettings, opts *cmdopts.Options) metrics.Measurements {
	enrichedData := make(metrics.Measurements, 0)
	for _, dr := range data {
		if opts.Sinks.RealDbnameField > "" && ver.RealDbname > "" {
			old, ok := dr[opts.Sinks.RealDbnameField]
			if !ok || old == "" {
				dr[opts.Sinks.RealDbnameField] = ver.RealDbname
			}
		}
		if opts.Sinks.SystemIdentifierField > "" && ver.SystemIdentifier > "" {
			old, ok := dr[opts.Sinks.SystemIdentifierField]
			if !ok || old == "" {
				dr[opts.Sinks.SystemIdentifierField] = ver.SystemIdentifier
			}
		}
		enrichedData = append(enrichedData, dr)
	}
	return enrichedData
}

var lastMonitoredDBsUpdate time.Time
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
		newRow := make(map[string]any)
		for k, v := range dr {
			newRow[k] = v
		}
		newData[i] = newRow
	}

	return newData
}

func FetchMetrics(ctx context.Context,
	msg MetricFetchConfig,
	hostState map[string]map[string]string,
	storageCh chan<- []metrics.MeasurementEnvelope,
	context string,
	opts *cmdopts.Options) ([]metrics.MeasurementEnvelope, error) {

	var dbSettings MonitoredDatabaseSettings
	var dbVersion int
	var err error
	var sql string
	var data, cachedData metrics.Measurements
	var md *sources.MonitoredDatabase
	var fromCache, isCacheable bool

	md, err = GetMonitoredDatabaseByUniqueName(msg.DBUniqueName)
	if err != nil {
		log.GetLogger(ctx).Errorf("[%s:%s] could not get monitored DB details", msg.DBUniqueName, err)
		return nil, err
	}

	dbSettings, err = GetMonitoredDatabaseSettings(ctx, msg.DBUniqueName, msg.Source, false)
	if err != nil {
		log.GetLogger(ctx).Error("failed to fetch pg version for ", msg.DBUniqueName, msg.MetricName, err)
		return nil, err
	}
	if msg.MetricName == specialMetricDbSize || msg.MetricName == specialMetricTableStats {
		if dbSettings.ExecEnv == execEnvAzureSingle && dbSettings.ApproxDBSizeB > 1e12 { // 1TB
			subsMetricName := msg.MetricName + "_approx"
			mvpApprox, err := GetMetricVersionProperties(subsMetricName, dbSettings, nil)
			if err == nil && mvpApprox.StorageName == msg.MetricName {
				log.GetLogger(ctx).Infof("[%s:%s] Transparently swapping metric to %s due to hard-coded rules...", msg.DBUniqueName, msg.MetricName, subsMetricName)
				msg.MetricName = subsMetricName
			}
		}
	}
	dbVersion = dbSettings.Version

	if msg.Source == sources.SourcePgBouncer {
		dbVersion = 0 // version is 0.0 for all pgbouncer sql per convention
	}

	mvp, err := GetMetricVersionProperties(msg.MetricName, dbSettings, nil)
	if err != nil && msg.MetricName != recoMetricName {
		epoch, ok := lastSQLFetchError.Load(msg.MetricName + dbMetricJoinStr + fmt.Sprintf("%v", dbVersion))
		if !ok || ((time.Now().Unix() - epoch.(int64)) > 3600) { // complain only 1x per hour
			log.GetLogger(ctx).Infof("Failed to get SQL for metric '%s', version '%s': %v", msg.MetricName, dbSettings.VersionStr, err)
			lastSQLFetchError.Store(msg.MetricName+dbMetricJoinStr+fmt.Sprintf("%v", dbVersion), time.Now().Unix())
		}
		if strings.Contains(err.Error(), "too old") {
			return nil, nil
		}
		return nil, err
	}

	isCacheable = IsCacheableMetric(msg, mvp)
	if isCacheable && opts.Metrics.InstanceLevelCacheMaxSeconds > 0 && msg.Interval.Seconds() > float64(opts.Metrics.InstanceLevelCacheMaxSeconds) {
		cachedData = GetFromInstanceCacheIfNotOlderThanSeconds(msg, opts.Metrics.InstanceLevelCacheMaxSeconds)
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

	if msg.MetricName == specialMetricChangeEvents && context != contextPrometheusScrape { // special handling, multiple queries + stateful
		CheckForPGObjectChangesAndStore(ctx, msg.DBUniqueName, dbSettings, storageCh, hostState) // TODO no hostState for Prometheus currently
	} else if msg.MetricName == recoMetricName && context != contextPrometheusScrape {
		if data, err = GetRecommendations(ctx, msg.DBUniqueName, dbSettings); err != nil {
			return nil, err
		}
	} else if msg.Source == sources.SourcePgPool {
		if data, err = FetchMetricsPgpool(ctx, msg, dbSettings, mvp); err != nil {
			return nil, err
		}
	} else {
		data, err = DBExecReadByDbUniqueName(ctx, msg.DBUniqueName, sql)

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

			if strings.Contains(err.Error(), "connection refused") {
				SetDBUnreachableState(msg.DBUniqueName)
			}

			log.GetLogger(ctx).Infof("[%s:%s] failed to fetch metrics: %s", msg.DBUniqueName, msg.MetricName, err)

			return nil, err
		}

		log.GetLogger(ctx).WithFields(map[string]any{"source": msg.DBUniqueName, "metric": msg.MetricName, "rows": len(data)}).Info("measurements fetched")
		// if regexIsPgbouncerMetrics.MatchString(msg.MetricName) { // clean unwanted pgbouncer pool stats here as not possible in SQL
		// 	data = FilterPgbouncerData(ctx, data, md.GetDatabaseName(), dbSettings)
		// }

		ClearDBUnreachableStateIfAny(msg.DBUniqueName)

	}

	if isCacheable && opts.Metrics.InstanceLevelCacheMaxSeconds > 0 && msg.Interval.Seconds() > float64(opts.Metrics.InstanceLevelCacheMaxSeconds) {
		PutToInstanceCache(msg, data)
	}

send_to_storageChannel:

	if (opts.Sinks.RealDbnameField > "" || opts.Sinks.SystemIdentifierField > "") && msg.Source == sources.SourcePostgres {
		MonitoredDatabasesSettingsLock.RLock()
		ver := MonitoredDatabasesSettings[msg.DBUniqueName]
		MonitoredDatabasesSettingsLock.RUnlock()
		data = AddDbnameSysinfoIfNotExistsToQueryResultData(data, ver, opts)
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
