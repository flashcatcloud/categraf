package googlex

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	legacygoogle "golang.org/x/oauth2/google"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cloud.google.com/go/auth/credentials"
	"cloud.google.com/go/compute/metadata"
	"flashcat.cloud/categraf/internal/nettest"
	"flashcat.cloud/categraf/pkg/netx"
	"google.golang.org/api/option"
	apihttp "google.golang.org/api/transport/http"
)

func TestOAuthRefreshAndMetadataPolicy(t *testing.T) {
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy"} {
		t.Setenv(key, "")
	}
	t.Setenv("PREFER_IPV4_STACK", "true")
	counts := nettest.ObserveDNS(t, "example.com.", net.IPv4(127, 0, 0, 1), net.IPv6loopback)
	var refreshes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/computeMetadata/") {
			w.Header().Set("Metadata-Flavor", "Google")
			fmt.Fprint(w, "project-fixture")
			return
		}
		refreshes.Add(1)
		_ = r.ParseForm()
		if r.Form.Get("refresh_token") != "test-refresh" {
			t.Error("refresh credentials lost")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"test-access","token_type":"Bearer","expires_in":1}`)
	}))
	defer server.Close()
	endpoint := strings.Replace(server.URL, "127.0.0.1", "example.com.", 1)
	policy := FromEnv()
	defer policy.Close()
	source := &http.Client{Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	clone := policy.Client(source)
	if clone == source || clone.Timeout != source.Timeout || clone.Transport == source.Transport {
		t.Fatal("client settings or ownership lost")
	}
	t.Setenv("PREFER_IPV4_STACK", "false")
	creds, err := credentials.DetectDefault(&credentials.DetectOptions{CredentialsJSON: []byte(`{"type":"authorized_user","client_id":"test-client","client_secret":"test-secret","refresh_token":"test-refresh"}`), TokenURL: endpoint + "/token", Client: source, HTTPClientConfigurer: policy.Client, LookupHost: netx.LookupHostIPv4(net.DefaultResolver), Scopes: []string{"test"}})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		token, err := creds.Token(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if token.Value != "test-access" {
			t.Fatal("wrong token")
		}
		if i == 0 {
			time.Sleep(time.Until(token.Expiry) + 10*time.Millisecond)
		}
	}
	t.Setenv("GCE_METADATA_HOST", strings.TrimPrefix(endpoint, "http://"))
	meta := metadata.NewWithOptions(&metadata.Options{UseDefaultClient: true, HTTPClientConfigurer: policy.Client, LookupHost: netx.LookupHostIPv4(net.DefaultResolver)})
	if got, err := meta.ProjectIDWithContext(context.Background()); err != nil || got != "project-fixture" {
		t.Fatalf("metadata=%s err=%v", got, err)
	}
	a, aaaa := counts()
	if refreshes.Load() < 2 || a == 0 || aaaa != 0 {
		t.Fatalf("refreshes=%d A=%d AAAA=%d", refreshes.Load(), a, aaaa)
	}
}

func TestUnsupportedS2AIsExplicit(t *testing.T) {
	t.Setenv("PREFER_IPV4_STACK", "true")
	t.Setenv("EXPERIMENTAL_GOOGLE_API_USE_S2A", "true")
	p := FromEnv()
	defer p.Close()
	options := append(p.Options(), option.WithoutAuthentication())
	if _, _, err := apihttp.NewClient(context.Background(), options...); err == nil || !strings.Contains(err.Error(), "S2A") {
		t.Fatalf("expected explicit S2A boundary: %v", err)
	}
}

func TestMutualTLSCredentialTransport(t *testing.T) {
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy"} {
		t.Setenv(key, "")
	}
	t.Setenv("PREFER_IPV4_STACK", "true")
	counts := nettest.ObserveDNS(t, "example.com.", net.IPv4(127, 0, 0, 1), net.IPv6loopback)
	config, caFile := nettest.TLSFixture(t)
	config.ClientAuth = tls.RequireAnyClientCert
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(r.TLS.PeerCertificates) != 1 {
			t.Error("credential HTTP lost its client certificate")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"fixture","token_type":"Bearer","expires_in":3600}`)
	}))
	server.TLS = config
	server.StartTLS()
	defer server.Close()
	pem, err := os.ReadFile(caFile)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(pem)
	native := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, GetClientCertificate: func(*tls.CertificateRequestInfo) (*tls.Certificate, error) { return &config.Certificates[0], nil }}}, Timeout: time.Second}
	policy := FromEnv()
	defer policy.Close()
	endpoint := strings.Replace(server.URL, "127.0.0.1", "example.com.", 1)
	creds, err := credentials.DetectDefault(&credentials.DetectOptions{CredentialsJSON: []byte(`{"type":"authorized_user","client_id":"fixture","client_secret":"fixture","refresh_token":"fixture"}`), TokenURL: endpoint, Client: native, HTTPClientConfigurer: policy.Client, Scopes: []string{"fixture"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = creds.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	a, aaaa := counts()
	if a == 0 || aaaa != 0 {
		t.Fatalf("A=%d AAAA=%d", a, aaaa)
	}
}

func TestLegacyADCMetadataPolicy(t *testing.T) {
	t.Setenv("GOOGLE_API_GO_EXPERIMENTAL_DISABLE_NEW_AUTH_LIB", "true")
	// Avoid changing or reading any developer credentials. The fixture only runs
	// when ADC has no well-known credential file to take priority over metadata.
	dir, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".config", "gcloud", "application_default_credentials.json")); err == nil {
		t.Skip("local ADC file takes priority over metadata")
	}
	for _, key := range []string{"GOOGLE_APPLICATION_CREDENTIALS", "HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy"} {
		t.Setenv(key, "")
	}
	t.Setenv("PREFER_IPV4_STACK", "true")
	queries := nettest.ObserveDNSQueries(t, "example.com.", net.IPv4(127, 0, 0, 1), net.IPv6loopback)
	var tokens atomic.Int32
	var apiRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Metadata-Flavor", "Google")
		switch {
		case r.URL.Path == "/api":
			apiRequests.Add(1)
			if r.Header.Get("Authorization") != "Bearer metadata-fixture" {
				t.Error("legacy ADC authentication lost")
			}
			w.WriteHeader(204)
		case strings.Contains(r.URL.Path, "/token"):
			tokens.Add(1)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"access_token":"metadata-fixture","token_type":"Bearer","expires_in":1}`)
		case strings.Contains(r.URL.Path, "project-id"):
			fmt.Fprint(w, "project-fixture")
		case strings.Contains(r.URL.Path, "universe_domain"):
			fmt.Fprint(w, "googleapis.com")
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	endpoint := strings.Replace(server.URL, "127.0.0.1", "example.com.", 1)
	t.Setenv("GCE_METADATA_HOST", strings.TrimPrefix(endpoint, "http://"))
	policy := FromEnv()
	defer policy.Close()
	metadataClient := metadata.NewWithOptions(&metadata.Options{UseDefaultClient: true, HTTPClientConfigurer: policy.Client, LookupHost: netx.LookupHostIPv4(net.DefaultResolver)})
	creds, err := legacygoogle.FindDefaultCredentialsWithParams(context.Background(), legacygoogle.CredentialsParams{Scopes: []string{"fixture"}, MetadataClient: metadataClient})
	if err != nil {
		t.Fatal(err)
	}
	if creds.ProjectID != "project-fixture" {
		t.Fatalf("project=%s", creds.ProjectID)
	}
	t.Setenv("PREFER_IPV4_STACK", "false")
	for i := 0; i < 2; i++ {
		token, err := creds.TokenSource.Token()
		if err != nil {
			t.Fatal(err)
		}
		if token.AccessToken != "metadata-fixture" {
			t.Fatalf("token=%s", token.AccessToken)
		}
	}
	options := append(policy.Options(), option.WithScopes("fixture"))
	transport, err := apihttp.NewTransport(context.Background(), policy.Client(&http.Client{}).Transport, options...)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: transport}
	defer client.CloseIdleConnections()
	response, err := client.Get(endpoint + "/api")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	a, aaaa := queries("example.com.")
	if apiRequests.Load() != 1 {
		t.Fatal("legacy API transport was not used")
	}
	if tokens.Load() != 3 || a == 0 || aaaa != 0 {
		t.Fatalf("metadata tokens=%d A=%d AAAA=%d", tokens.Load(), a, aaaa)
	}
}
