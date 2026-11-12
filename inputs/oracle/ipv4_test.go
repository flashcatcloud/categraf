package oracle

import (
	"context"
	"flashcat.cloud/categraf/internal/nettest"
	"fmt"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

func TestTNSConnectIPv4(t *testing.T) {
	for _, value := range []string{"false", "true"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("PREFER_IPV4_STACK", value)
			counts := nettest.ObserveDNS(t, "example.com.", net.IPv4(127, 0, 0, 1), net.IPv6loopback)
			var requests atomic.Int32
			addr := nettest.TCPServer(t, nil, func(c net.Conn) {
				var header [8]byte
				if _, err := io.ReadFull(c, header[:]); err == nil && header[4] == 1 {
					requests.Add(1)
				}
			})
			_, port, _ := net.SplitHostPort(addr)
			db, err := openPoolFromEnv(fmt.Sprintf("oracle://fixture:fixture@example.com.:%s/service?CONNECTION%%20TIMEOUT=1", port))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if value == "true" {
				t.Setenv("PREFER_IPV4_STACK", "false")
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if err = db.PingContext(ctx); err == nil {
				t.Fatal("invalid TNS fixture accepted")
			}
			a, aaaa := counts()
			if requests.Load() == 0 || a == 0 || value == "true" && aaaa != 0 {
				t.Fatalf("TNS requests=%d A=%d AAAA=%d", requests.Load(), a, aaaa)
			}
		})
	}
}
