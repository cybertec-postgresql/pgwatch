package metrics_test

import (
	"context"
	"testing"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/log"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/metrics"
	"github.com/jackc/pgx/v5/pgconn"
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

	t.Run("SchemaExists", func(*testing.T) {
		conn.ExpectQuery(`SELECT EXISTS`).WithArgs("pgwatch").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
		conn.ExpectPing()
		rw, err := metrics.NewPostgresMetricReaderWriterConn(ctx, conn)
		a.NoError(err)
		a.NotNil(rw)
		a.NoError(conn.ExpectationsWereMet())
	})
}

func newTestReaderWriter(t *testing.T) (pgxmock.PgxPoolIface, metrics.ReaderWriter) {
	t.Helper()
	conn, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	conn.ExpectQuery(`SELECT EXISTS`).WithArgs("pgwatch").
		WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	conn.ExpectPing()
	rw, err := metrics.NewPostgresMetricReaderWriterConn(ctx, conn)
	if err != nil {
		t.Fatalf("NewPostgresMetricReaderWriterConn: %v", err)
	}
	return conn, rw
}

func TestMetricsToPostgres(t *testing.T) {
	conn, rw := newTestReaderWriter(t)

	metricsRows := func() *pgxmock.Rows {
		return pgxmock.NewRows([]string{"name", "sqls", "init_sql", "description", "node_status", "gauges", "is_instance_level", "storage_name"}).
			AddRow("test", metrics.SQLs{11: "select"}, "init", "desc", "primary", []string{"*"}, true, "storage")
	}
	presetRows := func() *pgxmock.Rows {
		return pgxmock.NewRows([]string{"name", "description", "metrics"}).
			AddRow("test", "desc", metrics.MetricIntervals{"metric": 30})
	}

	t.Run("GetMetrics", func(t *testing.T) {
		conn.ExpectQuery(`SELECT.+FROM.+metric`).WillReturnRows(metricsRows())
		conn.ExpectQuery(`SELECT.+FROM.+preset`).WillReturnRows(presetRows())
		m, err := rw.GetMetrics()
		assert.NoError(t, err)
		assert.Len(t, m.MetricDefs, 1)
	})

	t.Run("GetMetricsFail", func(t *testing.T) {
		conn.ExpectQuery(`SELECT.+FROM.+metric`).WillReturnError(assert.AnError)
		_, err := rw.GetMetrics()
		assert.Error(t, err)
	})

	t.Run("GetPresetsFail", func(t *testing.T) {
		conn.ExpectQuery(`SELECT.+FROM.+metric`).WillReturnRows(metricsRows())
		conn.ExpectQuery(`SELECT.+FROM.+preset`).WillReturnError(assert.AnError)
		_, err := rw.GetMetrics()
		assert.Error(t, err)
	})

	t.Run("GetMetricsScanFail", func(t *testing.T) {
		conn.ExpectQuery(`SELECT.+FROM.+metric`).WillReturnRows(metricsRows().RowError(0, assert.AnError))
		_, err := rw.GetMetrics()
		assert.Error(t, err)
	})

	t.Run("GetPresetsScanFail", func(t *testing.T) {
		conn.ExpectQuery(`SELECT.+FROM.+metric`).WillReturnRows(metricsRows())
		conn.ExpectQuery(`SELECT.+FROM.+preset`).WillReturnRows(presetRows().RowError(0, assert.AnError))
		_, err := rw.GetMetrics()
		assert.Error(t, err)
	})

	t.Run("WriteMetrics", func(t *testing.T) {
		conn.ExpectBegin().WillReturnError(assert.AnError)
		assert.Error(t, rw.WriteMetrics(&metrics.Metrics{}))
	})

	t.Run("DeleteMetric", func(t *testing.T) {
		conn.ExpectExec(`DELETE.+metric`).WithArgs("test").WillReturnResult(pgxmock.NewResult("DELETE", 1))
		assert.NoError(t, rw.DeleteMetric("test"))
	})

	t.Run("UpdateMetric", func(t *testing.T) {
		conn.ExpectExec(`INSERT.+metric`).WithArgs(AnyArgs(8)...).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		assert.NoError(t, rw.UpdateMetric("test", metrics.Metric{}))
	})

	t.Run("FailUpdateMetric", func(t *testing.T) {
		conn.ExpectExec(`INSERT.+metric`).WithArgs(AnyArgs(8)...).WillReturnResult(pgxmock.NewResult("UPDATE", 0))
		assert.ErrorIs(t, rw.UpdateMetric("test", metrics.Metric{}), metrics.ErrMetricNotFound)
	})

	t.Run("DeletePreset", func(t *testing.T) {
		conn.ExpectExec(`DELETE.+preset`).WithArgs("test").WillReturnResult(pgxmock.NewResult("DELETE", 1))
		assert.NoError(t, rw.DeletePreset("test"))
	})

	t.Run("UpdatePreset", func(t *testing.T) {
		conn.ExpectExec(`INSERT.+preset`).WithArgs(AnyArgs(3)...).WillReturnResult(pgxmock.NewResult("INSERT", 1))
		assert.NoError(t, rw.UpdatePreset("test", metrics.Preset{}))
	})

	t.Run("FailUpdatePreset", func(t *testing.T) {
		conn.ExpectExec(`INSERT.+preset`).WithArgs(AnyArgs(3)...).WillReturnResult(pgxmock.NewResult("INSERT", 0))
		assert.ErrorIs(t, rw.UpdatePreset("test", metrics.Preset{}), metrics.ErrPresetNotFound)
	})

	t.Run("DeleteMetricError", func(t *testing.T) {
		conn.ExpectExec(`DELETE.+metric`).WithArgs("test").WillReturnError(assert.AnError)
		assert.Error(t, rw.DeleteMetric("test"))
	})

	t.Run("UpdateMetricExecError", func(t *testing.T) {
		conn.ExpectExec(`INSERT.+metric`).WithArgs(AnyArgs(8)...).WillReturnError(assert.AnError)
		assert.Error(t, rw.UpdateMetric("test", metrics.Metric{}))
	})

	t.Run("DeletePresetError", func(t *testing.T) {
		conn.ExpectExec(`DELETE.+preset`).WithArgs("test").WillReturnError(assert.AnError)
		assert.Error(t, rw.DeletePreset("test"))
	})

	t.Run("UpdatePresetExecError", func(t *testing.T) {
		conn.ExpectExec(`INSERT.+preset`).WithArgs(AnyArgs(3)...).WillReturnError(assert.AnError)
		assert.Error(t, rw.UpdatePreset("test", metrics.Preset{}))
	})

	assert.NoError(t, conn.ExpectationsWereMet())
}

func TestCreateMetric(t *testing.T) {
	conn, rw := newTestReaderWriter(t)

	t.Run("Success", func(t *testing.T) {
		conn.ExpectExec(`INSERT.+metric`).WithArgs(AnyArgs(8)...).WillReturnResult(pgxmock.NewResult("INSERT", 1))
		assert.NoError(t, rw.CreateMetric("new_metric", metrics.Metric{}))
	})

	t.Run("Duplicate", func(t *testing.T) {
		conn.ExpectExec(`INSERT.+metric`).WithArgs(AnyArgs(8)...).WillReturnError(&pgconn.PgError{Code: "23505"})
		assert.ErrorIs(t, rw.CreateMetric("existing_metric", metrics.Metric{}), metrics.ErrMetricExists)
	})

	t.Run("ExecError", func(t *testing.T) {
		conn.ExpectExec(`INSERT.+metric`).WithArgs(AnyArgs(8)...).WillReturnError(assert.AnError)
		err := rw.CreateMetric("fail_metric", metrics.Metric{})
		assert.Error(t, err)
		assert.NotErrorIs(t, err, metrics.ErrMetricExists)
	})

	assert.NoError(t, conn.ExpectationsWereMet())
}

func TestCreatePreset(t *testing.T) {
	conn, rw := newTestReaderWriter(t)

	t.Run("Success", func(t *testing.T) {
		conn.ExpectExec(`INSERT.+preset`).WithArgs(AnyArgs(3)...).WillReturnResult(pgxmock.NewResult("INSERT", 1))
		assert.NoError(t, rw.CreatePreset("new_preset", metrics.Preset{}))
	})

	t.Run("Duplicate", func(t *testing.T) {
		conn.ExpectExec(`INSERT.+preset`).WithArgs(AnyArgs(3)...).WillReturnError(&pgconn.PgError{Code: "23505"})
		assert.ErrorIs(t, rw.CreatePreset("existing_preset", metrics.Preset{}), metrics.ErrPresetExists)
	})

	t.Run("ExecError", func(t *testing.T) {
		conn.ExpectExec(`INSERT.+preset`).WithArgs(AnyArgs(3)...).WillReturnError(assert.AnError)
		err := rw.CreatePreset("fail_preset", metrics.Preset{})
		assert.Error(t, err)
		assert.NotErrorIs(t, err, metrics.ErrPresetExists)
	})

	assert.NoError(t, conn.ExpectationsWereMet())
}
