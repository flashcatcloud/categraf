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
	dial := t.DialContext
	if dial == nil {
		legacyDial := t.Dial //nolint:staticcheck // Preserve caller-provided legacy callbacks when DialContext is absent.
		if legacyDial != nil {
			dial = func(_ context.Context, network, addr string) (net.Conn, error) {
				return legacyDial(network, addr)
			}
		} else {
			dial = (&net.Dialer{}).DialContext
		}
	}
	t.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return dial(ctx, networkForIPv4(network, true), addr)
	}
	dialTLS := t.DialTLSContext
	if dialTLS == nil {
		legacyDialTLS := t.DialTLS //nolint:staticcheck // Preserve caller-provided legacy TLS callbacks when DialTLSContext is absent.
		if legacyDialTLS != nil {
			dialTLS = func(_ context.Context, network, addr string) (net.Conn, error) {
				return legacyDialTLS(network, addr)
			}
		}
	}
	if dialTLS != nil {
		t.DialTLSContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialTLS(ctx, networkForIPv4(network, true), addr)
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
