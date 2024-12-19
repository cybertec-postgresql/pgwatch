package log

import (
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"gopkg.in/natefinch/lumberjack.v2"
)

func TestGetLogFileWriter(t *testing.T) {
	assert.IsType(t, getLogFileWriter(CmdOpts{LogFileRotate: true}), &lumberjack.Logger{})
	assert.IsType(t, getLogFileWriter(CmdOpts{LogFileRotate: false}), "string")
}

func TestGetLogFileFormatter(t *testing.T) {
	assert.IsType(t, getLogFileFormatter(CmdOpts{LogFileFormat: "json"}), &logrus.JSONFormatter{})
	assert.IsType(t, getLogFileFormatter(CmdOpts{LogFileFormat: "blah"}), &logrus.JSONFormatter{})
	assert.IsType(t, getLogFileFormatter(CmdOpts{LogFileFormat: "text"}), &Formatter{})
}
