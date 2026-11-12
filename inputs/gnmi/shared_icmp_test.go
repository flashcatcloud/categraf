package gnmi

import (
	"context"
	"sync"
	"testing"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/types"
)

func testSharedICMP(t *testing.T, concurrency int, instances ...*Instance) *GNMI {
	t.Helper()
	plugin := &GNMI{Instances: instances, ICMPMaxConcurrency: concurrency}
	if err := plugin.Init(); err != nil {
		t.Fatal(err)
	}
	for _, instance := range instances {
		if instance.AgentHostTag == "" {
			instance.AgentHostTag = "agent_host"
		}
		if err := instance.initHealth(); err != nil {
			t.Fatal(err)
		}
		if err := instance.initICMP(); err != nil {
			t.Fatal(err)
		}
		instance.SetInitialized()
	}
	plugin.icmp = plugin.newICMPMonitor()
	return plugin
}

func TestICMPSharingUsesEffectiveProbeSettings(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*Instance)
		shared bool
	}{
		{name: "identical", shared: true},
		{name: "report labels and session cadence", shared: true, change: func(c *Instance) {
			c.Labels = map[string]string{"job": "other"}
			c.GNMIUpInterval = config.Duration(time.Minute)
		}},
		{name: "explicit default privilege", shared: true, change: func(c *Instance) { v := true; c.ICMPPrivileged = &v }},
		{name: "interval", change: func(c *Instance) { c.ICMPInterval = config.Duration(time.Minute) }},
		{name: "timeout", change: func(c *Instance) { c.ICMPTimeout = config.Duration(4 * time.Second) }},
		{name: "count", change: func(c *Instance) { c.ICMPCount = 4 }},
		{name: "packet interval", change: func(c *Instance) { c.ICMPPacketInterval = config.Duration(300 * time.Millisecond) }},
		{name: "concurrency", change: func(c *Instance) { c.ICMPConcurrency = 1 }},
		{name: "privilege", change: func(c *Instance) { v := false; c.ICMPPrivileged = &v }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			first := &Instance{Addresses: []string{"HOST:1", "host:2"}, EnableICMP: true}
			second := &Instance{Addresses: []string{"host:3"}, EnableICMP: true}
			if tc.change != nil {
				tc.change(second)
			}
			plugin := testSharedICMP(t, 0, first, second)
			key := first.icmpTargets[first.Addresses[0]]
			same := key == second.icmpTargets[second.Addresses[0]]
			if same != tc.shared || first.icmpTargets[first.Addresses[1]] != key {
				t.Fatalf("incorrect probe sharing: %v / %v", first.icmpTargets, second.icmpTargets)
			}
			want := 2
			if tc.shared {
				want = 1
			}
			if len(plugin.icmp.keys) != want || first.icmp != plugin.icmp || second.icmp != plugin.icmp {
				t.Fatal("instances did not use the expected shared pool")
			}
		})
	}
}

type sharedProbeCall struct {
	host    string
	options icmpOptions
}
type sharedRecordingProbe struct {
	mu                         sync.Mutex
	calls                      []sharedProbeCall
	active, peak               int
	activeByCount, peakByCount map[int]int
	started                    chan sharedProbeCall
	release                    <-chan struct{}
}

func (p *sharedRecordingProbe) Probe(ctx context.Context, host string, options icmpOptions) (icmpResult, error) {
	call := sharedProbeCall{host, options}
	p.mu.Lock()
	p.calls = append(p.calls, call)
	p.active++
	p.peak = max(p.peak, p.active)
	if p.activeByCount == nil {
		p.activeByCount = make(map[int]int)
		p.peakByCount = make(map[int]int)
	}
	p.activeByCount[options.Count]++
	p.peakByCount[options.Count] = max(p.peakByCount[options.Count], p.activeByCount[options.Count])
	p.mu.Unlock()
	defer func() { p.mu.Lock(); p.active--; p.activeByCount[options.Count]--; p.mu.Unlock() }()
	if p.started != nil {
		select {
		case p.started <- call:
		case <-ctx.Done():
			return icmpResult{}, ctx.Err()
		}
	}
	if p.release != nil {
		select {
		case <-p.release:
		case <-ctx.Done():
			return icmpResult{}, ctx.Err()
		}
	}
	return icmpResult{Sent: options.Count, Received: options.Count, AvgRTT: time.Millisecond}, nil
}

func TestSharedICMPProbesOnceAndKeepsInstanceOutput(t *testing.T) {
	first := &Instance{Addresses: []string{"HOST:1", "host:2", "other-a:1"}, EnableICMP: true, DisableGNMIUp: true, ICMPConcurrency: 1, ICMPCount: 1}
	second := &Instance{Addresses: []string{"host:3", "other-b:1"}, EnableICMP: true, DisableGNMIUp: true, ICMPConcurrency: 1, ICMPCount: 1}
	third := &Instance{Addresses: []string{"host:4"}, EnableICMP: true, DisableGNMIUp: true, ICMPConcurrency: 1, ICMPCount: 2}
	disabled := &Instance{Addresses: []string{"disabled:1"}, ICMPCount: -1, DisableGNMIUp: true}
	plugin := testSharedICMP(t, 2, first, second, third, disabled)
	if len(plugin.icmp.keys) != 4 || disabled.icmp != nil {
		t.Fatal("disabled/duplicate targets included in shared pool")
	}
	probe := &sharedRecordingProbe{}
	monitor := plugin.icmp
	monitor.probe = probe
	monitor.random = func() float64 { return 0 }
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { monitor.run(ctx); close(done) }()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("shared pool leaked")
		}
	}()
	for _, key := range monitor.keys {
		waitHealth(t, func() bool { snapshot, ok := monitor.snapshot(key); return ok && snapshot.Success })
	}
	probe.mu.Lock()
	calls := append([]sharedProbeCall(nil), probe.calls...)
	probe.mu.Unlock()
	if len(calls) != 4 {
		t.Fatalf("network rounds=%d, want 4 distinct effective targets", len(calls))
	}
	hostCount := map[int]int{}
	for _, call := range calls {
		if call.host == "host" {
			hostCount[call.options.Count]++
		}
	}
	if hostCount[1] != 1 || hostCount[2] != 1 {
		t.Fatalf("host/config rounds were lost or repeated: %v", hostCount)
	}
	for index, instance := range []*Instance{first, second, third} {
		instance.Labels = map[string]string{"job": []string{"interfaces", "cpu", "memory"}[index]}
		list := types.NewSampleList()
		instance.gatherHealth(list, time.Now())
		samples := list.PopBackAll()
		if len(samples) != 5*len(instance.Addresses) {
			t.Fatalf("legacy ICMP sample count changed: %d", len(samples))
		}
		addresses := map[string]bool{}
		for _, sample := range samples {
			if sample.Labels["job"] != instance.Labels["job"] {
				t.Fatal("instance labels lost on shared result")
			}
			addresses[sample.Labels["address"]] = true
			if sample.Metric == "gnmi_icmp_probe_success" && sample.Value != 1 {
				t.Fatal("round validated using another consumer's count")
			}
		}
		if len(addresses) != len(instance.Addresses) {
			t.Fatal("ports collapsed in instance output")
		}
	}
	// Rebuilding the shared monitor must not treat prior internal IDs as hosts.
	again := plugin.newICMPMonitor()
	if len(again.keys) != 4 {
		t.Fatal("shared target construction is not repeatable")
	}
	for _, target := range again.targets {
		if target.host != "host" && target.host != "other-a" && target.host != "other-b" {
			t.Fatalf("probe ID leaked into host identity: %+v", target)
		}
	}
}

func TestSharedICMPRespectsCapsAndDrop(t *testing.T) {
	for _, limit := range []int{1, 2} {
		first := &Instance{Addresses: []string{"a:1", "b:1"}, EnableICMP: true, ICMPConcurrency: 1, ICMPCount: 1}
		second := &Instance{Addresses: []string{"c:1", "d:1"}, EnableICMP: true, ICMPConcurrency: 1, ICMPCount: 2}
		plugin := testSharedICMP(t, limit, first, second)
		probe := &sharedRecordingProbe{started: make(chan sharedProbeCall, 10), release: make(chan struct{})}
		plugin.icmp.probe = probe
		plugin.icmp.random = func() float64 { return 0 }
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		plugin.icmpCancel, plugin.icmpDone = cancel, done
		go func() { plugin.icmp.run(ctx); close(done) }()
		for i := 0; i < limit; i++ {
			select {
			case <-probe.started:
			case <-time.After(3 * time.Second):
				cancel()
				t.Fatal("shared workers did not start")
			}
		}
		plugin.Drop()
		plugin.Drop()
		probe.mu.Lock()
		if probe.active != 0 || probe.peak != limit || probe.peakByCount[1] > 1 || probe.peakByCount[2] > 1 {
			t.Errorf("concurrency/drop failure: active=%d peak=%d groups=%v", probe.active, probe.peak, probe.peakByCount)
		}
		probe.mu.Unlock()
		if err := plugin.Start(nil); err != nil {
			t.Fatal(err)
		}
		if plugin.icmpDone != done {
			t.Fatal("Start restarted an already dropped pool")
		}
	}
}

func TestSharedICMPServiceLifecycle(t *testing.T) {
	first := &Instance{Addresses: []string{"127.0.0.1:1"}, EnableICMP: true}
	second := &Instance{Addresses: []string{"127.0.0.1:2"}, EnableICMP: true}
	plugin := &GNMI{Instances: []*Instance{first, second}}
	if err := plugin.Init(); err != nil {
		t.Fatal(err)
	}
	defer plugin.Drop()
	for _, instance := range plugin.Instances {
		if err := instance.Init(); err != nil {
			t.Fatal(err)
		}
		instance.SetInitialized()
	}
	if err := plugin.Start(nil); err != nil {
		t.Fatal(err)
	}
	done := plugin.icmpDone
	if len(plugin.icmp.keys) != 1 || first.icmp != second.icmp || first.icmpTargets[first.Addresses[0]] != second.icmpTargets[second.Addresses[0]] {
		t.Fatal("service Start did not bind compatible consumers to one probe")
	}
	if err := plugin.Start(nil); err != nil {
		t.Fatal(err)
	}
	if plugin.icmpDone != done {
		t.Fatal("service Start launched a second monitor")
	}
	plugin.Drop()
	select {
	case <-done:
	default:
		t.Fatal("Drop returned before shared monitor stopped")
	}
	for _, instance := range plugin.Instances {
		for _, session := range instance.health {
			if session.isUp() {
				t.Fatal("Drop retained a live session")
			}
		}
	}
}
