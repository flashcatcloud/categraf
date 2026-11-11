package netx

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"flashcat.cloud/categraf/internal/nettest"
)

func TestHTTPTransportLegacyDialCompatibility(t *testing.T) {
	t.Setenv("PREFER_IPV4_STACK", "true")
	for _, secure := range []bool{false, true} {
		for _, modern := range []bool{false, true} {
			t.Run(fmt.Sprintf("TLS=%t/context-priority=%t", secure, modern), func(t *testing.T) {
				type contextKey struct{}
				ctx := context.WithValue(context.Background(), contextKey{}, "retained")
				wantErr := errors.New("callback error")
				var gotNetwork, gotAddress string
				legacyCalls, contextCalls := 0, 0
				legacy := func(network, addr string) (net.Conn, error) {
					legacyCalls++
					gotNetwork, gotAddress = network, addr
					return nil, wantErr
				}
				withContext := func(c context.Context, network, addr string) (net.Conn, error) {
					contextCalls++
					if c.Value(contextKey{}) != "retained" {
						t.Error("context lost")
					}
					gotNetwork, gotAddress = network, addr
					return nil, wantErr
				}
				original := &http.Transport{}
				if secure {
					original.DialTLS = legacy //nolint:staticcheck // Exercise compatibility with a caller's legacy TLS hook.
					if modern {
						original.DialTLSContext = withContext
					}
				} else {
					original.Dial = legacy //nolint:staticcheck // Exercise compatibility with a caller's legacy dial hook.
					if modern {
						original.DialContext = withContext
					}
				}
				transport := HTTPTransportFromEnv(original)
				dial := transport.DialContext
				if secure {
					dial = transport.DialTLSContext
				}
				if dial == nil {
					t.Fatal("missing context dial callback")
				}
				for _, network := range []string{"tcp", "tcp6"} {
					_, err := dial(ctx, network, "example.invalid:443")
					wantNetwork := network
					if network == "tcp" {
						wantNetwork = "tcp4"
					}
					if !errors.Is(err, wantErr) || gotNetwork != wantNetwork || gotAddress != "example.invalid:443" {
						t.Fatalf("callback: network=%s address=%s err=%v", gotNetwork, gotAddress, err)
					}
				}
				if (modern && (contextCalls != 2 || legacyCalls != 0)) || (!modern && (legacyCalls != 2 || contextCalls != 0)) {
					t.Fatalf("callback priority changed: legacy=%d context=%d", legacyCalls, contextCalls)
				}
				if !modern && (original.DialContext != nil || original.DialTLSContext != nil) {
					t.Fatal("mutated original transport")
				}
			})
		}
	}
}

func newHTTPTestClient(t *testing.T, endpoint, caFile string) func() error {
	t.Helper()
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// Keep speculative dials from inflating exact fixture counts under load.
	// A closed connection still requires a new dial, so reuse remains observable.
	transport.MaxConnsPerHost = 1
	if caFile != "" {
		pem, err := os.ReadFile(caFile)
		if err != nil {
			t.Fatal(err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			t.Fatal("invalid test CA")
		}
		transport.TLSClientConfig = &tls.Config{RootCAs: pool}
	}
	client := HTTPClientFromEnv(&http.Client{Transport: transport, Timeout: 2 * time.Second})
	t.Cleanup(client.CloseIdleConnections)
	return func() error {
		resp, err := client.Post(endpoint, "application/octet-stream", bytes.NewBufferString("test"))
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		_, err = io.Copy(io.Discard, resp.Body)
		if err == nil && resp.StatusCode != http.StatusNoContent {
			return fmt.Errorf("unexpected status: %s", resp.Status)
		}
		return err
	}
}

func TestHTTPTransportSharedHTTP2(t *testing.T) {
	t.Setenv("PREFER_IPV4_STACK", "true")
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	for _, used := range []bool{false, true} {
		t.Run(fmt.Sprintf("already-used=%t", used), func(t *testing.T) {
			original := &http.Transport{}
			defer original.CloseIdleConnections()
			if used {
				plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
				defer plain.Close()
				resp, err := (&http.Client{Transport: original, Timeout: 2 * time.Second}).Get(plain.URL)
				if err != nil {
					t.Fatal(err)
				}
				resp.Body.Close()
			}
			// Clone the same source repeatedly, rather than cloning a prior result.
			for i := 0; i < 3; i++ {
				transport := HTTPTransportFromEnv(original)
				transport.TLSClientConfig.RootCAs = roots
				client := &http.Client{Transport: transport, Timeout: 2 * time.Second}
				resp, err := client.Get(server.URL)
				if err != nil {
					t.Fatalf("clone %d: %v", i, err)
				}
				resp.Body.Close()
				client.CloseIdleConnections()
				if resp.ProtoMajor != 2 {
					t.Fatalf("clone %d negotiated %s", i, resp.Proto)
				}
			}
		})
	}
	// Clone performs synchronized initialization. Concurrent wrappers must not
	// inspect fields while that initialization is writing them.
	original := &http.Transport{}
	defer original.CloseIdleConnections()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			transport := HTTPTransportFromEnv(original)
			transport.TLSClientConfig.RootCAs = roots
			client := &http.Client{Transport: transport, Timeout: 2 * time.Second}
			defer client.CloseIdleConnections()
			resp, err := client.Get(server.URL)
			if err != nil {
				t.Error(err)
				return
			}
			defer resp.Body.Close()
			if resp.ProtoMajor != 2 {
				t.Errorf("negotiated %s", resp.Proto)
			}
		}()
	}
	wg.Wait()
}

func TestHTTPTransportKeepsHTTP1Only(t *testing.T) {
	t.Setenv("PREFER_IPV4_STACK", "true")
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	for _, explicitMap := range []bool{false, true} {
		original := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots}}
		if explicitMap {
			original.TLSNextProto = make(map[string]func(string, *tls.Conn) http.RoundTripper)
		}
		for i := 0; i < 2; i++ {
			transport := HTTPTransportFromEnv(original)
			client := &http.Client{Transport: transport, Timeout: 2 * time.Second}
			resp, err := client.Get(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			client.CloseIdleConnections()
			if transport.ForceAttemptHTTP2 || resp.ProtoMajor != 1 {
				t.Fatalf("HTTP/1-only transport changed: %s", resp.Proto)
			}
		}
	}
}

func TestHTTPClientFromEnvDNS(t *testing.T)   { nettest.CheckHTTPClient(t, newHTTPTestClient) }
func TestHTTPClientFromEnvProxy(t *testing.T) { nettest.CheckHTTPProxy(t, newHTTPTestClient) }

func TestHTTPTransportFromEnvPreservesSettings(t *testing.T) {
	t.Setenv("PREFER_IPV4_STACK", "true")
	var network, address string
	type ctxKey struct{}
	ctx := context.WithValue(context.Background(), ctxKey{}, "retained")
	original := &http.Transport{
		MaxIdleConnsPerHost: 17, ResponseHeaderTimeout: 3 * time.Second,
		TLSClientConfig: &tls.Config{ServerName: "original.example"},
		DialContext: func(c context.Context, n, a string) (net.Conn, error) {
			if c.Value(ctxKey{}) != "retained" {
				t.Error("context lost")
			}
			network, address = n, a
			return nil, fmt.Errorf("test dial")
		},
	}
	transport := HTTPTransportFromEnv(original)
	if transport == original || transport.MaxIdleConnsPerHost != 17 || transport.ResponseHeaderTimeout != 3*time.Second || transport.TLSClientConfig.ServerName != "original.example" {
		t.Fatal("transport settings changed or original reused")
	}
	// Capture at construction; later environment changes cannot change this client.
	t.Setenv("PREFER_IPV4_STACK", "false")
	_, _ = transport.DialContext(ctx, "tcp", "original.example:443")
	if network != "tcp4" || address != "original.example:443" {
		t.Fatalf("dial got %s %s", network, address)
	}
	_, _ = original.DialContext(ctx, "tcp", "original.example:443")
	if network != "tcp" {
		t.Fatal("mutated original transport")
	}
	if HTTPTransportFromEnv(original) != original {
		t.Fatal("default mode changed transport")
	}
}

func TestHTTPTransportPreservesExplicitHTTP2Choice(t *testing.T) {
	t.Setenv("PREFER_IPV4_STACK", "true")
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	original := &http.Transport{Protocols: protocols}
	got := HTTPTransportFromEnv(original)
	if got.Protocols.HTTP2() || !got.Protocols.HTTP1() {
		t.Fatal("explicit protocol selection changed")
	}
}
