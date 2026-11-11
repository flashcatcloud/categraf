package netx

import (
	"context"
	"net"
)

// LookupIPAddr resolves only A records when IPv4-only mode is enabled. Keep the
// standard lookup otherwise, including its IPv6 zone handling and address order.
func LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	if !PreferIPv4() {
		return net.DefaultResolver.LookupIPAddr(ctx, host)
	}
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip4", host)
	if err != nil {
		return nil, err
	}
	addrs := make([]net.IPAddr, len(ips))
	for i, ip := range ips {
		addrs[i] = net.IPAddr{IP: ip}
	}
	return addrs, nil
}
