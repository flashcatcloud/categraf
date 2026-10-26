package system

import (
	"math"
	"testing"

	"github.com/shirou/gopsutil/v4/cpu"
)

func TestCPUUsageWithInterrupts(t *testing.T) {
	before := cpu.TimesStat{User: 100, System: 200, Idle: 300, Irq: 20}
	after := cpu.TimesStat{User: 120, System: 230, Idle: 350, Irq: 30}
	for _, tt := range []struct {
		goos      string
		wantTotal float64
		wantUsage float64
	}{
		{goos: "windows", wantTotal: 100, wantUsage: 50},
		{goos: "linux", wantTotal: 110, wantUsage: 100 * 60.0 / 110},
	} {
		t.Run(tt.goos, func(t *testing.T) {
			totalDelta := totalCPUTime(after, tt.goos) - totalCPUTime(before, tt.goos)
			if totalDelta != tt.wantTotal {
				t.Fatalf("total delta = %v, want %v", totalDelta, tt.wantTotal)
			}
			activeDelta := totalDelta - (after.Idle - before.Idle)
			usage := 100 * activeDelta / totalDelta
			if math.Abs(usage-tt.wantUsage) > 1e-9 {
				t.Errorf("usage = %v, want %v", usage, tt.wantUsage)
			}
		})
	}
}

func TestTotalCPUTimeFields(t *testing.T) {
	stats := cpu.TimesStat{
		User: 10, System: 20, Nice: 2, Iowait: 3,
		Irq: 5, Softirq: 4, Steal: 6, Idle: 50,
		Guest: 8, GuestNice: 2,
	}
	for _, tt := range []struct {
		goos string
		want float64
	}{
		{goos: "windows", want: 95},
		{goos: "linux", want: 100},
	} {
		t.Run(tt.goos, func(t *testing.T) {
			if got := totalCPUTime(stats, tt.goos); got != tt.want {
				t.Errorf("total = %v, want %v", got, tt.want)
			}
		})
	}
}
