package system

import (
	"runtime"

	"github.com/shirou/gopsutil/v4/cpu"
)

// TotalCPUTime sums CPU counters without double-counting overlapping fields.
func TotalCPUTime(t cpu.TimesStat) float64 {
	return totalCPUTime(t, runtime.GOOS)
}

func totalCPUTime(t cpu.TimesStat, goos string) float64 {
	irq := t.Irq
	if goos == "windows" {
		// Windows System already includes Irq. gopsutil v4.26.7 also
		// reports Irq for cpu-total, so do not count it a second time.
		irq = 0
	}
	return t.User + t.System + t.Nice + t.Iowait + irq + t.Softirq + t.Steal + t.Idle
}
