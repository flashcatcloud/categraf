package sqlserver

import (
	"context"
	"database/sql"
	"flashcat.cloud/categraf/internal/nettest"
	"fmt"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

func TestDriverPreloginIPv4(t *testing.T) {
	for _, value := range []string{"false", "true"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("PREFER_IPV4_STACK", value)
			// Empty AAAA answers keep this fixture on one candidate. The native
			// driver races multiple candidates in background goroutines with no join hook.
			counts := nettest.ObserveDNS(t, "example.com.", net.IPv4(127, 0, 0, 1), nil)
			var requests atomic.Int32
			addr := nettest.TCPServer(t, nil, func(c net.Conn) {
				var header [8]byte
				if _, err := io.ReadFull(c, header[:]); err == nil && header[0] == 0x12 {
					requests.Add(1)
				}
			})
			_, port, _ := net.SplitHostPort(addr)
			dsn := fmt.Sprintf("server=example.com.;port=%s;user id=fixture;password=fixture;connection timeout=1;encrypt=disable", port)
			db, err := openPoolFromEnv(dsn)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			original, err := sql.Open("mssql", dsn)
			if err != nil {
				t.Fatal(err)
			}
			defer original.Close()
			if db.Driver() != original.Driver() {
				t.Fatal("changed registered query-processing driver")
			}
			if value == "true" {
				t.Setenv("PREFER_IPV4_STACK", "false")
			}
			for i := 0; i < 2; i++ {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				err = db.PingContext(ctx)
				cancel()
				if err == nil {
					t.Fatal("invalid prelogin fixture accepted")
				}
			}
			a, aaaa := counts()
			if requests.Load() != 2 || a == 0 || value == "true" && aaaa != 0 {
				t.Fatalf("prelogin=%d A=%d AAAA=%d", requests.Load(), a, aaaa)
			}
		})
	}
}
