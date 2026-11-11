package inputs

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/internal/nettest"
)

func TestHTTPProviderDNSAddressFamily(t *testing.T) {
	originalConfig, originalHostInfo := config.Config, config.HostInfo
	config.Config, config.HostInfo = &config.ConfigType{}, &config.HostInfoCache{}
	t.Cleanup(func() { config.Config, config.HostInfo = originalConfig, originalHostInfo })
	for _, value := range []string{"false", "true"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("PREFER_IPV4_STACK", value)
			counts := nettest.ObserveDNS(t, "provider.invalid.", net.IPv4(127, 0, 0, 1), nil)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("X-Test") != "preserved" {
					t.Error("lost configured headers")
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"version":"test","configs":{}}`))
			}))
			defer server.Close()
			provider := &HTTPProvider{RemoteUrl: strings.Replace(server.URL, "127.0.0.1", "provider.invalid.", 1), Headers: []string{"X-Test", "preserved"}}
			if err := provider.check(); err != nil {
				t.Fatal(err)
			}
			defer provider.client.CloseIdleConnections()
			if _, err := provider.doReq(); err != nil {
				t.Fatal(err)
			}
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
