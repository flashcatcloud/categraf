package gnmi

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"sort"
	"strings"
	"time"

	"flashcat.cloud/categraf/config"
	"golang.org/x/net/icmp"
)

type icmpOptions struct {
	Count                   int
	PacketInterval, Timeout time.Duration
	Privileged              bool
}
type icmpResult struct {
	Sent, Received int
	AvgRTT         time.Duration
}
type icmpSnapshot struct {
	Result      icmpResult
	CompletedAt time.Time
	Success     bool
}
type icmpProbe interface {
	Probe(context.Context, string, icmpOptions) (icmpResult, error)
}
type nativeICMPProbe struct{}

func selectICMPAddress(addresses []net.IPAddr) (net.IPAddr, error) {
	if len(addresses) == 0 {
		return net.IPAddr{}, fmt.Errorf("no resolved ICMP address")
	}
	// Stable preference within each family avoids random DNS answer ordering.
	sort.Slice(addresses, func(i, j int) bool {
		a, b := addresses[i], addresses[j]
		if (a.IP.To4() != nil) != (b.IP.To4() != nil) {
			return a.IP.To4() != nil
		}
		return a.String() < b.String()
	})
	return addresses[0], nil
}
func (p nativeICMPProbe) Probe(ctx context.Context, host string, o icmpOptions) (icmpResult, error) {
	ctx, cancel := context.WithTimeout(ctx, o.Timeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return icmpResult{}, err
	}
	var address net.IPAddr
	if ip, err := netip.ParseAddr(host); err == nil {
		address = net.IPAddr{IP: net.IP(ip.AsSlice()), Zone: ip.Zone()}
	} else {
		ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return icmpResult{}, err
		}
		address, err = selectICMPAddress(ips)
		if err != nil {
			return icmpResult{}, err
		}
	}
	if err := ctx.Err(); err != nil {
		return icmpResult{}, err
	}
	network, bind := "ip4:icmp", "0.0.0.0"
	if address.IP.To4() == nil {
		network, bind = "ip6:ipv6-icmp", "::"
	}
	if !o.Privileged {
		if address.IP.To4() != nil {
			network = "udp4"
		} else {
			network = "udp6"
		}
	}
	conn, err := icmp.ListenPacket(network, bind)
	if err != nil {
		return icmpResult{}, err
	}
	defer func() { _ = conn.Close() }()
	return runICMPRound(ctx, conn, &address, o)
}
func icmpHost(host string) string {
	if ip, err := netip.ParseAddr(host); err == nil {
		return ip.String()
	}
	return strings.ToLower(host)
}
func (c *Instance) initICMP() error {
	if !c.EnableICMP {
		return nil
	}
	if c.ICMPCount > 65535 {
		return fmt.Errorf("icmp_count must not exceed 65535 (Echo sequence capacity)")
	}
	if c.ICMPCount < 0 || c.ICMPConcurrency < 0 || c.ICMPInterval < 0 || c.ICMPTimeout < 0 || c.ICMPPacketInterval < 0 {
		return fmt.Errorf("ICMP options must be positive")
	}
	if c.ICMPCount == 0 {
		c.ICMPCount = 3
	}
	if c.ICMPConcurrency == 0 {
		c.ICMPConcurrency = 10
	}
	if c.ICMPInterval == 0 {
		c.ICMPInterval = config.Duration(30 * time.Second)
	}
	if c.ICMPTimeout == 0 {
		c.ICMPTimeout = config.Duration(3 * time.Second)
	}
	if c.ICMPPacketInterval == 0 {
		c.ICMPPacketInterval = config.Duration(200 * time.Millisecond)
	}
	timeout, packet := time.Duration(c.ICMPTimeout), time.Duration(c.ICMPPacketInterval)
	if time.Duration(c.ICMPInterval) <= timeout || int64(c.ICMPCount-1) > int64(timeout/packet) || time.Duration(c.ICMPCount-1)*packet >= timeout {
		return fmt.Errorf("ICMP round requires timeout > (count-1)*packet_interval and interval > timeout")
	}
	// Health labels must remain unambiguous even if session metrics are disabled.
	if err := c.validateHealthIdentity(); err != nil {
		return err
	}
	hosts := make([]string, 0, len(c.Addresses))
	seen := map[string]bool{}
	for _, a := range c.Addresses {
		host, err := healthHost(a)
		if err != nil {
			return err
		}
		host = icmpHost(host)
		if !seen[host] {
			hosts = append(hosts, host)
			seen[host] = true
		}
	}
	privileged := true
	if c.ICMPPrivileged != nil {
		privileged = *c.ICMPPrivileged
	}
	c.icmp = newICMPMonitor(hosts, icmpOptions{c.ICMPCount, packet, timeout, privileged}, time.Duration(c.ICMPInterval), c.ICMPConcurrency, nativeICMPProbe{})
	return nil
}
