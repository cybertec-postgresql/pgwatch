package reaper

import (
	"context"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"sync/atomic"

	"github.com/cybertec-postgresql/pgwatch/v6/internal/cmdopts"
	"github.com/cybertec-postgresql/pgwatch/v6/internal/log"
	"github.com/cybertec-postgresql/pgwatch/v6/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v6/internal/sinks"
	"github.com/cybertec-postgresql/pgwatch/v6/internal/sources"
	"golang.org/x/sync/errgroup"
)

const (
	specialMetricChangeEvents         = "change_events"
	specialMetricServerLogEventCounts = "server_log_event_counts"
	specialMetricInstanceUp           = "instance_up"
)

// maxConcurrentSourceConnects bounds how many sources may be connected
// concurrently during one refresh sweep. It is intentionally fixed: it is
// not configurable and does not scale with the size of the monitored fleet.
const maxConcurrentSourceConnects = 32

var metricDefs = NewConcurrentMetricDefs()

type Reaper interface {
	Reap(ctx context.Context)
}

type Readier interface {
	Ready() bool
}

type ReadierReaper interface {
	Reaper
	Readier
}

// reaper is the struct that responsible for fetching metrics measurements from the sources and storing them to the sinks
type reaper struct {
	*cmdopts.Options
	ready            atomic.Bool
	measurementCh    chan metrics.MeasurementEnvelope
	measurementCache *InstanceMetricCache
	logger           log.Logger
	// monitoredSources and prevLoopMonitoredDBs are only mutated in the
	// sequential sections of the main loop (LoadSources before the sweep and
	// CleanupRemovedWorkers after the sweep barrier); they are read-only
	// while per-source workers run, so they need no lock.
	monitoredSources     sources.SourceConns
	prevLoopMonitoredDBs sources.SourceConns
	// mu guards srcRecoveryStatus and cancelFuncs.
	mu                sync.Mutex
	srcRecoveryStatus map[string]bool
	cancelFuncs       map[string]context.CancelFunc // [sourceName]cancel() — one per source
}

func NewReaper(ctx context.Context, opts *cmdopts.Options) ReadierReaper {
	return newReaper(ctx, opts)
}

func newReaper(ctx context.Context, opts *cmdopts.Options) (r *reaper) {
	return &reaper{
		Options:              opts,
		measurementCh:        make(chan metrics.MeasurementEnvelope, 256),
		measurementCache:     NewInstanceMetricCache(),
		logger:               log.GetLogger(ctx),
		monitoredSources:     make(sources.SourceConns, 0),
		prevLoopMonitoredDBs: make(sources.SourceConns, 0),
		srcRecoveryStatus:    make(map[string]bool),
		cancelFuncs:          make(map[string]context.CancelFunc), // [sourceName]cancel()
	}
}

// Ready() returns true if the service is healthy and operating correctly
func (r *reaper) Ready() bool {
	return r.ready.Load()
}

func (r *reaper) PrintMemStats() {
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
func (r *reaper) Reap(ctx context.Context) {
	var err error

	go r.WriteMeasurements(ctx)

	r.ready.Store(true)

	for { //main loop
		if r.Logging.LogLevel == "debug" {
			r.PrintMemStats()
		}
		if err = r.LoadSources(ctx); err != nil {
			r.logger.Error("could not refresh active sources, using last valid cache:", err)
		}
		if err = r.LoadMetrics(); err != nil {
			r.logger.Error("could not refresh metric definitions, using last valid cache:", err)
		}

		// Sources are processed with bounded concurrency so that a single
		// hanging source cannot serialize the whole refresh. Per-source
		// processing order is not guaranteed.
		var g errgroup.Group
		g.SetLimit(maxConcurrentSourceConnects)
		for _, monitoredSource := range r.monitoredSources {
			g.Go(func() error {
				src := monitoredSource.GetSource()
				srcL := r.logger.WithField("source", src.Name)
				srcCtx := log.WithLogger(ctx, srcL)

				if err := monitoredSource.Connect(srcCtx, r.Sources); err != nil {
					r.WriteInstanceDown(src.Name)
					srcL.Warning("could not init connection, retrying on next iteration:", err)
					return nil
				}

				switch md := monitoredSource.(type) {
				case *sources.DbConn:
					if err := md.FetchRuntimeInfo(srcCtx, true); err != nil {
						srcL.Error("could not start metric gathering:", err)
						return nil
					}
					if r.FilterSource(srcCtx, md) {
						return nil
					}
					r.CreateSourceHelpers(srcCtx, md)
					r.TrackRecoveryStatus(srcCtx, md)
					r.SyncMetricsToSinks(srcCtx, md)
					r.StartWorker(srcCtx, src.Name, NewDbConnReaper(r, md))
				case *sources.PromConn:
					r.StartWorker(srcCtx, src.Name, NewPromSourceReaper(r, md))
				}
				return nil
			})
		}
		// Barrier: cleanup stays sequential and runs only after every source
		// of this sweep has been processed.
		_ = g.Wait()
		r.CleanupRemovedWorkers(ctx)
		select {
		case <-time.After(time.Second * time.Duration(r.Sources.Refresh)):
			r.logger.Debugf("wake up after %d seconds", r.Sources.Refresh)
		case <-ctx.Done():
			return
		}
	}
}

// StartWorker launches a source reaper goroutine for the given source if one
// is not already running. It is a no-op when a worker for that name exists.
func (r *reaper) StartWorker(ctx context.Context, sourceName string, sr Reaper) {
	sourceCtx, cancelFunc := context.WithCancel(ctx)
	r.mu.Lock()
	if _, exists := r.cancelFuncs[sourceName]; exists {
		r.mu.Unlock()
		cancelFunc()
		return
	}
	r.cancelFuncs[sourceName] = cancelFunc
	r.mu.Unlock()
	log.GetLogger(ctx).Info("starting source reaper")
	go sr.Reap(sourceCtx)
}

// FilterSource snapshots the mutable RuntimeInfo under a single RLock, logs
// the connection status, and applies both eligibility filters:
//   - OnlyIfMaster: skip standby sources that must not be monitored as standbys.
//   - MinDbSizeMB:  skip sources whose database is below the configured size threshold.
//
// Returns true when the source should be skipped for this loop iteration.
func (r *reaper) FilterSource(ctx context.Context, md *sources.DbConn) bool {
	md.RLock()
	isInRecovery := md.IsInRecovery
	versionStr := md.VersionStr
	DBSizeMB := md.ApproxDbSize / 1048576
	md.RUnlock()

	l := log.GetLogger(ctx)

	if isInRecovery && md.OnlyIfMaster {
		l.Info("not added to monitoring due to 'master only' property and status change")
		r.ShutdownWorker(ctx, md.Name)
		return true
	}

	if DBSizeMB != 0 && DBSizeMB < r.Sources.MinDbSizeMB {
		l.Infof("ignored due to the --min-db-size-mb filter, current size %d MB", DBSizeMB)
		r.ShutdownWorker(ctx, md.Name)
		return true
	}

	l.WithField("recovery", isInRecovery).Infof("Connect OK. Version: %s", versionStr)
	return false
}

// TrackRecoveryStatus logs any primary/standby role changes and updates the
// per-source recovery-status cache.
func (r *reaper) TrackRecoveryStatus(ctx context.Context, md *sources.DbConn) {
	md.RLock()
	isInRecovery := md.IsInRecovery
	hasStandbyConfig := len(md.MetricsStandby) > 0
	md.RUnlock()

	r.mu.Lock()
	statusChanged := r.srcRecoveryStatus[md.Name] != isInRecovery
	r.srcRecoveryStatus[md.Name] = isInRecovery
	r.mu.Unlock()

	if statusChanged {
		l := log.GetLogger(ctx)
		if isInRecovery && hasStandbyConfig {
			l.Warning("Switching metrics collection to standby config...")
		} else if !isInRecovery {
			l.Warning("Switching metrics collection to primary config...")
		}
		// else: standby without a dedicated standby config keeps primary config, no warn
	}
}

// SyncMetricsToSinks syncs metric names with sinks for the active config
func (r *reaper) SyncMetricsToSinks(ctx context.Context, md *sources.DbConn) {
	l := log.GetLogger(ctx)
	for metricName := range md.ActiveMetrics() {
		mvp, metricDefExists := metricDefs.GetMetricDef(metricName)
		if !metricDefExists {
			epoch, ok := lastSQLFetchError.Load(metricName)
			if !ok || ((time.Now().Unix() - epoch.(int64)) > 3600) {
				l.WithField("metric", metricName).Warning("metric definition not found")
				lastSQLFetchError.Store(metricName, time.Now().Unix())
			}
			continue
		}
		metricNameForStorage := metricName
		if !r.isSpecialMetric(metricName) && mvp.StorageName > "" {
			metricNameForStorage = mvp.StorageName
		}
		if err := r.SinksWriter.SyncMetric(md.Name, metricNameForStorage, sinks.AddOp); err != nil {
			l.Error(err)
		}
	}
}

// CreateSourceHelpers creates the extensions and metric helpers for the monitored source
func (r *reaper) CreateSourceHelpers(ctx context.Context, monitoredSource *sources.DbConn) {
	if r.prevLoopMonitoredDBs.GetMonitoredDatabase(monitoredSource.Name) != nil {
		return // already created
	}
	monitoredSource.RLock()
	isInRecovery := monitoredSource.IsInRecovery
	monitoredSource.RUnlock()
	if !monitoredSource.IsPostgresSource() || isInRecovery {
		return // no need to create anything for non-postgres sources
	}

	l := log.GetLogger(ctx)
	if r.Sources.TryCreateListedExtsIfMissing > "" {
		l.Info("trying to create extensions if missing")
		extsToCreate := strings.Split(r.Sources.TryCreateListedExtsIfMissing, ",")
		extsCreated, err := monitoredSource.TryCreateMissingExtensions(ctx, extsToCreate)
		if err != nil {
			l.Warning(err)
		}
		if extsCreated != "" {
			l.Infof("%d/%d extensions created: %s", len(extsCreated), len(extsToCreate), extsCreated)
		}
	}

	if r.Sources.CreateHelpers {
		l.Info("trying to create helper objects if missing")
		if err := monitoredSource.TryCreateMetricsHelpers(ctx, func(metric string) string {
			if m, ok := metricDefs.GetMetricDef(metric); ok {
				return m.InitSQL
			}
			return ""
		}); err != nil {
			l.Warning(err)
		}
	}
}

// isSpecialMetric reports whether a metric name has special handling that
// bypasses the StorageName override.
func (r *reaper) isSpecialMetric(name string) bool {
	return name == specialMetricChangeEvents || name == specialMetricServerLogEventCounts
}

// ShutdownWorker stops the source reaper for a single named source, closes its
// connection pool, and deregisters it from the sinks.
func (r *reaper) ShutdownWorker(_ context.Context, sourceName string) {
	r.mu.Lock()
	cancelFunc, exists := r.cancelFuncs[sourceName]
	if exists {
		delete(r.cancelFuncs, sourceName)
	}
	r.mu.Unlock()
	if exists {
		r.logger.WithField("source", sourceName).Info("stopping source reaper...")
		cancelFunc()
	}
	if db := r.monitoredSources.GetMonitoredDatabase(sourceName); db != nil {
		db.Close()
	}
	if err := r.SinksWriter.SyncMetric(sourceName, "", sinks.DeleteOp); err != nil {
		r.logger.Error(err)
	}
}

// CleanupRemovedWorkers stops workers for sources that are no longer in
// monitoredSources or whose context has been cancelled, and closes connections
// for any sources that disappeared from the previous loop without a running worker.
func (r *reaper) CleanupRemovedWorkers(ctx context.Context) {
	r.logger.Debug("checking if any workers need to be shut down...")
	// Snapshot the worker names under the lock; ShutdownWorker locks per call,
	// so iterating the live map while deleting from it is not an option.
	r.mu.Lock()
	sourceNames := make([]string, 0, len(r.cancelFuncs))
	for sourceName := range r.cancelFuncs {
		sourceNames = append(sourceNames, sourceName)
	}
	r.mu.Unlock()
	for _, sourceName := range sourceNames {
		md := r.monitoredSources.GetMonitoredDatabase(sourceName)
		if ctx.Err() == nil && md != nil {
			continue // source still active
		}
		if md == nil {
			r.logger.Debugf("Source %s removed from config, shutting down source reaper...", sourceName)
		}
		r.ShutdownWorker(ctx, sourceName)
	}
	// Close connections for sources that disappeared without ever having a worker.
	for _, prevDB := range r.prevLoopMonitoredDBs {
		if r.monitoredSources.GetMonitoredDatabase(prevDB.GetSource().Name) == nil {
			prevDB.Close()
			_ = r.SinksWriter.SyncMetric(prevDB.GetSource().Name, "", sinks.DeleteOp)
		}
	}
	r.prevLoopMonitoredDBs = slices.Clone(r.monitoredSources)
}

// LoadSources loads sources from the reader
func (r *reaper) LoadSources(ctx context.Context) (err error) {
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
		// filter out disabled sources and sources with group not in the list of groups to monitor
		return !s.IsEnabled || len(r.Sources.Groups) > 0 && !slices.Contains(r.Sources.Groups, s.Group)
	})

	if newSrcs, err = srcs.ResolveDatabases(r.WriteInstanceDown); err != nil {
		// discover dtabases for continuous monitoring sources
		r.logger.WithError(err).Error("could not resolve databases from sources")
	}

	for i, newMD := range newSrcs {
		md := r.monitoredSources.GetMonitoredDatabase(newMD.GetSource().Name)
		if md == nil {
			continue
		}
		if md.GetSource().Equal(newMD.GetSource()) {
			// replace with the existing connection if the source is the same
			newSrcs[i] = md
			continue
		}
		// Source configs changed, stop all running gatherers to trigger a restart
		// TODO: Optimize this for single metric addition/deletion/interval-change cases to not do a full restart
		r.logger.WithField("source", md.GetSource().Name).Info("Source configs changed, restarting all gatherers...")
		r.ShutdownWorker(ctx, md.GetSource().Name)
	}
	r.monitoredSources = newSrcs
	r.logger.WithField("sources", len(r.monitoredSources)).Info("sources refreshed")
	return nil
}

// WriteInstanceDown writes instance_up = 0 metric to sinks for the given source
func (r *reaper) WriteInstanceDown(name string) {
	r.measurementCh <- metrics.MeasurementEnvelope{
		DBName:     name,
		MetricName: specialMetricInstanceUp,
		Data: metrics.Measurements{metrics.Measurement{
			metrics.EpochColumnName: time.Now().UnixNano(),
			specialMetricInstanceUp: 0},
		},
	}
}

// GetMeasurementCache returns the instance-level metric cache
func (r *reaper) GetMeasurementCache(key string) metrics.Measurements {
	return r.measurementCache.Get(key, r.Metrics.CacheAge())
}

// WriteMeasurements() writes the metrics to the sinks
func (r *reaper) WriteMeasurements(ctx context.Context) {
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

func (r *reaper) AddSysinfoToMeasurements(data metrics.Measurements, md *sources.DbConn) {
	md.RLock()
	realDbname := md.RealDbname
	systemIdentifier := md.SystemIdentifier
	md.RUnlock()
	for _, dr := range data {
		if r.Sinks.RealDbnameField > "" && realDbname > "" {
			dr[r.Sinks.RealDbnameField] = realDbname
		}
		if r.Sinks.SystemIdentifierField > "" && systemIdentifier > "" {
			dr[r.Sinks.SystemIdentifierField] = systemIdentifier
		}
	}
}
