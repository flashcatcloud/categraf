package ldap

import (
	"crypto/tls"
	"crypto/x509"
	"flashcat.cloud/categraf/internal/nettest"
	"io"
	"net"
	"os"
	"testing"
)

func TestLDAPAndLDAPS(t *testing.T) {
	for _, mode := range []string{"ldap", "ldaps"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("PREFER_IPV4_STACK", "true")
			counts := nettest.ObserveDNS(t, "example.com.", net.IPv4(127, 0, 0, 1), net.IPv6loopback)
			var serverConfig, clientConfig *tls.Config
			if mode == "ldaps" {
				cfg, caFile := nettest.TLSFixture(t)
				serverConfig = cfg
				pem, err := os.ReadFile(caFile)
				if err != nil {
					t.Fatal(err)
				}
				pool := x509.NewCertPool()
				pool.AppendCertsFromPEM(pem)
				clientConfig = &tls.Config{RootCAs: pool}
			}
			address := nettest.TCPServer(t, serverConfig, func(c net.Conn) { _, _ = io.Copy(io.Discard, c) })
			_, port, _ := net.SplitHostPort(address)
			ins := &Instance{Server: "example.com.:" + port, mode: mode, tlsCfg: clientConfig}
			conn, err := ins.connect()
			if err != nil {
				t.Fatal(err)
			}
			conn.Close()
			a, aaaa := counts()
			if a == 0 || aaaa != 0 {
				t.Fatalf("A=%d AAAA=%d", a, aaaa)
			}
		})
	}
}
