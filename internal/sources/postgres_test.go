package sources_test

import (
	"errors"
	"testing"

	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"

	"github.com/cybertec-postgresql/pgwatch/v6/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v6/internal/sources"
)

func TestNewPostgresSourcesReaderWriter(t *testing.T) {
	a := assert.New(t)
	t.Run("ConnectionError", func(*testing.T) {
		pgrw, err := sources.NewPostgresSourcesReaderWriter(ctx, "postgres://user:pass@foohost:5432/db1")
		a.Error(err) // connection error
		a.NotNil(t, pgrw)
	})
	t.Run("InvalidConnStr", func(*testing.T) {
		pgrw, err := sources.NewPostgresSourcesReaderWriter(ctx, "invalid_connstr")
		a.Error(err)
		a.Nil(pgrw)
	})
}

func TestNewPostgresSourcesReaderWriterConn(t *testing.T) {
	a := assert.New(t)
	conn, err := pgxmock.NewPool()
	a.NoError(err)
	conn.ExpectQuery(`SELECT EXISTS`).WithArgs("pgwatch").
		WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	conn.ExpectPing()

	pgrw, err := sources.NewPostgresSourcesReaderWriterConn(ctx, conn)
	a.NoError(err)
	a.NotNil(t, pgrw)
	a.NoError(conn.ExpectationsWereMet())
}

func TestGetMonitoredDatabases(t *testing.T) {
	a := assert.New(t)
	conn, err := pgxmock.NewPool()
	a.NoError(err)
	conn.ExpectQuery(`SELECT EXISTS`).WithArgs("pgwatch").
		WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	conn.ExpectPing()
	conn.ExpectQuery(`select \/\* pgwatch_generated \*\/`).WillReturnRows(pgxmock.NewRows([]string{
		"name", "group", "dbtype", "connstr", "config", "config_standby", "preset_config",
		"preset_config_standby", "include_pattern", "exclude_pattern",
		"custom_tags", "only_if_master", "is_enabled",
	}).AddRow(
		"db1", "group1", sources.Kind("postgres"), "postgres://user:pass@localhost:5432/db1",
		metrics.MetricIntervals{"metric": 60}, metrics.MetricIntervals{"standby_metric": 60}, "exhaustive", "exhaustive",
		".*", `\_.+`, map[string]string{"tag": "value"}, true, true,
	))
	pgrw, err := sources.NewPostgresSourcesReaderWriterConn(ctx, conn)
	a.NoError(err)

	dbs, err := pgrw.GetSources()
	a.NoError(err)
	a.Len(dbs, 1)
	a.NoError(conn.ExpectationsWereMet())

	// check failed query
	conn.ExpectQuery(`select \/\* pgwatch_generated \*\/`).WillReturnError(errors.New("failed query"))
	dbs, err = pgrw.GetSources()
	a.Error(err)
	a.Nil(dbs)
	a.NoError(conn.ExpectationsWereMet())
}

func TestDeleteDatabase(t *testing.T) {
	a := assert.New(t)
	conn, err := pgxmock.NewPool()
	a.NoError(err)
	conn.ExpectQuery(`SELECT EXISTS`).WithArgs("pgwatch").
		WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	conn.ExpectPing()
	conn.ExpectExec(`delete from pgwatch\.source where name = \$1`).WithArgs("db1").WillReturnResult(pgxmock.NewResult("DELETE", 1))
	pgrw, err := sources.NewPostgresSourcesReaderWriterConn(ctx, conn)
	a.NoError(err)

	err = pgrw.DeleteSource("db1")
	a.NoError(err)
	a.NoError(conn.ExpectationsWereMet())
}

func TestUpdateDatabase(t *testing.T) {
	a := assert.New(t)
	conn, err := pgxmock.NewPool()
	a.NoError(err)

	md := sources.Source{
		Name:           "db1",
		Group:          "group1",
		Kind:           sources.Kind("postgres"),
		ConnStr:        "postgres://user:pass@localhost:5432/db1",
		Metrics:        metrics.MetricIntervals{"metric": 60},
		MetricsStandby: metrics.MetricIntervals{"standby_metric": 60},
		IncludePattern: ".*",
		ExcludePattern: `\_.+`,
		CustomTags:     map[string]string{"tag": "value"},
	}
	conn.ExpectQuery(`SELECT EXISTS`).WithArgs("pgwatch").
		WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	conn.ExpectPing()
	conn.ExpectExec(`insert into pgwatch\.source`).WithArgs(
		md.Name, md.Group, md.Kind,
		md.ConnStr, `{"metric":60}`, `{"standby_metric":60}`,
		md.PresetMetrics, md.PresetMetricsStandby,
		md.IncludePattern, md.ExcludePattern, `{"tag":"value"}`,
		md.OnlyIfMaster, md.IsEnabled,
	).WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	pgrw, err := sources.NewPostgresSourcesReaderWriterConn(ctx, conn)
	a.NoError(err)
	err = pgrw.UpdateSource(md)
	a.NoError(err)
	a.NoError(conn.ExpectationsWereMet())
}

func TestWriteMonitoredDatabases(t *testing.T) {
	var (
		pgrw sources.ReaderWriter
		err  error
	)
	a := assert.New(t)
	conn, err := pgxmock.NewPool()
	a.NoError(err)
	md := sources.Source{
		Name:           "db1",
		Group:          "group1",
		Kind:           sources.Kind("postgres"),
		ConnStr:        "postgres://user:pass@localhost:5432/db1",
		Metrics:        metrics.MetricIntervals{"metric": 60},
		MetricsStandby: metrics.MetricIntervals{"standby_metric": 60},
		IncludePattern: ".*",
		ExcludePattern: `\_.+`,
		CustomTags:     map[string]string{"tag": "value"},
	}
	mds := sources.Sources{md}

	t.Run("happy path", func(*testing.T) {
		conn.ExpectQuery(`SELECT EXISTS`).WithArgs("pgwatch").
			WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
		conn.ExpectPing()
		conn.ExpectBegin()
		conn.ExpectExec(`truncate pgwatch\.source`).WillReturnResult(pgxmock.NewResult("TRUNCATE", 1))
		conn.ExpectExec(`insert into pgwatch\.source`).WithArgs(
			md.Name, md.Group, md.Kind,
			md.ConnStr, `{"metric":60}`, `{"standby_metric":60}`, md.PresetMetrics, md.PresetMetricsStandby,
			md.IncludePattern, md.ExcludePattern, `{"tag":"value"}`,
			md.OnlyIfMaster, md.IsEnabled,
		).WillReturnResult(pgxmock.NewResult("INSERT", 1))
		conn.ExpectCommit()
		conn.ExpectRollback() // deferred rollback

		pgrw, err = sources.NewPostgresSourcesReaderWriterConn(ctx, conn)
		a.NoError(err)
		err = pgrw.WriteSources(mds)
		a.NoError(err)
		a.NoError(conn.ExpectationsWereMet())
	})

	t.Run("failed transaction begin", func(*testing.T) {
		conn.ExpectBegin().WillReturnError(errors.New("failed transaction begin"))

		err = pgrw.WriteSources(mds)
		a.Error(err)
		a.NoError(conn.ExpectationsWereMet())
	})

	t.Run("failed truncate", func(*testing.T) {
		conn.ExpectBegin()
		conn.ExpectExec(`truncate pgwatch\.source`).WillReturnError(errors.New("failed truncate"))

		err = pgrw.WriteSources(mds)
		a.Error(err)
		a.NoError(conn.ExpectationsWereMet())
	})

	t.Run("failed insert", func(*testing.T) {
		conn.ExpectBegin()
		conn.ExpectExec(`truncate pgwatch\.source`).WillReturnResult(pgxmock.NewResult("TRUNCATE", 1))
		conn.ExpectExec(`insert into pgwatch\.source`).WithArgs(
			md.Name, md.Group, md.Kind,
			md.ConnStr, `{"metric":60}`, `{"standby_metric":60}`, md.PresetMetrics, md.PresetMetricsStandby,
			md.IncludePattern, md.ExcludePattern, `{"tag":"value"}`,
			md.OnlyIfMaster, md.IsEnabled,
		).WillReturnError(errors.New("failed insert"))
		conn.ExpectRollback()

		err = pgrw.WriteSources(mds)
		a.Error(err)
		a.NoError(conn.ExpectationsWereMet())
	})
}

// anyArgs returns a slice of pgxmock.AnyArg() of length n for matching INSERT/UPDATE calls.
func anyArgs(n int) []any {
	args := make([]any, n)
	for i := range args {
		args[i] = pgxmock.AnyArg()
	}
	return args
}

func TestNewPostgresSourcesReaderWriterConn_Bootstrap(t *testing.T) {
	a := assert.New(t)

	t.Run("FullBootstrap", func(*testing.T) {
		df := metrics.GetDefaultMetrics()
		metricsCount := len(df.MetricDefs)
		presetsCount := len(df.PresetDefs)

		conn, err := pgxmock.NewPool()
		a.NoError(err)
		conn.ExpectQuery(`SELECT EXISTS`).WithArgs("pgwatch").
			WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))
		conn.ExpectBegin()
		conn.ExpectExec("CREATE SCHEMA IF NOT EXISTS pgwatch").
			WillReturnResult(pgxmock.NewResult("CREATE", 1))
		conn.ExpectBegin()
		conn.ExpectExec(`INSERT.+metric`).WithArgs(anyArgs(8)...).
			WillReturnResult(pgxmock.NewResult("INSERT", 1)).Times(uint(metricsCount))
		conn.ExpectExec(`INSERT.+preset`).WithArgs(anyArgs(3)...).
			WillReturnResult(pgxmock.NewResult("INSERT", 1)).Times(uint(presetsCount))
		conn.ExpectCommit()
		conn.ExpectCommit()
		conn.ExpectPing()

		rw, err := sources.NewPostgresSourcesReaderWriterConn(ctx, conn)
		a.NoError(err)
		a.NotNil(rw)
		a.NoError(conn.ExpectationsWereMet())
	})

	t.Run("SchemaQueryFail", func(*testing.T) {
		conn, err := pgxmock.NewPool()
		a.NoError(err)
		conn.ExpectQuery(`SELECT EXISTS`).WithArgs("pgwatch").
			WillReturnError(assert.AnError)
		rw, err := sources.NewPostgresSourcesReaderWriterConn(ctx, conn)
		a.Error(err)
		a.Nil(rw)
		a.NoError(conn.ExpectationsWereMet())
	})
}

func TestSourcesNeedsMigration(t *testing.T) {
	a := assert.New(t)
	conn, err := pgxmock.NewPool()
	a.NoError(err)
	conn.ExpectQuery(`SELECT EXISTS`).WithArgs("pgwatch").
		WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	conn.ExpectPing()
	conn.ExpectQuery(`SELECT to_regclass`).
		WithArgs("pgwatch.migration").
		WillReturnRows(pgxmock.NewRows([]string{"to_regclass"}).AddRow(true))
	conn.ExpectQuery(`SELECT count`).WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(0))

	rw, err := sources.NewPostgresSourcesReaderWriterConn(ctx, conn)
	a.NoError(err)

	needs, err := rw.(interface {
		NeedsMigration() (bool, error)
	}).NeedsMigration()
	a.NoError(err)
	a.True(needs)
	a.NoError(conn.ExpectationsWereMet())
}
