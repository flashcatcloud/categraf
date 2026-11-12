package gnmi

import (
	"context"
	"flashcat.cloud/categraf/types"
	"log"
	"math/rand"
	"sync"
	"time"
)

type icmpSettings struct {
	options     icmpOptions
	interval    time.Duration
	concurrency int
}
type icmpTarget struct {
	host     string
	settings icmpSettings
}
type icmpMonitor struct {
	mu          sync.Mutex
	keys        []string
	targets     map[string]icmpTarget // Immutable after Start.
	results     map[string]icmpSnapshot
	concurrency int
	probe       icmpProbe
	now         func() time.Time
	random      func() float64
}

func newICMPMonitor(hosts []string, options icmpOptions, interval time.Duration, concurrency int, probe icmpProbe) *icmpMonitor {
	m := &icmpMonitor{targets: make(map[string]icmpTarget), results: make(map[string]icmpSnapshot), concurrency: concurrency, probe: probe, now: time.Now, random: rand.Float64}
	for _, host := range hosts {
		if _, exists := m.targets[host]; exists {
			continue
		}
		m.keys = append(m.keys, host)
		m.targets[host] = icmpTarget{host: host, settings: icmpSettings{options, interval, concurrency}}
	}
	return m
}
func (m *icmpMonitor) store(key string, result icmpResult, err error, now time.Time) {
	target, exists := m.targets[key]
	if !exists {
		return
	}
	valid := err == nil && result.Sent == target.settings.options.Count && result.Received >= 0 && result.Received <= result.Sent && result.AvgRTT >= 0
	m.mu.Lock()
	m.results[key] = icmpSnapshot{Result: result, CompletedAt: now, Success: valid}
	m.mu.Unlock()
}
func (m *icmpMonitor) snapshot(key string) (icmpSnapshot, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	snapshot, exists := m.results[key]
	return snapshot, exists
}

// One worker pool schedules all compatible probe groups. Each group retains
// its instance-level concurrency ceiling, below the plugin-wide cap.
func (m *icmpMonitor) run(ctx context.Context) {
	workers := min(m.concurrency, len(m.keys))
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
				case key, ok := <-jobs:
					if !ok || ctx.Err() != nil {
						return
					}
					target := m.targets[key]
					result, err := m.probe.Probe(ctx, target.host, target.settings.options)
					m.store(key, result, err, m.now())
					if err != nil && ctx.Err() == nil {
						log.Printf("W! gNMI ICMP probe could not complete for %s (check DNS, socket permissions and reachability)", target.host)
					}
					select {
					case done <- key:
					case <-ctx.Done():
						return
					}
				}
			}
		}()
	}
	defer func() { close(jobs); wg.Wait() }()
	due := make(map[string]time.Time)
	busy := make(map[string]bool)
	active := make(map[icmpSettings]int)
	start := m.now()
	for _, key := range m.keys {
		due[key] = start.Add(time.Duration(m.random() * float64(m.targets[key].settings.interval)))
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	next := 0
	for {
		select {
		case <-ctx.Done():
			return
		case key := <-done:
			settings := m.targets[key].settings
			busy[key] = false
			active[settings]--
			due[key] = m.now().Add(time.Duration(float64(settings.interval) * (0.9 + 0.2*m.random())))
		case <-ticker.C:
			now := m.now()
			start := next
			for offset := 0; offset < len(m.keys); offset++ {
				index := (start + offset) % len(m.keys)
				key := m.keys[index]
				settings := m.targets[key].settings
				if busy[key] || now.Before(due[key]) || active[settings] >= settings.concurrency {
					continue
				}
				select {
				case jobs <- key:
					busy[key] = true
					active[settings]++
					next = (index + 1) % len(m.keys)
				default:
				}
			}
		}
	}
}

// Keep the #1510 output contract: each instance emits its latest snapshot on
// every Gather, even though the network work is shared across instances.
func (c *Instance) gatherICMP(sl *types.SampleList, now time.Time) {
	if !c.EnableICMP {
		return
	}
	seen := make(map[string]bool)
	for _, address := range c.Addresses {
		if seen[address] {
			continue
		}
		seen[address] = true
		var snapshot icmpSnapshot
		var exists bool
		if key, ok := c.icmpTargets[address]; ok && c.icmp != nil {
			snapshot, exists = c.icmp.snapshot(key)
		}
		labels := c.healthLabels(address)
		success := 0
		if exists && snapshot.Success && now.Sub(snapshot.CompletedAt) <= 2*time.Duration(c.ICMPInterval)+time.Duration(c.ICMPTimeout) {
			success = 1
		}
		sl.PushSample(inputName, "icmp_probe_success", success, labels)
		sl.PushSample(inputName, "icmp_last_probe_timestamp_seconds", unixSeconds(snapshot.CompletedAt), labels)
		if success == 0 {
			continue
		}
		up := 0
		if snapshot.Result.Received > 0 {
			up = 1
			sl.PushSample(inputName, "icmp_rtt_seconds", snapshot.Result.AvgRTT.Seconds(), labels)
		}
		sl.PushSample(inputName, "icmp_up", up, labels)
		sl.PushSample(inputName, "icmp_packet_loss_ratio", float64(snapshot.Result.Sent-snapshot.Result.Received)/float64(snapshot.Result.Sent), labels)
	}
}
