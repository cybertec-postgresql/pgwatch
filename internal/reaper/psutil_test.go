package reaper

import (
	"slices"
	"testing"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/metrics"

	"maps"
	"os"
	"github.com/stretchr/testify/assert"
)

func TestGetGoPsutilCPU(t *testing.T) {
	a := assert.New(t)

	// Call the GetGoPsutilCPU function with a 1-second interval
	result, err := GetGoPsutilCPU(1.0)
	a.NoError(err)
	a.NotEmpty(result)

	// Check if the result contains the expected keys
	expectedKeys := []string{metrics.EpochColumnName, "cpu_utilization", "load_1m_norm", "load_1m", "load_5m_norm", "load_5m", "user", "system", "idle", "iowait", "irqs", "other"}
	resultKeys := slices.Collect(maps.Keys(result[0]))
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
	resultKeys := slices.Collect(maps.Keys(result[0]))
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
	resultKeys := slices.Collect(maps.Keys(result[0]))
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
	resultKeys := slices.Collect(maps.Keys(result[0]))
	a.ElementsMatch(resultKeys, expectedKeys)
}

func TestGetPathUnderlyingDeviceID(t *testing.T) {
	tmpDir := t.TempDir()
	createFile := func(name string) string {
		f, err := os.CreateTemp(tmpDir, name)
		if err != nil {
			t.Fatalf("failed to create temp file: %v", err)
		}
		defer f.Close()
		return f.Name()
	}

	// Test with a valid file
	file1Path := createFile("file1")
	devID1, err := GetPathUnderlyingDeviceID(file1Path)
	if err != nil {
		t.Fatalf("GetPathUnderlyingDeviceID failed: %v", err)
	}

	// Consistency Check
	file2Path := createFile("file2")
	devID2, err := GetPathUnderlyingDeviceID(file2Path)
	if err != nil {
		t.Fatalf("GetPathUnderlyingDeviceID failed on second file: %v", err)
	}

	if devID1 != devID2 {
		t.Errorf("Expected files in same directory to have same DeviceID, got %d and %d", devID1, devID2)
	}
}

func TestGetPathUnderlyingDeviceID_NotFound(t *testing.T) {
	_, err := GetPathUnderlyingDeviceID("/this/path/should/not/exist/nofile")
	if err == nil {
		t.Error("Expected error for non-existent file, got nil")
	}
}
