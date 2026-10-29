package netx

import (
	"context"
	"errors"
	"net"
	"os"
	"testing"
	"time"
)

func ipv6Listener(t *testing.T) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Skipf("IPv6 loopback unavailable: %v", err)
	}
	t.Cleanup(func() { listener.Close() })
	return listener
}

func TestDialContextFromEnvValues(t *testing.T) {
	listener := ipv6Listener(t)
	for _, value := range []string{"unset", "", "false", "true", "TRUE", "1", " true"} {
		t.Run("env="+value, func(t *testing.T) {
			t.Setenv("PREFER_IPV4_STACK", value)
			if value == "unset" {
				if err := os.Unsetenv("PREFER_IPV4_STACK"); err != nil {
					t.Fatal(err)
				}
			}
			dial := DialContextFromEnv(&net.Dialer{Timeout: time.Second})
			conn, err := dial(context.Background(), "tcp", listener.Addr().String())
			if conn != nil {
				conn.Close()
			}
			if value == "true" && err == nil {
				t.Fatal("IPv4-only dial unexpectedly reached IPv6 literal")
			}
			if value != "true" && err != nil {
				t.Fatalf("default dial should reach IPv6 literal: %v", err)
			}
		})
	}
}

func TestDialContextFromEnvCapturesEnvironment(t *testing.T) {
	listener := ipv6Listener(t)
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "initially-disabled", true: "initially-enabled"}[enabled], func(t *testing.T) {
			value, next := "false", "true"
			if enabled {
				value, next = next, value
			}
			t.Setenv("PREFER_IPV4_STACK", value)
			dial := DialContextFromEnv(&net.Dialer{Timeout: time.Second})
			t.Setenv("PREFER_IPV4_STACK", next)
			conn, err := dial(context.Background(), "tcp", listener.Addr().String())
			if conn != nil {
				conn.Close()
			}
			if (err != nil) != enabled {
				t.Fatalf("existing dialer changed with environment: enabled=%t err=%v", enabled, err)
			}
			conn, err = DialContextFromEnv(&net.Dialer{Timeout: time.Second})(context.Background(), "tcp", listener.Addr().String())
			if conn != nil {
				conn.Close()
			}
			if (err != nil) == enabled {
				t.Fatalf("new dialer did not pick up environment: enabled=%t err=%v", enabled, err)
			}
		})
	}
}

func TestDialContextFromEnvPreservesExplicitNetworks(t *testing.T) {
	t.Setenv("PREFER_IPV4_STACK", "true")
	for _, network := range []string{"tcp4", "tcp6", "udp4", "udp6"} {
		t.Run(network, func(t *testing.T) {
			address := "127.0.0.1:0"
			if network[len(network)-1] == '6' {
				address = "[::1]:0"
			}
			if network[:3] == "tcp" {
				listener, err := net.Listen(network, address)
				if err != nil {
					if network == "tcp6" {
						t.Skipf("IPv6 unavailable: %v", err)
					}
					t.Fatal(err)
				}
				defer listener.Close()
				address = listener.Addr().String()
			} else {
				listener, err := net.ListenPacket(network, address)
				if err != nil {
					if network == "udp6" {
						t.Skipf("IPv6 unavailable: %v", err)
					}
					t.Fatal(err)
				}
				defer listener.Close()
				address = listener.LocalAddr().String()
			}
			conn, err := DialContextFromEnv(&net.Dialer{Timeout: time.Second})(context.Background(), network, address)
			if err != nil {
				t.Fatalf("explicit network %s was not preserved: %v", network, err)
			}
			conn.Close()
		})
	}
}

func TestDialContextFromEnvCancellationAndTimeout(t *testing.T) {
	t.Setenv("PREFER_IPV4_STACK", "true")
	t.Run("canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := DialContextFromEnv(&net.Dialer{})(ctx, "tcp", "127.0.0.1:1")
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v, want context.Canceled", err)
		}
	})
	t.Run("dialer-timeout", func(t *testing.T) {
		dialer := &net.Dialer{
			Timeout: 50 * time.Millisecond,
			Resolver: &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			}},
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		start := time.Now()
		_, err := DialContextFromEnv(dialer)(ctx, "tcp", "timeout.invalid.:80")
		var netErr net.Error
		if !errors.As(err, &netErr) || !netErr.Timeout() || time.Since(start) >= time.Second {
			t.Fatalf("dialer timeout not preserved: elapsed=%v err=%v", time.Since(start), err)
		}
	})
}
