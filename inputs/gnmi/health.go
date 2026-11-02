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
	s.state.Up = false
	return s.generation
}
func (s *sessionHealth) observe(g uint64, now time.Time, data bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if g != s.generation {
		return
	}
	s.state.Up = true
	if data {
		s.state.LastUpdate = now
	}
}
func (s *sessionHealth) finish(g uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if g == s.generation {
		s.state.Up = false
	}
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
	if err := c.validateHealthIdentity(); err != nil {
		return err
	}
	c.health = make(map[string]*sessionHealth, len(c.Addresses))
	for _, a := range c.Addresses {
		c.health[a] = &sessionHealth{}
	}
	return nil
}
func (c *Instance) validateHealthIdentity() error {
	seen := map[string]bool{}
	for _, a := range c.Addresses {
		if _, err := healthHost(a); err != nil {
			return err
		}
		if seen[a] {
			return fmt.Errorf("duplicate gNMI address %q", a)
		}
		seen[a] = true
	}
	return nil
}
func (c *Instance) healthLabels(address string) map[string]string {
	labels := make(map[string]string)
	host, _ := healthHost(address)
	labels["address"] = address
	labels[c.AgentHostTag] = host
	// Match the output pipeline: existing static labels take precedence.
	for k, v := range c.GetLabels() {
		labels[k] = v
	}
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
	for _, address := range c.Addresses {
		isUp, emit := c.health[address].report(now, time.Duration(c.GNMIUpInterval))
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
