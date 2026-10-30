package gnmi

import (
	"net"
	"testing"
	"time"

	"flashcat.cloud/categraf/types"
	gnmiLib "github.com/openconfig/gnmi/proto/gnmi"
	"google.golang.org/grpc"
)

type keyedSubscriptionTestServer struct {
	gnmiLib.UnimplementedGNMIServer
	requests chan *gnmiLib.SubscribeRequest
}

func (s *keyedSubscriptionTestServer) Subscribe(stream gnmiLib.GNMI_SubscribeServer) error {
	request, err := stream.Recv()
	if err != nil {
		return err
	}
	s.requests <- request
	<-stream.Context().Done()
	return stream.Context().Err()
}

func TestDisjointTagKeysInitialize(t *testing.T) {
	subs := []TagSubscription{
		{Subscription: Subscription{Name: "ifname", Path: "/interfaces/interface[name=Ethernet1]/state/name", DisableConcatenation: true}, Match: "name"},
		{Subscription: Subscription{Name: "ifname", Path: "/interfaces/interface[name=Ethernet2]/state/name", DisableConcatenation: true}, Match: "name"},
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	server := grpc.NewServer()
	defer server.Stop()
	device := &keyedSubscriptionTestServer{requests: make(chan *gnmiLib.SubscribeRequest, 1)}
	gnmiLib.RegisterGNMIServer(server, device)
	go server.Serve(listener)
	for i := range subs {
		subs[i].SubscriptionMode = "sample"
	}
	instance := &Instance{Addresses: []string{listener.Addr().String()}, TagSubscriptions: subs}
	if err := instance.Init(); err != nil {
		t.Fatalf("disjoint interface subscriptions rejected: %v", err)
	}
	defer instance.Drop()
	select {
	case request := <-device.requests:
		wireSubs := request.GetSubscribe().GetSubscription()
		if len(wireSubs) != 2 || wireSubs[0].Path.Elem[1].Key["name"] != "Ethernet1" || wireSubs[1].Path.Elem[1].Key["name"] != "Ethernet2" {
			t.Fatalf("wire selectors changed: %v", wireSubs)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("device never received the valid keyed subscriptions")
	}
}

func TestTagKeyMatching(t *testing.T) {
	tests := []struct {
		name, configured, actual string
		match                    bool
	}{
		{"unqualified", "/interfaces/interface/state/name", "/interfaces/interface[name=Ethernet1]/state/name", true},
		{"slash in key", "/interfaces/interface[name=Ethernet1/2]/state/name", "/interfaces/interface[name=Ethernet1/2]/state/name", true},
		{"different key", "/interfaces/interface[name=Ethernet1]/state/name", "/interfaces/interface[name=Ethernet2]/state/name", false},
		{"missing key", "/interfaces/interface[name=Ethernet1]/state/name", "/interfaces/interface/state/name", false},
		{"wildcard", "/interfaces/interface[name=*]/state/name", "/interfaces/interface[name=Ethernet1]/state/name", true},
		{"wildcard missing key", "/interfaces/interface[name=*]/state/name", "/interfaces/interface/state/name", false},
		{"partial selector", "/interfaces/interface[name=Ethernet1]/state/name", "/interfaces/interface[name=Ethernet1][id=2]/state/name", true},
		{"multiple keys", "/interfaces/interface[name=Ethernet1][id=2]/state/name", "/interfaces/interface[name=Ethernet1][id=3]/state/name", false},
		{"repeated nodes", "/node[id=outer]/node[id=inner]/name", "/node[id=outer]/node[id=other]/name", false},
		{"inline root", "/module:interface[name=Ethernet1]/name", "/module:interface[name=Ethernet1]/name", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configured := newInfoFromPath(originTestPath(t, "", tt.configured))
			actual := newInfoFromPath(originTestPath(t, "", tt.actual))
			if got := tagMatchRank(configured, actual) > 0; got != tt.match {
				t.Fatalf("key match = %t, want %t", got, tt.match)
			}
		})
	}
	configured := newInfoFromPath(originTestPath(t, "", "/interfaces/interface[name=Ethernet1]/state/name"))
	prefix := newInfoFromPath(originTestPath(t, "", "/interfaces"))
	actual := prefix.append(originTestPath(t, "", "/interface[name=Ethernet1]/state/name"))
	if tagMatchRank(configured, actual) == 0 {
		t.Fatal("appended update lost its selector position")
	}
}

func TestTagKeyOverlapValidation(t *testing.T) {
	tests := []struct {
		name, first, second string
		valid               bool
	}{
		{"disjoint", "/interface[name=Ethernet1]/name", "/interface[name=Ethernet2]/name", true},
		{"duplicate", "/interface[name=Ethernet1]/name", "/interface[name=Ethernet1]/name", false},
		{"wildcard overlap", "/interface[name=*]/name", "/interface[name=Ethernet1]/name", false},
		{"omitted selector", "/interface/name", "/interface[name=Ethernet1]/name", false},
		{"different keys overlap", "/interface[name=Ethernet1]/name", "/interface[id=2]/name", false},
		{"multiple keys disjoint", "/interface[name=Ethernet1][id=2]/name", "/interface[name=Ethernet1][id=3]/name", true},
		{"repeated node disjoint", "/node[id=outer]/node[id=inner]/name", "/node[id=outer]/node[id=other]/name", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			subs := []TagSubscription{
				{Subscription: Subscription{Name: "first", Path: tt.first}},
				{Subscription: Subscription{Name: "second", Path: tt.second}},
			}
			for i := range subs {
				if err := subs[i].buildFullPath(&Instance{}); err != nil {
					t.Fatal(err)
				}
			}
			if err := validateTagSubscriptions(subs); (err == nil) != tt.valid {
				t.Fatalf("valid=%t: %v", tt.valid, err)
			}
		})
	}
}

func TestKeyedTagSelectionAndCacheIsolation(t *testing.T) {
	subs := []TagSubscription{
		{Subscription: Subscription{Name: "role", Path: "/interfaces/interface[name=Ethernet1]/state/description", DisableConcatenation: true}, Match: "unconditional"},
		{Subscription: Subscription{Name: "role", Path: "/interfaces/interface[name=Ethernet2]/state/description", DisableConcatenation: true}, Match: "unconditional"},
	}
	h := newOriginTestHandler(t, nil, subs)
	list := types.NewSampleList()
	update := func(index int, value string) {
		t.Helper()
		h.handleSubscribeResponseUpdate(list, &gnmiLib.SubscribeResponse_Update{Update: &gnmiLib.Notification{
			Update: []*gnmiLib.Update{{Path: originTestPath(t, "", subs[index].Path), Val: &gnmiLib.TypedValue{Value: &gnmiLib.TypedValue_StringVal{StringVal: value}}}},
		}}, nil)
		if samples := list.PopBackAll(); len(samples) != 0 {
			t.Fatalf("tag emitted as metric: %v", samples)
		}
	}
	metric := newInfoFromString("/metrics/value")
	update(0, "edge")
	if got := h.tagStore.lookup(metric, nil)["role"]; got != "edge" {
		t.Fatalf("role=%q, want edge", got)
	}
	update(1, "core")
	if _, exists := h.tagStore.lookup(metric, nil)["role"]; exists {
		t.Fatal("distinct keyed sources overwrote each other instead of detecting a conflict")
	}
	update(1, "")
	if got := h.tagStore.lookup(metric, nil)["role"]; got != "edge" {
		t.Fatalf("clearing Ethernet2 also cleared Ethernet1: %q", got)
	}
	other := newInfoFromPath(originTestPath(t, "", "/interfaces/interface[name=Ethernet3]/state/description"))
	if selected, err := h.selectTagSubscription(other); selected != -1 || err != nil {
		t.Fatalf("unselected interface matched or warned: %d, %v", selected, err)
	}
}
