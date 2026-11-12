package net_response

import (
	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/internal/nettest"
	"net"
	"testing"
	"time"
)

func TestTCPAndUDPPolicy(t *testing.T) {
	for _, protocol := range []string{"tcp", "udp"} {
		t.Run(protocol, func(t *testing.T) {
			t.Setenv("PREFER_IPV4_STACK", "true")
			counts := nettest.ObserveDNS(t, "example.com.", net.IPv4(127, 0, 0, 1), net.IPv6loopback)
			var address string
			if protocol == "tcp" {
				address = nettest.TCPServer(t, nil, func(c net.Conn) { _, _ = c.Write([]byte("fixture")) })
			} else {
				conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				address = conn.LocalAddr().String()
				done := make(chan struct{})
				go func() {
					defer close(done)
					data := make([]byte, 1024)
					for {
						_, peer, err := conn.ReadFrom(data)
						if err != nil {
							return
						}
						_, _ = conn.WriteTo([]byte("fixture"), peer)
					}
				}()
				t.Cleanup(func() { conn.Close(); <-done })
			}
			_, port, _ := net.SplitHostPort(address)
			ins := &Instance{Timeout: config.Duration(time.Second), ReadTimeout: config.Duration(time.Second), Send: "probe", Expect: "fixture"}
			var fields map[string]interface{}
			var err error
			if protocol == "tcp" {
				_, fields, err = ins.TCPGather("example.com.:" + port)
			} else {
				_, fields, err = ins.UDPGather("example.com.:" + port)
			}
			if err != nil || fields["result_code"] != Success {
				t.Fatalf("result=%v err=%v", fields, err)
			}
			a, aaaa := counts()
			if a == 0 || aaaa != 0 {
				t.Fatalf("A=%d AAAA=%d", a, aaaa)
			}
		})
	}
}
