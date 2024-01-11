package sinks

import (
	"context"
	"encoding/json"
	"os"
	"sync/atomic"

	"github.com/cybertec-postgresql/pgwatch3/log"
	"github.com/cybertec-postgresql/pgwatch3/metrics"
)

var (
	totalMetricsDroppedCounter    uint64
	datastoreWriteFailuresCounter uint64
)

type JSONWriter struct {
	ctx      context.Context
	filename string
}

func NewJSONWriter(ctx context.Context, fname string) (*JSONWriter, error) {
	if jf, err := os.Create(fname); err != nil {
		return nil, err
	} else if err = jf.Close(); err != nil {
		return nil, err
	}
	return &JSONWriter{
		ctx:      ctx,
		filename: fname,
	}, nil
}

func (jw *JSONWriter) Write(msgs []metrics.MetricStoreMessage) error {
	if len(msgs) == 0 {
		return nil
	}
	logger := log.GetLogger(jw.ctx)
	jsonOutFile, err := os.OpenFile(jw.filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0640)
	if err != nil {
		atomic.AddUint64(&datastoreWriteFailuresCounter, 1)
		return err
	}
	defer func() { _ = jsonOutFile.Close() }()
	logger.Infof("Writing %d metric sets to JSON file at \"%s\"...", len(msgs), jw.filename)
	enc := json.NewEncoder(jsonOutFile)
	for _, msg := range msgs {
		dataRow := map[string]any{
			"metric":      msg.MetricName,
			"data":        msg.Data,
			"dbname":      msg.DBName,
			"custom_tags": msg.CustomTags,
		}
		err = enc.Encode(dataRow)
		if err != nil {
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
