package sinks

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"strings"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"
)

// Writer is an interface that writes metrics values
type Writer interface {
	SyncMetric(dbUnique, metricName string, op SyncOp) error
	Write(msgs metrics.MeasurementEnvelope) error
}

// MetricDefiner is an interface for passing metric definitions to a sink.
type MetricsDefiner interface {
	DefineMetrics(metric *metrics.Metrics) error
}

// MultiWriter ensures the simultaneous storage of data in several storages.
type MultiWriter struct {
	writers []Writer
	sync.Mutex
}

// NewSinkWriter creates and returns new instance of MultiWriter struct.
func NewSinkWriter(ctx context.Context, opts *CmdOpts) (w Writer, err error) {
	if len(opts.Sinks) == 0 {
		return nil, errors.New("no sinks specified for measurements")
	}
	mw := &MultiWriter{}
	for _, s := range opts.Sinks {
		scheme, path, found := strings.Cut(s, "://")
		if !found || scheme == "" || path == "" {
			return nil, fmt.Errorf("malformed sink URI %s", s)
		}
		switch scheme {
		case "jsonfile":
			w, err = NewJSONWriter(ctx, path)
		case "postgres", "postgresql":
			w, err = NewPostgresWriter(ctx, s, opts)
		case "prometheus":
			w, err = NewPrometheusWriter(ctx, path)
		case "rpc":
			w, err = NewRPCWriter(ctx, path)
		default:
			return nil, fmt.Errorf("unknown schema %s in sink URI %s", scheme, s)
		}
		if err != nil {
			return nil, err
		}
		mw.AddWriter(w)
	}
	if len(mw.writers) == 1 {
		return mw.writers[0], nil
	}
	return mw, nil
}

func (mw *MultiWriter) AddWriter(w Writer) {
	mw.Lock()
	mw.writers = append(mw.writers, w)
	mw.Unlock()
}

func (mw *MultiWriter) DefineMetrics(metrics *metrics.Metrics) (err error) {
	for _, w := range mw.writers {
		if definer, ok := w.(MetricsDefiner); ok {
			err = errors.Join(err, definer.DefineMetrics(metrics))
		}
	}
	return nil
}

func (mw *MultiWriter) SyncMetric(dbUnique, metricName string, op SyncOp) (err error) {
	for _, w := range mw.writers {
		err = errors.Join(err, w.SyncMetric(dbUnique, metricName, op))
	}
	return
}

func (mw *MultiWriter) Write(msg metrics.MeasurementEnvelope) (err error) {
	for _, w := range mw.writers {
		err = errors.Join(err, w.Write(msg))
	}
	return
}
