package gnmi

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"flashcat.cloud/categraf/types"
	g "github.com/openconfig/gnmi/proto/gnmi"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type duplicateHealthServer struct {
	g.UnimplementedGNMIServer
	calls   atomic.Int32
	updates chan *g.Notification
	stop    chan struct{}
}

func (s *duplicateHealthServer) Subscribe(stream g.GNMI_SubscribeServer) error {
	if _, err := stream.Recv(); err != nil {
		return err
	}
	if s.calls.Add(1) == 2 {
		return status.Error(codes.PermissionDenied, "second subscription denied")
	}
	if err := stream.Send(&g.SubscribeResponse{Response: &g.SubscribeResponse_SyncResponse{SyncResponse: true}}); err != nil {
		return err
	}
	for {
		select {
		case <-stream.Context().Done():
			return stream.Context().Err()
		case <-s.stop:
			return nil
		case update := <-s.updates:
			if err := stream.Send(&g.SubscribeResponse{Response: &g.SubscribeResponse_Update{Update: update}}); err != nil {
				return err
			}
		}
	}
}

func TestDuplicateTargetSessionHealthE2E(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	s := &duplicateHealthServer{updates: make(chan *g.Notification, 1), stop: make(chan struct{})}
	g.RegisterGNMIServer(server, s)
	go server.Serve(ln)
	defer server.Stop()
	address := ln.Addr().String()
	c := &Instance{Addresses: []string{address, address}, AgentHostTag: "agent_host"}
	if err := c.initHealth(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("subscription did not stop")
		}
	}()
	list := types.NewSampleList()
	request := &g.SubscribeRequest{Request: &g.SubscribeRequest_Subscribe{Subscribe: &g.SubscriptionList{}}}
	first := newOriginTestHandler(t, []Subscription{{Name: "gnmi_value", Path: "/system/value", DisableConcatenation: true}}, nil)
	first.address, first.health = address, c.health[address]
	go func() { done <- first.subscribeGNMI(ctx, list, nil, request) }()
	waitHealth(t, func() bool { return c.health[address].snapshot().Up })
	second := &handler{address: address, health: c.health[address], tagStore: newTagStore(nil)}
	if err := second.subscribeGNMI(ctx, list, nil, request); err == nil {
		t.Fatal("second subscription was not rejected")
	}
	s.updates <- &g.Notification{Update: []*g.Update{{
		Path: originTestPath(t, "", "/system/value"), Val: &g.TypedValue{Value: &g.TypedValue_UintVal{UintVal: 7}},
	}}}
	waitHealth(t, func() bool { return list.Len() > 0 })
	business := list.PopBackAll()
	if len(business) != 1 || business[0].Metric != "gnmi_value" || business[0].Value != uint64(7) {
		t.Fatalf("healthy subscription stopped delivering telemetry: %v", business)
	}
	c.SetInitialized()
	plugin := &GNMI{Instances: []*Instance{c}}
	if err := plugin.Init(); err != nil {
		t.Fatal(err)
	}
	checkUp := func(want int) {
		t.Helper()
		output := types.NewSampleList()
		plugin.gatherHealth(output, time.Now())
		samples := output.PopBackAll()
		if len(samples) != 1 || samples[0].Metric != "gnmi_up" || samples[0].Value != want {
			t.Fatalf("session health = %v, want one gnmi_up=%d sample", samples, want)
		}
	}
	checkUp(1)
	close(s.stop)
	waitHealth(t, func() bool { return !c.health[address].snapshot().Up })
	checkUp(0)
}
