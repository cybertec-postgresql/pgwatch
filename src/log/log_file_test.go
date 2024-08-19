package log

import (
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"gopkg.in/natefinch/lumberjack.v2"
)

func TestGetLogFileWriter(t *testing.T) {
	assert.IsType(t, getLogFileWriter(LoggingCmdOpts{LogFileRotate: true}), &lumberjack.Logger{})
	assert.IsType(t, getLogFileWriter(LoggingCmdOpts{LogFileRotate: false}), "string")
}

func TestGetLogFileFormatter(t *testing.T) {
	assert.IsType(t, getLogFileFormatter(LoggingCmdOpts{LogFileFormat: "json"}), &logrus.JSONFormatter{})
	assert.IsType(t, getLogFileFormatter(LoggingCmdOpts{LogFileFormat: "blah"}), &logrus.JSONFormatter{})
	assert.IsType(t, getLogFileFormatter(LoggingCmdOpts{LogFileFormat: "text"}), &Formatter{})
}
