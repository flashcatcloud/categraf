package gnmi

import (
	"context"
	"flashcat.cloud/categraf/types"
	g "github.com/openconfig/gnmi/proto/gnmi"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"io"
	"net"
	"testing"
	"time"
)

type healthServer struct {
	g.UnimplementedGNMIServer
	responses chan *g.SubscribeResponse
	end       chan error
	ready     chan struct{}
}

func (s *healthServer) Subscribe(stream g.GNMI_SubscribeServer) error {
	if _, err := stream.Recv(); err != nil {
		return err
	}
	close(s.ready)
	for {
		select {
		case <-stream.Context().Done():
			return stream.Context().Err()
		case r := <-s.responses:
			if err := stream.Send(r); err != nil {
				return err
			}
		case e := <-s.end:
			return e
		}
	}
}
func waitHealth(t *testing.T, f func() bool) {
	t.Helper()
	deadline := time.After(3 * time.Second)
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		if f() {
			return
		}
		select {
		case <-deadline:
			t.Fatal("health state did not converge")
		case <-tick.C:
		}
	}
}
func TestSessionHealthE2E(t *testing.T) {
	for _, end := range []string{"EOF", "permission", "protocol"} {
		t.Run(end, func(t *testing.T) {
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			server := grpc.NewServer()
			s := &healthServer{responses: make(chan *g.SubscribeResponse), end: make(chan error), ready: make(chan struct{})}
			g.RegisterGNMIServer(server, s)
			go server.Serve(ln)
			defer server.Stop()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			health := &sessionHealth{}
			h := &handler{address: ln.Addr().String(), health: health, tagStore: newTagStore(nil)}
			done := make(chan error, 1)
			go func() {
				done <- h.subscribeGNMI(ctx, types.NewSampleList(), nil, &g.SubscribeRequest{Request: &g.SubscribeRequest_Subscribe{Subscribe: &g.SubscriptionList{}}})
			}()
			select {
			case <-s.ready:
			case <-time.After(3 * time.Second):
				t.Fatal("no subscribe")
			}
			if health.snapshot().Up {
				t.Fatal("Send alone must not confirm subscription")
			}
			s.responses <- &g.SubscribeResponse{Response: &g.SubscribeResponse_SyncResponse{SyncResponse: false}}
			s.responses <- &g.SubscribeResponse{Response: &g.SubscribeResponse_SyncResponse{SyncResponse: true}}
			waitHealth(t, func() bool { return health.snapshot().Up })
			if !health.snapshot().LastUpdate.IsZero() {
				t.Fatal("sync is not data")
			}
			s.responses <- &g.SubscribeResponse{Response: &g.SubscribeResponse_Update{Update: &g.Notification{Delete: []*g.Path{{Elem: []*g.PathElem{{Name: "removed"}}}}}}}
			waitHealth(t, func() bool { return !health.snapshot().LastUpdate.IsZero() })
			switch end {
			case "EOF":
				s.end <- nil
			case "permission":
				s.end <- status.Error(codes.PermissionDenied, "denied")
			case "protocol":
				s.responses <- &g.SubscribeResponse{Response: &g.SubscribeResponse_Error{Error: &g.Error{Code: 7}}}
			}
			select {
			case err := <-done:
				if end != "EOF" && (err == nil || err == io.EOF) {
					t.Fatal("failure ignored")
				}
			case <-time.After(3 * time.Second):
				t.Fatal("receive did not exit")
			}
			if health.snapshot().Up {
				t.Fatal("ended session remains up")
			}
		})
	}
}
