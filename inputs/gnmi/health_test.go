package gnmi

import (
	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/pkg/cfg"
	"flashcat.cloud/categraf/types"
	"github.com/BurntSushi/toml"
	"sync"
	"testing"
	"time"
)

func TestSessionHealth(t *testing.T) {
	var s sessionHealth
	if s.snapshot().Up {
		t.Fatal("initially up")
	}
	g := s.beginAttempt()
	now := time.Unix(100, 0)
	s.observe(g, now, true)
	got := s.snapshot()
	if !got.Up || got.Reconnects != 0 || !got.LastUpdate.Equal(now) {
		t.Fatalf("first response: %+v", got)
	}
	s.finish(g) // A single subscription finishes its old attempt before retrying.
	g2 := s.beginAttempt()
	s.observe(g2, now.Add(time.Second), false)
	s.finish(g)
	s.observe(g, now.Add(10*time.Second), true)
	got = s.snapshot()
	if !got.Up || got.Reconnects != 1 || !got.LastUpdate.Equal(now) {
		t.Fatalf("stale generation changed snapshot: %+v", got)
	}
	s.finish(g2)
	if s.snapshot().Up || !s.snapshot().LastUpdate.Equal(now) {
		t.Fatal("finish must retain timestamps and clear up")
	}
}

func TestGatherHealthAcrossFiles(t *testing.T) {
	var plugin GNMI
	err := cfg.LoadConfigs([]cfg.ConfigWithFormat{
		{Format: cfg.TomlFormat, Config: "interval = \"1s\"\ngnmi_up_interval = \"5s\""},
		{Format: cfg.TomlFormat, Config: "[[instances]]\naddresses = [\"host:123\"]\ndisable_gnmi_up = true\ngnmi_up_interval = \"1s\"\nlabels = { address = \"instance-one\" }"},
		{Format: cfg.TomlFormat, Config: "[[instances]]\naddresses = [\"host:123\", \"host:124\"]\n[[instances]]\naddresses = [\"host:123\"]"},
	}, &plugin)
	if err != nil {
		t.Fatal(err)
	}
	if err := plugin.Init(); err != nil {
		t.Fatal(err)
	}
	for _, instance := range plugin.Instances {
		if err := instance.initHealth(); err != nil {
			t.Fatal(err)
		}
		instance.SetInitialized()
	}
	if plugin.Interval != config.Duration(time.Second) || plugin.GNMIUpInterval != config.Duration(5*time.Second) {
		t.Fatal("top-level intervals were not loaded")
	}
	up := plugin.Instances[0].health["host:123"]
	up.observe(up.beginAttempt(), time.Now(), false)
	sl := types.NewSampleList()
	plugin.Gather(sl)
	samples := sl.PopBackAll()
	if len(samples) != 2 {
		t.Fatalf("expected one sample per endpoint, got %v", samples)
	}
	want := map[string]int{"host:123": 1, "host:124": 0}
	for _, sample := range samples {
		address := sample.Labels["address"]
		if value, ok := want[address]; !ok || sample.Value != value || sample.Metric != "gnmi_up" {
			t.Fatalf("unexpected sample: %+v", sample)
		}
		delete(want, address)
	}
	plugin.DisableGNMIUp = true
	plugin.Gather(sl)
	if len(sl.PopBackAll()) != 0 {
		t.Fatal("plugin disable must suppress gnmi_up")
	}
}

func TestSessionHealthOverlappingAttempts(t *testing.T) {
	var s sessionHealth
	now := time.Unix(100, 0)
	first := s.beginAttempt()
	s.observe(first, now, true)
	failed := s.beginAttempt()
	if !s.snapshot().Up {
		t.Fatal("a new attempt hid a healthy session")
	}
	s.finish(failed)
	if !s.snapshot().Up {
		t.Fatal("a failed attempt hid a healthy session")
	}
	second := s.beginAttempt()
	s.observe(second, now.Add(time.Second), true)
	s.finish(first)
	s.finish(first) // Finishing twice must not clear another session's state.
	s.observe(first, now.Add(10*time.Second), true)
	got := s.snapshot()
	if !got.Up || !got.LastUpdate.Equal(now.Add(time.Second)) {
		t.Fatalf("closing an older session changed the remaining session: %+v", got)
	}
	s.finish(second)
	s.observe(second, now.Add(20*time.Second), true)
	if s.snapshot().Up {
		t.Fatal("a completed attempt resurrected a closed session")
	}
}

func TestHealthConfig(t *testing.T) {
	legacy := &Instance{Addresses: []string{"dns:///host:123"}, healthDisabled: true, AgentHostTag: "address"}
	if err := legacy.Init(); err != nil {
		t.Fatal(err)
	}
	legacy.Drop()
	for _, a := range [][]string{{"dns:///host:123"}, {"bad"}, {"host:1", "host:1"}, {"[fe80::1%eth0]:123"}} {
		c := &Instance{Addresses: a}
		if err := c.Init(); err != nil {
			t.Fatalf("health must not reject legacy targets %v: %v", a, err)
		}
		c.Drop()
		c.Drop()
	}
	c := &Instance{Addresses: []string{"host:1"}, AgentHostTag: "address"}
	if err := c.Init(); err != nil {
		t.Fatal(err)
	}
	c.Drop()
}

func TestHealthLabels(t *testing.T) {
	c := &Instance{Addresses: []string{"[fe80::1%eth0]:123"}, AgentHostTag: "device"}
	c.Labels = map[string]string{"site": "lab"}
	if err := c.Init(); err != nil {
		t.Fatal(err)
	}
	defer c.Drop()
	sl := types.NewSampleList()
	c.Gather(sl)
	for _, s := range sl.PopBackAll() {
		if s.Labels["device"] != "fe80::1%eth0" || s.Labels["site"] != "lab" || s.Labels["address"] != "[fe80::1%eth0]:123" {
			t.Fatalf("labels: %v", s.Labels)
		}
	}
	for _, key := range []string{"address", "agent_host"} {
		bad := &Instance{Addresses: []string{"host:123"}}
		bad.Labels = map[string]string{key: "override"}
		if err := bad.Init(); err != nil {
			t.Fatal(err)
		}
		bad.Drop()
	}
}

func TestSessionHealthConcurrent(t *testing.T) {
	var s sessionHealth
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < 100; n++ {
				g := s.beginAttempt()
				s.observe(g, time.Unix(10, 0), true)
				s.snapshot()
				s.finish(g)
			}
		}()
	}
	wg.Wait()
	if got := s.snapshot(); got.Reconnects != 799 || got.Up {
		t.Fatalf("lost attempts or retained completed sessions: %+v", got)
	}
}

func TestGNMIUpReporting(t *testing.T) {
	c := &Instance{Addresses: []string{"host:123", "other:123"}, AgentHostTag: "agent_host"}
	if err := c.initHealth(); err != nil {
		t.Fatal(err)
	}
	c.SetInitialized()
	peer := &Instance{Addresses: []string{"host:123"}}
	if err := peer.initHealth(); err != nil {
		t.Fatal(err)
	}
	peer.SetInitialized()
	plugin := &GNMI{Instances: []*Instance{c, peer}}
	if err := plugin.Init(); err != nil {
		t.Fatal(err)
	}
	now := time.Unix(100, 0)
	check := func(offset time.Duration, want map[string]int) {
		t.Helper()
		sl := types.NewSampleList()
		plugin.gatherHealth(sl, now.Add(offset))
		samples := sl.PopBackAll()
		if len(samples) != len(want) {
			t.Fatalf("offset %v: want %v, got %v", offset, want, samples)
		}
		for _, s := range samples {
			v, ok := want[s.Labels["address"]]
			if !ok || s.Metric != "gnmi_up" || s.Value != v {
				t.Fatalf("unexpected sample: %+v", s)
			}
			delete(want, s.Labels["address"])
		}
	}
	check(0, map[string]int{"host:123": 0, "other:123": 0})
	check(time.Second, map[string]int{})
	g := c.health["host:123"].beginAttempt()
	c.health["host:123"].observe(g, now, true)
	check(2*time.Second, map[string]int{"host:123": 1})
	check(29*time.Second, map[string]int{})
	check(30*time.Second, map[string]int{"other:123": 0})
	check(32*time.Second, map[string]int{"host:123": 1})
	other := peer.health["host:123"]
	otherGeneration := other.beginAttempt()
	other.observe(otherGeneration, now, false)
	c.health["host:123"].finish(g)
	check(33*time.Second, map[string]int{})
	other.finish(otherGeneration)
	check(33*time.Second, map[string]int{"host:123": 0})
	check(34*time.Second, map[string]int{})
	check(63*time.Second, map[string]int{"host:123": 0, "other:123": 0})
	plugin.Instances = []*Instance{peer}
	check(93*time.Second, map[string]int{"host:123": 0})
	plugin.Instances = []*Instance{c, peer}
	check(94*time.Second, map[string]int{"other:123": 0})
}
func TestGNMIUpIntervalConfig(t *testing.T) {
	for _, tc := range []struct {
		interval config.Duration
		want     time.Duration
		invalid  bool
	}{{0, 30 * time.Second, false}, {config.Duration(5 * time.Second), 5 * time.Second, false}, {-1, 0, true}} {
		c := &GNMI{GNMIUpInterval: tc.interval}
		err := c.Init()
		if (err != nil) != tc.invalid {
			t.Fatalf("interval %v: %v", tc.interval, err)
		}
		if err == nil && time.Duration(c.GNMIUpInterval) != tc.want {
			t.Fatalf("interval default: %v", c.GNMIUpInterval)
		}
	}
}

func (s *sessionHealth) snapshot() sessionSnapshot { s.mu.Lock(); defer s.mu.Unlock(); return s.state }

func TestHealthPreservesLegacyStaticLabels(t *testing.T) {
	for _, icmp := range []bool{false, true} {
		c := &Instance{Addresses: []string{"127.0.0.1:50051"}, EnableICMP: icmp}
		c.Labels = map[string]string{"address": "my-device", "agent_host": "custom-host"}
		if err := c.Init(); err != nil {
			t.Fatalf("existing labels must not stop telemetry (icmp=%v): %v", icmp, err)
		}
		sl := types.NewSampleList()
		c.Gather(sl)
		c.Drop()
		for _, sample := range sl.PopBackAll() {
			if sample.Labels["address"] != "my-device" || sample.Labels["agent_host"] != "custom-host" {
				t.Fatalf("static labels overwritten: %v", sample.Labels)
			}
		}
		if c.Labels["address"] != "my-device" {
			t.Fatal("instance labels mutated")
		}
	}
}

func TestHealthReviewerConfiguration(t *testing.T) {
	var plugin GNMI
	_, err := toml.Decode(`interval = "1s"
[[instances]]
addresses = ["127.0.0.1:50051"]
labels = { address = "my-device" }
`, &plugin)
	if err != nil {
		t.Fatal(err)
	}
	c := plugin.Instances[0]
	if err := c.Init(); err != nil {
		t.Fatal(err)
	}
	defer c.Drop()
	sl := types.NewSampleList()
	c.Gather(sl)
	samples := sl.PopBackAll()
	if len(samples) != 0 {
		t.Fatalf("legacy config output: %v", samples)
	}
}
