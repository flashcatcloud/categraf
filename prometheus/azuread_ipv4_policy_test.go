//go:build !no_prometheus

package prometheus

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"flashcat.cloud/categraf/internal/nettest"
	"github.com/prometheus/prometheus/storage/remote/azuread"
)

func TestAzureADCredentialRefreshAndCleanup(t *testing.T) {
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy", "MSI_ENDPOINT", "MSI_SECRET", "IMDS_ENDPOINT"} {
		t.Setenv(key, "")
	}
	t.Setenv("IDENTITY_HEADER", "identity-secret")
	for _, value := range []string{"false", "true"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("PREFER_IPV4_STACK", value)
			counts := nettest.ObserveDNS(t, "example.com.", net.IPv4(127, 0, 0, 1), net.IPv6loopback)
			var tokens, writes atomic.Int32
			closed := make(chan struct{}, 16)
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/identity" {
					tokens.Add(1)
					if r.Header.Get("X-IDENTITY-HEADER") != "identity-secret" {
						t.Error("lost managed identity header")
					}
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprintf(w, `{"access_token":"fixture-token","token_type":"Bearer","expires_on":"%d","resource":"https://monitor.azure.com/"}`, time.Now().Add(2*time.Second).Unix())
					return
				}
				writes.Add(1)
				if r.Header.Get("Authorization") != "Bearer fixture-token" {
					t.Error("lost access token")
				}
				fmt.Fprint(w, "ok")
			}))
			server.Config.ConnState = func(_ net.Conn, s http.ConnState) {
				if s == http.StateClosed {
					closed <- struct{}{}
				}
			}
			server.Start()
			defer server.Close()
			endpoint := strings.Replace(server.URL, "127.0.0.1", "example.com.", 1)
			t.Setenv("IDENTITY_ENDPOINT", endpoint+"/identity")
			policy, closePolicy := newNetworkPolicy()
			defer closePolicy()
			var credentialClient *http.Client
			if policy != nil {
				register := policy.OwnHTTPClient
				policy.OwnHTTPClient = func(c *http.Client) { credentialClient = c; register(c) }
			}
			rt, err := azuread.NewAzureADRoundTripper(&azuread.AzureADConfig{Cloud: azuread.AzurePublic, ManagedIdentity: &azuread.ManagedIdentityConfig{}}, http.DefaultTransport, policy)
			if err != nil {
				t.Fatal(err)
			}
			if value == "true" {
				if credentialClient == nil {
					t.Fatal("no owned credential client")
				}
				tr := credentialClient.Transport.(*http.Transport)
				if tr.MaxIdleConnsPerHost != 10 || tr.IdleConnTimeout != 90*time.Second || tr.TLSClientConfig.MinVersion != tls.VersionTLS12 || tr.TLSClientConfig.Renegotiation != tls.RenegotiateFreelyAsClient || tr.TLSNextProto["h2"] == nil {
					t.Fatal("native Azure transport defaults lost")
				}
				t.Setenv("PREFER_IPV4_STACK", "false")
			}
			// Keep the fixture's business request separate; inspect credential DNS only.
			for i := 0; i < 2; i++ {
				if i > 0 {
					time.Sleep(2100 * time.Millisecond)
				}
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				req, _ := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/write", nil)
				resp, err := rt.RoundTrip(req)
				if err == nil {
					_, err = io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
				}
				cancel()
				if err != nil {
					t.Fatal(err)
				}
			}
			a, aaaa := counts()
			if tokens.Load() != 2 || writes.Load() != 2 || a == 0 || value == "true" && aaaa != 0 || value == "false" && aaaa == 0 {
				t.Fatalf("tokens=%d writes=%d A=%d AAAA=%d", tokens.Load(), writes.Load(), a, aaaa)
			}
			if value == "true" {
				closePolicy()
				select {
				case <-closed:
				case <-time.After(time.Second):
					t.Fatal("credential pool did not close on shutdown")
				}
			}
		})
	}
}
