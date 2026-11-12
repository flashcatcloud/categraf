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

type upReport struct {
	at time.Time
	up bool
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

func (s *sessionHealth) isUp() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state.Up
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
func (c *GNMI) Init() error {
	if c.EnableEndpointUp {
		if c.EndpointUpInterval < 0 {
			return fmt.Errorf("endpoint_up_interval must not be negative")
		}
		if c.EndpointUpInterval == 0 {
			c.EndpointUpInterval = config.Duration(30 * time.Second)
		}
	}
	if c.ICMPMaxConcurrency < 0 {
		return fmt.Errorf("icmp_max_concurrency must not be negative")
	}
	if c.ICMPMaxConcurrency == 0 {
		c.ICMPMaxConcurrency = 10
	}
	c.endpointReports = make(map[string]upReport)
	return nil
}

func (c *Instance) initHealth() error {
	if c.GNMIUpInterval < 0 {
		return fmt.Errorf("gnmi_up_interval must not be negative")
	}
	if c.GNMIUpInterval == 0 {
		c.GNMIUpInterval = config.Duration(30 * time.Second)
	}
	// Keep observing even when instance reporting is disabled: the optional
	// endpoint aggregate has an independent switch and reporting cadence.
	c.health = make(map[string]*sessionHealth, len(c.Addresses))
	for _, address := range c.Addresses {
		c.health[address] = &sessionHealth{}
	}
	return nil
}

func healthIdentityTag(primary, fallback string, labels map[string]string, reserved ...string) string {
	used := make(map[string]bool)
	for key := range labels {
		used[tagLabelReplacer.Replace(key)] = true
	}
	for _, key := range reserved {
		used[tagLabelReplacer.Replace(key)] = true
	}
	if config.Config != nil {
		for key := range config.Config.Global.Labels {
			// Exact global defaults cannot replace an emitted label, but aliases
			// would introduce duplicate names after output normalization.
			if key != primary {
				used[tagLabelReplacer.Replace(key)] = true
			}
		}
	}
	if !used[primary] {
		return primary
	}
	key := fallback
	for suffix := 1; used[key]; suffix++ {
		key = fallback + "_" + strconv.Itoa(suffix)
	}
	return key
}

func (c *Instance) healthLabels(address string) map[string]string {
	host, err := healthHost(address)
	if err != nil {
		host = address
	}
	labels := map[string]string{"address": address, c.AgentHostTag: host}
	for key, value := range c.GetLabels() {
		labels[key] = value
	}
	tag := healthIdentityTag("address", "gnmi_target", c.GetLabels(), c.AgentHostTag)
	labels[tag] = address
	return labels
}

func unixSeconds(t time.Time) float64 {
	if t.IsZero() {
		return 0
	}
	return float64(t.UnixNano()) / 1e9
}

func (s *sessionHealth) report(now time.Time, interval time.Duration) (bool, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.reported && s.lastUp == s.state.Up && now.Sub(s.lastReport) < interval {
		return false, false
	}
	s.reported, s.lastUp, s.lastReport = true, s.state.Up, now
	return s.state.Up, true
}

func (c *Instance) gatherHealth(sl *types.SampleList, now time.Time) {
	c.gatherICMP(sl, now)
	if c.DisableGNMIUp {
		return
	}
	for address, session := range c.health {
		up, emit := session.report(now, time.Duration(c.GNMIUpInterval))
		if !emit {
			continue
		}
		value := 0
		if up {
			value = 1
		}
		sl.PushSample(inputName, "up", value, c.healthLabels(address))
	}
}

func (c *GNMI) Gather(sl *types.SampleList) { c.gatherEndpointHealth(sl, time.Now()) }

func (c *GNMI) gatherEndpointHealth(sl *types.SampleList, now time.Time) {
	if !c.EnableEndpointUp {
		return
	}
	states := make(map[string]bool)
	for _, instance := range c.Instances {
		if !instance.Initialized() {
			continue
		}
		for address, session := range instance.health {
			states[address] = states[address] || session.isUp()
		}
	}
	for address := range c.endpointReports {
		if _, exists := states[address]; !exists {
			delete(c.endpointReports, address)
		}
	}
	tag := healthIdentityTag("address", "gnmi_target", c.GetLabels())
	for address, up := range states {
		last, reported := c.endpointReports[address]
		if reported && last.up == up && now.Sub(last.at) < time.Duration(c.EndpointUpInterval) {
			continue
		}
		c.endpointReports[address] = upReport{at: now, up: up}
		value := 0
		if up {
			value = 1
		}
		sl.PushSample(inputName, "endpoint_up", value, map[string]string{tag: address})
	}
}
