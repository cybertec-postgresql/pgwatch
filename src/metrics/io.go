package metrics

import (
	"context"
	"errors"
	"sync"

	"github.com/cybertec-postgresql/pgwatch3/log"
)

// Writer is an interface that writes metrics values
type Writer interface {
	SyncMetric(dbUnique, metricName, op string) error
	Write(msgs []MetricStoreMessage) error
}

type MultiWriter struct {
	writers []Writer
	sync.Mutex
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

func (mw *MultiWriter) WriteMetrics(ctx context.Context, storageCh <-chan []MetricStoreMessage) {
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
