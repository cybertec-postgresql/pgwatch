package cmdopts

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMetricPrintInit_Execute(t *testing.T) {

	var err error

	w := &strings.Builder{}
	os.Args = []string{0: "config_test", "metric", "print-init", "test1"}
	_, err = New(w)
	assert.Empty(t, w.String())
	assert.NoError(t, err, "should not error when no metrics found")

	w.Reset()
	os.Args = []string{0: "config_test", "metric", "print-init", "cpu_load"}
	_, err = New(w)
	assert.Contains(t, w.String(), "-- cpu_load")
	assert.NoError(t, err)

	w.Reset()
	os.Args = []string{0: "config_test", "metric", "print-init", "standard"}
	_, err = New(w)
	assert.Contains(t, w.String(), "-- cpu_load")
	assert.NoError(t, err)

	os.Args = []string{0: "config_test", "--metrics=foo", "metric", "print-init", "standard"}
	_, err = New(w)
	assert.Error(t, err, "should error when no metric definitions found")

	os.Args = []string{0: "config_test", "--metrics=postgresql://foo@bar/fail", "metric", "print-init", "standard"}
	_, err = New(w)
	assert.Error(t, err, "should error when no config database found")
}

func TestMetricPrintSQL_Execute(t *testing.T) {
	var err error

	w := &strings.Builder{}
	os.Args = []string{0: "config_test", "metric", "print-sql", "test1"}
	_, err = New(w)
	assert.Empty(t, w.String())
	assert.NoError(t, err, "should not error when no metrics found")

	w.Reset()
	os.Args = []string{0: "config_test", "metric", "print-sql", "cpu_load"}
	_, err = New(w)
	assert.Contains(t, w.String(), "get_load_average()")
	assert.NoError(t, err)

	w.Reset()
	os.Args = []string{0: "config_test", "metric", "print-sql", "cpu_load", "--version=10"}
	_, err = New(w)
	assert.Empty(t, w.String(), "should not print anything for deprecated version")
	assert.NoError(t, err)

	os.Args = []string{0: "config_test", "--metrics=foo", "metric", "print-sql", "foo"}
	_, err = New(w)
	assert.Error(t, err, "should error when no metric definitions found")

	os.Args = []string{0: "config_test", "--metrics=postgresql://foo@bar/fail", "metric", "print-sql", "foo"}
	_, err = New(w)
	assert.Error(t, err, "should error when no config database found")
}
