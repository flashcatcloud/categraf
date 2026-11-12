package gnmi

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/types"
)

func TestHealthIdentityThroughOutputPipeline(t *testing.T) {
	previous, previousHostInfo := config.Config, config.HostInfo
	config.Config = &config.ConfigType{Global: config.Global{OmitHostname: true}}
	config.HostInfo = &config.HostInfoCache{}
	t.Cleanup(func() { config.Config = previous; config.HostInfo = previousHostInfo })
	for _, tc := range []struct {
		name, addressTag, hostTag string
		labels, globals           map[string]string
	}{
		{name: "defaults", addressTag: "address", hostTag: "icmp_host"},
		{name: "static identities", addressTag: "gnmi_target", hostTag: "gnmi_icmp_host", labels: map[string]string{"address": "configured-endpoint", "icmp_host": "configured-host"}},
		{name: "static host alias", addressTag: "address", hostTag: "gnmi_icmp_host", labels: map[string]string{"icmp-host": "fleet"}},
		{name: "global host alias", addressTag: "address", hostTag: "gnmi_icmp_host", globals: map[string]string{"icmp-host": "fleet"}},
		{name: "fallback aliases", addressTag: "gnmi_target_2", hostTag: "gnmi_icmp_host_2", labels: map[string]string{"address": "fleet", "icmp_host": "fleet", "gnmi-target": "static-endpoint", "gnmi-icmp-host": "static-host"}, globals: map[string]string{"gnmi.target.1": "global-endpoint", "gnmi.icmp.host.1": "global-host"}},
		{name: "deleted primary labels", addressTag: "gnmi_target", hostTag: "gnmi_icmp_host", labels: map[string]string{"address": "-", "icmp_host": "-"}},
		{name: "global defaults", addressTag: "address", hostTag: "icmp_host", globals: map[string]string{"address": "global-endpoint", "icmp_host": "global-host"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config.Config.Global.Labels = tc.globals
			addresses := []string{"192.0.2.1:50051", "192.0.2.2:50051"}
			hosts := []string{"192.0.2.1", "192.0.2.2"}
			plugin := testICMPPlugin(t, addresses)
			plugin.Labels = tc.labels
			instance := plugin.Instances[0]
			if err := instance.initHealth(); err != nil {
				t.Fatal(err)
			}
			now := time.Now()
			session := instance.health[addresses[0]]
			session.observe(session.beginAttempt(), now, true)
			for _, host := range hosts {
				plugin.icmp.store(host, icmpResult{Sent: 3, Received: 3, AvgRTT: time.Millisecond}, nil, now)
			}
			list := types.NewSampleList()
			plugin.gatherHealth(list, now)
			plugin.gatherICMP(list, now)
			samples := plugin.Process(list).PopBackAll()
			if len(samples) != 12 {
				t.Fatalf("expected two endpoint samples and ten ICMP samples, got %d", len(samples))
			}
			seen := make(map[string]bool)
			for _, sample := range samples {
				series := sample.ConvertTimeSeries("")
				if series == nil {
					t.Fatalf("sample could not be exported: %+v", sample)
				}
				labels := make(map[string]string)
				for _, label := range series.Labels {
					if _, exists := labels[label.Name]; exists {
						t.Fatalf("duplicate exported label %q: %v", label.Name, series.Labels)
					}
					labels[label.Name] = label.Value
				}
				tag, targets := tc.addressTag, addresses
				if strings.HasPrefix(sample.Metric, "gnmi_icmp_") {
					tag, targets = tc.hostTag, hosts
				}
				if labels[tag] != targets[0] && labels[tag] != targets[1] {
					t.Fatalf("target identity lost for %s: %v", sample.Metric, labels)
				}
				for key, value := range tc.labels {
					key = tagLabelReplacer.Replace(key)
					if value == "-" {
						if _, exists := labels[key]; exists {
							t.Fatalf("deleted label %q was restored: %v", key, labels)
						}
					} else if labels[key] != value {
						t.Fatalf("static label %q was overwritten: %v", key, labels)
					}
				}
				for key, value := range tc.globals {
					key = tagLabelReplacer.Replace(key)
					// Global defaults never replace an existing identity label.
					if key != tag && labels[key] != value {
						t.Fatalf("global label %q was overwritten: %v", key, labels)
					}
				}
				identity := fmt.Sprint(series.Labels)
				if seen[identity] {
					t.Fatalf("different targets collapsed to the same series: %v", series.Labels)
				}
				seen[identity] = true
			}
		})
	}
}
