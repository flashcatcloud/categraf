package redis_sentinel

import (
	"context"
	"net"
	"testing"
	"time"

	"flashcat.cloud/categraf/internal/nettest"
)

func TestSentinelIPv4TLS(t *testing.T) {
	for _, value := range []string{"false", "true"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("PREFER_IPV4_STACK", value)
			counts := nettest.ObserveDNS(t, "example.com.", net.IPv4(127, 0, 0, 1), nil)
			config, caFile := nettest.TLSFixture(t)
			addr := nettest.RedisServer(t, config)
			_, port, _ := net.SplitHostPort(addr)
			ins := &Instance{Servers: []string{"tcp://:test-password@" + net.JoinHostPort("example.com.", port)}}
			ins.UseTLS = true
			ins.TLSCA = caFile
			if err := ins.Init(); err != nil {
				t.Fatal(err)
			}
			defer ins.clients[0].sentinel.Close()
			if value == "true" {
				t.Setenv("PREFER_IPV4_STACK", "false")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := ins.clients[0].sentinel.Ping(ctx).Err(); err != nil {
				t.Fatal(err)
			}
			a, aaaa := counts()
			if a == 0 || value == "true" && aaaa != 0 || value == "false" && aaaa == 0 {
				t.Fatalf("A=%d AAAA=%d", a, aaaa)
			}
		})
	}
}
