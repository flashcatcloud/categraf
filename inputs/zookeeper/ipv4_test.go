package zookeeper

import (
	"crypto/tls"
	"flashcat.cloud/categraf/internal/nettest"
	"io"
	"net"
	"testing"
)

func TestZKConnectionPolicyAndTLSName(t *testing.T) {
	for _, secure := range []bool{false, true} {
		t.Run(map[bool]string{false: "tcp", true: "tls"}[secure], func(t *testing.T) {
			t.Setenv("PREFER_IPV4_STACK", "true")
			counts := nettest.ObserveDNS(t, "example.com.", net.IPv4(127, 0, 0, 1), net.IPv6loopback)
			ins := &Instance{Timeout: 1}
			var serverConfig *tls.Config
			if secure {
				cfg, caFile := nettest.TLSFixture(t)
				serverConfig = cfg
				ins.UseTLS = true
				ins.TLSCA = caFile
			}
			address := nettest.TCPServer(t, serverConfig, func(c net.Conn) { _, _ = io.Copy(io.Discard, c) })
			_, port, _ := net.SplitHostPort(address)
			for i := 0; i < 2; i++ {
				conn, err := ins.ZkConnect("example.com.:" + port)
				if err != nil {
					t.Fatal(err)
				}
				conn.Close()
			}
			a, aaaa := counts()
			if a == 0 || aaaa != 0 {
				t.Fatalf("A=%d AAAA=%d", a, aaaa)
			}
		})
	}
}
