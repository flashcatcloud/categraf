package modbus

import (
	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/internal/nettest"
	"io"
	"net"
	"testing"
	"time"
)

func TestAllTCPModesIPv4(t *testing.T) {
	for _, mode := range []string{"TCP", "RTUoverTCP", "ASCIIoverTCP"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("PREFER_IPV4_STACK", "true")
			counts := nettest.ObserveDNS(t, "example.com.", net.IPv4(127, 0, 0, 1), net.IPv6loopback)
			address := nettest.TCPServer(t, nil, func(c net.Conn) { _, _ = io.Copy(io.Discard, c) })
			_, port, _ := net.SplitHostPort(address)
			ins := &Instance{Controller: "tcp://example.com.:" + port, TransmissionMode: mode, Timeout: config.Duration(time.Second)}
			if err := ins.initClient(); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PREFER_IPV4_STACK", "false")
			for i := 0; i < 2; i++ {
				if err := ins.connect(); err != nil {
					t.Fatal(err)
				}
				if err := ins.disconnect(); err != nil {
					t.Fatal(err)
				}
			}
			a, aaaa := counts()
			if a == 0 || aaaa != 0 {
				t.Fatalf("A=%d AAAA=%d", a, aaaa)
			}
		})
	}
}
