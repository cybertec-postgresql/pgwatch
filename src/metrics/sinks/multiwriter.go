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

func NewMultiWriter(ctx context.Context, opts *config.CmdOptions) (*MultiWriter, error) {
	logger := log.GetLogger(ctx)
	mw := &MultiWriter{}
	if opts.Metric.Datastore == DSjson {
		jw, err := NewJSONWriter(ctx, opts)
		if err != nil {
			return nil, err
		}
		mw.AddWriter(jw)
		logger.WithField("file", opts.Metric.JSONStorageFile).Info(`JSON output enabled`)
	}

	if opts.Metric.Datastore == DSpostgres {
		pgw, err := NewPostgresWriter(ctx, opts)
		if err != nil {
			return nil, err
		}
		mw.AddWriter(pgw)
		logger.WithField("connstr", opts.Metric.PGMetricStoreConnStr).Info(`PostgreSQL output enabled`)
	}

	if opts.Metric.Datastore == DSprometheus {
		promw, err := NewPrometheusWriter(ctx, opts)
		if err != nil {
			return nil, err
		}
		mw.AddWriter(promw)
		logger.WithField("connstr", opts.Metric.PrometheusListenAddr).Info(`Prometheus output enabled`)
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
