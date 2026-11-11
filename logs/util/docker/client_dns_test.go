//go:build !no_logs

package docker

import (
	"context"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"flashcat.cloud/categraf/internal/dockertest"
	"flashcat.cloud/categraf/internal/nettest"
	"github.com/moby/moby/client"
)

func TestClientFromEnvDNSAddressFamily(t *testing.T) {
	for _, key := range []string{"DOCKER_CERT_PATH", "DOCKER_TLS_VERIFY", "DOCKER_API_VERSION", "HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy"} {
		t.Setenv(key, "")
	}
	for _, value := range []string{"false", "true"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("PREFER_IPV4_STACK", value)
			counts := nettest.ObserveDNS(t, "docker.invalid.", net.IPv4(127, 0, 0, 1), nil)
			server := dockertest.NewServer(t, "1.51", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"ID":"test-daemon"}`))
			}))
			t.Setenv(client.EnvOverrideHost, strings.Replace(strings.Replace(server.URL, "http://", "tcp://", 1), "127.0.0.1", "docker.invalid.", 1))
			cli, err := newClientFromEnv()
			if err != nil {
				t.Fatal(err)
			}
			defer cli.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if _, err := cli.Info(ctx, client.InfoOptions{}); err != nil {
				t.Fatal(err)
			}
			a, aaaa := counts()
			if a == 0 || (value == "true" && aaaa != 0) || (value == "false" && aaaa == 0) {
				t.Fatalf("A=%d AAAA=%d", a, aaaa)
			}
		})
	}
}

func TestClientFromEnvKeepsUnixSocket(t *testing.T) {
	t.Setenv("PREFER_IPV4_STACK", "true")
	for _, key := range []string{"DOCKER_CERT_PATH", "DOCKER_TLS_VERIFY", "DOCKER_API_VERSION"} {
		t.Setenv(key, "")
	}
	address := dockertest.NewUnixServer(t, "1.51", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"ID":"unix-daemon"}`)) }))
	t.Setenv(client.EnvOverrideHost, address)
	cli, err := newClientFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := cli.Info(ctx, client.InfoOptions{}); err != nil {
		t.Fatal(err)
	}
}
