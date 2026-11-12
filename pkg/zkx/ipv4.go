// Package zkx adapts ZooKeeper's eager host-provider resolution.
package zkx

import (
	"context"
	"flashcat.cloud/categraf/pkg/netx"
	"github.com/samuel/go-zookeeper/zk"
	"net"
	"time"
)

type hostProvider struct {
	zk.DNSHostProvider
	lookup  func(context.Context, string) ([]string, error)
	timeout time.Duration
}

func (p *hostProvider) Init(servers []string) error {
	ctx := context.Background()
	if p.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.timeout)
		defer cancel()
	}
	var addresses []string
	for _, server := range servers {
		host, port, err := net.SplitHostPort(server)
		if err != nil {
			return err
		}
		ips, err := p.lookup(ctx, host)
		if err != nil {
			return err
		}
		for _, ip := range ips {
			addresses = append(addresses, net.JoinHostPort(ip, port))
		}
	}
	// The native provider retains its original shuffling and rotation. Literal
	// inputs here cannot trigger its old dual-stack domain lookup.
	return p.DNSHostProvider.Init(addresses)
}
func NewHostProvider(timeout time.Duration) zk.HostProvider {
	return NewHostProviderWithLookup(timeout, netx.LookupHostFromEnv(net.DefaultResolver))
}
func DialerFromEnv() zk.Dialer {
	policy := netx.NetworkPolicyFromEnv()
	return func(network, address string, timeout time.Duration) (net.Conn, error) {
		return (&net.Dialer{Timeout: timeout}).Dial(policy(network), address)
	}
}

// NewHostProviderWithLookup accepts the lookup captured by a parent subsystem.
func NewHostProviderWithLookup(timeout time.Duration, lookup func(context.Context, string) ([]string, error)) zk.HostProvider {
	return &hostProvider{lookup: lookup, timeout: timeout}
}
