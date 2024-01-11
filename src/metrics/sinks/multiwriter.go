package sinks

import (
	"context"
	"errors"
	"sync"

	"github.com/cybertec-postgresql/pgwatch3/config"
	"github.com/cybertec-postgresql/pgwatch3/log"
	"github.com/cybertec-postgresql/pgwatch3/metrics"
)

const (
	DSjson       = "json"
	DSpostgres   = "postgres"
	DSprometheus = "prometheus"
)

// Writer is an interface that writes metrics values
type Writer interface {
	SyncMetric(dbUnique, metricName, op string) error
	Write(msgs []metrics.MetricStoreMessage) error
}

type MultiWriter struct {
	writers []Writer
	sync.Mutex
}

func NewMultiWriter(ctx context.Context, opts *config.Options, metricDefs metrics.MetricVersionDefs) (*MultiWriter, error) {
	logger := log.GetLogger(ctx)
	mw := &MultiWriter{}
	for _, f := range opts.Metric.JSONStorageFile {
		jw, err := NewJSONWriter(ctx, f)
		if err != nil {
			return nil, err
		}
		mw.AddWriter(jw)
		logger.WithField("file", f).Info(`JSON output enabled`)
	}

	for _, connstr := range opts.Metric.PGMetricStoreConnStr {
		pgw, err := NewPostgresWriter(ctx, connstr, opts, metricDefs)
		if err != nil {
			return nil, err
		}
		mw.AddWriter(pgw)
		logger.WithField("connstr", connstr).Info(`PostgreSQL output enabled`)
	}

	if opts.Metric.PrometheusListenAddr > "" {
		promw, err := NewPrometheusWriter(ctx, opts)
		if err != nil {
			return nil, err
		}
		mw.AddWriter(promw)
		logger.WithField("listen", opts.Metric.PrometheusListenAddr).Info(`Prometheus output enabled`)
	}
	if len(mw.writers) == 0 {
		return nil, errors.New("no storages specified for metrics")
	}
	return mw, nil
}

func (mw *MultiWriter) AddWriter(w Writer) {
	mw.Lock()
	mw.writers = append(mw.writers, w)
	mw.Unlock()
}

func (mw *MultiWriter) SyncMetrics(dbUnique, metricName, op string) (err error) {
	for _, w := range mw.writers {
		err = errors.Join(err, w.SyncMetric(dbUnique, metricName, op))
	}
	return
}

func (mw *MultiWriter) WriteMetrics(ctx context.Context, storageCh <-chan []metrics.MetricStoreMessage) {
	var err error
	logger := log.GetLogger(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-storageCh:
			for _, w := range mw.writers {
				err = w.Write(msg)
				if err != nil {
					logger.Error(err)
				}
			}
		}
	}
}
