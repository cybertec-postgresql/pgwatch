package reaper

import (
	"math"
	"os"
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

func init() {
	// initialize the cache with current stats so the first call returns a meaningful delta
	if probe, err := cpu.Times(false); err == nil {
		prevCPULoadTimeStats = probe[0]
		prevCPULoadTimestamp = time.Now()
	}
}

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

// GetGoPsutilCPU simulates "psutil" metric output. Assumes the result from last call as input
func GetGoPsutilCPU(interval time.Duration) (metrics.Measurements, error) {
	prevCPULoadTimeStatsLock.RLock()
	prevTime := prevCPULoadTimestamp
	prevTimeStat := prevCPULoadTimeStats
	prevCPULoadTimeStatsLock.RUnlock()

	curCallStats, err := cpu.Times(false)
	if err != nil {
		return nil, err
	}
	if time.Since(prevTime) >= interval {
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

	return metrics.Measurements{retMap}, nil
}

func GetGoPsutilMem() (metrics.Measurements, error) {
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

	return metrics.Measurements{retMap}, nil
}

func GetGoPsutilDiskTotals() (metrics.Measurements, error) {
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

	return metrics.Measurements{retMap}, nil
}

func GetLoadAvgLocal() (metrics.Measurements, error) {
	la, err := load.Avg()
	if err != nil {
		return nil, err
	}

	row := metrics.NewMeasurement(time.Now().UnixNano())
	row["load_1min"] = la.Load1
	row["load_5min"] = la.Load5
	row["load_15min"] = la.Load15

	return metrics.Measurements{row}, nil
}

func CheckFolderExistsAndReadable(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.ReadDir(path)
	return err == nil
}

func GetGoPsutilDiskPG(pgDirs metrics.Measurements) (metrics.Measurements, error) {
	usageCache := make(map[uint64]*disk.UsageStat)
	retRows := make(metrics.Measurements, 0)
	epochNs := time.Now().UnixNano()

	for _, row := range pgDirs {
		path := row["path"].(string)
		name := row["name"].(string)

		if !CheckFolderExistsAndReadable(path) { // syslog etc considered out of scope
			continue
		}

		devID, err := GetPathUnderlyingDeviceID(path)
		if err != nil {
			return nil, err
		}

		usage, ok := usageCache[devID]
		if !ok {
			usage, err = disk.Usage(path)
			if err != nil {
				return nil, err
			}
			usageCache[devID] = usage
		}

		m := metrics.NewMeasurement(epochNs)
		m["tag_dir_or_tablespace"] = name
		m["tag_path"] = path
		m["total"] = float64(usage.Total)
		m["used"] = float64(usage.Used)
		m["free"] = float64(usage.Free)
		m["percent"] = math.Round(100*usage.UsedPercent) / 100
		retRows = append(retRows, m)
	}

	return retRows, nil
}
