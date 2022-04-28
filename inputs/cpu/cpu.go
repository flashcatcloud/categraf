package cpu

import (
	"log"

	cpuUtil "github.com/shirou/gopsutil/v3/cpu"
	"github.com/toolkits/pkg/container/list"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/inputs/system"
)

const inputName = "cpu"

type CPUStats struct {
	ps        system.PS
	lastStats map[string]cpuUtil.TimesStat

	Interval      config.Duration `toml:"interval"`
	CollectPerCPU bool            `toml:"collect_per_cpu"`
}

func init() {
	ps := system.NewSystemPS()
	inputs.Add(inputName, func() inputs.Input {
		return &CPUStats{
			ps: ps,
		}
	})
}

func (s *CPUStats) Prefix() string {
	return inputName
}

func (s *CPUStats) GetInterval() config.Duration {
	return s.Interval
}

// overwrite func
func (c *CPUStats) Init() error {
	return nil
}

func (c *CPUStats) Drop() {
}

func (c *CPUStats) Gather(slist *list.SafeList) {
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
			"usage_user":       100 * (cts.User - lastCts.User - (cts.Guest - lastCts.Guest)) / totalDelta,
			"usage_system":     100 * (cts.System - lastCts.System) / totalDelta,
			"usage_idle":       100 * (cts.Idle - lastCts.Idle) / totalDelta,
			"usage_nice":       100 * (cts.Nice - lastCts.Nice - (cts.GuestNice - lastCts.GuestNice)) / totalDelta,
			"usage_iowait":     100 * (cts.Iowait - lastCts.Iowait) / totalDelta,
			"usage_irq":        100 * (cts.Irq - lastCts.Irq) / totalDelta,
			"usage_softirq":    100 * (cts.Softirq - lastCts.Softirq) / totalDelta,
			"usage_steal":      100 * (cts.Steal - lastCts.Steal) / totalDelta,
			"usage_guest":      100 * (cts.Guest - lastCts.Guest) / totalDelta,
			"usage_guest_nice": 100 * (cts.GuestNice - lastCts.GuestNice) / totalDelta,
			"usage_active":     100 * (active - lastActive) / totalDelta,
		}

		inputs.PushSamples(slist, fields, tags)
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
