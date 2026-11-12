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
)

func TestMetricsDockerIPv4BothConstructors(t *testing.T) {
	for _, key := range []string{"DOCKER_CERT_PATH", "DOCKER_TLS_VERIFY", "DOCKER_API_VERSION", "HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy"} {
		t.Setenv(key, "")
	}
	for _, value := range []string{"false", "true"} {
		for _, fromEnv := range []bool{false, true} {
			name := value + map[bool]string{false: "/endpoint", true: "/env"}[fromEnv]
			t.Run(name, func(t *testing.T) {
				t.Setenv("PREFER_IPV4_STACK", value)
				counts := nettest.ObserveDNS(t, "docker.invalid.", net.IPv4(127, 0, 0, 1), nil)
				server := dockertest.NewServer(t, "1.51", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"ID":"test-daemon"}`)) }))
				host := strings.Replace(strings.Replace(server.URL, "http://", "tcp://", 1), "127.0.0.1", "docker.invalid.", 1)
				var client Client
				var err error
				if fromEnv {
					t.Setenv("DOCKER_HOST", host)
					client, err = NewEnvClient()
				} else {
					client, err = NewClient(host, nil)
				}
				if err != nil {
					t.Fatal(err)
				}
				defer client.Close()
				if value == "true" {
					t.Setenv("PREFER_IPV4_STACK", "false")
				}
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				if _, err := client.Info(ctx); err != nil {
					t.Fatal(err)
				}
				a, aaaa := counts()
				if a == 0 || value == "true" && aaaa != 0 || value == "false" && aaaa == 0 {
					t.Fatalf("A=%d AAAA=%d", a, aaaa)
				}
			})
		}
	}
}
