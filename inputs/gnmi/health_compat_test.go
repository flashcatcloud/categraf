package gnmi

import (
	"fmt"
	"testing"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/types"
)

func TestHealthEndpointIdentityThroughPipeline(t *testing.T) {
	previous, previousHostInfo := config.Config, config.HostInfo
	config.Config = &config.ConfigType{Global: config.Global{OmitHostname: true}}
	config.HostInfo = &config.HostInfoCache{}
	t.Cleanup(func() { config.Config = previous; config.HostInfo = previousHostInfo })

	for _, tc := range []struct {
		name, hostTag, targetTag string
		labels, globalLabels     map[string]string
	}{
		{name: "default", hostTag: "agent_host", targetTag: "address"},
		{name: "host tag owns address", hostTag: "address", targetTag: "gnmi_target"},
		{name: "static identity labels", hostTag: "agent_host", targetTag: "gnmi_target", labels: map[string]string{"address": "my-device", "agent_host": "my-host"}},
		{name: "normalized fallback collisions", hostTag: "address", targetTag: "gnmi_target_2", labels: map[string]string{"gnmi-target": "existing", "gnmi.target.1": "existing-1"}},
		{name: "host tag owns fallback", hostTag: "gnmi_target", targetTag: "gnmi_target_1", labels: map[string]string{"address": "my-device"}},
		{name: "global normalized fallback", hostTag: "agent_host", targetTag: "gnmi_target_1", labels: map[string]string{"address": "my-device"}, globalLabels: map[string]string{"gnmi-target": "fleet-a"}},
		{name: "global and instance fallback chain", hostTag: "address", targetTag: "gnmi_target_3", labels: map[string]string{"gnmi-target_1": "instance"}, globalLabels: map[string]string{"gnmi-target": "fleet-a", "gnmi.target.2": "fleet-b"}},
	} {
		for _, icmpOnly := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/icmp_only=%t", tc.name, icmpOnly), func(t *testing.T) {
				config.Config.Global.Labels = tc.globalLabels
				c := &Instance{
					Addresses:    []string{"127.0.0.1:50051", "127.0.0.1:50052"},
					AgentHostTag: tc.hostTag, EnableICMP: true, DisableGNMIUp: icmpOnly,
				}
				c.Labels = tc.labels
				if err := c.initHealth(); err != nil {
					t.Fatal(err)
				}
				if err := c.initICMP(); err != nil {
					t.Fatal(err)
				}
				now := time.Now()
				if !icmpOnly {
					h := c.health[c.Addresses[0]]
					h.observe(h.beginAttempt(), now, true)
				}
				c.icmp.store("127.0.0.1", icmpResult{Sent: 3, Received: 3, AvgRTT: time.Millisecond}, nil, now)
				list := types.NewSampleList()
				c.gatherHealth(list, now)
				samples := c.Process(list).PopBackAll()
				want := 12 // Five ICMP metrics and one session metric per endpoint.
				if icmpOnly {
					want = 10
				}
				if len(samples) != want {
					t.Fatalf("samples = %d, want %d", len(samples), want)
				}
				seen := make(map[string]bool)
				for _, sample := range samples {
					series := sample.ConvertTimeSeries("")
					labels := make(map[string]string)
					for _, label := range series.Labels {
						if _, exists := labels[label.Name]; exists {
							t.Fatalf("duplicate exported label %q", label.Name)
						}
						labels[label.Name] = label.Value
					}
					target := labels[tc.targetTag]
					if target != c.Addresses[0] && target != c.Addresses[1] {
						t.Fatalf("lost endpoint identity: %v", labels)
					}
					for key, value := range tc.labels {
						if labels[tagLabelReplacer.Replace(key)] != value {
							t.Fatalf("static label %q was overwritten: %v", key, labels)
						}
					}
					for key, value := range tc.globalLabels {
						if labels[tagLabelReplacer.Replace(key)] != value {
							t.Fatalf("global label %q was overwritten: %v", key, labels)
						}
					}
					identity := fmt.Sprint(series.Labels)
					if seen[identity] {
						t.Fatalf("different endpoints emitted the same series: %v", labels)
					}
					seen[identity] = true
					if sample.Metric == "gnmi_up" {
						wantUp := 0.0
						if target == c.Addresses[0] {
							wantUp = 1
						}
						if series.Samples[0].Value != wantUp {
							t.Fatalf("wrong session state for %s: %v", target, series.Samples)
						}
					}
				}
			})
		}
	}
}

func TestHealthLegacyTargetsAndICMPSkips(t *testing.T) {
	const direct = "127.0.0.1:50051"
	const resolver = "dns:///127.0.0.1:50051"
	c := &Instance{Addresses: []string{direct, direct, resolver}, AgentHostTag: "agent_host", EnableICMP: true}
	if err := c.initHealth(); err != nil {
		t.Fatal(err)
	}
	if err := c.initICMP(); err != nil {
		t.Fatal(err)
	}
	if len(c.icmp.hosts) != 1 || c.icmp.hosts[0] != "127.0.0.1" {
		t.Fatalf("unsupported resolver target must not be probed: %v", c.icmp.hosts)
	}
	now := time.Now()
	c.icmp.store("127.0.0.1", icmpResult{Sent: 3, Received: 3, AvgRTT: time.Millisecond}, nil, now)
	list := types.NewSampleList()
	c.gatherHealth(list, now)
	counts := map[string]int{}
	for _, sample := range list.PopBackAll() {
		target := sample.Labels["address"]
		counts[target]++
		if target == resolver {
			switch sample.Metric {
			case "gnmi_up", "gnmi_icmp_probe_success":
				if sample.Value != 0 {
					t.Fatalf("unexpected successful result for resolver target: %+v", sample)
				}
			case "gnmi_icmp_last_probe_timestamp_seconds":
				if sample.Value != float64(0) {
					t.Fatalf("skipped probe has a completion timestamp: %+v", sample)
				}
			default:
				t.Fatalf("skipped probe emitted a reachability result: %+v", sample)
			}
		}
	}
	if counts[direct] != 6 || counts[resolver] != 3 {
		t.Fatalf("unexpected metrics or duplicate endpoint output: %v", counts)
	}
}
