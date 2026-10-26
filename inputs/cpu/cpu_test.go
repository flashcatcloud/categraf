package cpu

import (
	"math"
	"runtime"
	"testing"
	"time"

	cpuUtil "github.com/shirou/gopsutil/v4/cpu"

	"flashcat.cloud/categraf/inputs/system"
	"flashcat.cloud/categraf/types"
)

type cpuTimesPS struct {
	system.PS
	times []cpuUtil.TimesStat
}

func (p *cpuTimesPS) CPUTimes(bool, bool) ([]cpuUtil.TimesStat, error) {
	return p.times, nil
}

func gatherCPUUsage(c *CPUStats) map[string]map[string]float64 {
	samples := types.NewSampleList()
	c.Gather(samples)
	metrics := make(map[string]map[string]float64)
	for _, sample := range samples.PopBackAll() {
		name := sample.Labels["cpu"]
		if metrics[name] == nil {
			metrics[name] = make(map[string]float64)
		}
		metrics[name][sample.Metric] = sample.Value.(float64)
	}
	return metrics
}

func assertCPUUsage(t *testing.T, got, want map[string]float64, tolerance float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d metrics, want %d: %v", len(got), len(want), got)
	}
	for name, expected := range want {
		value, ok := got[name]
		if !ok || math.IsNaN(value) || math.IsInf(value, 0) || math.Abs(value-expected) > tolerance {
			t.Errorf("%s = %v (present: %v), want %v +/- %v", name, value, ok, expected, tolerance)
		}
	}
}

// Nonzero interrupt time distinguishes Windows' overlapping System/Irq
// counters from Linux's independent counters. Check the exported metrics,
// including warm-up, rather than only checking the total-time helper.
func TestGatherCPUUsageWithInterrupts(t *testing.T) {
	p := &cpuTimesPS{times: []cpuUtil.TimesStat{
		{CPU: "cpu-total", User: 100, System: 200, Idle: 300, Irq: 20},
		{CPU: "cpu0", User: 50, System: 100, Idle: 150, Irq: 10},
	}}
	c := &CPUStats{ps: p, CollectPerCPU: true}
	if got := gatherCPUUsage(c); len(got) != 0 {
		t.Fatalf("first gather emitted usage without a baseline: %v", got)
	}
	p.times = []cpuUtil.TimesStat{
		{CPU: "cpu-total", User: 120, System: 230, Idle: 350, Irq: 30},
		{CPU: "cpu0", User: 60, System: 115, Idle: 175, Irq: 15},
	}
	total := 110.0
	if runtime.GOOS == "windows" {
		total = 100 // The 10 seconds of Irq are already in System.
	}
	want := map[string]float64{
		"cpu_usage_user": 2000 / total, "cpu_usage_system": 3000 / total,
		"cpu_usage_idle": 5000 / total, "cpu_usage_irq": 1000 / total,
		"cpu_usage_active": 100 * (total - 50) / total,
		"cpu_usage_nice":   0, "cpu_usage_iowait": 0, "cpu_usage_softirq": 0,
		"cpu_usage_steal": 0, "cpu_usage_guest": 0, "cpu_usage_guest_nice": 0,
	}
	got := gatherCPUUsage(c)
	if len(got) != 2 {
		t.Fatalf("got %d CPU series, want 2", len(got))
	}
	for _, name := range []string{"cpu-total", "cpu0"} {
		assertCPUUsage(t, got[name], want, 1e-9)
	}
}

func TestGatherLiveCPUUsage(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "windows" {
		t.Skip("live CPU regression covers Linux and Windows")
	}
	c := &CPUStats{ps: system.NewSystemPS(), CollectPerCPU: true}
	if got := gatherCPUUsage(c); len(got) != 0 {
		t.Fatalf("first gather emitted usage: %v", got)
	}
	if _, ok := c.lastStats["cpu-total"]; !ok || len(c.lastStats) < 2 {
		t.Fatalf("missing total or per-CPU counters: %v", c.lastStats)
	}
	for i := 0; i < 3; i++ {
		time.Sleep(250 * time.Millisecond)
		got := gatherCPUUsage(c)
		if len(got["cpu-total"]) != 11 {
			t.Fatalf("missing total CPU metrics: %v", got)
		}
		for cpu, fields := range got {
			for name, value := range fields {
				if math.IsNaN(value) || math.IsInf(value, 0) {
					t.Errorf("%s %s = %v, want a finite percentage", cpu, name, value)
				}
			}
			// Linux user and guest counters can advance on different ticks;
			// their difference can be slightly negative on an idle hypervisor.
			// Guest subtraction is checked deterministically by the proc fixture.
			for _, name := range []string{"cpu_usage_idle", "cpu_usage_active"} {
				value := fields[name]
				if value < -1e-6 || value > 100+1e-6 {
					t.Errorf("%s %s = %v, want a finite percentage in [0,100]", cpu, name, value)
				}
			}
			if math.Abs(fields["cpu_usage_active"]+fields["cpu_usage_idle"]-100) > 1e-6 {
				t.Errorf("%s active + idle != 100: %v", cpu, fields)
			}
		}
		t.Logf("%s sample %d cpu-total: %v", runtime.GOOS, i+1, got["cpu-total"])
	}
}
