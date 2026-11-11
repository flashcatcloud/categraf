//go:build !no_logs

package docker

import (
	"net/http"
	"os"

	"flashcat.cloud/categraf/pkg/netx"
	"github.com/docker/go-connections/sockets"
	"github.com/moby/moby/client"
)

// newClientFromEnv retains Docker's TLS, proxy, socket and API negotiation
// settings, adding the address-family policy only for remote TCP daemons.
func newClientFromEnv() (*client.Client, error) {
	opts := []client.Opt{client.FromEnv}
	if host := os.Getenv(client.EnvOverrideHost); host != "" && netx.PreferIPv4() {
		u, err := client.ParseHostURL(host)
		if err != nil {
			return nil, err
		}
		if u.Scheme != "unix" && u.Scheme != "npipe" {
			transport := &http.Transport{}
			if err := sockets.ConfigureTransport(transport, u.Scheme, u.Host); err != nil {
				return nil, err
			}
			opts = append(opts, client.WithDialContext(netx.HTTPTransportFromEnv(transport).DialContext))
		}
	}
	return client.New(opts...)
}
