package sinks

import (
	"context"
	"encoding/json"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"
	"gopkg.in/natefinch/lumberjack.v2"
)

// JSONWriter is a sink that writes metric measurements to a file in JSON format.
// It supports compression and rotation of output files. The default rotation is based on the file size (100Mb).
// JSONWriter is useful for debugging and testing purposes, as well as for integration with other systems,
// such as log aggregators, analytics systems, and data processing pipelines, ML models, etc.
type JSONWriter struct {
	ctx context.Context
	lw  *lumberjack.Logger
}

func NewJSONWriter(ctx context.Context, fname string) (*JSONWriter, error) {
	jw := &JSONWriter{
		ctx: ctx,
		lw:  &lumberjack.Logger{Filename: fname, Compress: true},
	}
	go jw.watchCtx()
	return jw, nil
}

func (jw *JSONWriter) Write(msgs []metrics.MeasurementEnvelope) error {
	if jw.ctx.Err() != nil {
		return jw.ctx.Err()
	}
	if len(msgs) == 0 {
		return nil
	}
	enc := json.NewEncoder(jw.lw)
	for _, msg := range msgs {
		dataRow := map[string]any{
			"metric":      msg.MetricName,
			"data":        msg.Data,
			"dbname":      msg.DBName,
			"custom_tags": msg.CustomTags,
		}
		if err := enc.Encode(dataRow); err != nil {
			return err
		}
	}
	return nil
}

func (jw *JSONWriter) watchCtx() {
	<-jw.ctx.Done()
	jw.lw.Close()
}

func (jw *JSONWriter) SyncMetric(_, _, _ string) error {
	if jw.ctx.Err() != nil {
		return jw.ctx.Err()
	}
	// do nothing, we don't care
	return nil
}
