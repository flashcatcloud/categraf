package gnmi

import (
	"testing"
	"time"

	"flashcat.cloud/categraf/pkg/cfg"
	"flashcat.cloud/categraf/types"
)

func testICMPPlugin(t *testing.T, addresses ...[]string) *GNMI {
	t.Helper()
	plugin := &GNMI{EnableICMP: true}
	for _, group := range addresses {
		instance := &Instance{Addresses: group}
		instance.SetInitialized()
		plugin.Instances = append(plugin.Instances, instance)
	}
	if err := plugin.Init(); err != nil {
		t.Fatal(err)
	}
	plugin.icmp = plugin.newICMPMonitor()
	plugin.icmpReports = make(map[string]icmpReport)
	return plugin
}

func TestICMPRequiresExplicitTopLevelEnable(t *testing.T) {
	for _, tc := range []struct {
		root string
		want bool
	}{
		{"", false},
		{"enable_icmp = false", false},
		{"enable_icmp = true", true},
	} {
		var plugin GNMI
		err := cfg.LoadConfigs([]cfg.ConfigWithFormat{
			{Format: cfg.TomlFormat, Config: "interval = \"15s\"\n" + tc.root},
			{Format: cfg.TomlFormat, Config: "[[instances]]\naddresses = [\"host:123\"]\nenable_icmp = true"},
			{Format: cfg.TomlFormat, Config: "[[instances]]\naddresses = [\"host:124\"]"},
		}, &plugin)
		if err != nil {
			t.Fatal(err)
		}
		if err := plugin.Init(); err != nil {
			t.Fatal(err)
		}
		for _, instance := range plugin.Instances {
			instance.SetInitialized()
		}
		if err := plugin.Start(nil); err != nil {
			t.Fatal(err)
		}
		plugin.Drop()
		if (plugin.icmp != nil) != tc.want {
			t.Fatalf("top-level %q: monitor enabled = %t", tc.root, plugin.icmp != nil)
		}
		if tc.want && (len(plugin.icmp.hosts) != 1 || plugin.icmp.hosts[0] != "host") {
			t.Fatalf("duplicate hosts: %v", plugin.icmp.hosts)
		}
	}
}

func TestICMPSkipsResolverTargets(t *testing.T) {
	plugin := testICMPPlugin(t, []string{"127.0.0.1:50051", "dns:///127.0.0.1:50051"})
	if len(plugin.icmp.hosts) != 1 || plugin.icmp.hosts[0] != "127.0.0.1" {
		t.Fatalf("unexpected probe targets: %v", plugin.icmp.hosts)
	}
}

func TestICMPReportCadence(t *testing.T) {
	plugin := testICMPPlugin(t, []string{"host:1"})
	start := time.Unix(100, 0)
	list := types.NewSampleList()
	check := func(seconds int, count int) {
		t.Helper()
		plugin.gatherICMP(list, start.Add(time.Duration(seconds)*time.Second))
		if samples := list.PopBackAll(); len(samples) != count {
			t.Fatalf("at %ds, want %d samples, got %v", seconds, count, samples)
		}
	}
	check(0, 0)
	reachable := icmpResult{Sent: 3, Received: 3}
	plugin.icmp.store("host", reachable, nil, start)
	check(0, 5)
	plugin.icmp.store("host", reachable, nil, start.Add(30*time.Second))
	check(30, 0)
	check(60, 5)
	check(61, 0)
	plugin.icmp.store("host", icmpResult{Sent: 3}, nil, start.Add(62*time.Second))
	check(62, 4)
	check(63, 0)
	plugin.icmp.store("host", reachable, nil, start.Add(64*time.Second))
	check(64, 5)
	check(124, 0)
	check(128, 2)
	check(129, 0)
}
