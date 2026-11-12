package prometheus

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/internal/nettest"
)

func TestInputHTTPIPv4Policy(t *testing.T) {
	nettest.CheckInputHTTPClient(t, func(t *testing.T, endpoint string) *http.Client {
		ins := new(Instance)
		client, err := ins.createHTTPClient()
		if err != nil {
			t.Fatal(err)
		}
		return client
	})
}

func TestPrometheusConsulDiscoveryIPv4(t *testing.T) {
	for _, value := range []string{"false", "true"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("PREFER_IPV4_STACK", value)
			counts := nettest.ObserveDNS(t, "consul.invalid.", net.IPv4(127, 0, 0, 1), nil)
			called := make(chan struct{}, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`[]`))
				called <- struct{}{}
			}))
			defer server.Close()
			ins := &Instance{ConsulConfig: ConsulConfig{Agent: strings.Replace(server.URL, "127.0.0.1", "consul.invalid.", 1), QueryInterval: config.Duration(time.Hour), Queries: []*ConsulQuery{{ServiceName: "test", ServiceURL: "http://127.0.0.1/metrics"}}}}
			ctx, cancel := context.WithCancel(context.Background())
			defer func() { cancel(); ins.wg.Wait() }()
			if err := ins.InitConsulClient(ctx); err != nil {
				t.Fatal(err)
			}
			if value == "true" {
				t.Setenv("PREFER_IPV4_STACK", "false")
			}
			select {
			case <-called:
			case <-time.After(2 * time.Second):
				t.Fatal("no discovery request")
			}
			cancel()
			ins.wg.Wait()
			a, aaaa := counts()
			if a == 0 || value == "true" && aaaa != 0 || value == "false" && aaaa == 0 {
				t.Fatalf("A=%d AAAA=%d", a, aaaa)
			}
		})
	}
}
