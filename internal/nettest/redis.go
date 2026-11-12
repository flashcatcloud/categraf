package nettest

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
)

// RedisServer implements authentication and PING over RESP, using the actual
// SDK's TLS, authentication and connection pool rather than a replaced Dialer.
func RedisServer(t *testing.T, config *tls.Config) string {
	t.Helper()
	return TCPServer(t, config, func(conn net.Conn) {
		reader := bufio.NewReader(conn)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			if !strings.HasPrefix(line, "*") {
				t.Errorf("RESP array: %q", line)
				return
			}
			n, err := strconv.Atoi(strings.TrimSpace(line[1:]))
			if err != nil {
				return
			}
			command := make([]string, n)
			for i := range command {
				length, err := reader.ReadString('\n')
				if err != nil {
					return
				}
				if !strings.HasPrefix(length, "$") {
					return
				}
				size, err := strconv.Atoi(strings.TrimSpace(length[1:]))
				if err != nil || size < 0 {
					return
				}
				payload := make([]byte, size+2)
				if _, err := io.ReadFull(reader, payload); err != nil {
					return
				}
				command[i] = string(payload[:size])
			}
			if len(command) == 0 {
				return
			}
			response := "+OK\r\n"
			switch strings.ToUpper(command[0]) {
			case "PING":
				response = "+PONG\r\n"
			case "AUTH":
				if command[len(command)-1] != "test-password" {
					response = "-ERR invalid password\r\n"
				}
			default:
				t.Errorf("unexpected Redis command: %v", command)
				return
			}
			if _, err := fmt.Fprint(conn, response); err != nil {
				return
			}
			if strings.EqualFold(command[0], "PING") {
				return
			} // Force pool reconnect on the next request.
		}
	})
}
