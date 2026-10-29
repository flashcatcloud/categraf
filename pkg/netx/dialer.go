package netx

import (
	"context"
	"net"
	"os"
)

// DialContextFromEnv wraps a non-nil dialer, using IPv4 for ordinary TCP
// connections when PREFER_IPV4_STACK is exactly "true". The environment is
// captured when the wrapper is created; explicitly selected networks are kept.
func DialContextFromEnv(dialer *net.Dialer) func(context.Context, string, string) (net.Conn, error) {
	preferIPv4 := os.Getenv("PREFER_IPV4_STACK") == "true"
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		if preferIPv4 && network == "tcp" {
			network = "tcp4"
		}
		return dialer.DialContext(ctx, network, addr)
	}
}
