package gnmi

import (
	"fmt"
	"log"
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
	if c.GNMIUpInterval < 0 {
		return fmt.Errorf("gnmi_up_interval must not be negative")
	}
	if c.GNMIUpInterval == 0 {
		c.GNMIUpInterval = config.Duration(30 * time.Second)
	}
	c.upReports = make(map[string]upReport)
	warned := false
	icmpWarned := false
	for _, instance := range c.Instances {
		instance.healthDisabled = c.DisableGNMIUp
		if !icmpWarned && (instance.EnableICMP || instance.ICMPInterval != 0 ||
			instance.ICMPTimeout != 0 || instance.ICMPCount != 0 || instance.ICMPConcurrency != 0 ||
			instance.ICMPPacketInterval != 0 || instance.ICMPPrivileged != nil) {
			log.Println("W! gNMI instance-level ICMP settings are ignored; configure them at plugin level")
			icmpWarned = true
		}
		if !warned && (instance.DisableGNMIUp || instance.GNMIUpInterval != 0) {
			log.Println("W! gNMI instance-level disable_gnmi_up and gnmi_up_interval are ignored; configure them at plugin level")
			warned = true
		}
	}
	return c.initICMP()
}

func (c *Instance) initHealth() error {
	if c.healthDisabled {
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
func healthAddressTag(used map[string]bool) string {
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
func unixSeconds(t time.Time) float64 {
	if t.IsZero() {
		return 0
	}
	return float64(t.UnixNano()) / 1e9
}
func (c *GNMI) Gather(sl *types.SampleList) {
	now := time.Now()
	c.gatherHealth(sl, now)
	c.gatherICMP(sl, now)
}

func (c *GNMI) gatherHealth(sl *types.SampleList, now time.Time) {
	if c.DisableGNMIUp {
		return
	}
	states := make(map[string]bool)
	used := make(map[string]bool)
	for key := range c.GetLabels() {
		used[tagLabelReplacer.Replace(key)] = true
	}
	addressTag := healthAddressTag(used)
	for _, instance := range c.Instances {
		if !instance.Initialized() {
			continue
		}
		for address, session := range instance.health {
			states[address] = states[address] || session.isUp()
		}
	}
	for address := range c.upReports {
		if _, exists := states[address]; !exists {
			delete(c.upReports, address)
		}
	}
	for address, isUp := range states {
		last, reported := c.upReports[address]
		if reported && last.up == isUp && now.Sub(last.at) < time.Duration(c.GNMIUpInterval) {
			continue
		}
		c.upReports[address] = upReport{at: now, up: isUp}
		up := 0
		if isUp {
			up = 1
		}
		sl.PushSample(inputName, "up", up, map[string]string{addressTag: address})
	}
}
