package moby

import (
	"context"
	"github.com/docker/go-connections/sockets"
	"github.com/moby/moby/client"
	"net"
	"net/http"
	"net/url"
)

// Reuse Docker's native TCP dialer defaults and modify only its network. This
// option is deliberately not applied to Unix sockets, named pipes or decorated
// HTTP transports, which keep their existing SDK and HTTP configuration.
func tcpNetworkPolicyOption(host *url.URL, networkPolicy func(string) string) (client.Opt, error) {
	transport := &http.Transport{}
	if err := sockets.ConfigureTransport(transport, host.Scheme, host.Host); err != nil {
		return nil, err
	}
	dial := transport.DialContext
	return client.WithDialContext(func(ctx context.Context, network, address string) (net.Conn, error) {
		return dial(ctx, networkPolicy(network), address)
	}), nil
}
