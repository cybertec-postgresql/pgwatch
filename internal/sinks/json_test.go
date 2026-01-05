package sinks_test

import (
	"context"
	"os"
	"testing"

	jsoniter "github.com/json-iterator/go"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/sinks"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/testutil"
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
	ctx, cancel := context.WithCancel(testutil.TestContext)
	jw, err := sinks.NewJSONWriter(ctx, tempFile)
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
	err = jsoniter.ConfigFastest.Unmarshal(file, &data)
	r.NoError(err)
	a.Equal(msg.MetricName, data["metric"])
	a.Equal(len(msg.Data), len(data["data"].([]any)))
	a.Equal(msg.DBName, data["dbname"])
	a.Equal(len(msg.CustomTags), len(data["custom_tags"].(map[string]any)))
}

func TestJSONWriter_SyncMetric(t *testing.T) {
	// Create a temporary file for testing
	tempFile := t.TempDir() + "/test.json"

	ctx, cancel := context.WithCancel(testutil.TestContext)
	jw, err := sinks.NewJSONWriter(ctx, tempFile)
	assert.NoError(t, err)

	// Call the function being tested
	err = jw.SyncMetric("", "", sinks.InvalidOp)
	assert.NoError(t, err)

	cancel()
	err = jw.SyncMetric("", "", sinks.InvalidOp)
	assert.Error(t, err, "context canceled")

}
