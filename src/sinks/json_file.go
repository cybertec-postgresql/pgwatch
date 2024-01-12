package sinks

import (
	"context"
	"encoding/json"
	"io"
	"sync/atomic"

	"github.com/cybertec-postgresql/pgwatch3/metrics"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	totalMetricsDroppedCounter    uint64
	datastoreWriteFailuresCounter uint64
)

type JSONWriter struct {
	ctx      context.Context
	filename string
	w        io.Writer
}

func NewJSONWriter(ctx context.Context, fname string) (*JSONWriter, error) {
	return &JSONWriter{
		ctx:      ctx,
		filename: fname,
		w:        &lumberjack.Logger{Filename: fname, Compress: true},
	}, nil
}

func (jw *JSONWriter) Write(msgs []metrics.MeasurementMessage) error {
	if len(msgs) == 0 {
		return nil
	}
	enc := json.NewEncoder(jw.w)
	for _, msg := range msgs {
		dataRow := map[string]any{
			"metric":      msg.MetricName,
			"data":        msg.Data,
			"dbname":      msg.DBName,
			"custom_tags": msg.CustomTags,
		}
		if err := enc.Encode(dataRow); err != nil {
			atomic.AddUint64(&datastoreWriteFailuresCounter, 1)
			return err
		}
	}
	return nil
}

func (jw *JSONWriter) SyncMetric(_, _, _ string) error {
	// do nothing, we don't care
	return nil
}
