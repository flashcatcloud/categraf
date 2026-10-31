package gnmi

import (
	"flashcat.cloud/categraf/types"
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

func TestGatherHealth(t *testing.T) {
	c := &Instance{Addresses: []string{"127.0.0.1:1234"}}
	if err := c.Init(); err != nil {
		t.Fatal(err)
	}
	defer c.Drop()
	sl := types.NewSampleList()
	c.Gather(sl)
	samples := sl.PopBackAll()
	if len(samples) != 3 {
		t.Fatalf("want 3 health metrics, got %d", len(samples))
	}
	want := map[string]bool{"gnmi_up": true, "gnmi_last_update_timestamp_seconds": true, "gnmi_reconnects_total": true}
	for _, v := range samples {
		if !want[v.Metric] {
			t.Fatalf("unexpected health metric: %s", v.Metric)
		}
		delete(want, v.Metric)
		if v.Labels["address"] != "127.0.0.1:1234" || v.Labels["agent_host"] != "127.0.0.1" {
			t.Fatalf("bad labels: %+v", v)
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing health metrics: %v", want)
	}
	c.DisableGNMIUp = true
	c.slist.PushSample("business", "value", 7)
	c.Gather(sl)
	if len(sl.PopBackAll()) != 1 {
		t.Fatal("disable should retain only business sample")
	}
}

func TestHealthConfig(t *testing.T) {
	legacy := &Instance{Addresses: []string{"dns:///host:123"}, DisableGNMIUp: true, AgentHostTag: "address"}
	if err := legacy.Init(); err != nil {
		t.Fatal(err)
	}
	legacy.Drop()
	for _, a := range [][]string{{"bad"}, {"host:1", "host:1"}, {"[fe80::1%eth0]:123"}} {
		c := &Instance{Addresses: a}
		err := c.Init()
		if len(a) == 1 && a[0] != "bad" {
			if err != nil {
				t.Fatal(err)
			}
			c.Drop()
			c.Drop()
		} else if err == nil {
			c.Drop()
			t.Fatalf("accepted invalid addresses: %v", a)
		}
	}
	c := &Instance{Addresses: []string{"host:1"}, AgentHostTag: "address"}
	if c.Init() == nil {
		c.Drop()
		t.Fatal("accepted reserved label")
	}
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
		if bad.Init() == nil {
			bad.Drop()
			t.Fatalf("accepted reserved %s", key)
		}
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
	if s.snapshot().Reconnects != 799 {
		t.Fatal("lost attempts")
	}
}
