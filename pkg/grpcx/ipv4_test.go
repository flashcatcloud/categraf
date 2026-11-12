package grpcx

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"flashcat.cloud/categraf/internal/nettest"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"
)

type echoServer interface {
	Echo(context.Context, *emptypb.Empty) (*emptypb.Empty, error)
}
type echo struct{}

func (echo) Echo(context.Context, *emptypb.Empty) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}
func startEcho(t *testing.T, secure bool) (string, credentials.TransportCredentials) {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var opts []grpc.ServerOption
	var creds credentials.TransportCredentials = insecure.NewCredentials()
	if secure {
		cfg, caFile := nettest.TLSFixture(t)
		opts = append(opts, grpc.Creds(credentials.NewTLS(cfg)))
		pem, err := os.ReadFile(caFile)
		if err != nil {
			t.Fatal(err)
		}
		roots := x509.NewCertPool()
		roots.AppendCertsFromPEM(pem)
		creds = credentials.NewClientTLSFromCert(roots, "example.com")
	}
	server := grpc.NewServer(opts...)
	server.RegisterService(&grpc.ServiceDesc{ServiceName: "test.Echo", HandlerType: (*echoServer)(nil), Methods: []grpc.MethodDesc{{MethodName: "Echo", Handler: func(srv any, ctx context.Context, decode func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
		in := new(emptypb.Empty)
		if err := decode(in); err != nil {
			return nil, err
		}
		return srv.(echoServer).Echo(ctx, in)
	}}}}, echo{})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	return listener.Addr().String(), creds
}
func invoke(t *testing.T, target string, creds credentials.TransportCredentials, extra ...grpc.DialOption) error {
	t.Helper()
	opts := []grpc.DialOption{grpc.WithTransportCredentials(creds)}
	opts = append(opts, DialOptionsFromEnv()...)
	opts = append(opts, extra...)
	conn, err := grpc.NewClient(target, opts...)
	if err != nil {
		return err
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 1200*time.Millisecond)
	defer cancel()
	for i := 0; i < 2; i++ {
		if err := conn.Invoke(ctx, "/test.Echo/Echo", &emptypb.Empty{}, &emptypb.Empty{}); err != nil {
			return err
		}
	}
	return nil
}
func TestDirectRPCPolicy(t *testing.T) {
	for _, key := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy", "NO_PROXY", "no_proxy"} {
		t.Setenv(key, "")
	}
	for _, scheme := range []string{"dns", "passthrough"} {
		for _, secure := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/TLS=%t", scheme, secure), func(t *testing.T) {
				t.Setenv("PREFER_IPV4_STACK", "true")
				counts := nettest.ObserveDNS(t, "example.com.", net.IPv4(127, 0, 0, 1), net.IPv6loopback)
				addr, creds := startEcho(t, secure)
				_, port, _ := net.SplitHostPort(addr)
				if err := invoke(t, scheme+":///example.com.:"+port, creds); err != nil {
					t.Fatal(err)
				}
				a, aaaa := counts()
				if a == 0 || aaaa != 0 {
					t.Fatalf("A=%d AAAA=%d", a, aaaa)
				}
			})
		}
	}
}

// Native proxy selection caches the process environment. Each case runs in its
// own process to verify the actual SDK behavior without registry mutation.
func TestNativeProxyPolicy(t *testing.T) {
	if mode := os.Getenv("CATEGRAF_GRPC_PROXY_CASE"); mode != "" {
		runProxyCase(t, mode)
		return
	}
	for _, mode := range []string{"remote", "lowercase", "off", "407", "502", "cancel", "ipv6proxy", "unreachable", "no_proxy_domain", "no_proxy_ip", "no_proxy_cidr", "localhost", "with_no_proxy", "local_dns"} {
		t.Run(mode, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestNativeProxyPolicy$", "-test.timeout=15s")
			cmd.Env = append(os.Environ(), "CATEGRAF_GRPC_PROXY_CASE="+mode)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%v\n%s", err, output)
			}
		})
	}
}
func runProxyCase(t *testing.T, mode string) {
	for _, key := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy", "NO_PROXY", "no_proxy"} {
		t.Setenv(key, "")
	}
	t.Setenv("PREFER_IPV4_STACK", "true")
	if mode == "off" {
		t.Setenv("PREFER_IPV4_STACK", "false")
	}
	ip := net.IPv4(127, 0, 0, 1)
	if mode == "ipv6proxy" {
		ip = nil
	}
	if mode == "local_dns" {
		ip = net.IPv4(192, 0, 2, 1)
	}
	queries := nettest.ObserveDNSQueries(t, "example.com.", ip, net.IPv6loopback)
	backend, creds := startEcho(t, false)
	_, port, _ := net.SplitHostPort(backend)
	target := "forbidden-direct.invalid.:" + port
	var calls atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodConnect {
			t.Error("expected CONNECT")
			w.WriteHeader(400)
			return
		}
		if r.Header.Get("Proxy-Authorization") != "Basic "+base64.StdEncoding.EncodeToString([]byte("user:password")) {
			t.Error("missing Basic proxy auth")
			w.WriteHeader(407)
			return
		}
		if mode == "407" || mode == "502" {
			if mode == "407" {
				w.WriteHeader(407)
			} else {
				w.WriteHeader(502)
			}
			return
		}
		if mode == "cancel" {
			h := w.(http.Hijacker)
			conn, _, _ := h.Hijack()
			defer conn.Close()
			_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
			_, _ = io.Copy(io.Discard, conn)
			return
		}
		if mode == "local_dns" {
			host, _, _ := net.SplitHostPort(r.Host)
			if net.ParseIP(host) == nil {
				t.Errorf("expected resolved CONNECT authority: %s", r.Host)
			}
		} else if r.Host != target {
			t.Errorf("CONNECT authority=%s want %s", r.Host, target)
		}
		upstream, err := net.Dial("tcp", backend)
		if err != nil {
			t.Error(err)
			w.WriteHeader(502)
			return
		}
		downstream, buffer, err := w.(http.Hijacker).Hijack()
		if err != nil {
			upstream.Close()
			return
		}
		defer downstream.Close()
		defer upstream.Close()
		_, _ = buffer.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
		_ = buffer.Flush()
		done := make(chan struct{})
		go func() { _, _ = io.Copy(upstream, buffer); upstream.Close(); close(done) }()
		_, _ = io.Copy(downstream, upstream)
		downstream.Close()
		<-done
	}))
	defer proxy.Close()
	proxyURL := strings.Replace(proxy.URL, "127.0.0.1", "example.com.", 1)
	if mode == "local_dns" {
		proxyURL = proxy.URL
	}
	proxyURL = strings.Replace(proxyURL, "http://", "http://user:password@", 1)
	if mode == "unreachable" {
		proxyURL = "http://user:password@example.com.:1"
	}
	key := "HTTPS_PROXY"
	if mode == "lowercase" {
		key = "https_proxy"
	}
	t.Setenv(key, proxyURL)
	var opts []grpc.DialOption
	direct := false
	switch mode {
	case "no_proxy_domain":
		target = "example.com.:" + port
		t.Setenv("NO_PROXY", "example.com.")
		direct = true
	case "no_proxy_ip":
		target = backend
		t.Setenv("NO_PROXY", "127.0.0.1")
		direct = true
	case "no_proxy_cidr":
		target = backend
		t.Setenv("NO_PROXY", "127.0.0.0/8")
		direct = true
	case "localhost":
		target = "localhost:" + port
		direct = true
	case "with_no_proxy":
		target = "example.com.:" + port
		opts = append(opts, grpc.WithNoProxy())
		direct = true
	case "local_dns":
		target = "example.com.:" + port
		opts = append(opts, grpc.WithLocalDNSResolution())
	}
	err := invoke(t, "dns:///"+target, creds, opts...)
	fail := mode == "407" || mode == "502" || mode == "cancel" || mode == "ipv6proxy" || mode == "unreachable"
	if fail && err == nil || !fail && err != nil {
		t.Fatalf("RPC error=%v", err)
	}
	if direct && calls.Load() != 0 {
		t.Fatalf("unexpected proxy calls=%d", calls.Load())
	}
	if !direct && !fail && calls.Load() == 0 {
		t.Fatal("proxy bypassed")
	}
	a, aaaa := queries("example.com.")
	if mode != "off" && aaaa != 0 {
		t.Fatalf("proxy/target A=%d AAAA=%d", a, aaaa)
	}
	if !direct && mode != "local_dns" {
		a, aaaa = queries("forbidden-direct.invalid.")
		if a != 0 || aaaa != 0 {
			t.Fatalf("locally resolved remote target: A=%d AAAA=%d", a, aaaa)
		}
	}
}
