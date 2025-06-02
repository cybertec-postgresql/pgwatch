package sinks

import (
	"context"
	"time"

	jsoniter "github.com/json-iterator/go"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/log"
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
	enc *jsoniter.Encoder
}

func NewJSONWriter(ctx context.Context, fname string) (*JSONWriter, error) {
	l := log.GetLogger(ctx).WithField("sink", "jsonfile").WithField("filename", fname)
	ctx = log.WithLogger(ctx, l)
	jw := &JSONWriter{
		ctx: ctx,
		lw:  &lumberjack.Logger{Filename: fname, Compress: true},
	}
	jw.enc = jsoniter.ConfigFastest.NewEncoder(jw.lw)
	go jw.watchCtx()
	return jw, nil
}

func (jw *JSONWriter) Write(msg metrics.MeasurementEnvelope) error {
	if jw.ctx.Err() != nil {
		return jw.ctx.Err()
	}
	if len(msg.Data) == 0 {
		return nil
	}
	t1 := time.Now()
	written := 0

	dataRow := map[string]any{
		"metric":      msg.MetricName,
		"data":        msg.Data,
		"dbname":      msg.DBName,
		"custom_tags": msg.CustomTags,
	}
	if err := jw.enc.Encode(dataRow); err != nil {
		return err
	}
	written += len(msg.Data)

	diff := time.Since(t1)
	log.GetLogger(jw.ctx).WithField("rows", written).WithField("elapsed", diff).Info("measurements written")
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
