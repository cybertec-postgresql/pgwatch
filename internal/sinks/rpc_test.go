package sinks

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/log"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/testutil"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var ctx = log.WithLogger(context.Background(), log.NewNoopLogger())

func TestMain(m *testing.M) {
	// Setup
	rpcTeardown, err := testutil.SetupRPCServers()
	if err != nil {
		rpcTeardown()
		panic(err)
	}

	// Execute all tests
	exitCode := m.Run()

	// Teardown
	rpcTeardown()
	os.Exit(exitCode)
}

// Tests begin from here ---------------------------------------------------------

func TestCACertParamValidation(t *testing.T) {
	a := assert.New(t)
	_, err := NewRPCWriter(ctx, testutil.TLSConnStr)
	a.NoError(err)

	_, _ = os.Create("badca.crt")
	defer func() { _ = os.Remove("badca.crt") }()

	BadRPCParams := map[string]string{
		"?sslrootca=file.txt":  "error loading CA file",
		"?sslrootca=":          "error loading CA file",
		"?sslrootca=badca.crt": "invalid CA file",
	}

	for param, errMsg := range BadRPCParams {
		_, err = NewRPCWriter(ctx, fmt.Sprintf("grpc://%s%s", testutil.TLSServerAddress, param))
		a.ErrorContains(err, errMsg)
	}
}

func TestRPCTLSWriter(t *testing.T) {
	a := assert.New(t)

	rw, err := NewRPCWriter(ctx, testutil.TLSConnStr)
	a.NoError(err)

	// no error for valid messages
	msgs := metrics.MeasurementEnvelope{
		DBName: "Db",
		Data:   metrics.Measurements{{"test": 1}},
	}
	err = rw.Write(msgs)
	a.NoError(err)
}

func TestRPCWrite(t *testing.T) {
	a := assert.New(t)

	rw, err := NewRPCWriter(ctx, testutil.PlainConnStr)
	a.NoError(err)

	// no error for valid messages
	msgs := metrics.MeasurementEnvelope{
		DBName: "Db",
		Data:   metrics.Measurements{{"test": 1}},
	}
	err = rw.Write(msgs)
	a.NoError(err)

	// error for invalid messages
	msgs.DBName = "invalid"
	err = rw.Write(msgs)
	a.ErrorIs(err, status.Error(codes.Unknown, "invalid message"))

	// error for empty messages
	err = rw.Write(metrics.MeasurementEnvelope{})
	a.ErrorIs(err, status.Error(codes.Unknown, "empty message"))

	// error for cancelled context
	ctx, cancel := context.WithCancel(ctx)
	rw, err = NewRPCWriter(ctx, testutil.PlainConnStr)
	a.NoError(err)
	cancel()
	err = rw.Write(msgs)
	a.Error(err)
}

func TestRPCSyncMetric(t *testing.T) {
	a := assert.New(t)

	rw, err := NewRPCWriter(ctx, testutil.PlainConnStr)
	a.NoError(err)

	// no error for valid Sync requests
	err = rw.SyncMetric("Test-DB", "DB-Metric", AddOp)
	a.NoError(err)

	// error for invalid Sync requests
	err = rw.SyncMetric("", "", InvalidOp)
	a.ErrorIs(err, status.Error(codes.Unknown, "invalid sync request"))

	// error for cancelled context
	ctx, cancel := context.WithCancel(ctx)
	rw, err = NewRPCWriter(ctx, testutil.PlainConnStr)
	a.NoError(err)
	cancel()
	err = rw.SyncMetric("Test-DB", "DB-Metric", AddOp)
	a.Error(err)
}

func TestRPCDefineMetric(t *testing.T) {
	a := assert.New(t)

	rw, err := NewRPCWriter(ctx, testutil.PlainConnStr)
	a.NoError(err)

	// Test that RPCWriter implements MetricsDefiner interface
	var writer Writer = rw
	definer, ok := writer.(MetricsDefiner)
	a.True(ok, "RPCWriter should implement MetricsDefiner interface")

	// Test with valid metrics
	testMetrics := &metrics.Metrics{
		MetricDefs: metrics.MetricDefs{
			"test_metric": metrics.Metric{
				SQLs: metrics.SQLs{
					11: "SELECT 1 as test_column",
					12: "SELECT 2 as test_column",
				},
				Description: "Test metric",
				Gauges:      []string{"test_column"},
			},
		},
		PresetDefs: metrics.PresetDefs{
			"test_preset": metrics.Preset{
				Description: "Test preset",
				Metrics:     map[string]float64{"test_metric": 30.0},
			},
		},
	}

	err = definer.DefineMetrics(testMetrics)
	a.NoError(err)

	// Test with empty metrics (should still work)
	emptyMetrics := &metrics.Metrics{
		MetricDefs: make(metrics.MetricDefs),
		PresetDefs: make(metrics.PresetDefs),
	}

	err = definer.DefineMetrics(emptyMetrics)
	a.NoError(err)

	// Test with cancelled context
	ctx, cancel := context.WithCancel(ctx)
	rw, err = NewRPCWriter(ctx, testutil.PlainConnStr)
	a.NoError(err)
	cancel()

	writer = rw
	definer = writer.(MetricsDefiner)
	err = definer.DefineMetrics(testMetrics)
	a.Error(err)
}

func TestAuthCredsSending(t *testing.T) {
	a := assert.New(t)

	unauthenticatedConnStr := "grpc://notpgwatch:notpgwatch@localhost:6060"
	rw, err := NewRPCWriter(ctx, unauthenticatedConnStr)
	a.NoError(err)

	err = rw.Write(metrics.MeasurementEnvelope{})
	a.Equal(err, status.Error(codes.Unauthenticated, "unauthenticated"))
}
