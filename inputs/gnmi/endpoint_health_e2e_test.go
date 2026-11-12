package gnmi

import (
	"net"
	"testing"
	"time"

	"flashcat.cloud/categraf/types"
	g "github.com/openconfig/gnmi/proto/gnmi"
	"google.golang.org/grpc"
)

func TestEndpointAndInstanceHealthE2E(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	service := &duplicateHealthServer{updates: make(chan *g.Notification, 1), stop: make(chan struct{})}
	g.RegisterGNMIServer(server, service)
	go server.Serve(listener)
	defer server.Stop()
	address := listener.Addr().String()
	plugin := &GNMI{EnableEndpointUp: true}
	for _, name := range []string{"interfaces", "cpu"} {
		instance := &Instance{Addresses: []string{address}, AgentHostTag: "ident", Subscriptions: []Subscription{{Name: "gnmi_value", Path: "/system/value", DisableConcatenation: true}}}
		instance.Labels = map[string]string{"subscription": name}
		plugin.Instances = append(plugin.Instances, instance)
	}
	if err := plugin.Init(); err != nil {
		t.Fatal(err)
	}
	defer plugin.Drop()
	for _, instance := range plugin.Instances {
		if err := instance.Init(); err != nil {
			t.Fatal(err)
		}
		instance.SetInitialized()
	}
	if err := plugin.Start(nil); err != nil {
		t.Fatal(err)
	}
	waitHealth(t, func() bool {
		if service.calls.Load() != 2 {
			return false
		}
		return plugin.Instances[0].health[address].isUp() || plugin.Instances[1].health[address].isUp()
	})
	service.updates <- &g.Notification{Update: []*g.Update{{Path: originTestPath(t, "", "/system/value"), Val: &g.TypedValue{Value: &g.TypedValue_UintVal{UintVal: 7}}}}}
	waitHealth(t, func() bool { return plugin.Instances[0].slist.Len()+plugin.Instances[1].slist.Len() > 0 })
	values := make(map[int]int)
	business := 0
	for _, instance := range plugin.Instances {
		list := types.NewSampleList()
		instance.Gather(list)
		upCount := 0
		for _, sample := range list.PopBackAll() {
			switch sample.Metric {
			case "gnmi_value":
				business++
			case "gnmi_up":
				upCount++
				if sample.Labels["subscription"] != instance.Labels["subscription"] || sample.Labels["ident"] != "127.0.0.1" {
					t.Fatalf("instance labels changed: %v", sample.Labels)
				}
				values[sample.Value.(int)]++
			default:
				t.Fatalf("unexpected instance metric %s", sample.Metric)
			}
		}
		if upCount != 1 {
			t.Fatal("missing instance health")
		}
	}
	if business != 1 || values[0] != 1 || values[1] != 1 {
		t.Fatalf("instance failures were hidden: business=%d health=%v", business, values)
	}
	aggregate := types.NewSampleList()
	plugin.Gather(aggregate)
	samples := aggregate.PopBackAll()
	if len(samples) != 1 || samples[0].Metric != "gnmi_endpoint_up" || samples[0].Value != 1 || samples[0].Labels["address"] != address {
		t.Fatalf("bad endpoint aggregate: %v", samples)
	}
	if _, exists := samples[0].Labels["subscription"]; exists {
		t.Fatal("endpoint metric inherited an instance label")
	}
	close(service.stop)
	waitHealth(t, func() bool {
		return !plugin.Instances[0].health[address].isUp() && !plugin.Instances[1].health[address].isUp()
	})
	plugin.Gather(aggregate)
	samples = aggregate.PopBackAll()
	if len(samples) != 1 || samples[0].Value != 0 {
		t.Fatal("endpoint remained up after all sessions ended")
	}
	// No timer wait is required for an observed transition inside the report interval.
	if time.Duration(plugin.EndpointUpInterval) <= 0 {
		t.Fatal("invalid endpoint report interval")
	}
}
