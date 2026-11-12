package snmp

import (
	"flashcat.cloud/categraf/internal/nettest"
	"github.com/gosnmp/gosnmp"
	"net"
	"testing"
	"time"
)

func TestSNMPUDPConnectedAndUnconnected(t *testing.T) {
	for _, unconnected := range []bool{false, true} {
		t.Run(map[bool]string{false: "connected", true: "unconnected"}[unconnected], func(t *testing.T) {
			t.Setenv("PREFER_IPV4_STACK", "true")
			counts := nettest.ObserveDNS(t, "example.com.", net.IPv4(127, 0, 0, 1), net.IPv6loopback)
			socket, err := net.ListenPacket("udp4", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer socket.Close()
			_, port, _ := net.SplitHostPort(socket.LocalAddr().String())
			wrapper := &GosnmpWrapper{GoSNMP: &gosnmp.GoSNMP{Community: "public", Version: gosnmp.Version2c, Timeout: time.Second, UseUnconnectedUDPSocket: unconnected}}
			if err := wrapper.SetAgent("udp://example.com.:" + port); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PREFER_IPV4_STACK", "false")
			if err := wrapper.Connect(); err != nil {
				t.Fatal(err)
			}
			wrapper.Conn.Close()
			a, aaaa := counts()
			if a == 0 || aaaa != 0 {
				t.Fatalf("A=%d AAAA=%d", a, aaaa)
			}
		})
	}
}

func TestPingResolverFamily(t *testing.T) {
	for _, family := range []string{"ip", "ip6"} {
		t.Run(family, func(t *testing.T) {
			t.Setenv("PREFER_IPV4_STACK", "true")
			counts := nettest.ObserveDNS(t, "example.com.", net.IPv4(127, 0, 0, 1), net.IPv6loopback)
			address, err := resolvePingTarget("example.com.", family)
			if err != nil {
				t.Fatal(err)
			}
			a, aaaa := counts()
			if family == "ip" {
				if address.IP.To4() == nil || a == 0 || aaaa != 0 {
					t.Fatalf("ordinary target A=%d AAAA=%d", a, aaaa)
				}
			} else if address.IP.To4() != nil || a != 0 || aaaa == 0 {
				t.Fatalf("explicit IPv6 A=%d AAAA=%d", a, aaaa)
			}
		})
	}
	t.Setenv("PREFER_IPV4_STACK", "true")
	if address, err := resolvePingTarget("fe80::1%fixture", "ip"); err != nil || address.Zone != "fixture" || address.IP.To4() != nil {
		t.Fatalf("IPv6 literal=%v err=%v", address, err)
	}
}
