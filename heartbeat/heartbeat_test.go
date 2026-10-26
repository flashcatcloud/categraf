package heartbeat

import (
	"math"
	"runtime"
	"testing"

	cpuUtil "github.com/shirou/gopsutil/v4/cpu"

	"flashcat.cloud/categraf/inputs/system"
)

type heartbeatCPUPS struct {
	system.PS
	times []cpuUtil.TimesStat
	calls int
	t     *testing.T
}

func (p *heartbeatCPUPS) CPUTimes(perCPU, totalCPU bool) ([]cpuUtil.TimesStat, error) {
	if perCPU || !totalCPU {
		p.t.Fatalf("CPUTimes(%v, %v), want (false, true)", perCPU, totalCPU)
	}
	if p.calls >= len(p.times) {
		p.t.Fatal("unexpected extra CPU sample")
	}
	stats := p.times[p.calls]
	p.calls++
	return []cpuUtil.TimesStat{stats}, nil
}

// Exercise the heartbeat sampling path with nonzero cumulative counters and
// interrupt time already included in Windows System time.
func TestCPUUsageWithInterrupts(t *testing.T) {
	ps := &heartbeatCPUPS{
		t: t,
		times: []cpuUtil.TimesStat{
			{CPU: "cpu-total", User: 100, System: 200, Idle: 300, Irq: 20},
			{CPU: "cpu-total", User: 120, System: 230, Idle: 350, Irq: 30},
		},
	}
	want := 100 * 60.0 / 110
	if runtime.GOOS == "windows" {
		want = 50
	}
	if got := cpuUsage(ps); math.IsNaN(got) || math.IsInf(got, 0) || math.Abs(got-want) > 1e-9 {
		t.Errorf("heartbeat cpu_util = %v, want %v", got, want)
	}
	if ps.calls != 2 {
		t.Errorf("CPU samples = %d, want 2", ps.calls)
	}
}
