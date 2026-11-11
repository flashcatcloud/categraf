//go:build !no_ibex

package client

import (
	"net"
	"net/rpc"
	"testing"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/internal/nettest"
	"github.com/ugorji/go/codec"
)

type pingRPCService struct{}

func (*pingRPCService) Ping(_ string, reply *string) error { *reply = "pong"; return nil }

func TestConnectDNSAddressFamily(t *testing.T) {
	original := config.Config
	t.Cleanup(func() { config.Config = original })
	for _, value := range []string{"false", "true"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("PREFER_IPV4_STACK", value)
			counts := nettest.ObserveDNS(t, "ibex.invalid.", net.IPv4(127, 0, 0, 1), nil)
			listener, err := net.Listen("tcp4", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			server := rpc.NewServer()
			if err := server.RegisterName("Server", &pingRPCService{}); err != nil {
				t.Fatal(err)
			}
			done := make(chan struct{})
			go func() {
				defer close(done)
				conn, err := listener.Accept()
				if err != nil {
					return
				}
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
				server.ServeCodec(codec.MsgpackSpecRpc.ServerCodec(conn, &codec.MsgpackHandle{}))
			}()
			t.Cleanup(func() { listener.Close(); <-done })
			_, port, _ := net.SplitHostPort(listener.Addr().String())
			config.Config = &config.ConfigType{Ibex: &config.IbexConfig{Servers: []string{net.JoinHostPort("ibex.invalid.", port)}}}
			client := connect()
			if client == nil {
				t.Fatal("RPC connection failed")
			}
			client.Close()
			a, aaaa := counts()
			wantAAAA := int64(1)
			if value == "true" {
				wantAAAA = 0
			}
			if a != 1 || aaaa != wantAAAA {
				t.Fatalf("A=%d AAAA=%d", a, aaaa)
			}
		})
	}
}
