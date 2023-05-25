package log_test

import (
	"context"
	"os"
	"testing"

	"github.com/cybertec-postgresql/pgwatch3/config"
	"github.com/cybertec-postgresql/pgwatch3/log"
	"github.com/jackc/pgx/v5/tracelog"
	"github.com/stretchr/testify/assert"
)

func TestInit(t *testing.T) {
	assert.NotNil(t, log.Init(config.LoggingOpts{LogLevel: "debug"}))
	l := log.Init(config.LoggingOpts{LogLevel: "foobar"})
	pgxl := log.NewPgxLogger(l)
	assert.NotNil(t, pgxl)
	ctx := log.WithLogger(context.Background(), l)
	assert.True(t, log.GetLogger(ctx) == l)
	assert.True(t, log.GetLogger(context.Background()) == log.FallbackLogger)
}

func TestFileLogger(t *testing.T) {
	l := log.Init(config.LoggingOpts{LogLevel: "debug", LogFile: "test.log", LogFileFormat: "text"})
	l.Info("test")
	assert.FileExists(t, "test.log", "Log file should be created")
	_ = os.Remove("test.log")
}

func TestPgxLog(_ *testing.T) {
	pgxl := log.NewPgxLogger(log.Init(config.LoggingOpts{LogLevel: "trace"}))
	var level tracelog.LogLevel
	for level = tracelog.LogLevelNone; level <= tracelog.LogLevelTrace; level++ {
		pgxl.Log(context.Background(), level, "foo", map[string]interface{}{"func": "TestPgxLog"})
	}
}
