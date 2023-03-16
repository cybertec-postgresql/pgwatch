package psutil

import (
	"math"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
)

// "cache" of last CPU utilization stats for GetGoPsutilCPU to get more exact results and not having to sleep
var prevCPULoadTimeStatsLock sync.RWMutex
var prevCPULoadTimeStats cpu.TimesStat
var prevCPULoadTimestamp time.Time

func goPsutilCalcCPUUtilization(probe0, probe1 cpu.TimesStat) float64 {
	return 100 - (100.0 * (probe1.Idle - probe0.Idle + probe1.Iowait - probe0.Iowait + probe1.Steal - probe0.Steal) / (probe1.Total() - probe0.Total()))
}

// Simulates "psutil" metric output. Assumes the result from last call as input, otherwise uses a 1s measurement
// https://github.com/cybertec-postgresql/pgwatch3/blob/master/pgwatch3/metrics/psutil_cpu/9.0/metric.sql
func GetGoPsutilCPU(interval time.Duration) ([]map[string]any, error) {
	prevCPULoadTimeStatsLock.RLock()
	prevTime := prevCPULoadTimestamp
	prevTimeStat := prevCPULoadTimeStats
	prevCPULoadTimeStatsLock.RUnlock()

	if prevTime.IsZero() || (time.Now().UnixNano()-prevTime.UnixNano()) < 1e9 { // give "short" stats on first run, based on a 1s probe
		probe0, err := cpu.Times(false)
		if err != nil {
			return nil, err
		}
		prevTimeStat = probe0[0]
		time.Sleep(1e9)
	}

	curCallStats, err := cpu.Times(false)
	if err != nil {
		return nil, err
	}
	if prevTime.IsZero() || time.Now().UnixNano()-prevTime.UnixNano() < 1e9 || time.Now().Unix()-prevTime.Unix() >= int64(interval.Seconds()) {
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

	retMap := make(map[string]any)
	retMap["epoch_ns"] = time.Now().UnixNano()
	retMap["cpu_utilization"] = math.Round(100*goPsutilCalcCPUUtilization(prevTimeStat, curCallStats[0])) / 100
	retMap["load_1m_norm"] = math.Round(100*la.Load1/float64(cpus)) / 100
	retMap["load_1m"] = math.Round(100*la.Load1) / 100
	retMap["load_5m_norm"] = math.Round(100*la.Load5/float64(cpus)) / 100
	retMap["load_5m"] = math.Round(100*la.Load5) / 100
	retMap["user"] = math.Round(10000.0*(curCallStats[0].User-prevTimeStat.User)/(curCallStats[0].Total()-prevTimeStat.Total())) / 100
	retMap["system"] = math.Round(10000.0*(curCallStats[0].System-prevTimeStat.System)/(curCallStats[0].Total()-prevTimeStat.Total())) / 100
	retMap["idle"] = math.Round(10000.0*(curCallStats[0].Idle-prevTimeStat.Idle)/(curCallStats[0].Total()-prevTimeStat.Total())) / 100
	retMap["iowait"] = math.Round(10000.0*(curCallStats[0].Iowait-prevTimeStat.Iowait)/(curCallStats[0].Total()-prevTimeStat.Total())) / 100
	retMap["irqs"] = math.Round(10000.0*(curCallStats[0].Irq-prevTimeStat.Irq+curCallStats[0].Softirq-prevTimeStat.Softirq)/(curCallStats[0].Total()-prevTimeStat.Total())) / 100
	retMap["other"] = math.Round(10000.0*(curCallStats[0].Steal-prevTimeStat.Steal+curCallStats[0].Guest-prevTimeStat.Guest+curCallStats[0].GuestNice-prevTimeStat.GuestNice)/(curCallStats[0].Total()-prevTimeStat.Total())) / 100

	return []map[string]any{retMap}, nil
}

func GetGoPsutilMem() ([]map[string]any, error) {
	vm, err := mem.VirtualMemory()
	if err != nil {
		return nil, err
	}

	retMap := make(map[string]any)
	retMap["epoch_ns"] = time.Now().UnixNano()
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

	retMap := make(map[string]any)
	var readBytes, writeBytes, reads, writes float64

	retMap["epoch_ns"] = time.Now().UnixNano()
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

	row := make(map[string]any)
	row["epoch_ns"] = time.Now().UnixNano()
	row["load_1min"] = la.Load1
	row["load_5min"] = la.Load5
	row["load_15min"] = la.Load15

	return []map[string]any{row}, nil
}
