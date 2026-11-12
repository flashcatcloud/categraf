package consul

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"flashcat.cloud/categraf/internal/nettest"
)

func TestConsulSDKIPv4(t *testing.T) {
	for _, value := range []string{"false", "true"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("PREFER_IPV4_STACK", value)
			counts := nettest.ObserveDNS(t, "consul.invalid.", net.IPv4(127, 0, 0, 1), nil)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("X-Consul-Token") != "test-token" {
					t.Error("Consul token lost")
				}
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Connection", "close")
				_, _ = w.Write([]byte(`[]`))
			}))
			defer server.Close()
			ins := &Instance{Address: strings.Replace(server.URL, "127.0.0.1", "consul.invalid.", 1), Token: "test-token"}
			if err := ins.Init(); err != nil {
				t.Fatal(err)
			}
			if value == "true" {
				t.Setenv("PREFER_IPV4_STACK", "false")
			}
			for i := 0; i < 2; i++ {
				if _, _, err := ins.client.Health().State("any", nil); err != nil {
					t.Fatal(err)
				}
			}
			a, aaaa := counts()
			if a < 2 || value == "true" && aaaa != 0 || value == "false" && aaaa == 0 {
				t.Fatalf("A=%d AAAA=%d", a, aaaa)
			}
		})
	}
}
