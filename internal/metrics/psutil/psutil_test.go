package psutil

import (
	"testing"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"

	"github.com/stretchr/testify/assert"
	"golang.org/x/exp/maps"
)

func TestGetGoPsutilCPU(t *testing.T) {
	a := assert.New(t)

	// Call the GetGoPsutilCPU function with a 1-second interval
	result, err := GetGoPsutilCPU(1 * time.Second)
	a.NoError(err)
	a.NotEmpty(result)

	// Check if the result contains the expected keys
	expectedKeys := []string{metrics.EpochColumnName, "cpu_utilization", "load_1m_norm", "load_1m", "load_5m_norm", "load_5m", "user", "system", "idle", "iowait", "irqs", "other"}
	resultKeys := maps.Keys(result[0])
	a.ElementsMatch(resultKeys, expectedKeys)

	// Check if the CPU utilization is within the expected range
	cpuUtilization := result[0]["cpu_utilization"].(float64)
	a.GreaterOrEqual(cpuUtilization, 0.0)
	a.LessOrEqual(cpuUtilization, 100.0)
}

func TestGetGoPsutilMem(t *testing.T) {
	a := assert.New(t)

	// Call the GetGoPsutilMem function
	result, err := GetGoPsutilMem()
	a.NoError(err)
	a.NotEmpty(result)

	// Check if the result contains the expected keys
	expectedKeys := []string{metrics.EpochColumnName, "total", "used", "free", "buff_cache", "available", "percent", "swap_total", "swap_used", "swap_free", "swap_percent"}
	resultKeys := maps.Keys(result[0])
	a.ElementsMatch(resultKeys, expectedKeys)
}

func TestGetGoPsutilDiskTotals(t *testing.T) {
	a := assert.New(t)

	// Call the GetGoPsutilDiskTotals function
	result, err := GetGoPsutilDiskTotals()
	if err != nil {
		t.Skip("skipping test; disk.IOCounters() failed")
	}
	a.NotEmpty(result)

	// Check if the result contains the expected keys
	expectedKeys := []string{metrics.EpochColumnName, "read_bytes", "write_bytes", "read_count", "write_count"}
	resultKeys := maps.Keys(result[0])
	a.ElementsMatch(resultKeys, expectedKeys)
}

func TestGetLoadAvgLocal(t *testing.T) {
	a := assert.New(t)

	// Call the GetLoadAvgLocal function
	result, err := GetLoadAvgLocal()
	a.NoError(err)
	a.NotEmpty(result)

	// Check if the result contains the expected keys
	expectedKeys := []string{metrics.EpochColumnName, "load_1min", "load_5min", "load_15min"}
	resultKeys := maps.Keys(result[0])
	a.ElementsMatch(resultKeys, expectedKeys)
}
