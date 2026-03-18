package reaper

import (
	"context"
	"testing"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/cmdopts"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/db"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/log"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/sinks"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/sources"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupIntegrationDB starts a real Postgres container and returns a SourceConn
// with a live pgxpool connection. The caller must call tearDown when done.
func setupIntegrationDB(t *testing.T) (*sources.SourceConn, func()) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	pgContainer, tearDown, err := testutil.SetupPostgresContainer()
	require.NoError(t, err, "failed to start postgres container")

	connStr, err := pgContainer.ConnectionString(testutil.TestContext, "sslmode=disable")
	require.NoError(t, err, "failed to get connection string")

	pool, err := db.New(testutil.TestContext, connStr)
	require.NoError(t, err, "failed to create connection pool")

	md := sources.NewSourceConn(sources.Source{
		Name: "integration_test",
		Kind: sources.SourcePostgres,
	})
	md.Conn = pool
	err = md.FetchRuntimeInfo(testutil.TestContext, true)
	require.NoError(t, err, "failed to fetch runtime info")

	return md, func() {
		pool.Close()
		tearDown()
	}
}

// TestIntegration_ExecuteBatch verifies the full executeBatch path against a real
// Postgres instance: builds a pgx.Batch from metric definitions, sends it, and
// receives MeasurementEnvelopes on the measurement channel.
func TestIntegration_ExecuteBatch(t *testing.T) {
	md, tearDown := setupIntegrationDB(t)
	defer tearDown()

	ctx := log.WithLogger(context.Background(), log.NewNoopLogger())

	metricDefs.MetricDefs["integ_version"] = metrics.Metric{
		SQLs: metrics.SQLs{0: "SELECT version() AS pg_version"},
	}
	metricDefs.MetricDefs["integ_uptime"] = metrics.Metric{
		SQLs: metrics.SQLs{0: "SELECT extract(epoch from now() - pg_postmaster_start_time())::int8 AS uptime_seconds"},
	}
	defer func() {
		delete(metricDefs.MetricDefs, "integ_version")
		delete(metricDefs.MetricDefs, "integ_uptime")
	}()

	md.Source.Metrics = metrics.MetricIntervals{
		"integ_version": 30,
		"integ_uptime":  60,
	}

	r := &Reaper{
		Options: &cmdopts.Options{
			Metrics: metrics.CmdOpts{},
			Sinks:   sinks.CmdOpts{},
		},
		measurementCh:    make(chan metrics.MeasurementEnvelope, 10),
		measurementCache: NewInstanceMetricCache(),
	}
	sr := NewSourceReaper(r, md)

	err := sr.executeBatch(ctx, []batchEntry{
		{name: "integ_version", metric: metricDefs.MetricDefs["integ_version"], sql: "SELECT version() AS pg_version"},
		{name: "integ_uptime", metric: metricDefs.MetricDefs["integ_uptime"], sql: "SELECT extract(epoch from now() - pg_postmaster_start_time())::int8 AS uptime_seconds"},
	})
	require.NoError(t, err)

	received := make(map[string]metrics.MeasurementEnvelope)
	for range 10 {
		select {
		case msg := <-r.measurementCh:
			received[msg.MetricName] = msg
		default:
		}
	}

	assert.Contains(t, received, "integ_version")
	assert.Contains(t, received, "integ_uptime")

	if msg, ok := received["integ_version"]; ok {
		assert.Equal(t, "integration_test", msg.DBName)
		assert.NotEmpty(t, msg.Data)
		assert.Contains(t, msg.Data[0]["pg_version"], "PostgreSQL")
	}

	if msg, ok := received["integ_uptime"]; ok {
		assert.Equal(t, "integration_test", msg.DBName)
		assert.NotEmpty(t, msg.Data)
		assert.NotNil(t, msg.Data[0]["uptime_seconds"])
	}
}

// TestIntegration_SourceReaper_RunCollectsMetrics verifies the full Run loop:
// creates a SourceReaper with two SQL metrics, lets it run one tick against
// a real Postgres container, and verifies that both metric envelopes arrive.
func TestIntegration_SourceReaper_RunCollectsMetrics(t *testing.T) {
	md, tearDown := setupIntegrationDB(t)
	defer tearDown()

	metricDefs.MetricDefs["integ_run_version"] = metrics.Metric{
		SQLs: metrics.SQLs{0: "SELECT version() AS pg_version"},
	}
	metricDefs.MetricDefs["integ_run_size"] = metrics.Metric{
		SQLs: metrics.SQLs{0: "SELECT pg_database_size(current_database()) AS db_size_bytes"},
	}
	defer func() {
		delete(metricDefs.MetricDefs, "integ_run_version")
		delete(metricDefs.MetricDefs, "integ_run_size")
	}()

	md.Source.Metrics = metrics.MetricIntervals{
		"integ_run_version": 5,
		"integ_run_size":    5,
	}

	r := &Reaper{
		Options: &cmdopts.Options{
			Metrics: metrics.CmdOpts{},
			Sinks:   sinks.CmdOpts{},
		},
		measurementCh:    make(chan metrics.MeasurementEnvelope, 20),
		measurementCache: NewInstanceMetricCache(),
	}
	sr := NewSourceReaper(r, md)

	ctx, cancel := context.WithCancel(log.WithLogger(context.Background(), log.NewNoopLogger()))

	done := make(chan struct{})
	go func() {
		sr.Run(ctx)
		close(done)
	}()

	received := make(map[string]metrics.MeasurementEnvelope)
	deadline := time.After(15 * time.Second)
	for len(received) < 2 {
		select {
		case msg := <-r.measurementCh:
			received[msg.MetricName] = msg
		case <-deadline:
			t.Fatal("timed out waiting for measurements")
		}
	}

	cancel()
	<-done

	assert.Contains(t, received, "integ_run_version")
	assert.Contains(t, received, "integ_run_size")

	vMsg := received["integ_run_version"]
	assert.Equal(t, "integration_test", vMsg.DBName)
	assert.NotEmpty(t, vMsg.Data)
	assert.Contains(t, vMsg.Data[0]["pg_version"], "PostgreSQL")

	sMsg := received["integ_run_size"]
	assert.Equal(t, "integration_test", sMsg.DBName)
	assert.NotEmpty(t, sMsg.Data)
}
