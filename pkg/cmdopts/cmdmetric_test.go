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
	assert.Error(t, err, "should error when metric not found")
	assert.Contains(t, err.Error(), "not found")

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
	assert.Error(t, err, "should error when metric not found")
	assert.Contains(t, err.Error(), "not found")

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

func TestMetricList_Execute(t *testing.T) {
	var err error

	// Test: List all metrics and presets (no argument)
	w := &strings.Builder{}
	os.Args = []string{0: "config_test", "metric", "list"}
	_, err = New(w)
	assert.NoError(t, err, "should not error when listing all metrics")
	assert.Contains(t, w.String(), "metrics:")
	assert.Contains(t, w.String(), "presets:")
	assert.Contains(t, w.String(), "cpu_load")
	assert.Contains(t, w.String(), "standard")

	// Test: List specific metric
	w.Reset()
	os.Args = []string{0: "config_test", "metric", "list", "cpu_load"}
	_, err = New(w)
	assert.NoError(t, err, "should not error when listing specific metric")
	assert.Contains(t, w.String(), "cpu_load")
	assert.Contains(t, w.String(), "metrics:")
	// Should not contain other metrics
	assert.NotContains(t, w.String(), "presets:")

	// Test: List specific preset
	w.Reset()
	os.Args = []string{0: "config_test", "metric", "list", "standard"}
	_, err = New(w)
	assert.NoError(t, err, "should not error when listing preset")
	assert.Contains(t, w.String(), "presets:")
	assert.Contains(t, w.String(), "standard")
	assert.Contains(t, w.String(), "metrics:")
	// Should contain metrics from the preset
	assert.Contains(t, w.String(), "cpu_load")

	// Test: List non-existent metric/preset
	w.Reset()
	os.Args = []string{0: "config_test", "metric", "list", "nonexistent"}
	_, err = New(w)
	assert.Error(t, err, "should error when metric/preset not found")
	assert.Contains(t, err.Error(), "not found")

	// Test: Error handling - invalid metrics path
	os.Args = []string{0: "config_test", "--metrics=foo", "metric", "list"}
	_, err = New(w)
	assert.Error(t, err, "should error when no metric definitions found")

	os.Args = []string{0: "config_test", "--metrics=postgresql://foo@bar/fail", "metric", "list"}
	_, err = New(w)
	assert.Error(t, err, "should error when no config database found")
}
