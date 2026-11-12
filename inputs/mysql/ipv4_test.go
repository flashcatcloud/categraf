package mysql

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/internal/nettest"
)

func TestMySQLIPv4TLSAndPoolReconnect(t *testing.T) {
	previousConfig, previousHost := config.Config, config.HostInfo
	config.Config, config.HostInfo = &config.ConfigType{}, &config.HostInfoCache{}
	config.HostInfo.SetHostname("test")
	config.HostInfo.SetIP("127.0.0.1")
	t.Cleanup(func() { config.Config, config.HostInfo = previousConfig, previousHost })
	for _, value := range []string{"false", "true"} {
		for _, secure := range []bool{false, true} {
			t.Run(fmt.Sprintf("env=%s/tls=%t", value, secure), func(t *testing.T) {
				t.Setenv("PREFER_IPV4_STACK", value)
				counts := nettest.ObserveDNS(t, "example.com.", net.IPv4(127, 0, 0, 1), nil)
				var tlsConfig *tls.Config
				caFile := ""
				if secure {
					tlsConfig, caFile = nettest.TLSFixture(t)
				}
				addr := nettest.TCPServer(t, nil, func(conn net.Conn) { serveMySQL(conn, tlsConfig) })
				_, port, _ := net.SplitHostPort(addr)
				ins := &Instance{Address: net.JoinHostPort("example.com.", port), Username: "test", Password: "test-password", Parameters: "maxAllowedPacket=1048576"}
				if secure {
					ins.UseTLS = true
					ins.TLSCA = caFile
					ins.Parameters += "&tls=custom"
				}
				if err := ins.Init(); err != nil {
					t.Fatal(err)
				}
				if value == "true" {
					t.Setenv("PREFER_IPV4_STACK", "false")
				}
				db, err := ins.openDB()
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
				db.SetMaxIdleConns(0) // Each Ping exercises a pool replenishment.
				for i := 0; i < 2; i++ {
					ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
					err := db.PingContext(ctx)
					cancel()
					if err != nil {
						t.Fatal(err)
					}
				}
				a, aaaa := counts()
				if a < 2 || value == "true" && aaaa != 0 || value == "false" && aaaa == 0 {
					t.Fatalf("A=%d AAAA=%d", a, aaaa)
				}
			})
		}
	}
}

func readMySQLPacket(conn net.Conn) ([]byte, byte, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return nil, 0, err
	}
	n := int(header[0]) | int(header[1])<<8 | int(header[2])<<16
	if n > 1<<20 {
		return nil, 0, fmt.Errorf("packet too large")
	}
	data := make([]byte, n)
	_, err := io.ReadFull(conn, data)
	return data, header[3], err
}
func writeMySQLPacket(conn net.Conn, seq byte, data []byte) error {
	header := []byte{byte(len(data)), byte(len(data) >> 8), byte(len(data) >> 16), seq}
	_, err := conn.Write(append(header, data...))
	return err
}
func serveMySQL(conn net.Conn, tlsConfig *tls.Config) {
	caps := uint32(0x200 | 0x8000 | 0x80000 | 0x20000)
	if tlsConfig != nil {
		caps |= 0x800
	}
	hello := append([]byte{10}, []byte("8.0.33\x00")...)
	hello = append(hello, 1, 0, 0, 0)
	hello = append(hello, []byte("12345678\x00")...)
	hello = binary.LittleEndian.AppendUint16(hello, uint16(caps))
	hello = append(hello, 33, 2, 0)
	hello = binary.LittleEndian.AppendUint16(hello, uint16(caps>>16))
	hello = append(hello, 21)
	hello = append(hello, make([]byte, 10)...)
	hello = append(hello, []byte("abcdefghijkl\x00mysql_native_password\x00")...)
	if writeMySQLPacket(conn, 0, hello) != nil {
		return
	}
	packet, seq, err := readMySQLPacket(conn)
	if err != nil {
		return
	}
	if len(packet) == 32 && tlsConfig != nil {
		conn = tls.Server(conn, tlsConfig)
		_, seq, err = readMySQLPacket(conn)
		if err != nil {
			return
		}
	}
	ok := []byte{0, 0, 0, 2, 0, 0, 0}
	if writeMySQLPacket(conn, seq+1, ok) != nil {
		return
	}
	for {
		packet, _, err = readMySQLPacket(conn)
		if err != nil || len(packet) == 0 {
			return
		}
		if packet[0] == 1 {
			return
		}
		if packet[0] != 0x0e {
			return
		}
		if writeMySQLPacket(conn, 1, ok) != nil {
			return
		}
	}
}
