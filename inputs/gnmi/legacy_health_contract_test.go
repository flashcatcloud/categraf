package gnmi

import (
	"fmt"
	"testing"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
	"github.com/BurntSushi/toml"
)

// This fixture is also runnable against the merged #1510 implementation.
func TestLegacyHealthConfigurationContract(t *testing.T) {
	previous, previousHost := config.Config, config.HostInfo
	config.Config = &config.ConfigType{Global: config.Global{OmitHostname: true, Labels: map[string]string{"cluster": "lab"}}}
	config.HostInfo = &config.HostInfoCache{}
	t.Cleanup(func() { config.Config = previous; config.HostInfo = previousHost })
	for _, disabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("disable_gnmi_up=%t", disabled), func(t *testing.T) {
			var plugin GNMI
			_, err := toml.Decode(fmt.Sprintf(`interval="1s"
[[instances]]
addresses=["127.0.0.1:50051"]
agent_host_tag="ident"
labels={role="cpu"}
disable_gnmi_up=%t
gnmi_up_interval="7s"
enable_icmp=true
icmp_interval="20s"
icmp_timeout="2s"
icmp_count=2
icmp_packet_interval="100ms"
icmp_concurrency=2
icmp_privileged=false
[[instances.relabel_configs]]
source_labels=["role"]
target_label="purpose"
`, disabled), &plugin)
			if err != nil {
				t.Fatal(err)
			}
			if err := inputs.MayInit(&plugin); err != nil {
				t.Fatal(err)
			}
			instance := plugin.Instances[0]
			if err := instance.InitInternalConfig(); err != nil {
				t.Fatal(err)
			}
			if err := instance.initHealth(); err != nil {
				t.Fatal(err)
			}
			if err := instance.initICMP(); err != nil {
				t.Fatal(err)
			}
			if !instance.EnableICMP || instance.ICMPCount != 2 || instance.ICMPConcurrency != 2 || instance.ICMPPrivileged == nil || *instance.ICMPPrivileged || time.Duration(instance.GNMIUpInterval) != 7*time.Second || time.Duration(instance.ICMPInterval) != 20*time.Second || time.Duration(instance.ICMPTimeout) != 2*time.Second || time.Duration(instance.ICMPPacketInterval) != 100*time.Millisecond {
				t.Fatalf("legacy options changed: %+v", instance)
			}
			now := time.Unix(100, 0)
			if !disabled {
				s := instance.health[instance.Addresses[0]]
				s.observe(s.beginAttempt(), now, true)
			}
			instance.icmp.store("127.0.0.1", icmpResult{Sent: 2, Received: 1, AvgRTT: 2 * time.Millisecond}, nil, now)
			expected := map[string]float64{"gnmi_icmp_probe_success": 1, "gnmi_icmp_up": 1, "gnmi_icmp_packet_loss_ratio": 0.5, "gnmi_icmp_rtt_seconds": 0.002, "gnmi_icmp_last_probe_timestamp_seconds": 100}
			if !disabled {
				expected["gnmi_up"] = 1
			}
			list := types.NewSampleList()
			instance.gatherHealth(list, now)
			samples := instance.Process(list).PopBackAll()
			if len(samples) != len(expected) {
				t.Fatalf("metrics=%d, want %d", len(samples), len(expected))
			}
			for _, sample := range samples {
				ts := sample.ConvertTimeSeries("")
				value, exists := expected[sample.Metric]
				if !exists || ts.Samples[0].Value != value {
					t.Fatalf("unexpected metric %+v", sample)
				}
				delete(expected, sample.Metric)
				wantLabels := map[string]string{"__name__": sample.Metric, "address": "127.0.0.1:50051", "ident": "127.0.0.1", "role": "cpu", "purpose": "cpu", "cluster": "lab"}
				if len(ts.Labels) != len(wantLabels) {
					t.Fatalf("labels changed: %v", ts.Labels)
				}
				for _, label := range ts.Labels {
					if wantLabels[label.Name] != label.Value {
						t.Fatalf("label changed: %v", ts.Labels)
					}
				}
			}
			for _, seconds := range []int{1, 7} {
				instance.gatherHealth(list, now.Add(time.Duration(seconds)*time.Second))
				want := 5
				if seconds == 7 && !disabled {
					want++
				}
				if samples := list.PopBackAll(); len(samples) != want {
					t.Fatalf("at %ds got %d metrics, want %d", seconds, len(samples), want)
				}
			}
		})
	}
}
