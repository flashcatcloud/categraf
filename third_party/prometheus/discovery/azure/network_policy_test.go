package azure

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/miekg/dns"
	"github.com/prometheus/client_golang/prometheus"
	commonconfig "github.com/prometheus/common/config"
	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/util/httppolicy"
)

// Exercise the actual ARM credential path. Only the port is redirected to the
// local fixture; the original SDK dialer, family selection, Host and TLS SNI
// remain in use. No shared SDK transport or cloud registry is replaced.
func TestCredentialNetworkPolicyRefresh(t *testing.T) {
	for _, method := range []string{authMethodOAuth, authMethodManagedIdentity, authMethodWorkloadIdentity, authMethodSDK} {
		t.Run(method, func(t *testing.T) {
			for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy", "AZURE_AUTHORITY_HOST", "IDENTITY_ENDPOINT", "IDENTITY_HEADER", "MSI_ENDPOINT", "MSI_SECRET", "IMDS_ENDPOINT", "IDENTITY_SERVER_THUMBPRINT", "AZURE_TOKEN_CREDENTIALS"} {
				t.Setenv(key, "")
			}
			// Keep DefaultAzureCredential on deployed credentials; never invoke a developer CLI.
			t.Setenv("AZURE_TOKEN_CREDENTIALS", "prod")
			t.Setenv("AZURE_TENANT_ID", "fixture-tenant")
			t.Setenv("AZURE_CLIENT_ID", "fixture-client")
			t.Setenv("AZURE_CLIENT_SECRET", "fixture-secret")
			tokenFile := filepath.Join(t.TempDir(), "federated-token")
			if err := os.WriteFile(tokenFile, []byte("fixture-assertion"), 0600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("AZURE_FEDERATED_TOKEN_FILE", tokenFile)
			queries := observeCredentialDNS(t)
			var tokens, apiRequests atomic.Int32
			authClosed := make(chan struct{}, 8)
			var mu sync.Mutex
			authConnections := make(map[string]bool)
			const endpoint = "https://login.microsoftonline.com"
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.Host, "management.azure.com") {
					apiRequests.Add(1)
					if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer fixture-token") {
						t.Error("lost ARM authentication")
					}
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprint(w, `{"value":[]}`)
					return
				}
				mu.Lock()
				authConnections[r.RemoteAddr] = true
				mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.Contains(r.URL.Path, "openid-configuration"):
					fmt.Fprintf(w, `{"authorization_endpoint":%q,"token_endpoint":%q,"issuer":%q,"jwks_uri":%q}`, endpoint+"/fixture-tenant/oauth2/v2.0/authorize", endpoint+"/fixture-tenant/oauth2/v2.0/token", endpoint+"/fixture-tenant/v2.0", endpoint+"/keys")
				case strings.Contains(r.URL.Path, "discovery/instance"):
					fmt.Fprintf(w, `{"tenant_discovery_endpoint":%q,"metadata":[{"preferred_network":"login.microsoftonline.com","preferred_cache":"login.microsoftonline.com","aliases":["login.microsoftonline.com"]}]}`, endpoint+"/fixture-tenant/v2.0/.well-known/openid-configuration")
				case strings.HasSuffix(r.URL.Path, "/token") || r.URL.Path == "/identity":
					tokens.Add(1)
					if method == authMethodManagedIdentity {
						if r.Header.Get("X-IDENTITY-HEADER") != "fixture-header" {
							t.Error("managed identity header lost")
						}
						fmt.Fprintf(w, `{"access_token":"fixture-token","token_type":"Bearer","expires_on":"%d","resource":"https://management.azure.com/"}`, time.Now().Add(time.Second).Unix())
					} else {
						_ = r.ParseForm()
						if r.Form.Get("client_id") != "fixture-client" {
							t.Error("OAuth client ID lost")
						}
						if method == authMethodWorkloadIdentity && r.Form.Get("client_assertion") != "fixture-assertion" {
							t.Error("workload assertion lost")
						}
						fmt.Fprint(w, `{"access_token":"fixture-token","token_type":"Bearer","expires_in":1,"ext_expires_in":1}`)
					}
				default:
					t.Errorf("unexpected credential path: %s", r.URL.Path)
					http.NotFound(w, r)
				}
			})
			tlsConfig, caFile := credentialTLS(t)
			server := httptest.NewUnstartedServer(handler)
			server.TLS = tlsConfig
			server.Config.ConnState = func(conn net.Conn, state http.ConnState) {
				if state == http.StateClosed {
					mu.Lock()
					auth := authConnections[conn.RemoteAddr().String()]
					mu.Unlock()
					if auth {
						authClosed <- struct{}{}
					}
				}
			}
			server.StartTLS()
			defer server.Close()
			_, port, _ := net.SplitHostPort(strings.TrimPrefix(server.URL, "https://"))
			var managed *httptest.Server
			if method == authMethodManagedIdentity {
				managed = httptest.NewServer(handler)
				defer managed.Close()
				t.Setenv("IDENTITY_ENDPOINT", strings.Replace(managed.URL, "127.0.0.1", "identity.fixture.invalid.", 1)+"/identity")
				t.Setenv("IDENTITY_HEADER", "fixture-header")
			}
			network := func(network string) string {
				if network == "tcp" {
					return "tcp4"
				}
				return network
			}
			redirectPort := func(dial func(context.Context, string, string) (net.Conn, error)) func(context.Context, string, string) (net.Conn, error) {
				return func(ctx context.Context, network, address string) (net.Conn, error) {
					host, originalPort, err := net.SplitHostPort(address)
					if err != nil {
						return nil, err
					}
					if originalPort == "443" {
						address = net.JoinHostPort(host, port)
					}
					return dial(ctx, network, address)
				}
			}
			dial := redirectPort(func(ctx context.Context, networkName, address string) (net.Conn, error) {
				return (&net.Dialer{Timeout: time.Second}).DialContext(ctx, network(networkName), address)
			})
			policy := &httppolicy.Policy{Network: network, HTTPClientOptions: []commonconfig.HTTPClientOption{commonconfig.WithDialContextFunc(dial)}}
			cfg := DefaultSDConfig
			cfg.AuthenticationMethod = method
			cfg.SubscriptionID = "00000000-0000-0000-0000-000000000001"
			cfg.TenantID = "fixture-tenant"
			cfg.ClientID = "fixture-client"
			cfg.ClientSecret = "fixture-secret"
			cfg.ResourceGroup = "fixture"
			cfg.HTTPClientConfig.TLSConfig.CAFile = caFile
			registry := prometheus.NewRegistry()
			metrics := cfg.NewDiscovererMetrics(registry, discovery.NewRefreshMetrics(registry))
			d, err := NewDiscovery(&cfg, discovery.DiscovererOptions{Metrics: metrics, NetworkPolicy: policy})
			if err != nil {
				t.Fatal(err)
			}
			if d.credentialHTTPClient == nil {
				t.Fatal("discoverer has no private credential client")
			}
			t.Cleanup(d.credentialHTTPClient.CloseIdleConnections)
			transport := d.credentialHTTPClient.Transport.(*http.Transport)
			if transport.IdleConnTimeout != 90*time.Second || transport.TLSHandshakeTimeout != 10*time.Second || transport.MaxIdleConnsPerHost != 10 || transport.TLSClientConfig.MinVersion != tls.VersionTLS12 || transport.TLSClientConfig.Renegotiation != tls.RenegotiateFreelyAsClient || transport.TLSNextProto["h2"] == nil {
				t.Fatal("native Azure transport defaults lost")
			}
			pemData, err := os.ReadFile(caFile)
			if err != nil {
				t.Fatal(err)
			}
			roots := x509.NewCertPool()
			roots.AppendCertsFromPEM(pemData)
			transport.TLSClientConfig.RootCAs = roots
			transport.DialContext = redirectPort(transport.DialContext)
			client, err := d.createAzureClient()
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				_, err = client.getVMs(ctx, cfg.ResourceGroup)
				cancel()
				if err != nil {
					t.Fatal(err)
				}
				if i == 0 {
					time.Sleep(1100 * time.Millisecond)
				}
			}
			if tokens.Load() < 2 || apiRequests.Load() != 2 {
				t.Fatalf("tokens=%d API requests=%d", tokens.Load(), apiRequests.Load())
			}
			host := "login.microsoftonline.com."
			if method == authMethodManagedIdentity {
				host = "identity.fixture.invalid."
			}
			a, aaaa := queries(host)
			if a == 0 || aaaa != 0 {
				t.Fatalf("credential host %s A=%d AAAA=%d", host, a, aaaa)
			}
			// Run's shutdown closes the discoverer's credential pool. The canceled
			// refresh cannot create additional network activity.
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			d.Run(ctx, make(chan []*targetgroup.Group))
			if method != authMethodManagedIdentity {
				select {
				case <-authClosed:
				case <-time.After(time.Second):
					t.Fatal("credential pool remained open after discovery stopped")
				}
			}
		})
	}
}

func credentialTLS(t *testing.T) (*tls.Config, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "credential fixture"}, DNSNames: []string{"login.microsoftonline.com", "management.azure.com"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, BasicConstraintsValid: true, IsCA: true}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600); err != nil {
		t.Fatal(err)
	}
	return &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}, ca
}

func observeCredentialDNS(t *testing.T) func(string) (int, int) {
	t.Helper()
	socket, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	counts := make(map[string]map[uint16]int)
	ready := make(chan struct{})
	done := make(chan error, 1)
	server := &dns.Server{PacketConn: socket, NotifyStartedFunc: func() { close(ready) }, Handler: dns.HandlerFunc(func(w dns.ResponseWriter, request *dns.Msg) {
		reply := new(dns.Msg)
		reply.SetReply(request)
		for _, question := range request.Question {
			mu.Lock()
			if counts[question.Name] == nil {
				counts[question.Name] = make(map[uint16]int)
			}
			counts[question.Name][question.Qtype]++
			mu.Unlock()
			header := dns.RR_Header{Name: question.Name, Rrtype: question.Qtype, Class: dns.ClassINET, Ttl: 1}
			switch question.Qtype {
			case dns.TypeA:
				reply.Answer = append(reply.Answer, &dns.A{Hdr: header, A: net.IPv4(127, 0, 0, 1)})
			case dns.TypeAAAA:
				reply.Answer = append(reply.Answer, &dns.AAAA{Hdr: header, AAAA: net.IPv6loopback})
			}
		}
		_ = w.WriteMsg(reply)
	})}
	go func() { done <- server.ActivateAndServe() }()
	<-ready
	original := net.DefaultResolver
	net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "udp4", socket.LocalAddr().String())
	}}
	t.Cleanup(func() {
		net.DefaultResolver = original
		_ = server.Shutdown()
		if err := <-done; err != nil {
			t.Error(err)
		}
	})
	return func(host string) (int, int) {
		mu.Lock()
		defer mu.Unlock()
		return counts[host][dns.TypeA], counts[host][dns.TypeAAAA]
	}
}
