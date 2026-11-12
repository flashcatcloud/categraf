package postgresql

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"flashcat.cloud/categraf/internal/nettest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgproto3"
)

func TestPostgreSQLIPv4DNSAndFallback(t *testing.T) {
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
				var cancellations atomic.Int32
				addr := nettest.TCPServer(t, nil, func(conn net.Conn) {
					backend := pgproto3.NewBackend(conn, conn)
					msg, err := backend.ReceiveStartupMessage()
					if err != nil {
						return
					}
					if _, ok := msg.(*pgproto3.SSLRequest); ok {
						if tlsConfig == nil {
							return
						}
						if _, err := conn.Write([]byte{'S'}); err != nil {
							return
						}
						conn = tls.Server(conn, tlsConfig)
						backend = pgproto3.NewBackend(conn, conn)
						msg, err = backend.ReceiveStartupMessage()
						if err != nil {
							return
						}
					}
					if _, ok := msg.(*pgproto3.CancelRequest); ok {
						cancellations.Add(1)
						return
					}
					if _, ok := msg.(*pgproto3.StartupMessage); !ok {
						t.Errorf("startup: %T", msg)
						return
					}
					backend.Send(&pgproto3.AuthenticationOk{})
					backend.Send(&pgproto3.BackendKeyData{ProcessID: 123, SecretKey: []byte{0, 0, 0, 42}})
					backend.Send(&pgproto3.ParameterStatus{Name: "client_encoding", Value: "UTF8"})
					backend.Send(&pgproto3.ParameterStatus{Name: "standard_conforming_strings", Value: "on"})
					backend.Send(&pgproto3.ReadyForQuery{TxStatus: 'I'})
					if backend.Flush() != nil {
						return
					}
					for {
						msg, err := backend.Receive()
						if err != nil {
							return
						}
						switch msg.(type) {
						case *pgproto3.Query:
							backend.Send(&pgproto3.CommandComplete{CommandTag: []byte("SELECT 1")})
							backend.Send(&pgproto3.ReadyForQuery{TxStatus: 'I'})
							if backend.Flush() != nil {
								return
							}
						case *pgproto3.Terminate:
							return
						default:
							t.Errorf("query: %T", msg)
							return
						}
					}
				})
				_, port, _ := net.SplitHostPort(addr)
				closed, err := net.Listen("tcp4", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				_, closedPort, _ := net.SplitHostPort(closed.Addr().String())
				closed.Close()
				dsn := fmt.Sprintf("host=example.com.,example.com. port=%s,%s user=test sslmode=disable connect_timeout=2", closedPort, port)
				if secure {
					dsn = fmt.Sprintf("host=example.com.,example.com. port=%s,%s user=test sslmode=verify-full sslrootcert=%s connect_timeout=2", closedPort, port, caFile)
				}
				cfg, err := parseConnectionConfig(dsn)
				if err != nil {
					t.Fatal(err)
				}
				if value == "true" {
					t.Setenv("PREFER_IPV4_STACK", "false")
				}
				for i := 0; i < 2; i++ {
					ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
					conn, err := pgx.ConnectConfig(ctx, cfg.Copy())
					if err != nil {
						cancel()
						t.Fatal(err)
					}
					if err := conn.Ping(ctx); err != nil {
						t.Error(err)
					}
					if err := conn.PgConn().CancelRequest(ctx); err != nil && err != io.EOF {
						t.Error(err)
					}
					conn.Close(ctx)
					cancel()
				}
				if cancellations.Load() != 2 {
					t.Fatalf("cancel requests=%d", cancellations.Load())
				}
				a, aaaa := counts()
				if a == 0 || value == "true" && aaaa != 0 || value == "false" && aaaa == 0 {
					t.Fatalf("A=%d AAAA=%d", a, aaaa)
				}
			})
		}
	}
}
