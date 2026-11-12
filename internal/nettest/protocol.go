package nettest

import (
	"crypto/tls"
	"encoding/pem"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TLSFixture returns a local server certificate valid for example.com and the
// CA file that an actual plugin can load through its normal TLS configuration.
func TLSFixture(t *testing.T) (*tls.Config, string) {
	t.Helper()
	server := httptest.NewTLSServer(http.NotFoundHandler())
	defer server.Close()
	config := server.TLS.Clone()
	caFile := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	return config, caFile
}

// TCPServer runs a local protocol fixture. Closing the listener and all accepted
// connections before waiting ensures even a failed client cannot hang cleanup.
func TCPServer(t *testing.T, config *tls.Config, handler func(net.Conn)) string {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	connections := make(map[net.Conn]bool)
	var wg sync.WaitGroup
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			if config != nil {
				conn = tls.Server(conn, config)
			}
			mu.Lock()
			connections[conn] = true
			mu.Unlock()
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() {
					_ = conn.Close()
					mu.Lock()
					delete(connections, conn)
					mu.Unlock()
				}()
				_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
				handler(conn)
			}()
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		<-done
		mu.Lock()
		for conn := range connections {
			_ = conn.Close()
		}
		mu.Unlock()
		wg.Wait()
	})
	return listener.Addr().String()
}
