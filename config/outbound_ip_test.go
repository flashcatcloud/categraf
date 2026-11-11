package config

import (
	"net"
	"testing"

	"flashcat.cloud/categraf/internal/nettest"
)

func TestOutboundIPDNSAddressFamily(t *testing.T) {
	original := Config
	t.Cleanup(func() { Config = original })
	for _, value := range []string{"", "false", "TRUE", "true"} {
		t.Run("env="+value, func(t *testing.T) {
			t.Setenv("PREFER_IPV4_STACK", value)
			counts := nettest.ObserveDNS(t, "probe.invalid.", net.IPv4(127, 0, 0, 1), nil)
			Config = &ConfigType{Writers: []WriterOption{{Url: "http://probe.invalid.:18080/write"}}}
			// Exercise startup and repeated refreshes, independently of HTTP pools.
			for i := 0; i < 5; i++ {
				ip, err := GetOutboundIP()
				if err != nil || !ip.Equal(net.IPv4(127, 0, 0, 1)) {
					t.Fatalf("GetOutboundIP = %v, %v", ip, err)
				}
			}
			a, aaaa := counts()
			wantAAAA := int64(5)
			if value == "true" {
				wantAAAA = 0
			}
			if a != 5 || aaaa != wantAAAA {
				t.Fatalf("A=%d AAAA=%d, want A=5 AAAA=%d", a, aaaa, wantAAAA)
			}
		})
	}
}
