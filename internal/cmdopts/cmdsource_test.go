package cmdopts

import (
	"context"
	"os"
	"testing"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/db"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/sources"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSourcePingCommand_Execute(t *testing.T) {

	f, err := os.CreateTemp(t.TempDir(), "sample.config.yaml")
	require.NoError(t, err)
	defer f.Close()

	_, err = f.WriteString(`
- name: test1
  kind: postgres
  is_enabled: true
  conn_str: postgresql://foo@bar/baz
- name: test2
  kind: postgres-continuous-discovery
  is_enabled: true
  conn_str: postgresql://foo@bar/baz
- name: test3
  kind: patroni-namespace-discovery
  is_enabled: true
  conn_str: postgresql://foo@bar/baz`)

	require.NoError(t, err)

	sources.NewConn = func(_ context.Context, _ string, _ ...db.ConnConfigCallback) (db.PgxPoolIface, error) {
		return nil, assert.AnError
	}
	sources.NewConnWithConfig = func(_ context.Context, _ *pgxpool.Config, _ ...db.ConnConfigCallback) (db.PgxPoolIface, error) {
		return nil, assert.AnError
	}

	os.Args = []string{0: "config_test", "--sources=" + f.Name(), "source", "ping"}
	_, err = New(nil)
	assert.NoError(t, err)

	os.Args = []string{0: "config_test", "--sources=" + f.Name(), "source", "ping", "test1"}
	_, err = New(nil)
	assert.NoError(t, err)
}

func TestSourceResolveCommand_Execute(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "sample.config.yaml")
	require.NoError(t, err)
	defer f.Close()

	_, err = f.WriteString(`
- name: test0
  kind: postgres
  is_enabled: true
  conn_str: postgresql://foo@bar/baz
- name: test1
  kind: postgres-continuous-discovery
  is_enabled: true
  conn_str: postgresql://foo@bar/baz`)

	require.NoError(t, err)

	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	sources.NewConn = func(_ context.Context, _ string, _ ...db.ConnConfigCallback) (db.PgxPoolIface, error) {
		return mock, nil
	}

	t.Run("ResolveSuccess", func(t *testing.T) {
		r := func(args []string) {
			mock.ExpectQuery("select.+datname").
				WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).
				WillReturnRows(
					pgxmock.NewRows([]string{"datname"}).
						AddRow("foo"))
			os.Args = args
			_, err = New(nil)
			assert.NoError(t, err)
			assert.NoError(t, mock.ExpectationsWereMet())
		}
		r([]string{0: "config_test", "--sources=" + f.Name(), "source", "resolve"})
		r([]string{0: "config_test", "--sources=" + f.Name(), "source", "resolve", "test1"})
	})

	t.Run("ResolveError", func(t *testing.T) {
		mock.ExpectQuery("select.+datname").
			WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).
			WillReturnError(assert.AnError)
		os.Args = []string{0: "config_test", "--sources=" + f.Name(), "source", "resolve"}
		_, err = New(nil)
		assert.ErrorIs(t, err, assert.AnError)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

}
