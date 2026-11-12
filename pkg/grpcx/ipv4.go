// Package grpcx applies address-family policies without replacing gRPC's proxy,
// DNS service configuration, balancers or native keepalive behavior.
package grpcx

import (
	"context"
	"flashcat.cloud/categraf/pkg/netx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/resolver/dns"
	"net"
)

// DialOptionsFromEnv captures the policy for one ClientConn. Explicit custom
// resolvers and custom dialers retain their own contracts; they are not replaced.
func DialOptionsFromEnv() []grpc.DialOption {
	if !netx.PreferIPv4() {
		return nil
	}
	builder := dns.NewBuilderWithLookupHost(func(ctx context.Context, resolver *net.Resolver, host string) ([]string, error) {
		ips, err := resolver.LookupIP(ctx, "ip4", host)
		if err != nil {
			return nil, err
		}
		addresses := make([]string, len(ips))
		for i, ip := range ips {
			addresses[i] = ip.String()
		}
		return addresses, nil
	})
	return []grpc.DialOption{grpc.WithNetworkPolicy(netx.NetworkPolicyFromEnv()), grpc.WithResolvers(builder), grpc.WithProxyResolverBuilder(builder)}
}
