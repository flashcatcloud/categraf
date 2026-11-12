// Package dockerx contains shared Docker SDK connection options.
package dockerx

import (
	"net/http"

	"flashcat.cloud/categraf/pkg/netx"
	"github.com/docker/go-connections/sockets"
	"github.com/moby/moby/client"
)

// IPv4Options must be appended after host and TLS options: WithHost replaces
// the dial callback. Local sockets keep Docker's platform-specific transport.
func IPv4Options(host string) ([]client.Opt, error) {
	if !netx.PreferIPv4() || host == "" {
		return nil, nil
	}
	u, err := client.ParseHostURL(host)
	if err != nil {
		return nil, err
	}
	if u.Scheme == "unix" || u.Scheme == "npipe" {
		return nil, nil
	}
	transport := &http.Transport{}
	if err := sockets.ConfigureTransport(transport, u.Scheme, u.Host); err != nil {
		return nil, err
	}
	return []client.Opt{client.WithDialContext(netx.HTTPTransportFromEnv(transport).DialContext)}, nil
}
