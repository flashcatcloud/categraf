package cpu

import (
	"math"
	"testing"
	"time"
	"unsafe"

	cpuUtil "github.com/shirou/gopsutil/v4/cpu"
	"golang.org/x/sys/windows"
)

// Compare against the GetSystemTimes path used by gopsutil v4.25.8 on an
// ordinary, single-group Windows host. It cannot be a reference above 64 CPUs.
func TestWindowsCPUUsageMatchesLegacy(t *testing.T) {
	count, err := cpuUtil.Counts(true)
	if err != nil {
		t.Fatal(err)
	}
	if count > 64 {
		t.Skip("GetSystemTimes is not a whole-machine reference above 64 logical CPUs")
	}
	getSystemTimes := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetSystemTimes")
	legacyTimes := func() cpuUtil.TimesStat {
		t.Helper()
		var idle, kernel, user windows.Filetime
		r, _, err := getSystemTimes.Call(uintptr(unsafe.Pointer(&idle)), uintptr(unsafe.Pointer(&kernel)), uintptr(unsafe.Pointer(&user)))
		if r == 0 {
			t.Fatalf("GetSystemTimes: %v", err)
		}
		// Deliberately keep the old conversion for the before/after comparison.
		seconds := func(v windows.Filetime) float64 {
			return 0.0000001*4294967296*float64(v.HighDateTime) + 0.0000001*float64(v.LowDateTime)
		}
		idleSeconds := seconds(idle)
		return cpuUtil.TimesStat{CPU: "cpu-total", User: seconds(user), System: seconds(kernel) - idleSeconds, Idle: idleSeconds}
	}
	read := func() (cpuUtil.TimesStat, cpuUtil.TimesStat, time.Duration) {
		t.Helper()
		start := time.Now()
		before := legacyTimes()
		current, err := cpuUtil.Times(false)
		if err != nil {
			t.Fatal(err)
		}
		after := legacyTimes()
		skew := time.Since(start)
		if len(current) != 1 || current[0].CPU != "cpu-total" {
			t.Fatalf("unexpected total counters: %v", current)
		}
		// Bracket the new call to allow for CPU time advancing between APIs.
		slack := 0.1 * float64(count)
		for name, values := range map[string][3]float64{
			"user":   {before.User, current[0].User, after.User},
			"system": {before.System, current[0].System, after.System},
			"idle":   {before.Idle, current[0].Idle, after.Idle},
		} {
			if values[1] < values[0]-slack || values[1] > values[2]+slack {
				t.Errorf("%s counter %v outside legacy bracket [%v,%v] +/- %v", name, values[1], values[0], values[2], slack)
			}
		}
		return before, current[0], skew
	}
	legacy1, current1, skew1 := read()
	time.Sleep(time.Second)
	legacy2, current2, skew2 := read()
	usage := func(first, second cpuUtil.TimesStat) map[string]float64 {
		p := &cpuTimesPS{times: []cpuUtil.TimesStat{first}}
		c := &CPUStats{ps: p}
		gatherCPUUsage(c)
		p.times = []cpuUtil.TimesStat{second}
		return gatherCPUUsage(c)["cpu-total"]
	}
	legacy, current := usage(legacy1, legacy2), usage(current1, current2)
	if len(legacy) != 11 || len(current) != 11 {
		t.Fatalf("missing usage metrics: legacy=%v current=%v", legacy, current)
	}
	// Bound the difference by call skew and Windows counter granularity.
	interval := math.Min(totalCPUTime(legacy2)-totalCPUTime(legacy1), totalCPUTime(current2)-totalCPUTime(current1))
	if interval <= 0 {
		t.Fatalf("nonpositive CPU interval: %v", interval)
	}
	tolerance := 1 + 100*(skew1.Seconds()+skew2.Seconds()+0.05)*float64(count)/interval
	for _, name := range []string{"cpu_usage_user", "cpu_usage_system", "cpu_usage_idle", "cpu_usage_active"} {
		if math.Abs(legacy[name]-current[name]) > tolerance {
			t.Errorf("%s: legacy=%v upgraded=%v, tolerance=%v percentage points", name, legacy[name], current[name], tolerance)
		}
	}
	t.Logf("logical CPUs=%d legacy=%v upgraded=%v tolerance=%.3f percentage points", count, legacy, current, tolerance)
}
