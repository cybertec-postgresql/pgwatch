package testutil

import (
	"github.com/cybertec-postgresql/pgwatch/v3/internal/sources"
	"github.com/pashagolub/pgxmock/v4"
)

// Helper function to create a test SourceConn with pgxmock
func CreateTestSourceConn() (*sources.SourceConn, pgxmock.PgxPoolIface, error) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		return nil, nil, err
	}

	md := &sources.SourceConn{
		Conn:   mock,
		Source: sources.Source{Name: "testdb"},
		RuntimeInfo: sources.RuntimeInfo{
			Version:     120000,
			ChangeState: make(map[string]map[string]string),
		},
	}
	return md, mock, nil
}

// Helper function to set up transaction expectations for PostgreSQL sources
func ExpectTransaction(mock pgxmock.PgxPoolIface, queryRows *pgxmock.Rows) {
	mock.ExpectBegin()
	mock.ExpectExec("SET LOCAL lock_timeout").WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery("SELECT").WillReturnRows(queryRows)
	mock.ExpectCommit()
}
