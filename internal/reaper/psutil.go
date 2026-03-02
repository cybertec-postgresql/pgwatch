package reaper

import (
	"math"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/metrics"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
)

// "cache" of last CPU utilization stats for GetGoPsutilCPU to get more exact results and not having to sleep
var prevCPULoadTimeStatsLock sync.RWMutex
var prevCPULoadTimeStats cpu.TimesStat
var prevCPULoadTimestamp time.Time

// cpuTotal returns the total number of seconds across all CPU states.
// Guest and GuestNice are intentionally excluded because on Linux they are
// already counted within User and Nice respectively (/proc/stat semantics),
// so including them would double-count and skew percentage calculations.
func cpuTotal(c cpu.TimesStat) float64 {
	return c.User + c.System + c.Idle + c.Nice + c.Iowait + c.Irq +
		c.Softirq + c.Steal
}

func goPsutilCalcCPUUtilization(probe0, probe1 cpu.TimesStat) float64 {
	return 100 - (100.0 * (probe1.Idle - probe0.Idle + probe1.Iowait - probe0.Iowait + probe1.Steal - probe0.Steal) / (cpuTotal(probe1) - cpuTotal(probe0)))
}

// Simulates "psutil" metric output. Assumes the result from last call as input, otherwise uses a 1s measurement
func GetGoPsutilCPU(interval float64) ([]map[string]any, error) {
	prevCPULoadTimeStatsLock.RLock()
	prevTime := prevCPULoadTimestamp
	prevTimeStat := prevCPULoadTimeStats
	prevCPULoadTimeStatsLock.RUnlock()

	if prevTime.IsZero() || time.Since(prevTime) < time.Second { // give "short" stats on first run, based on a 1s probe
		probe0, err := cpu.Times(false)
		if err != nil {
			return nil, err
		}
		prevTimeStat = probe0[0]
		time.Sleep(time.Second)
	}

	curCallStats, err := cpu.Times(false)
	if err != nil {
		return nil, err
	}
	if prevTime.IsZero() || time.Since(prevTime) < time.Second || time.Since(prevTime) >= time.Duration(float64(time.Second)*interval) {
		prevCPULoadTimeStatsLock.Lock() // update the cache
		prevCPULoadTimeStats = curCallStats[0]
		prevCPULoadTimestamp = time.Now()
		prevCPULoadTimeStatsLock.Unlock()
	}

	la, err := load.Avg()
	if err != nil {
		return nil, err
	}

	cpus, err := cpu.Counts(true)
	if err != nil {
		return nil, err
	}

	retMap := metrics.NewMeasurement(time.Now().UnixNano())
	retMap["cpu_utilization"] = math.Round(100*goPsutilCalcCPUUtilization(prevTimeStat, curCallStats[0])) / 100
	retMap["load_1m_norm"] = math.Round(100*la.Load1/float64(cpus)) / 100
	retMap["load_1m"] = math.Round(100*la.Load1) / 100
	retMap["load_5m_norm"] = math.Round(100*la.Load5/float64(cpus)) / 100
	retMap["load_5m"] = math.Round(100*la.Load5) / 100
	totalDiff := cpuTotal(curCallStats[0]) - cpuTotal(prevTimeStat)
	retMap["user"] = math.Round(10000.0*(curCallStats[0].User-prevTimeStat.User)/totalDiff) / 100
	retMap["system"] = math.Round(10000.0*(curCallStats[0].System-prevTimeStat.System)/totalDiff) / 100
	retMap["idle"] = math.Round(10000.0*(curCallStats[0].Idle-prevTimeStat.Idle)/totalDiff) / 100
	retMap["iowait"] = math.Round(10000.0*(curCallStats[0].Iowait-prevTimeStat.Iowait)/totalDiff) / 100
	retMap["irqs"] = math.Round(10000.0*(curCallStats[0].Irq-prevTimeStat.Irq+curCallStats[0].Softirq-prevTimeStat.Softirq)/totalDiff) / 100
	retMap["other"] = math.Round(10000.0*(curCallStats[0].Steal-prevTimeStat.Steal+curCallStats[0].Guest-prevTimeStat.Guest+curCallStats[0].GuestNice-prevTimeStat.GuestNice)/totalDiff) / 100

	return []map[string]any{retMap}, nil
}

func GetGoPsutilMem() ([]map[string]any, error) {
	vm, err := mem.VirtualMemory()
	if err != nil {
		return nil, err
	}

	retMap := metrics.NewMeasurement(time.Now().UnixNano())
	retMap["total"] = int64(vm.Total)
	retMap["used"] = int64(vm.Used)
	retMap["free"] = int64(vm.Free)
	retMap["buff_cache"] = int64(vm.Buffers)
	retMap["available"] = int64(vm.Available)
	retMap["percent"] = math.Round(100*vm.UsedPercent) / 100
	retMap["swap_total"] = int64(vm.SwapTotal)
	retMap["swap_used"] = int64(vm.SwapCached)
	retMap["swap_free"] = int64(vm.SwapFree)
	retMap["swap_percent"] = math.Round(100*float64(vm.SwapCached)/float64(vm.SwapTotal)) / 100

	return []map[string]any{retMap}, nil
}

func GetGoPsutilDiskTotals() ([]map[string]any, error) {
	d, err := disk.IOCounters()
	if err != nil {
		return nil, err
	}

	retMap := metrics.NewMeasurement(time.Now().UnixNano())
	var readBytes, writeBytes, reads, writes float64

	for _, v := range d { // summarize all disk devices
		readBytes += float64(v.ReadBytes) // datatype float is just an oversight in the original psutil helper
		// but can't change it without causing problems on storage level (InfluxDB)
		writeBytes += float64(v.WriteBytes)
		reads += float64(v.ReadCount)
		writes += float64(v.WriteCount)
	}
	retMap["read_bytes"] = readBytes
	retMap["write_bytes"] = writeBytes
	retMap["read_count"] = reads
	retMap["write_count"] = writes

	return []map[string]any{retMap}, nil
}

func GetLoadAvgLocal() ([]map[string]any, error) {
	la, err := load.Avg()
	if err != nil {
		return nil, err
	}

	row := metrics.NewMeasurement(time.Now().UnixNano())
	row["load_1min"] = la.Load1
	row["load_5min"] = la.Load5
	row["load_15min"] = la.Load15

	return []map[string]any{row}, nil
}

func CheckFolderExistsAndReadable(path string) bool {
	_, err := os.ReadDir(path)
	return err == nil
}

func GetGoPsutilDiskPG(DataDirs, TblspaceDirs []map[string]any) ([]map[string]any, error) {
	var ddDevice, ldDevice, walDevice uint64

	data := DataDirs

	dataDirPath := data[0]["dd"].(string)
	ddUsage, err := disk.Usage(dataDirPath)
	if err != nil {
		return nil, err
	}

	retRows := make([]map[string]any, 0)
	epochNs := time.Now().UnixNano()
	dd := metrics.NewMeasurement(epochNs)
	dd["tag_dir_or_tablespace"] = "data_directory"
	dd["tag_path"] = dataDirPath
	dd["total"] = float64(ddUsage.Total)
	dd["used"] = float64(ddUsage.Used)
	dd["free"] = float64(ddUsage.Free)
	dd["percent"] = math.Round(100*ddUsage.UsedPercent) / 100
	retRows = append(retRows, dd)

	ddDevice, err = GetPathUnderlyingDeviceID(dataDirPath)
	if err != nil {
		return nil, err
	}

	logDirPath := data[0]["ld"].(string)
	if !strings.HasPrefix(logDirPath, "/") {
		logDirPath = path.Join(dataDirPath, logDirPath)
	}
	if logDirPath != "" && CheckFolderExistsAndReadable(logDirPath) { // syslog etc considered out of scope
		ldDevice, err = GetPathUnderlyingDeviceID(logDirPath)
		if err != nil {
			return nil, err
		}
		if ldDevice != ddDevice { // no point to report same data in case of single folder configuration
			ld := metrics.NewMeasurement(epochNs)
			ldUsage, err := disk.Usage(logDirPath)
			if err != nil {
				return nil, err
			}

			ld["tag_dir_or_tablespace"] = "log_directory"
			ld["tag_path"] = logDirPath
			ld["total"] = float64(ldUsage.Total)
			ld["used"] = float64(ldUsage.Used)
			ld["free"] = float64(ldUsage.Free)
			ld["percent"] = math.Round(100*ldUsage.UsedPercent) / 100
			retRows = append(retRows, ld)
		}
	}

	var walDirPath string
	if CheckFolderExistsAndReadable(path.Join(dataDirPath, "pg_wal")) {
		walDirPath = path.Join(dataDirPath, "pg_wal")
	}

	if walDirPath != "" {
		walDevice, err = GetPathUnderlyingDeviceID(walDirPath)
		if err != nil {
			return nil, err
		}

		if walDevice != ddDevice || walDevice != ldDevice { // no point to report same data in case of single folder configuration
			walUsage, err := disk.Usage(walDirPath)
			if err != nil {
				return nil, err
			}

			wd := metrics.NewMeasurement(epochNs)
			wd["tag_dir_or_tablespace"] = "pg_wal"
			wd["tag_path"] = walDirPath
			wd["total"] = float64(walUsage.Total)
			wd["used"] = float64(walUsage.Used)
			wd["free"] = float64(walUsage.Free)
			wd["percent"] = math.Round(100*walUsage.UsedPercent) / 100
			retRows = append(retRows, wd)
		}
	}

	data = TblspaceDirs
	if len(data) > 0 {
		for _, row := range data {
			tsPath := row["location"].(string)
			tsName := row["name"].(string)

			tsDevice, err := GetPathUnderlyingDeviceID(tsPath)
			if err != nil {
				return nil, err
			}

			if tsDevice == ddDevice || tsDevice == ldDevice || tsDevice == walDevice {
				continue
			}
			tsUsage, err := disk.Usage(tsPath)
			if err != nil {
				return nil, err
			}
			ts := metrics.NewMeasurement(epochNs)
			ts["tag_dir_or_tablespace"] = tsName
			ts["tag_path"] = tsPath
			ts["total"] = float64(tsUsage.Total)
			ts["used"] = float64(tsUsage.Used)
			ts["free"] = float64(tsUsage.Free)
			ts["percent"] = math.Round(100*tsUsage.UsedPercent) / 100
			retRows = append(retRows, ts)
		}
	}

	return retRows, nil
}
