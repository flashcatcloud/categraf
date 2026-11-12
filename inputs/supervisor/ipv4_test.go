package supervisor

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"flashcat.cloud/categraf/internal/nettest"
)

func TestSupervisorXMLRPCIPv4(t *testing.T) {
	for _, value := range []string{"false", "true"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("PREFER_IPV4_STACK", value)
			counts := nettest.ObserveDNS(t, "rpc.invalid.", net.IPv4(127, 0, 0, 1), nil)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/xml")
				_, _ = w.Write([]byte(`<methodResponse><params><param><value><string>fixture</string></value></param></params></methodResponse>`))
			}))
			defer server.Close()
			ins := &Instance{Url: strings.Replace(server.URL, "127.0.0.1", "rpc.invalid.", 1)}
			if err := ins.Init(); err != nil {
				t.Fatal(err)
			}
			defer ins.rpcClient.Close()
			if value == "true" {
				t.Setenv("PREFER_IPV4_STACK", "false")
			}
			var result string
			if err := ins.rpcClient.Call("supervisor.getIdentification", nil, &result); err != nil || result != "fixture" {
				t.Fatalf("RPC result=%q error=%v", result, err)
			}
			a, aaaa := counts()
			if a == 0 || value == "true" && aaaa != 0 || value == "false" && aaaa == 0 {
				t.Fatalf("A=%d AAAA=%d", a, aaaa)
			}
		})
	}
}
