//go:build !no_prometheus

package prometheus

import (
	"net"
	"net/http"
	"sync"
	"time"

	internalaws "flashcat.cloud/categraf/pkg/aws"
	"flashcat.cloud/categraf/pkg/googlex"
	"flashcat.cloud/categraf/pkg/netx"
	"flashcat.cloud/categraf/pkg/zkx"
	"github.com/go-zookeeper/zk"
	commonconfig "github.com/prometheus/common/config"
	"github.com/prometheus/prometheus/util/httppolicy"
)

// newNetworkPolicy captures the policy before any subsystem creates clients.
// HTTP options also apply to common/config's independent OAuth token transport.
func newNetworkPolicy() (*httppolicy.Policy, func()) {
	if !netx.PreferIPv4() {
		return nil, func() {}
	}
	credentials := googlex.FromEnv()
	var ownedMu sync.Mutex
	var owned []*http.Client
	dial := netx.DialFuncFromEnv((&net.Dialer{}).DialContext)
	network := netx.NetworkPolicyFromEnv()
	lookup := netx.LookupHostIPv4(net.DefaultResolver)
	return &httppolicy.Policy{
			HTTPClientOptions:   []commonconfig.HTTPClientOption{commonconfig.WithDialContextFunc(dial), commonconfig.WithOAuth2DialContextFunc(dial)},
			ConfigureHTTPClient: credentials.Client,
			OwnHTTPClient:       func(client *http.Client) { ownedMu.Lock(); defer ownedMu.Unlock(); owned = append(owned, client) },
			LookupHost:          lookup,
			Network:             network,
			AWSOptions:          internalaws.HTTPClientOptionsFromEnv(),
			GoogleOptions:       credentials.Options(),
			ZKHostProvider:      func(timeout time.Duration) zk.HostProvider { return zkx.NewHostProviderWithLookup(timeout, lookup) },
			ZKDialer: func(networkName, address string, timeout time.Duration) (net.Conn, error) {
				return (&net.Dialer{Timeout: timeout}).Dial(network(networkName), address)
			},
		}, func() {
			credentials.Close()
			ownedMu.Lock()
			defer ownedMu.Unlock()
			for _, client := range owned {
				client.CloseIdleConnections()
			}
		}
}
