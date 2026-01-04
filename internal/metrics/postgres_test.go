package metrics_test

import (
	"context"
	"testing"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/log"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
)

var ctx = log.WithLogger(context.Background(), log.NewNoopLogger())

func AnyArgs(n int) []any {
	args := make([]any, n)
	for i := range args {
		args[i] = pgxmock.AnyArg()
	}
	return args
}

func TestNewPostgresMetricReaderWriter(t *testing.T) {
	a := assert.New(t)

	t.Run("ConnectionError", func(*testing.T) {
		pgrw, err := metrics.NewPostgresMetricReaderWriter(ctx, "postgres://user:pass@foohost:5432/db1")
		a.Error(err)
		a.Nil(pgrw)
	})
	t.Run("InvalidConnStr", func(*testing.T) {
		pgrw, err := metrics.NewPostgresMetricReaderWriter(ctx, "invalid_connstr")
		a.Error(err)
		a.Nil(pgrw)
	})
}

func TestNewPostgresMetricReaderWriterConn(t *testing.T) {
	df := metrics.GetDefaultMetrics()
	metricsCount := len(df.MetricDefs)
	presetsCount := len(df.PresetDefs)

	a := assert.New(t)
	conn, err := pgxmock.NewPool()
	a.NoError(err)

	doesntExist := func() *pgxmock.Rows { return pgxmock.NewRows([]string{"exists"}).AddRow(false) }

	t.Run("FullBoostrap", func(*testing.T) {
		conn.ExpectQuery(`SELECT EXISTS`).WithArgs("pgwatch").WillReturnRows(doesntExist())
		conn.ExpectBegin()
		conn.ExpectExec("CREATE SCHEMA IF NOT EXISTS pgwatch").WillReturnResult(pgxmock.NewResult("CREATE", 1))
		conn.ExpectBegin()
		conn.ExpectExec(`INSERT.+metric`).WithArgs(AnyArgs(8)...).WillReturnResult(pgxmock.NewResult("INSERT", 1)).Times(uint(metricsCount))
		conn.ExpectExec(`INSERT.+preset`).WithArgs(AnyArgs(3)...).WillReturnResult(pgxmock.NewResult("INSERT", 1)).Times(uint(presetsCount))
		conn.ExpectCommit()
		conn.ExpectCommit()
		// Expect migration check
		conn.ExpectQuery(`SELECT to_regclass`).WithArgs("pgwatch.migration").WillReturnRows(pgxmock.NewRows([]string{"to_regclass"}).AddRow(true))
		conn.ExpectQuery(`SELECT count`).WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(metrics.ExpectedMigrationsCount))
		conn.ExpectPing()

		readerWriter, err := metrics.NewPostgresMetricReaderWriterConn(ctx, conn)
		a.NoError(err)
		a.NotNil(readerWriter)
		a.NoError(conn.ExpectationsWereMet())
	})

	t.Run("SchemaQueryFail", func(*testing.T) {
		conn.ExpectQuery(`SELECT EXISTS`).WithArgs("pgwatch").WillReturnError(assert.AnError)
		rw, err := metrics.NewPostgresMetricReaderWriterConn(ctx, conn)
		a.Error(err)
		a.Nil(rw)
		a.NoError(conn.ExpectationsWereMet())
	})

	t.Run("BeginFail", func(*testing.T) {
		conn.ExpectQuery(`SELECT EXISTS`).WithArgs("pgwatch").WillReturnRows(doesntExist())
		conn.ExpectBegin().WillReturnError(assert.AnError)
		rw, err := metrics.NewPostgresMetricReaderWriterConn(ctx, conn)
		a.Error(err)
		a.Nil(rw)
		a.NoError(conn.ExpectationsWereMet())
	})

	t.Run("CreateSchemaFail", func(*testing.T) {
		conn.ExpectQuery(`SELECT EXISTS`).WithArgs("pgwatch").WillReturnRows(doesntExist())
		conn.ExpectBegin()
		conn.ExpectExec("CREATE SCHEMA IF NOT EXISTS pgwatch").WillReturnError(assert.AnError)
		conn.ExpectRollback()
		rw, err := metrics.NewPostgresMetricReaderWriterConn(ctx, conn)
		a.Error(err)
		a.Nil(rw)
		a.NoError(conn.ExpectationsWereMet())
	})

	t.Run("WriteDefaultMetricsBeginFail", func(*testing.T) {
		conn.ExpectQuery(`SELECT EXISTS`).WithArgs("pgwatch").WillReturnRows(doesntExist())
		conn.ExpectBegin()
		conn.ExpectExec("CREATE SCHEMA IF NOT EXISTS pgwatch").WillReturnResult(pgxmock.NewResult("CREATE", 1))
		conn.ExpectBegin().WillReturnError(assert.AnError)
		conn.ExpectRollback()
		rw, err := metrics.NewPostgresMetricReaderWriterConn(ctx, conn)
		a.Error(err)
		a.Nil(rw)
		a.NoError(conn.ExpectationsWereMet())
	})

	t.Run("WriteInsertMetricsFail", func(*testing.T) {
		conn.ExpectQuery(`SELECT EXISTS`).WithArgs("pgwatch").WillReturnRows(doesntExist())
		conn.ExpectBegin()
		conn.ExpectExec("CREATE SCHEMA IF NOT EXISTS pgwatch").WillReturnResult(pgxmock.NewResult("CREATE", 1))
		conn.ExpectBegin()
		conn.ExpectExec(`INSERT.+metric`).WithArgs(AnyArgs(8)...).WillReturnError(assert.AnError)
		conn.ExpectRollback()
		rw, err := metrics.NewPostgresMetricReaderWriterConn(ctx, conn)
		a.Error(err)
		a.Nil(rw)
		a.NoError(conn.ExpectationsWereMet())
	})

	t.Run("WriteInsertPresetsFail", func(*testing.T) {
		conn.ExpectQuery(`SELECT EXISTS`).WithArgs("pgwatch").WillReturnRows(doesntExist())
		conn.ExpectBegin()
		conn.ExpectExec("CREATE SCHEMA IF NOT EXISTS pgwatch").WillReturnResult(pgxmock.NewResult("CREATE", 1))
		conn.ExpectBegin()
		conn.ExpectExec(`INSERT.+metric`).WithArgs(AnyArgs(8)...).WillReturnResult(pgxmock.NewResult("INSERT", 1)).Times(uint(metricsCount))
		conn.ExpectExec(`INSERT.+preset`).WithArgs(AnyArgs(3)...).WillReturnError(assert.AnError)
		conn.ExpectRollback()
		rw, err := metrics.NewPostgresMetricReaderWriterConn(ctx, conn)
		a.Error(err)
		a.Nil(rw)
		a.NoError(conn.ExpectationsWereMet())
	})

	t.Run("DefaultMetricsCommitFail", func(*testing.T) {
		conn.ExpectQuery(`SELECT EXISTS`).WithArgs("pgwatch").WillReturnRows(doesntExist())
		conn.ExpectBegin()
		conn.ExpectExec("CREATE SCHEMA IF NOT EXISTS pgwatch").WillReturnResult(pgxmock.NewResult("CREATE", 1))
		conn.ExpectBegin()
		conn.ExpectExec(`INSERT.+metric`).WithArgs(AnyArgs(8)...).WillReturnResult(pgxmock.NewResult("INSERT", 1)).Times(uint(metricsCount))
		conn.ExpectExec(`INSERT.+preset`).WithArgs(AnyArgs(3)...).WillReturnResult(pgxmock.NewResult("INSERT", 1)).Times(uint(presetsCount))
		conn.ExpectCommit().WillReturnError(assert.AnError)
		conn.ExpectRollback()
		rw, err := metrics.NewPostgresMetricReaderWriterConn(ctx, conn)
		a.Error(err)
		a.Nil(rw)
		a.NoError(conn.ExpectationsWereMet())
	})

	t.Run("CommitFail", func(*testing.T) {
		conn.ExpectQuery(`SELECT EXISTS`).WithArgs("pgwatch").WillReturnRows(doesntExist())
		conn.ExpectBegin()
		conn.ExpectExec("CREATE SCHEMA IF NOT EXISTS pgwatch").WillReturnResult(pgxmock.NewResult("CREATE", 1))
		conn.ExpectBegin()
		conn.ExpectExec(`INSERT.+metric`).WithArgs(AnyArgs(8)...).WillReturnResult(pgxmock.NewResult("INSERT", 1)).Times(uint(metricsCount))
		conn.ExpectExec(`INSERT.+preset`).WithArgs(AnyArgs(3)...).WillReturnResult(pgxmock.NewResult("INSERT", 1)).Times(uint(presetsCount))
		conn.ExpectCommit()
		conn.ExpectCommit().WillReturnError(assert.AnError)
		rw, err := metrics.NewPostgresMetricReaderWriterConn(ctx, conn)
		a.Error(err)
		a.Nil(rw)
		a.NoError(conn.ExpectationsWereMet())
	})

	t.Run("MigrationCheckFail", func(*testing.T) {
		conn.ExpectQuery(`SELECT EXISTS`).WithArgs("pgwatch").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
		conn.ExpectQuery(`SELECT to_regclass`).WithArgs("pgwatch.migration").WillReturnError(assert.AnError)
		rw, err := metrics.NewPostgresMetricReaderWriterConn(ctx, conn)
		a.Error(err)
		a.Nil(rw)
		a.NoError(conn.ExpectationsWereMet())
	})

	t.Run("MigrationNeeded", func(*testing.T) {
		conn.ExpectQuery(`SELECT EXISTS`).WithArgs("pgwatch").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
		conn.ExpectQuery(`SELECT to_regclass`).WithArgs("pgwatch.migration").WillReturnRows(pgxmock.NewRows([]string{"to_regclass"}).AddRow(true))
		conn.ExpectQuery(`SELECT count`).WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(metrics.ExpectedMigrationsCount - 1))
		rw, err := metrics.NewPostgresMetricReaderWriterConn(ctx, conn)
		a.Error(err)
		a.ErrorContains(err, "config database schema is outdated")
		a.ErrorContains(err, "pgwatch config upgrade")
		a.Nil(rw)
		a.NoError(conn.ExpectationsWereMet())
	})
}

func TestMetricsToPostgres(t *testing.T) {
	a := assert.New(t)
	conn, err := pgxmock.NewPool()
	a.NoError(err)

	conn.ExpectQuery(`SELECT EXISTS`).WithArgs("pgwatch").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	// Expect migration check
	conn.ExpectQuery(`SELECT to_regclass`).WithArgs("pgwatch.migration").WillReturnRows(pgxmock.NewRows([]string{"to_regclass"}).AddRow(true))
	conn.ExpectQuery(`SELECT count`).WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(metrics.ExpectedMigrationsCount))
	conn.ExpectPing()

	readerWriter, err := metrics.NewPostgresMetricReaderWriterConn(ctx, conn)
	a.NoError(err)
	a.NotNil(readerWriter)

	metricsRows := func() *pgxmock.Rows {
		return pgxmock.NewRows([]string{"name", "sqls", "init_sql", "description", "node_status", "gauges", "is_instance_level", "storage_name"}).
			AddRow("test", metrics.SQLs{11: "select"}, "init", "desc", "primary", []string{"*"}, true, "storage")
	}
	presetRows := func() *pgxmock.Rows {
		return pgxmock.NewRows([]string{"name", "description", "metrics"}).
			AddRow("test", "desc", map[string]float64{"metric": 30})
	}

	t.Run("GetMetrics", func(*testing.T) {
		conn.ExpectQuery(`SELECT.+FROM.+metric`).WillReturnRows(metricsRows())
		conn.ExpectQuery(`SELECT.+FROM.+preset`).WillReturnRows(presetRows())

		m, err := readerWriter.GetMetrics()
		a.NoError(err)
		a.Len(m.MetricDefs, 1)
	})

	t.Run("GetMetricsFail", func(*testing.T) {
		conn.ExpectQuery(`SELECT.+FROM.+metric`).WillReturnError(assert.AnError)
		_, err = readerWriter.GetMetrics()
		a.Error(err)
	})

	t.Run("GetPresetsFail", func(*testing.T) {
		conn.ExpectQuery(`SELECT.+FROM.+metric`).WillReturnRows(metricsRows())
		conn.ExpectQuery(`SELECT.+FROM.+preset`).WillReturnError(assert.AnError)
		_, err = readerWriter.GetMetrics()
		a.Error(err)
	})

	t.Run("GetMetricsScanFail", func(*testing.T) {
		conn.ExpectQuery(`SELECT.+FROM.+metric`).WillReturnRows(metricsRows().RowError(0, assert.AnError))
		_, err = readerWriter.GetMetrics()
		a.Error(err)
	})

	t.Run("GetPresetsScanFail", func(*testing.T) {
		conn.ExpectQuery(`SELECT.+FROM.+metric`).WillReturnRows(metricsRows())
		conn.ExpectQuery(`SELECT.+FROM.+preset`).WillReturnRows(presetRows().RowError(0, assert.AnError))
		_, err = readerWriter.GetMetrics()
		a.Error(err)
	})

	t.Run("WriteMetrics", func(*testing.T) {
		conn.ExpectBegin().WillReturnError(assert.AnError)
		err = readerWriter.WriteMetrics(&metrics.Metrics{})
		a.Error(err)
	})

	t.Run("DeleteMetric", func(*testing.T) {
		conn.ExpectExec(`DELETE.+metric`).WithArgs("test").WillReturnResult(pgxmock.NewResult("DELETE", 1))
		err = readerWriter.DeleteMetric("test")
		a.NoError(err)
	})

	t.Run("UpdateMetric", func(*testing.T) {
		conn.ExpectExec(`INSERT.+metric`).WithArgs(AnyArgs(8)...).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		err = readerWriter.UpdateMetric("test", metrics.Metric{})
		a.NoError(err)
	})

	t.Run("FailUpdateMetric", func(*testing.T) {
		conn.ExpectExec(`INSERT.+metric`).WithArgs(AnyArgs(8)...).WillReturnResult(pgxmock.NewResult("UPDATE", 0))
		err = readerWriter.UpdateMetric("test", metrics.Metric{})
		a.ErrorIs(err, metrics.ErrMetricNotFound)
	})

	t.Run("DeletePreset", func(*testing.T) {
		conn.ExpectExec(`DELETE.+preset`).WithArgs("test").WillReturnResult(pgxmock.NewResult("DELETE", 1))
		err = readerWriter.DeletePreset("test")
		a.NoError(err)
	})

	t.Run("UpdatePreset", func(*testing.T) {
		conn.ExpectExec(`INSERT.+preset`).WithArgs(AnyArgs(3)...).WillReturnResult(pgxmock.NewResult("INSERT", 1))
		err = readerWriter.UpdatePreset("test", metrics.Preset{})
		a.NoError(err)
	})

	t.Run("FailUpdatePreset", func(*testing.T) {
		conn.ExpectExec(`INSERT.+preset`).WithArgs(AnyArgs(3)...).WillReturnResult(pgxmock.NewResult("INSERT", 0))
		err = readerWriter.UpdatePreset("test", metrics.Preset{})
		a.ErrorIs(err, metrics.ErrPresetNotFound)
	})

	// check all expectations were met
	a.NoError(conn.ExpectationsWereMet())
}

// Additional tests for GetMetrics, WriteMetrics, DeleteMetric, UpdateMetric, DeletePreset, and UpdatePreset follow a similar pattern.
