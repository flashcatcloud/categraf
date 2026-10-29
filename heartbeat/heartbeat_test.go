package heartbeat

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"net/http"
	"runtime"
	"testing"

	cpuUtil "github.com/shirou/gopsutil/v4/cpu"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs/system"
	"flashcat.cloud/categraf/internal/nettest"
)

func newTestHeartbeatClient(t *testing.T, endpoint, caFile string) func() error {
	t.Helper()
	original := config.Config
	config.Config = &config.ConfigType{Heartbeat: &config.HeartbeatConfig{
		Url: endpoint, DialTimeout: 2000, Timeout: 2000, MaxIdleConnsPerHost: 10,
	}}
	config.Config.Heartbeat.TLSCA = caFile
	t.Cleanup(func() { config.Config = original })
	client, err := newHTTPClient()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.CloseIdleConnections)
	return func() error {
		req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewBufferString("test payload"))
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if _, err := io.Copy(io.Discard, resp.Body); err != nil {
			return err
		}
		if resp.StatusCode != http.StatusNoContent {
			return fmt.Errorf("unexpected heartbeat status: %s", resp.Status)
		}
		return nil
	}
}

func TestHTTPClientPreferIPv4Stack(t *testing.T) {
	nettest.CheckHTTPClient(t, newTestHeartbeatClient)
}

func TestHTTPClientPreferIPv4StackProxy(t *testing.T) {
	nettest.CheckHTTPProxy(t, newTestHeartbeatClient)
}

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
