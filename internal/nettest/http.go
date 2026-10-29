// Package nettest provides local DNS and HTTP fixtures for client integration tests.
package nettest

import (
	"context"
	"crypto/tls"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/miekg/dns"
)

// ClientFactory creates the actual application client and registers its cleanup.
// send must POST to endpoint and consume and close the response body.
type ClientFactory func(t *testing.T, endpoint, caFile string) (send func() error)

func clearProxyEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "all_proxy", "no_proxy"} {
		t.Setenv(key, "")
	}
}

type dnsCounts struct {
	a    atomic.Int64
	aaaa atomic.Int64
}

// localDNS normally redirects the Go resolver without touching system files.
// CATEGRAF_TEST_SYSTEM_DNS=1 instead uses the real system resolver and port 53.
// That opt-in mode must run in an isolated network/mount namespace with its own
// resolv.conf; it permits testing both Go and cgo without altering the host DNS.
func localDNS(t *testing.T, host string, ipv4, ipv6 net.IP) *dnsCounts {
	t.Helper()
	address := "127.0.0.1:0"
	systemDNS := os.Getenv("CATEGRAF_TEST_SYSTEM_DNS") == "1"
	if systemDNS {
		address = "127.0.0.1:53"
	}
	pc, err := net.ListenPacket("udp4", address)
	if err != nil {
		t.Fatal(err)
	}
	counts := new(dnsCounts)
	ready := make(chan struct{})
	server := &dns.Server{
		PacketConn:        pc,
		NotifyStartedFunc: func() { close(ready) },
		Handler: dns.HandlerFunc(func(w dns.ResponseWriter, req *dns.Msg) {
			reply := new(dns.Msg)
			reply.SetReply(req)
			reply.Authoritative = true
			for _, q := range req.Question {
				if q.Name != dns.Fqdn(host) {
					reply.Rcode = dns.RcodeNameError
					continue
				}
				header := dns.RR_Header{Name: q.Name, Rrtype: q.Qtype, Class: dns.ClassINET, Ttl: 60}
				switch q.Qtype {
				case dns.TypeA:
					counts.a.Add(1)
					if ipv4 != nil {
						reply.Answer = append(reply.Answer, &dns.A{Hdr: header, A: ipv4})
					}
				case dns.TypeAAAA:
					counts.aaaa.Add(1)
					if ipv6 != nil {
						reply.Answer = append(reply.Answer, &dns.AAAA{Hdr: header, AAAA: ipv6})
					}
				}
			}
			if err := w.WriteMsg(reply); err != nil {
				t.Errorf("DNS response: %v", err)
			}
		}),
	}
	done := make(chan error, 1)
	go func() { done <- server.ActivateAndServe() }()
	<-ready
	t.Cleanup(func() {
		if err := server.Shutdown(); err != nil {
			t.Error(err)
		}
		if err := <-done; err != nil {
			t.Error(err)
		}
	})
	if !systemDNS {
		original := net.DefaultResolver
		net.DefaultResolver = &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
				return (&net.Dialer{Timeout: time.Second}).DialContext(ctx, network, pc.LocalAddr().String())
			},
		}
		t.Cleanup(func() { net.DefaultResolver = original })
	}
	return counts
}

func setIPv4Env(t *testing.T, value string) {
	t.Helper()
	t.Setenv("PREFER_IPV4_STACK", value)
	if value == "unset" {
		if err := os.Unsetenv("PREFER_IPV4_STACK"); err != nil {
			t.Fatal(err)
		}
	}
}

// CheckHTTPClient counts actual DNS questions through the supplied client. It
// must not run in parallel because it changes environment and resolver state.
func CheckHTTPClient(t *testing.T, newClient ClientFactory) {
	t.Helper()
	clearProxyEnv(t)
	for _, value := range []string{"unset", "false", "true"} {
		for _, secure := range []bool{false, true} {
			for _, closeConn := range []bool{false, true} {
				name := fmt.Sprintf("env=%s/https=%t/close=%t", value, secure, closeConn)
				t.Run(name, func(t *testing.T) {
					setIPv4Env(t, value)
					counts := localDNS(t, "example.com.", net.IPv4(127, 0, 0, 1), nil)
					server, connections, caFile := httpServer(t, secure, closeConn, false)
					endpoint := strings.Replace(server.URL, "127.0.0.1", "example.com.", 1)
					send := newClient(t, endpoint, caFile)
					for i := 0; i < 5; i++ {
						if err := send(); err != nil {
							t.Fatal(err)
						}
					}
					want := int64(1)
					if closeConn {
						want = 5
					}
					wantAAAA := want
					if value == "true" {
						wantAAAA = 0
					}
					t.Logf("requests=5 connections=%d A=%d AAAA=%d", connections.Load(), counts.a.Load(), counts.aaaa.Load())
					if connections.Load() != want || counts.a.Load() != want || counts.aaaa.Load() != wantAAAA {
						t.Fatalf("want connections=%d A=%d AAAA=%d", want, want, wantAAAA)
					}
				})
			}
		}
	}
	t.Run("ipv4-literal", func(t *testing.T) {
		setIPv4Env(t, "true")
		counts := localDNS(t, "example.com.", net.IPv4(127, 0, 0, 1), nil)
		server, _, _ := httpServer(t, false, false, false)
		if err := newClient(t, server.URL, "")(); err != nil {
			t.Fatal(err)
		}
		if counts.a.Load() != 0 || counts.aaaa.Load() != 0 {
			t.Fatal("literal IP caused DNS queries")
		}
	})
	for _, value := range []string{"false", "true"} {
		t.Run("ipv6-only/env="+value, func(t *testing.T) {
			setIPv4Env(t, value)
			server, _, _ := httpServer(t, false, false, true)
			counts := localDNS(t, "example.com.", nil, net.IPv6loopback)
			endpoint := strings.Replace(server.URL, "[::1]", "example.com.", 1)
			err := newClient(t, endpoint, "")()
			if value == "true" {
				if err == nil || counts.a.Load() == 0 || counts.aaaa.Load() != 0 {
					t.Fatalf("IPv4-only must fail without IPv6 fallback: err=%v A=%d AAAA=%d", err, counts.a.Load(), counts.aaaa.Load())
				}
			} else if err != nil || counts.aaaa.Load() == 0 {
				t.Fatalf("default mode must reach IPv6-only server: err=%v AAAA=%d", err, counts.aaaa.Load())
			}
		})
	}
	t.Run("untrusted-https", func(t *testing.T) {
		setIPv4Env(t, "true")
		localDNS(t, "example.com.", net.IPv4(127, 0, 0, 1), nil)
		server, _, _ := httpServer(t, true, false, false)
		endpoint := strings.Replace(server.URL, "127.0.0.1", "example.com.", 1)
		if err := newClient(t, endpoint, "")(); err == nil {
			t.Fatal("untrusted TLS certificate was accepted")
		}
	})
}

func httpServer(t *testing.T, secure, closeConn, ipv6 bool) (*httptest.Server, *atomic.Int64, string) {
	t.Helper()
	connections := new(atomic.Int64)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			t.Error(err)
		}
		if closeConn {
			w.Header().Set("Connection", "close")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	if ipv6 {
		if err := server.Listener.Close(); err != nil {
			t.Fatal(err)
		}
		listener, err := net.Listen("tcp6", "[::1]:0")
		if err != nil {
			t.Skipf("IPv6 loopback unavailable: %v", err)
		}
		server.Listener = listener
	}
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			connections.Add(1)
		}
	}
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	caFile := ""
	if secure {
		server.TLS = &tls.Config{GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
			if hello.ServerName != "example.com" {
				return nil, fmt.Errorf("unexpected SNI %q", hello.ServerName)
			}
			return nil, nil
		}}
		server.StartTLS()
		caFile = filepath.Join(t.TempDir(), "ca.pem")
		certificate := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
		if err := os.WriteFile(caFile, certificate, 0600); err != nil {
			t.Fatal(err)
		}
	} else {
		server.Start()
	}
	t.Cleanup(server.Close)
	return server, connections, caFile
}

// CheckHTTPProxy uses a subprocess to avoid http.ProxyFromEnvironment's process-
// wide environment cache, then tests the real environment-based proxy path.
func CheckHTTPProxy(t *testing.T, newClient ClientFactory) {
	t.Helper()
	if os.Getenv("CATEGRAF_TEST_PROXY_CHILD") != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^"+regexp.QuoteMeta(t.Name())+"$", "-test.v")
		cmd.Env = append(os.Environ(), "CATEGRAF_TEST_PROXY_CHILD=1")
		out, err := cmd.CombinedOutput()
		t.Logf("%s", out)
		if err != nil {
			t.Fatal(err)
		}
		return
	}
	clearProxyEnv(t)
	counts := localDNS(t, "proxy.example.test.", net.IPv4(127, 0, 0, 1), nil)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Host != "backend.invalid" || r.Header.Get("Proxy-Authorization") != "Basic dXNlcjpwYXNz" {
			t.Errorf("proxy request: URL=%s proxy authorization=%q", r.URL, r.Header.Get("Proxy-Authorization"))
		}
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			t.Error(err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(proxy.Close)
	proxyURL := strings.Replace(proxy.URL, "127.0.0.1", "user:pass@proxy.example.test.", 1)
	t.Setenv("HTTP_PROXY", proxyURL)
	for _, value := range []string{"false", "true"} {
		t.Run("env="+value, func(t *testing.T) {
			setIPv4Env(t, value)
			beforeA, beforeAAAA := counts.a.Load(), counts.aaaa.Load()
			if err := newClient(t, "http://backend.invalid/write", "")(); err != nil {
				t.Fatal(err)
			}
			a, aaaa := counts.a.Load()-beforeA, counts.aaaa.Load()-beforeAAAA
			t.Logf("proxy A=%d AAAA=%d", a, aaaa)
			if a != 1 || (value == "true" && aaaa != 0) || (value == "false" && aaaa != 1) {
				t.Fatalf("unexpected proxy DNS queries: A=%d AAAA=%d", a, aaaa)
			}
		})
	}
}
