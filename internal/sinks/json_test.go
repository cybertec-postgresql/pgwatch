package sinks

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJSONWriter_Write(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)
	// Define test data
	msg := metrics.MeasurementEnvelope{
		MetricName: "test_metric",
		Data: metrics.Measurements{
			{"number": 1, "string": "test_data"},
		},
		DBName:     "test_db",
		CustomTags: map[string]string{"foo": "boo"},
	}

	tempFile := t.TempDir() + "/test.json"
	ctx, cancel := context.WithCancel(context.Background())
	jw, err := NewJSONWriter(ctx, tempFile)
	r.NoError(err)

	err = jw.Write(msg)
	a.NoError(err, "write successful")
	err = jw.Write(metrics.MeasurementEnvelope{})
	r.NoError(err, "empty write successful")

	cancel()
	err = jw.Write(msg)
	a.Error(err, "context canceled")

	// Read the contents of the file
	var data map[string]any
	file, err := os.ReadFile(tempFile)
	r.NoError(err)
	err = json.Unmarshal(file, &data)
	r.NoError(err)
	a.Equal(msg.MetricName, data["metric"])
	a.Equal(len(msg.Data), len(data["data"].([]any)))
	a.Equal(msg.DBName, data["dbname"])
	a.Equal(len(msg.CustomTags), len(data["custom_tags"].(map[string]any)))
}

func TestJSONWriter_SyncMetric(t *testing.T) {
	// Create a temporary file for testing
	tempFile := t.TempDir() + "/test.json"

	ctx, cancel := context.WithCancel(context.Background())
	jw, err := NewJSONWriter(ctx, tempFile)
	assert.NoError(t, err)

	// Call the function being tested
	err = jw.SyncMetric("", "", "")
	assert.NoError(t, err)

	cancel()
	err = jw.SyncMetric("", "", "")
	assert.Error(t, err, "context canceled")

}
