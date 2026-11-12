package netx

import (
	"context"
	"net"
	"net/http"
)

// ConfigureHTTPTransportFromEnv configures a fresh, exclusively owned transport
// in place. SDKs retaining its identity for pool cleanup must use this instead
// of replacing the transport. Call it before the first request, never on a
// shared transport or concurrently with requests.
func ConfigureHTTPTransportFromEnv(transport *http.Transport) {
	configureHTTPTransport(transport, PreferIPv4())
}

// HTTPTransportConfigurerFromEnv captures the policy for SDKs which construct
// or clone their final transport lazily, including credential refresh clients.
func HTTPTransportConfigurerFromEnv() func(*http.Transport) {
	enabled := PreferIPv4()
	return func(transport *http.Transport) { configureHTTPTransport(transport, enabled) }
}

func configureHTTPTransport(transport *http.Transport, enabled bool) {
	if !enabled {
		return
	}
	configured := httpTransportWithPolicy(transport, true)
	transport.DialContext = configured.DialContext
	transport.DialTLSContext = configured.DialTLSContext
	transport.ForceAttemptHTTP2 = configured.ForceAttemptHTTP2
}

// HTTPTransportFromEnv returns a private transport when IPv4 mode is enabled,
// retaining TLS, proxy, timeout and protocol settings. Otherwise it returns the
// source unchanged. The source may have been used already, but must not be
// reconfigured concurrently with this call.
func HTTPTransportFromEnv(transport *http.Transport) *http.Transport {
	return httpTransportWithPolicy(transport, PreferIPv4())
}

func httpTransportWithPolicy(transport *http.Transport, enabled bool) *http.Transport {
	if !enabled {
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

// HTTPTransportConfigurerFromIPv4 returns a fixed IPv4 configurer for a policy
// that has already been captured by an owning SDK client.
func HTTPTransportConfigurerFromIPv4() func(*http.Transport) {
	return func(t *http.Transport) { configureHTTPTransport(t, true) }
}
