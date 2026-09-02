package sinks

import (
	"sync"
	"testing"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v6/internal/db"
	"github.com/cybertec-postgresql/pgwatch/v6/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v6/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const feedbackMetric = "db_stats"

// setupFeedbackSink starts a real Postgres container and returns a fully
// initialised PostgresWriter pointed at it.
func setupFeedbackSink(t *testing.T) *PostgresWriter {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	pgContainer, tearDown, err := testutil.SetupPostgresContainer()
	require.NoError(t, err, "failed to start postgres container")
	t.Cleanup(tearDown)

	connStr, err := pgContainer.ConnectionString(testutil.TestContext, "sslmode=disable")
	require.NoError(t, err)

	pool, err := db.New(testutil.TestContext, connStr)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	opts := &CmdOpts{
		BatchingDelay:       950 * time.Millisecond,
		PartitionInterval:   "1 day",
		RetentionInterval:   "14 days",
		MaintenanceInterval: "12 hours",
	}
	pgw, err := NewWriterFromPostgresConn(ctx, pool, opts)
	require.NoError(t, err)
	return pgw
}

// writeAndFlush pushes one measurement and waits for the async poll loop to
// persist it, so the test observes only durably accepted data (REQ-011).
func writeAndFlush(t *testing.T, pgw *PostgresWriter, source string, at time.Time) {
	t.Helper()
	require.NoError(t, pgw.SyncMetric(source, feedbackMetric, AddOp))
	require.NoError(t, pgw.Write(metrics.MeasurementEnvelope{
		DBName:     source,
		MetricName: feedbackMetric,
		Data: metrics.Measurements{metrics.Measurement{
			metrics.EpochColumnName: at.UnixNano(),
			"numbackends":           int64(7),
		}},
	}))
	require.Eventually(t, func() bool {
		epoch, err := pgw.LastMeasurement(ctx, source, feedbackMetric)
		return err == nil && epoch == at.UnixNano()
	}, 30*time.Second, 100*time.Millisecond, "measurement was never flushed")
}

func TestPostgresFeedbackIntegration(t *testing.T) {
	pgw := setupFeedbackSink(t)

	t.Run("unwritten metric reports unsupported", func(t *testing.T) {
		// The table does not exist, so the optimistic CanFeedback is overruled
		// by the authoritative query (PGS-002, PGS-007).
		assert.True(t, pgw.CanFeedback("prod-db", "never_created_metric"))
		_, err := pgw.LastMeasurement(ctx, "prod-db", "never_created_metric")
		assert.ErrorIs(t, err, ErrFeedbackUnsupported)
	})

	t.Run("known metric without rows reports no data", func(t *testing.T) {
		require.NoError(t, pgw.SyncMetric("absent-src", feedbackMetric, AddOp))
		_, err := pgw.LastMeasurement(ctx, "absent-src", feedbackMetric)
		assert.ErrorIs(t, err, ErrNoFeedbackData)
	})

	t.Run("reports the newest stored epoch", func(t *testing.T) {
		const source = "epoch-src"
		older := time.Now().Add(-2 * time.Hour).Truncate(time.Microsecond)
		newer := time.Now().Add(-1 * time.Hour).Truncate(time.Microsecond)
		writeAndFlush(t, pgw, source, older)
		writeAndFlush(t, pgw, source, newer)

		epoch, err := pgw.LastMeasurement(ctx, source, feedbackMetric)
		require.NoError(t, err)
		// timestamptz keeps microseconds only (DAT-001).
		assert.Equal(t, newer.UnixNano(), epoch)
	})

	t.Run("scoped to the source", func(t *testing.T) {
		_, err := pgw.LastMeasurement(ctx, "some-other-source", feedbackMetric)
		assert.ErrorIs(t, err, ErrNoFeedbackData)
	})

	t.Run("retention bound keeps the query fast", func(t *testing.T) {
		const source = "perf-src"
		writeAndFlush(t, pgw, source, time.Now().Truncate(time.Microsecond))

		start := time.Now()
		_, err := pgw.LastMeasurement(ctx, source, feedbackMetric)
		require.NoError(t, err)
		assert.Less(t, time.Since(start), 100*time.Millisecond)
	})
}

// TestPostgresFeedbackRace pins PGS-008: neither feedback method takes
// PostgresWriter.mu, so a feedback query cannot stall the partition DDL that
// SyncMetric serialises. Run under -race.
func TestPostgresFeedbackRace(t *testing.T) {
	pgw := setupFeedbackSink(t)
	const source = "race-src"
	require.NoError(t, pgw.SyncMetric(source, feedbackMetric, AddOp))

	var wg sync.WaitGroup
	done := make(chan struct{})

	wg.Go(func() {
		for i := 0; ; i++ {
			select {
			case <-done:
				return
			default:
				_ = pgw.SyncMetric(source, feedbackMetric, AddOp)
				_ = pgw.Write(metrics.MeasurementEnvelope{
					DBName:     source,
					MetricName: feedbackMetric,
					Data: metrics.Measurements{metrics.Measurement{
						metrics.EpochColumnName: time.Now().UnixNano(),
						"numbackends":           int64(i),
					}},
				})
			}
		}
	})

	wg.Go(func() {
		for range 200 {
			pgw.CanFeedback(source, feedbackMetric)
		}
	})

	finished := make(chan struct{})
	go func() {
		defer close(finished)
		for range 20 {
			_, _ = pgw.LastMeasurement(ctx, source, feedbackMetric)
		}
	}()

	select {
	case <-finished:
	case <-time.After(60 * time.Second):
		t.Fatal("feedback queries stalled behind the writer")
	}
	close(done)
	wg.Wait()
}
