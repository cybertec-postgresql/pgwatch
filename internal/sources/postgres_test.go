package sources_test

import (
	"errors"
	"testing"

	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/sources"
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
	conn.ExpectPing()
	conn.ExpectQuery(`select \/\* pgwatch_generated \*\/`).WillReturnRows(pgxmock.NewRows([]string{
		"name", "group", "dbtype", "connstr", "config", "config_standby", "preset_config",
		"preset_config_standby", "is_superuser", "include_pattern", "exclude_pattern",
		"custom_tags", "host_config", "only_if_master", "is_enabled",
	}).AddRow(
		"db1", "group1", sources.Kind("postgres"), "postgres://user:pass@localhost:5432/db1",
		map[string]float64{"metric": 60}, map[string]float64{"standby_metric": 60}, "exhaustive", "exhaustive",
		true, ".*", `\_.+`, map[string]string{"tag": "value"}, nil, true, true,
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

func TestSyncFromReader(t *testing.T) {
	a := assert.New(t)
	conn, err := pgxmock.NewPool()
	a.NoError(err)
	conn.ExpectPing()
	conn.ExpectQuery(`select \/\* pgwatch_generated \*\/`).WillReturnRows(pgxmock.NewRows([]string{
		"name", "group", "dbtype", "connstr", "config", "config_standby", "preset_config",
		"preset_config_standby", "is_superuser", "include_pattern", "exclude_pattern",
		"custom_tags", "host_config", "only_if_master", "is_enabled",
	}).AddRow(
		"db1", "group1", sources.Kind("postgres"), "postgres://user:pass@localhost:5432/db1",
		map[string]float64{"metric": 60}, map[string]float64{"standby_metric": 60}, "exhaustive", "exhaustive",
		true, ".*", `\_.+`, map[string]string{"tag": "value"}, nil, true, true,
	))
	pgrw, err := sources.NewPostgresSourcesReaderWriterConn(ctx, conn)
	a.NoError(err)

	md := &sources.MonitoredDatabase{}
	md.Name = "db1"
	dbs := sources.MonitoredDatabases{md}
	dbs, err = dbs.SyncFromReader(pgrw)
	a.NoError(err)
	a.Len(dbs, 1)
	a.NoError(conn.ExpectationsWereMet())
}

func TestDeleteDatabase(t *testing.T) {
	a := assert.New(t)
	conn, err := pgxmock.NewPool()
	a.NoError(err)
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
		Metrics:        map[string]float64{"metric": 60},
		MetricsStandby: map[string]float64{"standby_metric": 60},
		IsSuperuser:    true,
		IncludePattern: ".*",
		ExcludePattern: `\_.+`,
		CustomTags:     map[string]string{"tag": "value"},
	}
	conn.ExpectPing()
	conn.ExpectExec(`insert into pgwatch\.source`).WithArgs(
		md.Name, md.Group, md.Kind,
		md.ConnStr, `{"metric":60}`, `{"standby_metric":60}`,
		md.PresetMetrics, md.PresetMetricsStandby,
		md.IsSuperuser, md.IncludePattern, md.ExcludePattern, `{"tag":"value"}`,
		nil, md.OnlyIfMaster,
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
		Metrics:        map[string]float64{"metric": 60},
		MetricsStandby: map[string]float64{"standby_metric": 60},
		IsSuperuser:    true,
		IncludePattern: ".*",
		ExcludePattern: `\_.+`,
		CustomTags:     map[string]string{"tag": "value"},
	}
	mds := sources.Sources{md}

	t.Run("happy path", func(*testing.T) {
		conn.ExpectPing()
		conn.ExpectBegin()
		conn.ExpectExec(`truncate pgwatch\.source`).WillReturnResult(pgxmock.NewResult("TRUNCATE", 1))
		conn.ExpectExec(`insert into pgwatch\.source`).WithArgs(
			md.Name, md.Group, md.Kind,
			md.ConnStr, `{"metric":60}`, `{"standby_metric":60}`, md.PresetMetrics, md.PresetMetricsStandby,
			md.IsSuperuser, md.IncludePattern, md.ExcludePattern, `{"tag":"value"}`,
			nil, md.OnlyIfMaster,
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
			md.IsSuperuser, md.IncludePattern, md.ExcludePattern, `{"tag":"value"}`,
			nil, md.OnlyIfMaster,
		).WillReturnError(errors.New("failed insert"))
		conn.ExpectRollback()

		err = pgrw.WriteSources(mds)
		a.Error(err)
		a.NoError(conn.ExpectationsWereMet())
	})
}
