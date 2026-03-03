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
	a := assert.New(t)
	tmpDir := t.TempDir()
	createFile := func(name string) string {
		f, err := os.CreateTemp(tmpDir, name)
		a.NoError(err)
		defer f.Close()
		return f.Name()
	}

	file1Path := createFile("file1")
	devID1, err := GetPathUnderlyingDeviceID(file1Path)
	a.NoError(err)

	file2Path := createFile("file2")
	devID2, err := GetPathUnderlyingDeviceID(file2Path)
	a.NoError(err)
	a.Equal(devID1, devID2)
}

func TestGetPathUnderlyingDeviceID_NotFound(t *testing.T) {
	_, err := GetPathUnderlyingDeviceID("/this/path/should/not/exist/nofile")
	assert.Error(t, err)
}

func TestGetGoPsutilDiskPG_Basic(t *testing.T) {
	a := assert.New(t)
	tmpDir := t.TempDir()

	a.True(CheckFolderExistsAndReadable(tmpDir))
	a.False(CheckFolderExistsAndReadable(""))

	pgDirs := metrics.Measurements{
		{"name": "data_directory", "path": tmpDir},
	}
	result, err := GetGoPsutilDiskPG(pgDirs)
	a.NoError(err)
	a.Len(result, 1)
	a.Equal("data_directory", result[0]["tag_dir_or_tablespace"])
	a.Equal(tmpDir, result[0]["tag_path"])

	expectedKeys := []string{metrics.EpochColumnName, "tag_dir_or_tablespace", "tag_path", "total", "used", "free", "percent"}
	resultKeys := slices.Collect(maps.Keys(result[0]))
	a.ElementsMatch(expectedKeys, resultKeys)
}

func TestGetGoPsutilDiskPG_SameDeviceCachedUsage(t *testing.T) {
	a := assert.New(t)
	tmpDir := t.TempDir()

	// Same path twice → same device ID → both rows reported with the same usage values
	pgDirs := metrics.Measurements{
		{"name": "data_directory", "path": tmpDir},
		{"name": "pg_wal", "path": tmpDir},
	}
	result, err := GetGoPsutilDiskPG(pgDirs)
	a.NoError(err)
	a.Len(result, 2)
	a.Equal("data_directory", result[0]["tag_dir_or_tablespace"])
	a.Equal("pg_wal", result[1]["tag_dir_or_tablespace"])
	// Usage values must be identical since they share a device
	a.Equal(result[0]["total"], result[1]["total"])
	a.Equal(result[0]["used"], result[1]["used"])
}

func TestGetGoPsutilDiskPG_NonExistentPathSkipped(t *testing.T) {
	a := assert.New(t)
	tmpDir := t.TempDir()

	pgDirs := metrics.Measurements{
		{"name": "log_directory", "path": "/this/path/does/not/exist"},
		{"name": "data_directory", "path": tmpDir},
	}
	result, err := GetGoPsutilDiskPG(pgDirs)
	a.NoError(err)
	a.Len(result, 1)
	a.Equal("data_directory", result[0]["tag_dir_or_tablespace"])
}

func TestGetGoPsutilDiskPG_Empty(t *testing.T) {
	result, err := GetGoPsutilDiskPG(metrics.Measurements{})
	assert.NoError(t, err)
	assert.Empty(t, result)
}
