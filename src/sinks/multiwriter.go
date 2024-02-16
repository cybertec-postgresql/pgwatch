package sinks

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/cybertec-postgresql/pgwatch3/config"
	"github.com/cybertec-postgresql/pgwatch3/log"
	"github.com/cybertec-postgresql/pgwatch3/metrics"
)

// Writer is an interface that writes metrics values
type Writer interface {
	SyncMetric(dbUnique, metricName, op string) error
	Write(msgs []metrics.MeasurementMessage) error
}

// MultiWriter ensures the simultaneous storage of data in several storages.
type MultiWriter struct {
	writers []Writer
	sync.Mutex
}

// NewMultiWriter creates and returns new instance of MultiWriter struct.
func NewMultiWriter(ctx context.Context, opts *config.Options, metricDefs metrics.MetricVersionDefs) (mw *MultiWriter, err error) {
	var w Writer
	logger := log.GetLogger(ctx)
	mw = &MultiWriter{}
	for _, s := range opts.Measurements.Sinks {
		scheme, path, found := strings.Cut(s, "://")
		if !found || scheme == "" || path == "" {
			return nil, fmt.Errorf("malformed sink URI %s", s)
		}
		switch scheme {
		case "jsonfile":
			w, err = NewJSONWriter(ctx, path)
		case "postgres", "postgresql":
			w, err = NewPostgresWriter(ctx, s, opts, metricDefs)
		case "prometheus":
			w, err = NewPrometheusWriter(ctx, path)
		default:
			return nil, fmt.Errorf("unknown schema %s in sink URI %s", scheme, s)
		}
		if err != nil {
			return nil, err
		}
		mw.AddWriter(w)
		logger.WithField("sink", s).Info(`measurements sink added`)
	}

	if len(mw.writers) == 0 {
		return nil, errors.New("no sinks specified for measurements")
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

func (mw *MultiWriter) WriteMeasurements(ctx context.Context, storageCh <-chan []metrics.MeasurementMessage) {
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
