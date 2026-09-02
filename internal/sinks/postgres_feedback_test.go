package sinks

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newFeedbackWriter(t *testing.T, opts *CmdOpts) (*PostgresWriter, pgxmock.PgxPoolIface) {
	t.Helper()
	conn, err := pgxmock.NewPool()
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, conn.ExpectationsWereMet()) })
	return &PostgresWriter{ctx: ctx, sinkDb: conn, opts: opts}, conn
}

func TestPostgresCanFeedback(t *testing.T) {
	enabled := &CmdOpts{RetentionInterval: "14 days"}
	disabled := &CmdOpts{RetentionInterval: "14 days", NoFeedback: true}

	for _, tc := range []struct {
		name           string
		opts           *CmdOpts
		source, metric string
		want           bool
	}{
		{"non-empty pair", enabled, "prod-db", "db_stats", true},
		{"empty source", enabled, "", "db_stats", false},
		{"empty metric", enabled, "prod-db", "", false},
		{"both empty", enabled, "", "", false},
		{"feedback disabled", disabled, "prod-db", "db_stats", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pgw, _ := newFeedbackWriter(t, tc.opts)
			assert.Equal(t, tc.want, pgw.CanFeedback(tc.source, tc.metric))
		})
	}
}

// TestPostgresCanFeedbackIgnoresPartitionMap pins PGS-002: partitionMapMetric
// is populated only by the flush path, so keying capability on it would deny
// every metric at process start.
func TestPostgresCanFeedbackIgnoresPartitionMap(t *testing.T) {
	pgw, _ := newFeedbackWriter(t, &CmdOpts{RetentionInterval: "14 days"})
	require.Empty(t, pgw.partitionMapMetric)
	assert.True(t, pgw.CanFeedback("prod-db", "never_written_metric"))
}

func TestPostgresLastMeasurement(t *testing.T) {
	opts := &CmdOpts{RetentionInterval: "14 days"}

	t.Run("returns newest epoch", func(t *testing.T) {
		pgw, conn := newFeedbackWriter(t, opts)
		stored := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)
		conn.ExpectQuery(`SELECT time FROM public."db_stats"`).
			WithArgs("prod-db", "14 days").
			WillReturnRows(pgxmock.NewRows([]string{"time"}).AddRow(stored))

		epoch, err := pgw.LastMeasurement(ctx, "prod-db", "db_stats")
		require.NoError(t, err)
		assert.Equal(t, stored.UnixNano(), epoch)
		assert.Positive(t, epoch, "a nil error implies a positive epoch")
	})

	t.Run("no rows", func(t *testing.T) {
		pgw, conn := newFeedbackWriter(t, opts)
		conn.ExpectQuery(`SELECT time FROM public."db_stats"`).
			WithArgs("prod-db", "14 days").
			WillReturnRows(pgxmock.NewRows([]string{"time"}))

		epoch, err := pgw.LastMeasurement(ctx, "prod-db", "db_stats")
		assert.ErrorIs(t, err, ErrNoFeedbackData)
		assert.Zero(t, epoch)
	})

	t.Run("undefined table", func(t *testing.T) {
		pgw, conn := newFeedbackWriter(t, opts)
		conn.ExpectQuery(`SELECT time FROM public."gone"`).
			WithArgs("prod-db", "14 days").
			WillReturnError(&pgconn.PgError{Code: "42P01", Message: `relation "gone" does not exist`})

		epoch, err := pgw.LastMeasurement(ctx, "prod-db", "gone")
		assert.ErrorIs(t, err, ErrFeedbackUnsupported)
		assert.Zero(t, epoch)
	})

	t.Run("other query error propagates", func(t *testing.T) {
		pgw, conn := newFeedbackWriter(t, opts)
		conn.ExpectQuery(`SELECT time FROM public."db_stats"`).
			WithArgs("prod-db", "14 days").
			WillReturnError(assert.AnError)

		epoch, err := pgw.LastMeasurement(ctx, "prod-db", "db_stats")
		assert.ErrorIs(t, err, assert.AnError)
		assert.Zero(t, epoch)
	})

	t.Run("feedback disabled issues no query", func(t *testing.T) {
		pgw, _ := newFeedbackWriter(t, &CmdOpts{RetentionInterval: "14 days", NoFeedback: true})

		epoch, err := pgw.LastMeasurement(ctx, "prod-db", "db_stats")
		assert.ErrorIs(t, err, ErrFeedbackUnsupported)
		assert.Zero(t, epoch)
	})

	t.Run("cancelled context issues no query", func(t *testing.T) {
		pgw, _ := newFeedbackWriter(t, opts)
		cancelled, cancel := context.WithCancel(ctx)
		cancel()

		epoch, err := pgw.LastMeasurement(cancelled, "prod-db", "db_stats")
		assert.ErrorIs(t, err, context.Canceled)
		assert.Zero(t, epoch)
	})
}

// TestPostgresLastMeasurementSQLSafety pins SEC-001: the source name travels
// as a bind parameter and the metric name is quoted as an identifier, so a
// hostile metric name cannot break out of the table reference.
func TestPostgresLastMeasurementSQLSafety(t *testing.T) {
	pgw, conn := newFeedbackWriter(t, &CmdOpts{RetentionInterval: "14 days"})
	hostile := `evil"; DROP TABLE admin.migration; --`
	conn.ExpectQuery(`SELECT time FROM public."evil""; DROP TABLE admin\.migration; --"`).
		WithArgs("prod-db'; DROP TABLE admin.migration; --", "14 days").
		WillReturnRows(pgxmock.NewRows([]string{"time"}))

	_, err := pgw.LastMeasurement(ctx, "prod-db'; DROP TABLE admin.migration; --", hostile)
	assert.ErrorIs(t, err, ErrNoFeedbackData)
}
