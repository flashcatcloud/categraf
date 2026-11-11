package netx

import (
	"context"
	"net"
	"os"
	"time"
)

// PreferIPv4 reports whether the process opted into IPv4-only address selection.
func PreferIPv4() bool { return os.Getenv("PREFER_IPV4_STACK") == "true" }

func networkForIPv4(network string, enabled bool) string {
	if enabled {
		switch network {
		case "tcp", "udp", "ip":
			return network + "4"
		}
	}
	return network
}

// NetworkFromEnv preserves explicit address families and non-IP networks.
// Use it before resolution as well as dialing; filtering resolved results is too late.
func NetworkFromEnv(network string) string {
	return networkForIPv4(network, PreferIPv4())
}

// Dialer captures the address-family policy while retaining the caller's dialer.
// It also implements the Dial and DialContext interfaces used by SDKs and proxies.
type Dialer struct {
	dialer *net.Dialer
	ipv4   bool
}

func NewDialer(dialer *net.Dialer) *Dialer {
	return &Dialer{dialer: dialer, ipv4: PreferIPv4()}
}

func (d *Dialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return d.dialer.DialContext(ctx, networkForIPv4(network, d.ipv4), addr)
}

func (d *Dialer) Dial(network, addr string) (net.Conn, error) {
	return d.DialContext(context.Background(), network, addr)
}

func Dial(network, addr string) (net.Conn, error) {
	return NewDialer(&net.Dialer{}).Dial(network, addr)
}

func DialTimeout(network, addr string, timeout time.Duration) (net.Conn, error) {
	return NewDialer(&net.Dialer{Timeout: timeout}).Dial(network, addr)
}

// DialContextFromEnv wraps a non-nil dialer, using IPv4 for ordinary TCP/UDP
// connections when PREFER_IPV4_STACK is exactly "true". The environment is
// captured when the wrapper is created; explicitly selected networks are kept.
func DialContextFromEnv(dialer *net.Dialer) func(context.Context, string, string) (net.Conn, error) {
	return NewDialer(dialer).DialContext
}
