package cpu

import (
	"fmt"
	"log"
	"strings"
	"time"

	cpuUtil "github.com/shirou/gopsutil/v3/cpu"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/inputs/system"
	"flashcat.cloud/categraf/types"
)

const InputName = "cpu"

type CPUStats struct {
	PrintConfigs    bool
	IntervalSeconds int64

	quit      chan struct{}
	lastStats map[string]cpuUtil.TimesStat

	ps system.PS

	CollectPerCPU bool
}

func init() {
	ps := system.NewSystemPS()
	inputs.Add(InputName, func() inputs.Input {
		return &CPUStats{
			ps:   ps,
			quit: make(chan struct{}),
		}
	})
}

func (c *CPUStats) getInterval() time.Duration {
	if c.IntervalSeconds != 0 {
		return time.Duration(c.IntervalSeconds) * time.Second
	}
	return config.GetInterval()
}

// overwrite func
func (c *CPUStats) Init() error {
	return nil
}

// overwrite func
func (c *CPUStats) StopGoroutines() {
	c.quit <- struct{}{}
}

// overwrite func
func (c *CPUStats) StartGoroutines(queue chan *types.Sample) {
	go c.LoopGather(queue)
}

func (c *CPUStats) LoopGather(queue chan *types.Sample) {
	interval := c.getInterval()
	for {
		select {
		case <-c.quit:
			close(c.quit)
			return
		default:
			time.Sleep(interval)
			c.Gather(queue)
		}
	}
}

// overwrite func
func (c *CPUStats) Gather(queue chan *types.Sample) {
	var samples []*types.Sample

	defer func() {
		if r := recover(); r != nil {
			if strings.Contains(fmt.Sprint(r), "closed channel") {
				return
			} else {
				log.Println("E! gather metrics panic:", r)
			}
		}

		now := time.Now()
		for i := 0; i < len(samples); i++ {
			samples[i].Timestamp = now
			samples[i].Metric = InputName + "_" + samples[i].Metric
			queue <- samples[i]
		}
	}()

	// ----------------------------------------------

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

		samples = append(samples, inputs.NewSamples(fields, tags)...)
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
