//go:build !no_logs

package docker

import (
	"os"

	"flashcat.cloud/categraf/pkg/dockerx"
	"github.com/moby/moby/client"
)

// newClientFromEnv retains Docker's TLS, proxy, socket and API negotiation
// settings, adding the address-family policy only for remote TCP daemons.
func newClientFromEnv() (*client.Client, error) {
	opts, err := dockerx.IPv4Options(os.Getenv(client.EnvOverrideHost))
	if err != nil {
		return nil, err
	}
	return client.New(append([]client.Opt{client.FromEnv}, opts...)...)
}
