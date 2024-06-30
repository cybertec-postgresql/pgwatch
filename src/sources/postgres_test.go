package sources_test

import (
	"errors"
	"testing"

	"github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"

	"github.com/cybertec-postgresql/pgwatch3/sources"
)

func TestNewPostgresSourcesReaderWriter(t *testing.T) {
	a := assert.New(t)
	conn, err := pgxmock.NewPool()
	a.NoError(err)
	conn.ExpectPing()

	pgrw, err := sources.NewPostgresSourcesReaderWriter(ctx, conn)
	a.NoError(err)
	a.NotNil(t, pgrw)
	a.NoError(conn.ExpectationsWereMet())
}

func TestGetMonitoredDatabases(t *testing.T) {
	a := assert.New(t)
	conn, err := pgxmock.NewPool()
	a.NoError(err)
	conn.ExpectPing()
	conn.ExpectQuery(`select \/\* pgwatch3_generated \*\/`).WillReturnRows(pgxmock.NewRows([]string{
		"name", "group", "dbtype", "connstr", "config", "config_standby", "preset_config",
		"preset_config_standby", "is_superuser", "include_pattern", "exclude_pattern",
		"custom_tags", "host_config", "only_if_master", "is_enabled",
	}).AddRow(
		"db1", "group1", sources.Kind("postgres"), "postgres://user:pass@localhost:5432/db1",
		map[string]float64{"metric": 60}, map[string]float64{"standby_metric": 60}, "exhaustive", "exhaustive",
		true, ".*", `\_.+`, map[string]string{"tag": "value"}, nil, true, true,
	))
	pgrw, err := sources.NewPostgresSourcesReaderWriter(ctx, conn)
	a.NoError(err)

	dbs, err := pgrw.GetMonitoredDatabases()
	a.NoError(err)
	a.Len(dbs, 1)
	a.NoError(conn.ExpectationsWereMet())

	// check failed query
	conn.ExpectQuery(`select \/\* pgwatch3_generated \*\/`).WillReturnError(errors.New("failed query"))
	dbs, err = pgrw.GetMonitoredDatabases()
	a.Error(err)
	a.Nil(dbs)
	a.NoError(conn.ExpectationsWereMet())
}
func TestDeleteDatabase(t *testing.T) {
	a := assert.New(t)
	conn, err := pgxmock.NewPool()
	a.NoError(err)
	conn.ExpectPing()
	conn.ExpectExec(`delete from pgwatch3\.source where name = \$1`).WithArgs("db1").WillReturnResult(pgxmock.NewResult("DELETE", 1))
	pgrw, err := sources.NewPostgresSourcesReaderWriter(ctx, conn)
	a.NoError(err)

	err = pgrw.DeleteDatabase("db1")
	a.NoError(err)
	a.NoError(conn.ExpectationsWereMet())
}

func TestUpdateDatabase(t *testing.T) {
	a := assert.New(t)
	conn, err := pgxmock.NewPool()
	a.NoError(err)

	md := &sources.MonitoredDatabase{}
	conn.ExpectPing()
	conn.ExpectExec(`insert into pgwatch3\.source`).WithArgs(
		md.DBUniqueName, md.Group, md.Kind,
		md.ConnStr, md.Metrics, md.MetricsStandby, md.PresetMetrics, md.PresetMetricsStandby,
		md.IsSuperuser, md.IncludePattern, md.ExcludePattern, md.CustomTags,
		md.HostConfig, md.OnlyIfMaster,
	).WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	pgrw, err := sources.NewPostgresSourcesReaderWriter(ctx, conn)
	a.NoError(err)
	err = pgrw.UpdateDatabase(md)
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
	md := &sources.MonitoredDatabase{}
	mds := sources.MonitoredDatabases{md}

	t.Run("happy path", func(*testing.T) {
		conn.ExpectPing()
		conn.ExpectBegin()
		conn.ExpectExec(`truncate pgwatch3\.source`).WillReturnResult(pgxmock.NewResult("TRUNCATE", 1))
		conn.ExpectExec(`insert into pgwatch3\.source`).WithArgs(
			md.DBUniqueName, md.Group, md.Kind,
			md.ConnStr, md.Metrics, md.MetricsStandby, md.PresetMetrics, md.PresetMetricsStandby,
			md.IsSuperuser, md.IncludePattern, md.ExcludePattern, md.CustomTags,
			md.HostConfig, md.OnlyIfMaster,
		).WillReturnResult(pgxmock.NewResult("INSERT", 1))
		conn.ExpectCommit()
		conn.ExpectRollback() // deferred rollback

		pgrw, err = sources.NewPostgresSourcesReaderWriter(ctx, conn)
		a.NoError(err)
		err = pgrw.WriteMonitoredDatabases(mds)
		a.NoError(err)
		a.NoError(conn.ExpectationsWereMet())
	})

	t.Run("failed transaction begin", func(*testing.T) {
		conn.ExpectBegin().WillReturnError(errors.New("failed transaction begin"))

		err = pgrw.WriteMonitoredDatabases(mds)
		a.Error(err)
		a.NoError(conn.ExpectationsWereMet())
	})

	t.Run("failed truncate", func(*testing.T) {
		conn.ExpectBegin()
		conn.ExpectExec(`truncate pgwatch3\.source`).WillReturnError(errors.New("failed truncate"))

		err = pgrw.WriteMonitoredDatabases(mds)
		a.Error(err)
		a.NoError(conn.ExpectationsWereMet())
	})

	t.Run("failed insert", func(*testing.T) {
		conn.ExpectBegin()
		conn.ExpectExec(`truncate pgwatch3\.source`).WillReturnResult(pgxmock.NewResult("TRUNCATE", 1))
		conn.ExpectExec(`insert into pgwatch3\.source`).WithArgs(
			md.DBUniqueName, md.Group, md.Kind,
			md.ConnStr, md.Metrics, md.MetricsStandby, md.PresetMetrics, md.PresetMetricsStandby,
			md.IsSuperuser, md.IncludePattern, md.ExcludePattern, md.CustomTags,
			md.HostConfig, md.OnlyIfMaster,
		).WillReturnError(errors.New("failed insert"))
		conn.ExpectRollback()

		err = pgrw.WriteMonitoredDatabases(mds)
		a.Error(err)
		a.NoError(conn.ExpectationsWereMet())
	})
}
