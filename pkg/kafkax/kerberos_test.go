package kafkax

import (
	"encoding/binary"
	"flashcat.cloud/categraf/internal/nettest"
	"fmt"
	"github.com/IBM/sarama"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func TestKerberosKDCIPv4(t *testing.T) {
	for _, protocol := range []string{"tcp", "udp"} {
		t.Run(protocol, func(t *testing.T) {
			t.Setenv("PREFER_IPV4_STACK", "true")
			counts := nettest.ObserveDNS(t, "example.com.", net.IPv4(127, 0, 0, 1), net.IPv6loopback)
			var packets atomic.Int32
			address := ""
			preference := 1
			if protocol == "tcp" {
				address = nettest.TCPServer(t, nil, func(c net.Conn) {
					var header [4]byte
					if _, err := io.ReadFull(c, header[:]); err != nil {
						return
					}
					data := make([]byte, binary.BigEndian.Uint32(header[:]))
					if _, err := io.ReadFull(c, data); err != nil {
						return
					}
					packets.Add(1)
					_, _ = c.Write([]byte{0, 0, 0, 1, 0})
				})
			} else {
				conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				address = conn.LocalAddr().String()
				done := make(chan struct{})
				go func() {
					defer close(done)
					data := make([]byte, 65535)
					for {
						_, peer, err := conn.ReadFrom(data)
						if err != nil {
							return
						}
						packets.Add(1)
						_, _ = conn.WriteTo([]byte{0}, peer)
					}
				}()
				t.Cleanup(func() { conn.Close(); <-done })
				preference = 32700
			}
			_, port, _ := net.SplitHostPort(address)
			path := filepath.Join(t.TempDir(), "krb5.conf")
			content := fmt.Sprintf("[libdefaults]\n default_realm = EXAMPLE.TEST\n dns_lookup_kdc = false\n udp_preference_limit = %d\n[realms]\n EXAMPLE.TEST = {\n kdc = example.com.:%s\n }\n", preference, port)
			if err := os.WriteFile(path, []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			cfg := sarama.NewConfig()
			cfg.Net.SASL.GSSAPI = sarama.GSSAPIConfig{Username: "fixture", Password: "fixture", Realm: "EXAMPLE.TEST", KerberosConfigPath: path, DisablePAFXFAST: true}
			prepared, _, err := PrepareIPv4Config(cfg, []string{"localhost:9092"})
			if err != nil {
				t.Fatal(err)
			}
			t.Setenv("PREFER_IPV4_STACK", "false")
			client, err := sarama.NewKerberosClient(&prepared.Net.SASL.GSSAPI)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Destroy()
			if err = client.Login(); err == nil {
				t.Fatal("invalid KDC response accepted")
			}
			a, aaaa := counts()
			if packets.Load() == 0 || a == 0 || aaaa != 0 {
				t.Fatalf("KDC packets=%d A=%d AAAA=%d", packets.Load(), a, aaaa)
			}
		})
	}
}
