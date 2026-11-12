package gnmi

import (
	"context"
	"log"
	"math/rand"
	"sync"
	"time"

	"flashcat.cloud/categraf/types"
)

type icmpMonitor struct {
	mu          sync.Mutex
	hosts       []string
	results     map[string]icmpSnapshot
	opts        icmpOptions
	interval    time.Duration
	concurrency int
	probe       icmpProbe
	now         func() time.Time
	random      func() float64
}

func newICMPMonitor(hosts []string, o icmpOptions, interval time.Duration, concurrency int, probe icmpProbe) *icmpMonitor {
	return &icmpMonitor{hosts: hosts, results: make(map[string]icmpSnapshot), opts: o, interval: interval, concurrency: concurrency, probe: probe, now: time.Now, random: rand.Float64}
}
func (m *icmpMonitor) store(host string, r icmpResult, err error, now time.Time) {
	valid := err == nil && r.Sent == m.opts.Count && r.Received >= 0 && r.Received <= r.Sent && r.AvgRTT >= 0
	m.mu.Lock()
	m.results[host] = icmpSnapshot{Round: m.results[host].Round + 1, Result: r, CompletedAt: now, Success: valid}
	m.mu.Unlock()
}
func (m *icmpMonitor) snapshot(host string) (icmpSnapshot, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.results[host]
	return s, ok
}
func (m *icmpMonitor) run(ctx context.Context) {
	workers := min(m.concurrency, len(m.hosts))
	if workers == 0 {
		return
	}
	jobs := make(chan string)
	done := make(chan string, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case host, ok := <-jobs:
					if !ok {
						return
					}
					if ctx.Err() != nil {
						return
					}
					result, err := m.probe.Probe(ctx, host, m.opts)
					m.store(host, result, err, m.now())
					if err != nil && ctx.Err() == nil {
						log.Printf("W! gNMI ICMP probe could not complete for %s (check DNS, socket permissions and reachability)", host)
					}
					select {
					case done <- host:
					case <-ctx.Done():
						return
					}
				}
			}
		}()
	}
	defer func() { close(jobs); wg.Wait() }()
	due := map[string]time.Time{}
	busy := map[string]bool{}
	start := m.now()
	for _, host := range m.hosts {
		due[host] = start.Add(time.Duration(m.random() * float64(m.interval)))
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	next := 0
	for {
		select {
		case <-ctx.Done():
			return
		case host := <-done:
			busy[host] = false
			due[host] = m.now().Add(time.Duration(float64(m.interval) * (0.9 + 0.2*m.random())))
		case <-ticker.C:
			now := m.now()
			start := next
			for offset := 0; offset < len(m.hosts); offset++ {
				index := (start + offset) % len(m.hosts)
				host := m.hosts[index]
				if busy[host] || now.Before(due[host]) {
					continue
				}
				select {
				case jobs <- host:
					busy[host] = true
					next = (index + 1) % len(m.hosts)
				default:
				}
			}
		}
	}
}

type icmpStatus uint8

const (
	icmpFailed icmpStatus = iota
	icmpUnreachable
	icmpReachable
	icmpExpired
)

type icmpReport struct {
	round  uint64
	at     time.Time
	status icmpStatus
}

func (c *GNMI) gatherICMP(sl *types.SampleList, now time.Time) {
	if c.icmp == nil {
		return
	}
	maxAge := 2*time.Duration(c.ICMPInterval) + time.Duration(c.ICMPTimeout)
	for _, host := range c.icmp.hosts {
		snapshot, ok := c.icmp.snapshot(host)
		if !ok {
			continue
		}
		status := icmpFailed
		switch {
		case now.Sub(snapshot.CompletedAt) > maxAge:
			status = icmpExpired
		case snapshot.Success && snapshot.Result.Received == 0:
			status = icmpUnreachable
		case snapshot.Success:
			status = icmpReachable
		}
		last, reported := c.icmpReports[host]
		if reported && last.status == status &&
			(last.round == snapshot.Round || status == icmpExpired || now.Sub(last.at) < time.Duration(c.ICMPReportInterval)) {
			continue
		}
		c.icmpReports[host] = icmpReport{round: snapshot.Round, at: now, status: status}
		c.emitICMP(sl, host, snapshot, status)
	}
}

func (c *GNMI) emitICMP(sl *types.SampleList, host string, snapshot icmpSnapshot, status icmpStatus) {
	hostTag := c.healthIdentityTag("icmp_host", "gnmi_icmp_host")
	labels := map[string]string{hostTag: host}
	success := 0
	if status == icmpReachable || status == icmpUnreachable {
		success = 1
	}
	sl.PushSample(inputName, "icmp_probe_success", success, labels)
	sl.PushSample(inputName, "icmp_last_probe_timestamp_seconds", unixSeconds(snapshot.CompletedAt), labels)
	if success == 0 {
		return
	}
	up := 0
	if status == icmpReachable {
		up = 1
		sl.PushSample(inputName, "icmp_rtt_seconds", snapshot.Result.AvgRTT.Seconds(), labels)
	}
	sl.PushSample(inputName, "icmp_up", up, labels)
	sl.PushSample(inputName, "icmp_packet_loss_ratio", float64(snapshot.Result.Sent-snapshot.Result.Received)/float64(snapshot.Result.Sent), labels)
}
