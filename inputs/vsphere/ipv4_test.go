package vsphere

import (
	"context"
	"flashcat.cloud/categraf/pkg/netx"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/session"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/soap"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/internal/nettest"
)

func TestVSphereFirstSOAPRequestIPv4TLS(t *testing.T) {
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy"} {
		t.Setenv(key, "")
	}
	for _, value := range []string{"false", "true"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("PREFER_IPV4_STACK", value)
			counts := nettest.ObserveDNS(t, "example.com.", net.IPv4(127, 0, 0, 1), nil)
			var requests atomic.Int32
			closed := make(chan struct{}, 8)
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.Header.Get("SOAPAction") == "" {
					t.Error("missing SOAP action")
				}
				http.Error(w, "fixture stops after the first SOAP request", 500)
			}))
			tlsConfig, caFile := nettest.TLSFixture(t)
			server.TLS = tlsConfig
			server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
				if state == http.StateClosed {
					closed <- struct{}{}
				}
			}
			server.StartTLS()
			defer server.Close()
			endpoint, err := url.Parse(strings.Replace(server.URL, "127.0.0.1", "example.com.", 1))
			if err != nil {
				t.Fatal(err)
			}
			ins := &Instance{Timeout: config.Duration(time.Second)}
			ins.UseTLS = true
			ins.TLSCA = caFile
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if _, err := NewClient(ctx, endpoint, ins); err == nil {
				t.Fatal("expected fixture's SOAP error")
			}
			select {
			case <-closed:
			case <-time.After(time.Second):
				t.Fatal("constructor failure leaked its HTTP pool")
			}
			a, aaaa := counts()
			if requests.Load() == 0 || a == 0 || value == "true" && aaaa != 0 || value == "false" && aaaa == 0 {
				t.Fatalf("SOAP requests=%d A=%d AAAA=%d", requests.Load(), a, aaaa)
			}
		})
	}
}

func TestVSphereOwnedTransportCleanup(t *testing.T) {
	for _, mode := range []string{"SDK", "Logout", "LogoutFailure", "FactoryClose", "FactoryCloseFailure", "PluginDrop", "PluginDropFailure"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("PREFER_IPV4_STACK", "true")
			closed := make(chan struct{}, 8)
			var logouts atomic.Int32
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					logouts.Add(1)
					if strings.Contains(mode, "Failure") {
						http.Error(w, "SOAP logout failure", 500)
						return
					}
					w.Header().Set("Content-Type", "text/xml")
					_, _ = io.WriteString(w, `<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/"><soapenv:Body><LogoutResponse xmlns="urn:vim25"/></soapenv:Body></soapenv:Envelope>`)
				} else {
					w.WriteHeader(204)
				}
			}))
			server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
				if state == http.StateClosed {
					closed <- struct{}{}
				}
			}
			server.Start()
			defer server.Close()
			endpoint, _ := url.Parse(server.URL)
			sc := soap.NewClient(endpoint, false)
			original := sc.DefaultTransport()
			sc.SetTransportConfigurer(netx.HTTPTransportConfigurerFromEnv())
			if sc.Transport != original {
				t.Fatal("SDK transport identity changed")
			}
			child := sc.NewServiceClient("/child", "vim25")
			childResponse, err := child.Get(server.URL + "/child")
			if err != nil {
				t.Fatal(err)
			}
			childResponse.Body.Close()
			response, err := sc.Get(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			vc := &vim25.Client{Client: sc, RoundTripper: sc, ServiceContent: vimtypes.ServiceContent{SessionManager: &vimtypes.ManagedObjectReference{Type: "SessionManager", Value: "SessionManager"}}}
			gc := &govmomi.Client{Client: vc, SessionManager: session.NewManager(vc)}
			switch mode {
			case "SDK":
				sc.CloseIdleConnections()
			case "Logout", "LogoutFailure":
				err := gc.Logout(context.Background())
				if (err != nil) != strings.Contains(mode, "Failure") {
					t.Fatalf("Logout err=%v", err)
				}
			case "FactoryClose", "FactoryCloseFailure":
				factory := &ClientFactory{client: &Client{Client: gc, Timeout: time.Second}}
				factory.Close()
				factory.Close()
			case "PluginDrop", "PluginDropFailure":
				_, cancel := context.WithCancel(context.Background())
				instance := &Instance{cancel: cancel, endpoints: &Endpoint{URL: endpoint, clientFactory: &ClientFactory{client: &Client{Client: gc, Timeout: time.Second}}}}
				instance.Drop()
				instance.Drop()
			}
			for i := 0; i < 2; i++ {
				select {
				case <-closed:
				case <-time.After(time.Second):
					t.Fatal("parent or child HTTP connection remained open")
				}
			}
			if mode != "SDK" && logouts.Load() != 1 {
				t.Fatalf("Logout requests=%d", logouts.Load())
			}
		})
	}
}
