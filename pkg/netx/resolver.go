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

// LookupHostFromEnv captures the policy before an SDK's pre-dial resolution.
// It returns every IPv4 address, retaining the resolver's context and errors.
func LookupHostFromEnv(resolver *net.Resolver) func(context.Context, string) ([]string, error) {
	if !PreferIPv4() {
		return resolver.LookupHost
	}
	return func(ctx context.Context, host string) ([]string, error) {
		ips, err := resolver.LookupIP(ctx, "ip4", host)
		if err != nil {
			return nil, err
		}
		addresses := make([]string, len(ips))
		for i, ip := range ips {
			addresses[i] = ip.String()
		}
		return addresses, nil
	}
}

// LookupHostIPv4 resolves all IPv4 addresses through the supplied resolver.
func LookupHostIPv4(resolver *net.Resolver) func(context.Context, string) ([]string, error) {
	return func(ctx context.Context, host string) ([]string, error) {
		ips, err := resolver.LookupIP(ctx, "ip4", host)
		if err != nil {
			return nil, err
		}
		out := make([]string, len(ips))
		for i, ip := range ips {
			out[i] = ip.String()
		}
		return out, nil
	}
}
