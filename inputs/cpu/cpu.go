package cpu

import (
	"log"

	cpuUtil "github.com/shirou/gopsutil/v3/cpu"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/inputs/system"
	"flashcat.cloud/categraf/types"
)

const inputName = "cpu"

type CPUStats struct {
	ps        system.PS
	lastStats map[string]cpuUtil.TimesStat

	config.PluginConfig
	CollectPerCPU bool `toml:"collect_per_cpu"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &CPUStats{
			ps: system.NewSystemPS(),
		}
	})
}

func (c *CPUStats) Clone() inputs.Input {
	return &CPUStats{
		ps: system.NewSystemPS(),
	}
}

func (c *CPUStats) Name() string {
	return inputName
}

func (c *CPUStats) Gather(slist *types.SampleList) {
	times, err := c.ps.CPUTimes(c.CollectPerCPU, true)
	if err != nil {
		log.Println("E! failed to get cpu metrics:", err)
		return
	}

	for _, cts := range times {
		tags := map[string]string{
			"cpu": cts.CPU,
		}

		total := totalCPUTime(cts)
		active := activeCPUTime(cts)

		// Add in percentage
		if len(c.lastStats) == 0 {
			// If it's the 1st gather, can't get CPU Usage stats yet
			continue
		}

		lastCts, ok := c.lastStats[cts.CPU]
		if !ok {
			continue
		}

		lastTotal := totalCPUTime(lastCts)
		lastActive := activeCPUTime(lastCts)
		totalDelta := total - lastTotal

		if totalDelta < 0 {
			log.Println("W! current total CPU time is less than previous total CPU time")
			break
		}

		if totalDelta == 0 {
			continue
		}

		fields := map[string]interface{}{
			"user":       100 * (cts.User - lastCts.User - (cts.Guest - lastCts.Guest)) / totalDelta,
			"system":     100 * (cts.System - lastCts.System) / totalDelta,
			"idle":       100 * (cts.Idle - lastCts.Idle) / totalDelta,
			"nice":       100 * (cts.Nice - lastCts.Nice - (cts.GuestNice - lastCts.GuestNice)) / totalDelta,
			"iowait":     100 * (cts.Iowait - lastCts.Iowait) / totalDelta,
			"irq":        100 * (cts.Irq - lastCts.Irq) / totalDelta,
			"softirq":    100 * (cts.Softirq - lastCts.Softirq) / totalDelta,
			"steal":      100 * (cts.Steal - lastCts.Steal) / totalDelta,
			"guest":      100 * (cts.Guest - lastCts.Guest) / totalDelta,
			"guest_nice": 100 * (cts.GuestNice - lastCts.GuestNice) / totalDelta,
			"active":     100 * (active - lastActive) / totalDelta,
		}

		slist.PushSamples("cpu_usage", fields, tags)
	}

	c.lastStats = make(map[string]cpuUtil.TimesStat)
	for _, cts := range times {
		c.lastStats[cts.CPU] = cts
	}
}

func totalCPUTime(t cpuUtil.TimesStat) float64 {
	total := t.User + t.System + t.Nice + t.Iowait + t.Irq + t.Softirq + t.Steal + t.Idle
	return total
}

func activeCPUTime(t cpuUtil.TimesStat) float64 {
	active := totalCPUTime(t) - t.Idle
	return active
}
