package gnmi

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"flashcat.cloud/categraf/types"
)

type blockingICMP struct {
	active, peak, calls atomic.Int32
	started             chan string
}

func (p *blockingICMP) Probe(ctx context.Context, host string, o icmpOptions) (icmpResult, error) {
	active := p.active.Add(1)
	defer p.active.Add(-1)
	p.calls.Add(1)
	for {
		old := p.peak.Load()
		if active <= old || p.peak.CompareAndSwap(old, active) {
			break
		}
	}
	select {
	case p.started <- host:
	case <-ctx.Done():
		return icmpResult{}, ctx.Err()
	}
	<-ctx.Done()
	return icmpResult{}, ctx.Err()
}
func TestICMPMonitorBoundedAndCanceled(t *testing.T) {
	p := &blockingICMP{started: make(chan string, 10)}
	m := newICMPMonitor([]string{"a", "b", "c", "d"}, icmpOptions{Count: 3, Timeout: time.Second}, time.Second, 2, p)
	m.random = func() float64 { return 0 }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { m.run(ctx); close(done) }()
	for i := 0; i < 2; i++ {
		select {
		case <-p.started:
		case <-time.After(time.Second):
			t.Fatal("workers did not start")
		}
	}
	if p.peak.Load() > 2 || p.calls.Load() != 2 {
		t.Fatal("unbounded or reentrant probes")
	}
	// Snapshot/metric output must return while network probes remain blocked.
	c := &Instance{Addresses: []string{"a:1"}, EnableICMP: true, DisableGNMIUp: true, AgentHostTag: "agent_host", icmp: m}
	gatherDone := make(chan struct{})
	go func() { c.gatherHealth(types.NewSampleList(), time.Now()); close(gatherDone) }()
	select {
	case <-gatherDone:
	case <-time.After(time.Second):
		t.Fatal("Gather waited for probe")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("monitor did not stop")
	}
	if p.active.Load() != 0 {
		t.Fatal("workers leaked")
	}
}
func TestICMPMonitorDeduplicatesPorts(t *testing.T) {
	c := &Instance{Addresses: []string{"HOST:1", "host:2"}, AgentHostTag: "agent_host", EnableICMP: true}
	if err := c.initICMP(); err != nil {
		t.Fatal(err)
	}
	if len(c.icmp.hosts) != 1 {
		t.Fatal("same host not deduplicated")
	}
	c.icmp.store("host", icmpResult{Sent: 3, Received: 3, AvgRTT: time.Millisecond}, nil, time.Unix(1, 0))
	sl := types.NewSampleList()
	c.DisableGNMIUp = true
	c.gatherHealth(sl, time.Unix(1, 0))
	seen := map[string]bool{}
	for _, sample := range sl.PopBackAll() {
		if sample.Metric == "gnmi_icmp_up" {
			seen[sample.Labels["address"]] = true
		}
	}
	if len(seen) != 2 {
		t.Fatalf("lost per-port identity: %v", seen)
	}
}
func TestICMPMonitorConcurrentSnapshots(t *testing.T) {
	m := newICMPMonitor(nil, icmpOptions{Count: 3}, time.Second, 1, nil)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				m.store("a", icmpResult{Sent: 3, Received: 3}, nil, time.Now())
				m.snapshot("a")
			}
		}()
	}
	wg.Wait()
}

type gatedICMP struct {
	started chan string
	release chan struct{}
}

func (p *gatedICMP) Probe(ctx context.Context, host string, o icmpOptions) (icmpResult, error) {
	select {
	case p.started <- host:
	case <-ctx.Done():
		return icmpResult{}, ctx.Err()
	}
	select {
	case <-p.release:
		return icmpResult{Sent: 1, Received: 1}, nil
	case <-ctx.Done():
		return icmpResult{}, ctx.Err()
	}
}
func TestICMPMonitorFairDispatch(t *testing.T) {
	p := &gatedICMP{started: make(chan string), release: make(chan struct{})}
	m := newICMPMonitor([]string{"a", "b", "c"}, icmpOptions{Count: 1}, time.Millisecond, 1, p)
	m.random = func() float64 { return 0 }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { m.run(ctx); close(done) }()
	defer func() { cancel(); <-done }()
	seen := map[string]bool{}
	for i := 0; i < 3; i++ {
		select {
		case host := <-p.started:
			if seen[host] {
				t.Fatalf("host %s repeated while other hosts have no result", host)
			}
			seen[host] = true
		case <-time.After(time.Second):
			t.Fatal("queued host starved")
		}
		p.release <- struct{}{}
	}
}
