package gnmi

import (
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/types"
)

type sessionSnapshot struct {
	Up         bool
	LastUpdate time.Time
	Reconnects uint64
}
type sessionHealth struct {
	mu         sync.Mutex
	generation uint64
	active     map[uint64]bool
	upSessions int
	state      sessionSnapshot
	reported   bool
	lastReport time.Time
	lastUp     bool
}

func (s *sessionHealth) beginAttempt() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.generation > 0 {
		s.state.Reconnects++
	}
	s.generation++
	if s.active == nil {
		s.active = make(map[uint64]bool)
	}
	// Repeated targets can have concurrent subscriptions. A new attempt must
	// not invalidate another worker's confirmed, still-active session.
	s.active[s.generation] = false
	return s.generation
}
func (s *sessionHealth) observe(g uint64, now time.Time, data bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	up, active := s.active[g]
	if !active {
		return
	}
	if !up {
		s.active[g] = true
		s.upSessions++
	}
	s.state.Up = true
	if data {
		s.state.LastUpdate = now
	}
}
func (s *sessionHealth) finish(g uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	up, active := s.active[g]
	if !active {
		return
	}
	delete(s.active, g)
	if up {
		s.upSessions--
	}
	s.state.Up = s.upSessions > 0
}

// report atomically reserves an emission; each device has its own cadence.
func (s *sessionHealth) report(now time.Time, interval time.Duration) (bool, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.reported && s.lastUp == s.state.Up && now.Sub(s.lastReport) < interval {
		return false, false
	}
	s.reported, s.lastUp, s.lastReport = true, s.state.Up, now
	return s.state.Up, true
}
func healthHost(address string) (string, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil || host == "" {
		return "", fmt.Errorf("gNMI address must be host:port: %q", address)
	}
	n, e := strconv.Atoi(port)
	if e != nil || n < 1 || n > 65535 {
		return "", fmt.Errorf("invalid gNMI port in %q", address)
	}
	return host, nil
}
func (c *Instance) initHealth() error {
	if c.GNMIUpInterval < 0 {
		return fmt.Errorf("gnmi_up_interval must not be negative")
	}
	if c.GNMIUpInterval == 0 {
		c.GNMIUpInterval = config.Duration(30 * time.Second)
	}
	if c.DisableGNMIUp {
		return nil
	}
	// Session health observes gRPC sessions, so it must accept every target
	// supported by the existing dialer, including resolver URIs.
	c.health = make(map[string]*sessionHealth, len(c.Addresses))
	for _, a := range c.Addresses {
		c.health[a] = &sessionHealth{}
	}
	return nil
}

// Keep the usual address label unless existing configuration owns that name.
// Fallback names also avoid collisions after output label normalization.
func (c *Instance) healthAddressTag() string {
	used := map[string]bool{tagLabelReplacer.Replace(c.AgentHostTag): true}
	for key := range c.GetLabels() {
		used[tagLabelReplacer.Replace(key)] = true
	}
	if !used["address"] {
		return "address"
	}
	// The pipeline appends global defaults before normalizing label names.
	// Avoid aliases such as global "gnmi-target" alongside "gnmi_target".
	if config.Config != nil {
		for key := range config.Config.Global.Labels {
			used[tagLabelReplacer.Replace(key)] = true
		}
	}
	key := "gnmi_target"
	for suffix := 1; used[key]; suffix++ {
		key = "gnmi_target_" + strconv.Itoa(suffix)
	}
	return key
}
func (c *Instance) healthLabels(address string) map[string]string {
	labels := make(map[string]string)
	host, err := healthHost(address)
	if err != nil {
		// A resolver URI need not identify a single host. Retain the target
		// rather than rejecting it or emitting an empty host label.
		host = address
	}
	labels["address"] = address
	labels[c.AgentHostTag] = host
	// Match the output pipeline: existing static labels take precedence.
	for k, v := range c.GetLabels() {
		labels[k] = v
	}
	labels[c.healthAddressTag()] = address
	return labels
}
func unixSeconds(t time.Time) float64 {
	if t.IsZero() {
		return 0
	}
	return float64(t.UnixNano()) / 1e9
}
func (c *Instance) gatherHealth(sl *types.SampleList, now time.Time) {
	c.gatherICMP(sl, now)
	if c.DisableGNMIUp {
		return
	}
	for address, health := range c.health {
		isUp, emit := health.report(now, time.Duration(c.GNMIUpInterval))
		if !emit {
			continue
		}
		labels := c.healthLabels(address)
		up := 0
		if isUp {
			up = 1
		}
		sl.PushSample(inputName, "up", up, labels)
	}
}
