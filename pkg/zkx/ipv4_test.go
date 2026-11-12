package zkx

import (
	"flashcat.cloud/categraf/internal/nettest"
	"net"
	"testing"
	"time"
)

func TestHostProviderAndReconnectPolicy(t *testing.T) {
	t.Setenv("PREFER_IPV4_STACK", "true")
	counts := nettest.ObserveDNS(t, "example.com.", net.IPv4(127, 0, 0, 1), net.IPv6loopback)
	address := nettest.TCPServer(t, nil, func(c net.Conn) { _, _ = c.Write([]byte("ok")) })
	_, port, _ := net.SplitHostPort(address)
	provider := NewHostProvider(time.Second)
	dial := DialerFromEnv()
	t.Setenv("PREFER_IPV4_STACK", "false")
	if err := provider.Init([]string{"example.com.:" + port}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		address, _ := provider.Next()
		host, _, _ := net.SplitHostPort(address)
		if net.ParseIP(host).To4() == nil {
			t.Fatalf("host=%s", host)
		}
		conn, err := dial("tcp", address, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		conn.Close()
		provider.Connected()
	}
	a, aaaa := counts()
	if a == 0 || aaaa != 0 {
		t.Fatalf("A=%d AAAA=%d", a, aaaa)
	}
}
