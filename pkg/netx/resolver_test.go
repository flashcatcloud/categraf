package netx

import (
	"context"
	"net"
	"testing"

	"flashcat.cloud/categraf/internal/nettest"
)

func TestLookupIPAddrDNSAddressFamily(t *testing.T) {
	for _, value := range []string{"false", "true"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("PREFER_IPV4_STACK", value)
			counts := nettest.ObserveDNS(t, "lookup.invalid.", net.IPv4(127, 0, 0, 1), net.IPv6loopback)
			addrs, err := LookupIPAddr(context.Background(), "lookup.invalid.")
			if err != nil {
				t.Fatal(err)
			}
			a, aaaa := counts()
			if value == "true" {
				if a != 1 || aaaa != 0 || len(addrs) != 1 || addrs[0].IP.To4() == nil {
					t.Fatalf("IPv4 lookup: A=%d AAAA=%d addresses=%v", a, aaaa, addrs)
				}
			} else if a != 1 || aaaa != 1 || len(addrs) != 2 {
				t.Fatalf("default lookup: A=%d AAAA=%d addresses=%v", a, aaaa, addrs)
			}
		})
	}
}

func TestNetworkFromEnv(t *testing.T) {
	for _, value := range []string{"", "true", "false", "TRUE", "1", " true"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("PREFER_IPV4_STACK", value)
			for _, network := range []string{"tcp", "udp", "ip", "tcp4", "udp4", "ip4", "tcp6", "udp6", "ip6", "unix", "unixpacket", "unixgram"} {
				want := network
				if value == "true" && (network == "tcp" || network == "udp" || network == "ip") {
					want += "4"
				}
				if got := NetworkFromEnv(network); got != want {
					t.Errorf("%s: got %s, want %s", network, got, want)
				}
			}
		})
	}
}

func TestLookupHostCapturedPolicy(t *testing.T) {
	t.Setenv("PREFER_IPV4_STACK", "true")
	counts := nettest.ObserveDNS(t, "lookup.invalid.", net.IPv4(127, 0, 0, 1), net.IPv6loopback)
	lookup := LookupHostFromEnv(net.DefaultResolver)
	t.Setenv("PREFER_IPV4_STACK", "false")
	addresses, err := lookup(context.Background(), "lookup.invalid.")
	if err != nil || len(addresses) != 1 || addresses[0] != "127.0.0.1" {
		t.Fatalf("addresses=%v err=%v", addresses, err)
	}
	a, aaaa := counts()
	if a == 0 || aaaa != 0 {
		t.Fatalf("A=%d AAAA=%d", a, aaaa)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := lookup(ctx, "lookup.invalid."); err == nil {
		t.Fatal("canceled lookup succeeded")
	}
}
