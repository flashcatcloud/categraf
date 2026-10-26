package cpu

import (
	"os"
	"path/filepath"
	"testing"

	"flashcat.cloud/categraf/inputs/system"
)

// The same /proc/stat snapshots must produce the same metrics before and after
// a gopsutil upgrade, including guest subtraction and individual CPU labels.
func TestGatherLinuxProcStat(t *testing.T) {
	proc := t.TempDir()
	t.Setenv("HOST_PROC", proc)
	writeStat := func(contents string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(proc, "stat"), []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeStat("cpu 1000 100 400 8000 200 50 50 100 200 20\n" +
		"cpu0 500 50 200 4000 100 25 25 50 100 10\n" +
		"cpu1 500 50 200 4000 100 25 25 50 100 10\n" +
		"intr 1000\n")
	c := &CPUStats{ps: system.NewSystemPS(), CollectPerCPU: true}
	if got := gatherCPUUsage(c); len(got) != 0 {
		t.Fatalf("first gather emitted usage: %v", got)
	}
	writeStat("cpu 1200 120 500 8600 230 60 70 120 240 24\n" +
		"cpu0 600 60 250 4300 115 30 35 60 120 12\n" +
		"cpu1 600 60 250 4300 115 30 35 60 120 12\n" +
		"intr 1100\n")
	want := map[string]float64{
		"cpu_usage_user": 16, "cpu_usage_nice": 1.6, "cpu_usage_system": 10,
		"cpu_usage_idle": 60, "cpu_usage_iowait": 3, "cpu_usage_irq": 1,
		"cpu_usage_softirq": 2, "cpu_usage_steal": 2, "cpu_usage_guest": 4,
		"cpu_usage_guest_nice": 0.4, "cpu_usage_active": 40,
	}
	got := gatherCPUUsage(c)
	if len(got) != 3 {
		t.Fatalf("got %d CPU series, want 3: %v", len(got), got)
	}
	for _, name := range []string{"cpu-total", "cpu0", "cpu1"} {
		assertCPUUsage(t, got[name], want, 1e-9)
	}
}
