package gnmi

import (
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

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
func (s *sessionHealth) snapshot() sessionSnapshot { s.mu.Lock(); defer s.mu.Unlock(); return s.state }
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
	if c.AgentHostTag == "address" {
		return fmt.Errorf("agent_host_tag conflicts with reserved address label")
	}
	for _, key := range []string{"address", c.AgentHostTag} {
		if _, ok := c.GetLabels()[key]; ok {
			return fmt.Errorf("static label %q conflicts with health identity", key)
		}
	}
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
	for k, v := range c.GetLabels() {
		labels[k] = v
	}
	host, _ := healthHost(address)
	labels[c.AgentHostTag] = host
	labels["address"] = address
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
		state := c.health[address].snapshot()
		labels := c.healthLabels(address)
		up := 0
		if state.Up {
			up = 1
		}
		sl.PushSample(inputName, "up", up, labels)
		sl.PushSample(inputName, "last_update_timestamp_seconds", unixSeconds(state.LastUpdate), labels)
		sl.PushSample(inputName, "reconnects_total", state.Reconnects, labels)
	}
}
