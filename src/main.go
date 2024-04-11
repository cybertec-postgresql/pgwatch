package main

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"math"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/cybertec-postgresql/pgwatch3/config"
	"github.com/cybertec-postgresql/pgwatch3/db"
	"github.com/cybertec-postgresql/pgwatch3/log"
	"github.com/cybertec-postgresql/pgwatch3/metrics"
	"github.com/cybertec-postgresql/pgwatch3/reaper"
	"github.com/cybertec-postgresql/pgwatch3/sources"
	"github.com/cybertec-postgresql/pgwatch3/webserver"
)

// / internal statistics calculation
var lastSuccessfulDatastoreWriteTimeEpoch int64
var datastoreWriteFailuresCounter uint64
var datastoreWriteSuccessCounter uint64
var totalMetricFetchFailuresCounter uint64
var datastoreTotalWriteTimeMicroseconds uint64
var totalMetricsFetchedCounter uint64
var totalMetricsReusedFromCacheCounter uint64
var totalMetricsDroppedCounter uint64
var totalDatasetsFetchedCounter uint64
var metricPointsPerMinuteLast5MinAvg int64 = -1 // -1 means the summarization ticker has not yet run
var gathererStartTime = time.Now()

var logger log.LoggerHookerIface

func StatsServerHandler(w http.ResponseWriter, _ *http.Request) {
	jsonResponseTemplate := `
{
	"secondsFromLastSuccessfulDatastoreWrite": %d,
	"totalMetricsFetchedCounter": %d,
	"totalMetricsReusedFromCacheCounter": %d,
	"totalDatasetsFetchedCounter": %d,
	"metricPointsPerMinuteLast5MinAvg": %v,
	"metricsDropped": %d,
	"totalMetricFetchFailuresCounter": %d,
	"datastoreWriteFailuresCounter": %d,
	"datastoreSuccessfulWritesCounter": %d,
	"datastoreAvgSuccessfulWriteTimeMillis": %.1f,
	"databasesMonitored": %d,
	"databasesConfigured": %d,
	"unreachableDBs": %d,
	"gathererUptimeSeconds": %d
}
`
	now := time.Now()
	secondsFromLastSuccessfulDatastoreWrite := atomic.LoadInt64(&lastSuccessfulDatastoreWriteTimeEpoch)
	totalMetrics := atomic.LoadUint64(&totalMetricsFetchedCounter)
	cacheMetrics := atomic.LoadUint64(&totalMetricsReusedFromCacheCounter)
	totalDatasets := atomic.LoadUint64(&totalDatasetsFetchedCounter)
	metricsDropped := atomic.LoadUint64(&totalMetricsDroppedCounter)
	metricFetchFailuresCounter := atomic.LoadUint64(&totalMetricFetchFailuresCounter)
	datastoreFailures := atomic.LoadUint64(&datastoreWriteFailuresCounter)
	datastoreSuccess := atomic.LoadUint64(&datastoreWriteSuccessCounter)
	datastoreTotalTimeMicros := atomic.LoadUint64(&datastoreTotalWriteTimeMicroseconds) // successful writes only
	datastoreAvgSuccessfulWriteTimeMillis := float64(datastoreTotalTimeMicros) / float64(datastoreSuccess) / 1000.0
	gathererUptimeSeconds := uint64(now.Sub(gathererStartTime).Seconds())
	var metricPointsPerMinute int64
	metricPointsPerMinute = atomic.LoadInt64(&metricPointsPerMinuteLast5MinAvg)
	if metricPointsPerMinute == -1 { // calculate avg. on the fly if 1st summarization hasn't happened yet
		metricPointsPerMinute = int64((totalMetrics * 60) / gathererUptimeSeconds)
	}
	databasesConfigured := 0 // including replicas
	databasesMonitored := 0
	unreachableDBs := 0
	_, _ = io.WriteString(w, fmt.Sprintf(jsonResponseTemplate, time.Now().Unix()-secondsFromLastSuccessfulDatastoreWrite, totalMetrics, cacheMetrics, totalDatasets, metricPointsPerMinute, metricsDropped, metricFetchFailuresCounter, datastoreFailures, datastoreSuccess, datastoreAvgSuccessfulWriteTimeMillis, databasesMonitored, databasesConfigured, unreachableDBs, gathererUptimeSeconds))
}

// Calculates 1min avg metric fetching statistics for last 5min for StatsServerHandler to display
func StatsSummarizer(ctx context.Context) {
	var prevMetricsCounterValue uint64
	var currentMetricsCounterValue uint64
	ticker := time.NewTicker(time.Minute * 5)
	lastSummarization := gathererStartTime
	for {
		select {
		case now := <-ticker.C:
			currentMetricsCounterValue = atomic.LoadUint64(&totalMetricsFetchedCounter)
			atomic.StoreInt64(&metricPointsPerMinuteLast5MinAvg, int64(math.Round(float64(currentMetricsCounterValue-prevMetricsCounterValue)*60/now.Sub(lastSummarization).Seconds())))
			prevMetricsCounterValue = currentMetricsCounterValue
			lastSummarization = now
		case <-ctx.Done():
			return
		}
	}
}

var opts *config.Options

// version output variables
var (
	commit  = "unknown"
	version = "unknown"
	date    = "unknown"
	dbapi   = "00534"
)

func printVersion() {
	fmt.Printf(`pgwatch3:
  Version:      %s
  DB Schema:    %s
  Git Commit:   %s
  Built:        %s
`, version, dbapi, commit, date)
}

// SetupCloseHandler creates a 'listener' on a new goroutine which will notify the
// program if it receives an interrupt from the OS. We then handle this by calling
// our clean up procedure and exiting the program.
func SetupCloseHandler(cancel context.CancelFunc) {
	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		logger.Debug("SetupCloseHandler received an interrupt from OS. Closing session...")
		cancel()
		exitCode.Store(ExitCodeUserCancel)
	}()
}

const (
	ExitCodeOK int32 = iota
	ExitCodeConfigError
	ExitCodeInitError
	ExitCodeWebUIError
	ExitCodeUpgradeError
	ExitCodeUserCancel
	ExitCodeShutdownCommand
	ExitCodeFatalError
)

var exitCode atomic.Int32

var mainContext context.Context

func NewConfigurationReaders(opts *config.Options) (sourcesReader sources.ReaderWriter, metricsReader metrics.ReaderWriter, err error) {
	var configKind config.Kind
	configKind, err = opts.GetConfigKind()
	switch {
	case err != nil:
		return
	case configKind != config.ConfigPgURL:
		ctx := log.WithLogger(mainContext, logger.WithField("config", "files"))
		if sourcesReader, err = sources.NewYAMLSourcesReaderWriter(ctx, opts.Sources.Config); err != nil {
			return
		}
		metricsReader, err = metrics.NewYAMLMetricReaderWriter(ctx, opts.Metrics.Metrics)
	default:
		var configDb db.PgxPoolIface
		ctx := log.WithLogger(mainContext, logger.WithField("config", "postgres"))
		if configDb, err = db.New(ctx, opts.Sources.Config); err != nil {
			return
		}
		if metricsReader, err = metrics.NewPostgresMetricReaderWriter(ctx, configDb); err != nil {
			return
		}
		sourcesReader, err = sources.NewPostgresSourcesReaderWriter(ctx, configDb)
	}
	return
}

func main() {
	var (
		err    error
		cancel context.CancelFunc
	)
	exitCode.Store(ExitCodeOK)
	defer func() {
		if err := recover(); err != nil {
			exitCode.Store(ExitCodeFatalError)
			log.GetLogger(mainContext).WithField("callstack", string(debug.Stack())).Error(err)
		}
		os.Exit(int(exitCode.Load()))
	}()

	mainContext, cancel = context.WithCancel(context.Background())
	SetupCloseHandler(cancel)
	defer cancel()

	if opts, err = config.New(os.Stdout); err != nil {
		exitCode.Store(ExitCodeConfigError)
		fmt.Print(err)
		return
	}
	if opts.VersionOnly() {
		printVersion()
		return
	}
	logger = log.Init(opts.Logging)
	mainContext = log.WithLogger(mainContext, logger)

	logger.Debugf("opts: %+v", opts)

	// sourcesReaderWriter reads/writes the monitored sources (databases, patroni clusets, pgpools, etc.) information
	// metricsReaderWriter reads/writes the metric and preset definitions
	sourcesReaderWriter, metricsReaderWriter, err := NewConfigurationReaders(opts)
	if err != nil {
		exitCode.Store(ExitCodeInitError)
		logger.Error(err)
		return
	}
	if opts.Sources.Init {
		// At this point we have initialised the sources, metrics and presets configurations.
		// Any fatal errors are handled by the configuration readers. So me may exit gracefully.
		return
	}

	if !opts.Ping {
		go StatsSummarizer(mainContext)
		uifs, _ := fs.Sub(webuifs, "webui/build")
		ui := webserver.Init(opts.WebUI, uifs, metricsReaderWriter, sourcesReaderWriter, logger)
		if ui == nil {
			os.Exit(int(ExitCodeWebUIError))
		}
	}

	reaper := reaper.NewReaper(opts, sourcesReaderWriter, metricsReaderWriter)
	if err = reaper.Reap(mainContext); err != nil {
		logger.Error(err)
		exitCode.Store(ExitCodeFatalError)
	}
}
