package sinks

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"

	"github.com/cybertec-postgresql/pgwatch/v6/internal/metrics"
)

// Writer is an interface that writes metrics values
type Writer interface {
	SyncMetric(sourceName, metricName string, op SyncOp) error
	Write(msgs metrics.MeasurementEnvelope) error
}

// MetricDefiner is an interface for passing metric definitions to a sink.
type MetricsDefiner interface {
	DefineMetrics(metrics *metrics.Metrics) error
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
	for _, sinkConnStr := range opts.Sinks {
		scheme, target, found := strings.Cut(sinkConnStr, "://")
		if !found || scheme == "" || target == "" {
			return nil, fmt.Errorf("malformed sink URI %s", sinkConnStr)
		}
		switch scheme {
		case "jsonfile":
			w, err = NewJSONWriter(ctx, target)
		case "postgres", "postgresql":
			w, err = NewPostgresWriter(ctx, sinkConnStr, opts)
		case "prometheus":
			w, err = NewPrometheusWriter(ctx, target)
		case "rpc", "grpc":
			w, err = NewRPCWriter(ctx, sinkConnStr)
		default:
			return nil, fmt.Errorf("unknown schema %s in sink URI %s", scheme, sinkConnStr)
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

func (mw *MultiWriter) Count() int {
	return len(mw.writers)
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
	return
}

func (mw *MultiWriter) SyncMetric(sourceName, metricName string, op SyncOp) (err error) {
	for _, w := range mw.writers {
		err = errors.Join(err, w.SyncMetric(sourceName, metricName, op))
	}
	return
}

func (mw *MultiWriter) Write(msg metrics.MeasurementEnvelope) (err error) {
	for _, w := range mw.writers {
		err = errors.Join(err, w.Write(msg))
	}
	return
}

// Migrator interface implementation for MultiWriter

// Migrate runs migrations on all writers that support it
func (mw *MultiWriter) Migrate() (err error) {
	for _, w := range mw.writers {
		if m, ok := w.(interface {
			Migrate() error
		}); ok {
			err = errors.Join(err, m.Migrate())
		}
	}
	return
}

// NeedsMigration checks if any writer needs migration
func (mw *MultiWriter) NeedsMigration() (bool, error) {
	for _, w := range mw.writers {
		if m, ok := w.(interface {
			NeedsMigration() (bool, error)
		}); ok {
			if needs, err := m.NeedsMigration(); err != nil {
				return false, err
			} else if needs {
				return true, nil
			}
		}
	}
	return false, nil
}

// CanFeedback reports whether at least one contained writer can answer for
// the pair. Writers that do not implement Feedbacker are simply skipped.
func (mw *MultiWriter) CanFeedback(sourceName, metricName string) bool {
	for _, w := range mw.writers {
		if fb, ok := w.(Feedbacker); ok && fb.CanFeedback(sourceName, metricName) {
			return true
		}
	}
	return false
}

// LastMeasurement returns the oldest epoch reported by any feedback-capable
// writer. The minimum is deliberate: a consumer resuming from the maximum
// would leave the laggard sink permanently short, whereas resuming from the
// minimum only duplicates measurements the leading sink already has.
// A writer holding nothing at all is the extreme case of lagging, so it
// short-circuits the whole aggregate to ErrNoFeedbackData.
func (mw *MultiWriter) LastMeasurement(ctx context.Context, sourceName, metricName string) (int64, error) {
	var (
		oldest  int64 = math.MaxInt64
		answers int
		errs    error
	)
	for _, w := range mw.writers {
		fb, ok := w.(Feedbacker)
		if !ok {
			continue // a sink that cannot answer does not get to veto the ones that can
		}
		epoch, err := fb.LastMeasurement(ctx, sourceName, metricName)
		switch {
		case errors.Is(err, ErrFeedbackUnsupported):
			continue
		case errors.Is(err, ErrNoFeedbackData):
			return 0, ErrNoFeedbackData
		case err != nil:
			errs = errors.Join(errs, err)
		default:
			answers++
			oldest = min(oldest, epoch)
		}
	}
	if errs != nil {
		return 0, errs
	}
	if answers == 0 {
		return 0, ErrFeedbackUnsupported
	}
	return oldest, nil
}
