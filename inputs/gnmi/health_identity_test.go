package gnmi

import (
	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/types"
	"fmt"
	"testing"
	"time"
)

func TestInstanceAndEndpointIdentityThroughPipeline(t *testing.T) {
	previous, previousHost := config.Config, config.HostInfo
	config.Config = &config.ConfigType{Global: config.Global{OmitHostname: true}}
	config.HostInfo = &config.HostInfoCache{}
	t.Cleanup(func() { config.Config = previous; config.HostInfo = previousHost })
	for _, tc := range []struct {
		name, tag       string
		labels, globals map[string]string
	}{
		{name: "defaults", tag: "address"},
		{name: "static identity", tag: "gnmi_target", labels: map[string]string{"address": "fleet"}},
		{name: "fallback aliases", tag: "gnmi_target_2", labels: map[string]string{"address": "fleet", "gnmi-target": "configured"}, globals: map[string]string{"gnmi.target.1": "global"}},
		{name: "deleted primary", tag: "gnmi_target", labels: map[string]string{"address": "-"}},
		{name: "legacy ICMP alias", tag: "address", globals: map[string]string{"icmp-host": "global"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config.Config.Global.Labels = tc.globals
			instance := &Instance{Addresses: []string{"192.0.2.1:50051", "192.0.2.2:50051"}, AgentHostTag: "ident", EnableICMP: true}
			instance.Labels = tc.labels
			if err := instance.initHealth(); err != nil {
				t.Fatal(err)
			}
			if err := instance.initICMP(); err != nil {
				t.Fatal(err)
			}
			instance.SetInitialized()
			plugin := &GNMI{Instances: []*Instance{instance}, EnableEndpointUp: true}
			plugin.Labels = tc.labels
			if err := plugin.Init(); err != nil {
				t.Fatal(err)
			}
			now := time.Now()
			session := instance.health[instance.Addresses[0]]
			session.observe(session.beginAttempt(), now, true)
			for _, key := range instance.icmp.keys {
				instance.icmp.store(key, icmpResult{Sent: 3, Received: 3, AvgRTT: time.Millisecond}, nil, now)
			}
			local := types.NewSampleList()
			instance.gatherHealth(local, now)
			root := types.NewSampleList()
			plugin.gatherEndpointHealth(root, now)
			samples := instance.Process(local).PopBackAll()
			samples = append(samples, plugin.Process(root).PopBackAll()...)
			if len(samples) != 14 {
				t.Fatalf("samples=%d, want 12 instance and two endpoint metrics", len(samples))
			}
			seen := map[string]bool{}
			for _, sample := range samples {
				ts := sample.ConvertTimeSeries("")
				labels := map[string]string{}
				for _, label := range ts.Labels {
					if _, exists := labels[label.Name]; exists {
						t.Fatalf("duplicate label %s: %v", label.Name, ts.Labels)
					}
					labels[label.Name] = label.Value
				}
				address := labels[tc.tag]
				if address != instance.Addresses[0] && address != instance.Addresses[1] {
					t.Fatalf("target identity lost: %v", ts.Labels)
				}
				for key, value := range tc.labels {
					key = tagLabelReplacer.Replace(key)
					if value == "-" {
						if _, exists := labels[key]; exists {
							t.Fatalf("deleted label %s restored", key)
						}
					} else if labels[key] != value {
						t.Fatalf("configured label %s lost: %v", key, labels)
					}
				}
				for key, value := range tc.globals {
					if labels[tagLabelReplacer.Replace(key)] != value {
						t.Fatalf("global label %s lost", key)
					}
				}
				if sample.Metric != "gnmi_endpoint_up" && labels["ident"] != "192.0.2.1" && labels["ident"] != "192.0.2.2" {
					t.Fatalf("legacy host label lost: %v", labels)
				}
				identity := fmt.Sprint(ts.Labels)
				if seen[identity] {
					t.Fatalf("targets collapsed into one series: %v", ts.Labels)
				}
				seen[identity] = true
			}
		})
	}
}
