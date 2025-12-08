package testutil

import (
	"github.com/cybertec-postgresql/pgwatch/v3/internal/sources"
	"github.com/pashagolub/pgxmock/v4"
)

// Helper function to create a test SourceConn with pgxmock
func CreateTestSourceConn() (*sources.SourceConn, pgxmock.PgxPoolIface, error) {
	mock, err := pgxmock.NewPool()
	md := &sources.SourceConn{
		Conn:   mock,
		Source: sources.Source{Name: "testdb"},
		RuntimeInfo: sources.RuntimeInfo{
			Version:     120000,
			ChangeState: make(map[string]map[string]string),
		},
	}
	return md, mock, err
}
