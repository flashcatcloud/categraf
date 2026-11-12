package ping

import (
	"flashcat.cloud/categraf/internal/nettest"
	"net"
	"testing"
)

func TestNativeResolverFamily(t *testing.T) {
	for _, ipv6 := range []bool{false, true} {
		t.Run(map[bool]string{false: "ipv4", true: "explicit_ipv6"}[ipv6], func(t *testing.T) {
			t.Setenv("PREFER_IPV4_STACK", "true")
			counts := nettest.ObserveDNS(t, "example.com.", net.IPv4(127, 0, 0, 1), net.IPv6loopback)
			ins := &Instance{Targets: []string{"example.com."}, IPv6: ipv6}
			if err := ins.Init(); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PREFER_IPV4_STACK", "false")
			pinger, err := ins.resolvePinger("example.com.")
			if err != nil {
				t.Fatal(err)
			}
			a, aaaa := counts()
			if ipv6 {
				if a != 0 || aaaa == 0 || pinger.IPAddr().IP.To4() != nil {
					t.Fatalf("explicit IPv6 A=%d AAAA=%d", a, aaaa)
				}
			} else if a == 0 || aaaa != 0 || pinger.IPAddr().IP.To4() == nil {
				t.Fatalf("IPv4 A=%d AAAA=%d", a, aaaa)
			}
		})
	}
}
