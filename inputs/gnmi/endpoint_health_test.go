package gnmi

import (
	"testing"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/pkg/cfg"
	"flashcat.cloud/categraf/types"
)

func TestEndpointHealthIsOptInAndIndependent(t *testing.T) {
	address := "host:50051"
	first := &Instance{Addresses: []string{address}, AgentHostTag: "ident", GNMIUpInterval: config.Duration(5 * time.Second)}
	second := &Instance{Addresses: []string{address}, AgentHostTag: "ident", GNMIUpInterval: config.Duration(20 * time.Second)}
	first.Labels = map[string]string{"subscription": "interfaces"}
	second.Labels = map[string]string{"subscription": "cpu"}
	for _, instance := range []*Instance{first, second} {
		if err := instance.initHealth(); err != nil {
			t.Fatal(err)
		}
		instance.SetInitialized()
	}
	plugin := &GNMI{Instances: []*Instance{first, second}, EndpointUpInterval: config.Duration(10 * time.Second)}
	if err := plugin.Init(); err != nil {
		t.Fatal(err)
	}
	now := time.Unix(100, 0)
	s := first.health[address]
	generation := s.beginAttempt()
	s.observe(generation, now, true)
	root := types.NewSampleList()
	plugin.gatherEndpointHealth(root, now)
	if root.Len() != 0 {
		t.Fatal("endpoint aggregation enabled without opt-in")
	}
	plugin.EnableEndpointUp = true
	check := func(seconds int, firstUp, secondUp, endpointUp *int) {
		t.Helper()
		checkList := func(list *types.SampleList, metric string, want *int, label string) {
			t.Helper()
			samples := list.PopBackAll()
			if want == nil {
				if len(samples) != 0 {
					t.Fatalf("unexpected %s at %ds: %v", metric, seconds, samples)
				}
				return
			}
			if len(samples) != 1 || samples[0].Metric != metric || samples[0].Value != *want || samples[0].Labels["address"] != address {
				t.Fatalf("at %ds: got %v, want %s=%d", seconds, samples, metric, *want)
			}
			if label != "" && samples[0].Labels["subscription"] != label {
				t.Fatalf("instance label lost: %v", samples[0].Labels)
			}
			if label == "" {
				if _, exists := samples[0].Labels["subscription"]; exists {
					t.Fatal("endpoint aggregate inherited instance labels")
				}
			}
		}
		at := now.Add(time.Duration(seconds) * time.Second)
		local := types.NewSampleList()
		first.gatherHealth(local, at)
		checkList(local, "gnmi_up", firstUp, "interfaces")
		second.gatherHealth(local, at)
		checkList(local, "gnmi_up", secondUp, "cpu")
		plugin.gatherEndpointHealth(root, at)
		checkList(root, "gnmi_endpoint_up", endpointUp, "")
	}
	one, zero := 1, 0
	check(0, &one, &zero, &one)
	check(4, nil, nil, nil)
	check(5, &one, nil, nil)
	s.finish(generation)
	check(6, &zero, nil, &zero)
	first.DisableGNMIUp = true
	generation = s.beginAttempt()
	s.observe(generation, now, true)
	check(7, nil, nil, &one)
	plugin.EnableEndpointUp = false
	first.DisableGNMIUp = false
	check(8, &one, nil, nil)
	check(20, &one, &zero, nil)
}

func TestEndpointHealthOptions(t *testing.T) {
	plugin := &GNMI{EnableEndpointUp: true}
	if err := plugin.Init(); err != nil {
		t.Fatal(err)
	}
	if time.Duration(plugin.EndpointUpInterval) != 30*time.Second || plugin.ICMPMaxConcurrency != 10 {
		t.Fatalf("unexpected defaults: %+v", plugin)
	}
	for _, plugin := range []*GNMI{{EnableEndpointUp: true, EndpointUpInterval: -1}, {ICMPMaxConcurrency: -1}} {
		if plugin.Init() == nil {
			t.Fatal("accepted invalid plugin options")
		}
	}
}

func TestLegacySplitFilesAndSharedICMP(t *testing.T) {
	var plugin GNMI
	err := cfg.LoadConfigs([]cfg.ConfigWithFormat{
		{Format: cfg.TomlFormat, Config: "interval=\"1s\"\nenable_endpoint_up=true\nendpoint_up_interval=\"5s\""},
		{Format: cfg.TomlFormat, Config: "[[instances]]\naddresses=[\"host:50051\"]\nlabels={job=\"interfaces\"}\ndisable_gnmi_up=true\ngnmi_up_interval=\"7s\"\nenable_icmp=true"},
		{Format: cfg.TomlFormat, Config: "[[instances]]\naddresses=[\"host:50051\"]\nlabels={job=\"cpu\"}\ngnmi_up_interval=\"11s\"\nenable_icmp=true"},
	}, &plugin)
	if err != nil {
		t.Fatal(err)
	}
	if err := plugin.Init(); err != nil {
		t.Fatal(err)
	}
	for _, instance := range plugin.Instances {
		instance.AgentHostTag = "agent_host"
		if err := instance.initHealth(); err != nil {
			t.Fatal(err)
		}
		if err := instance.initICMP(); err != nil {
			t.Fatal(err)
		}
		instance.SetInitialized()
	}
	first, second := plugin.Instances[0], plugin.Instances[1]
	if !first.DisableGNMIUp || !first.EnableICMP || !second.EnableICMP || time.Duration(first.GNMIUpInterval) != 7*time.Second || time.Duration(second.GNMIUpInterval) != 11*time.Second || time.Duration(plugin.EndpointUpInterval) != 5*time.Second {
		t.Fatal("split-file options were ignored or overwritten")
	}
	plugin.icmp = plugin.newICMPMonitor()
	if len(plugin.icmp.keys) != 1 || first.icmp != second.icmp {
		t.Fatal("split-file consumers did not share their compatible ICMP probe")
	}
	now := time.Now()
	session := first.health["host:50051"]
	session.observe(session.beginAttempt(), now, true)
	local := types.NewSampleList()
	first.gatherHealth(local, now)
	second.gatherHealth(local, now)
	count := 0
	for _, sample := range local.PopBackAll() {
		if sample.Metric == "gnmi_up" {
			count++
			if sample.Value != 0 || sample.Labels["job"] != "cpu" {
				t.Fatal("disabled/failed instance reporting was changed")
			}
		}
	}
	if count != 1 {
		t.Fatalf("instance gnmi_up count=%d, want 1", count)
	}
	aggregate := types.NewSampleList()
	plugin.gatherEndpointHealth(aggregate, now)
	samples := aggregate.PopBackAll()
	if len(samples) != 1 || samples[0].Metric != "gnmi_endpoint_up" || samples[0].Value != 1 {
		t.Fatal("aggregate did not independently observe the healthy disabled-output instance")
	}
}
