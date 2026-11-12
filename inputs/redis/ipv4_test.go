package redis

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"flashcat.cloud/categraf/internal/nettest"
)

func TestRedisIPv4TLSAndReconnect(t *testing.T) {
	for _, value := range []string{"false", "true"} {
		for _, trusted := range []bool{true, false} {
			t.Run(fmt.Sprintf("env=%s/trusted=%t", value, trusted), func(t *testing.T) {
				t.Setenv("PREFER_IPV4_STACK", value)
				counts := nettest.ObserveDNS(t, "example.com.", net.IPv4(127, 0, 0, 1), nil)
				config, caFile := nettest.TLSFixture(t)
				addr := nettest.RedisServer(t, config)
				_, port, _ := net.SplitHostPort(addr)
				ins := &Instance{Address: net.JoinHostPort("example.com.", port), Password: "test-password"}
				ins.UseTLS = true
				if trusted {
					ins.TLSCA = caFile
				}
				if err := ins.Init(); err != nil {
					t.Fatal(err)
				}
				defer ins.client.Close()
				if value == "true" {
					t.Setenv("PREFER_IPV4_STACK", "false")
				}
				for i := 0; i < 2; i++ {
					ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
					err := ins.client.Ping(ctx).Err()
					cancel()
					if trusted && err != nil || !trusted && err == nil {
						t.Fatalf("Ping: %v", err)
					}
					if !trusted {
						break
					}
				}
				a, aaaa := counts()
				if a == 0 || value == "true" && aaaa != 0 || value == "false" && aaaa == 0 {
					t.Fatalf("A=%d AAAA=%d", a, aaaa)
				}
			})
		}
	}
}
