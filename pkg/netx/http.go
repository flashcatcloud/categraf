package netx

import (
	"context"
	"net"
	"net/http"
)

// HTTPTransportFromEnv returns a private transport when IPv4 mode is enabled,
// retaining TLS, proxy, timeout and protocol settings. Otherwise it returns the
// source unchanged. The source may have been used already, but must not be
// reconfigured concurrently with this call.
func HTTPTransportFromEnv(transport *http.Transport) *http.Transport {
	if !PreferIPv4() {
		return transport
	}
	t := transport.Clone()
	// Clone initializes the source's protocol defaults. Inspect the initialized
	// HTTP/2 handler rather than guessing from fields that initialization changes.
	// Otherwise a second clone can advertise h2 without an HTTP/2 transport.
	if t.Protocols == nil && transport.TLSNextProto["h2"] != nil {
		t.ForceAttemptHTTP2 = true
	}
	if t.DialContext != nil {
		dial := t.DialContext
		t.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dial(ctx, networkForIPv4(network, true), addr)
		}
	} else if t.Dial != nil {
		dial := t.Dial
		t.Dial = func(network, addr string) (net.Conn, error) {
			return dial(networkForIPv4(network, true), addr)
		}
	} else {
		t.DialContext = DialContextFromEnv(&net.Dialer{})
	}
	if t.DialTLSContext != nil {
		dial := t.DialTLSContext
		t.DialTLSContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dial(ctx, networkForIPv4(network, true), addr)
		}
	} else if t.DialTLS != nil {
		dial := t.DialTLS
		t.DialTLS = func(network, addr string) (net.Conn, error) {
			return dial(networkForIPv4(network, true), addr)
		}
	}
	return t
}

// HTTPClientFromEnv preserves client settings and wraps standard transports.
// Custom RoundTrippers must apply the policy to their underlying dialer instead.
func HTTPClientFromEnv(client *http.Client) *http.Client {
	if !PreferIPv4() {
		return client
	}
	c := *client
	transport := c.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	if t, ok := transport.(*http.Transport); ok {
		c.Transport = HTTPTransportFromEnv(t)
	}
	return &c
}
