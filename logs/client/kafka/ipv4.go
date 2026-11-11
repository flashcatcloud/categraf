//go:build !no_logs

package kafka

import (
	"context"
	"fmt"
	"net"
	"strings"

	"flashcat.cloud/categraf/pkg/netx"
	"github.com/IBM/sarama"
)

// prepareIPv4Config keeps shared configuration intact: each destination must
// canonicalize its own bootstrap addresses, including after a client rebuild.
func prepareIPv4Config(cfg *sarama.Config, brokers []string) (*sarama.Config, []string, error) {
	if !netx.PreferIPv4() {
		return cfg, brokers, nil
	}
	copy := *cfg
	if copy.Net.ResolveCanonicalBootstrapServers {
		// A custom proxy may own remote DNS resolution. Do not replace that
		// policy with local lookups or silently fall back to Sarama's LookupHost.
		if copy.Net.Proxy.Enable {
			return nil, nil, fmt.Errorf("PREFER_IPV4_STACK=true with ResolveCanonicalBootstrapServers and a custom proxy dialer is unsupported")
		}
		ctx := context.Background()
		if cfg.Net.DialTimeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, cfg.Net.DialTimeout)
			defer cancel()
		}
		var err error
		brokers, err = canonicalIPv4Brokers(ctx, brokers)
		if err != nil {
			return nil, nil, err
		}
		// Canonicalization is complete, with the same PTR-based names used by
		// Sarama for TLS/GSSAPI. Disable only its duplicate, dual-stack lookup.
		copy.Net.ResolveCanonicalBootstrapServers = false
	}
	configureIPv4Dialer(&copy)
	return &copy, brokers, nil
}

func canonicalIPv4Brokers(ctx context.Context, brokers []string) ([]string, error) {
	var canonical []string
	seen := make(map[string]bool)
	for _, broker := range brokers {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		host, port, err := net.SplitHostPort(broker)
		if err != nil {
			return nil, fmt.Errorf("bootstrap broker %q: %w", broker, err)
		}
		ips, err := net.DefaultResolver.LookupIP(ctx, "ip4", host)
		if err != nil {
			return nil, fmt.Errorf("bootstrap broker %q: %w", broker, err)
		}
		for _, ip := range ips {
			names, err := net.DefaultResolver.LookupAddr(ctx, ip.String())
			if err != nil {
				return nil, fmt.Errorf("canonical name for %s: %w", ip, err)
			}
			if len(names) == 0 {
				return nil, fmt.Errorf("no canonical name for %s", ip)
			}
			address := net.JoinHostPort(strings.TrimSuffix(names[0], "."), port)
			if !seen[address] {
				seen[address] = true
				canonical = append(canonical, address)
			}
		}
	}
	if len(canonical) == 0 {
		return nil, fmt.Errorf("no IPv4 bootstrap brokers")
	}
	return canonical, nil
}

// configureIPv4Dialer uses Sarama's dialer hook without changing its TCP options
// or TLS setup. Existing custom proxy dialers retain their own policy.
func configureIPv4Dialer(cfg *sarama.Config) {
	if !netx.PreferIPv4() || cfg.Net.Proxy.Enable {
		return
	}
	cfg.Net.Proxy.Dialer = netx.NewDialer(&net.Dialer{
		Timeout: cfg.Net.DialTimeout, KeepAlive: cfg.Net.KeepAlive, LocalAddr: cfg.Net.LocalAddr,
	})
	cfg.Net.Proxy.Enable = true
}
