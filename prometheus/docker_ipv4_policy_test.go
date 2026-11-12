//go:build !no_prometheus

package prometheus

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"flashcat.cloud/categraf/internal/dockertest"
	"flashcat.cloud/categraf/internal/nettest"
)

func TestDockerAndSwarmTCPPolicy(t *testing.T) {
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy"} {
		t.Setenv(key, "")
	}
	for _, value := range []string{"false", "true"} {
		for _, role := range []string{"docker", "nodes", "services", "tasks"} {
			t.Run(value+"/"+role, func(t *testing.T) {
				t.Setenv("PREFER_IPV4_STACK", value)
				counts := nettest.ObserveDNS(t, "example.com.", net.IPv4(127, 0, 0, 1), nil)
				var requests atomic.Int32
				server := dockertest.NewServer(t, "1.51", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					requests.Add(1)
					w.Header().Set("Content-Type", "application/json")
					switch r.URL.Path {
					case "/containers/json":
						io.WriteString(w, dockerDiscoveryContainers)
					case "/networks":
						io.WriteString(w, dockerDiscoveryNetworks)
					case "/nodes":
						io.WriteString(w, swarmDiscoveryNodes)
					case "/services":
						io.WriteString(w, swarmDiscoveryServices)
					case "/tasks":
						io.WriteString(w, swarmDiscoveryTasks)
					default:
						t.Errorf("unexpected Docker API: %s", r.URL.Path)
						http.NotFound(w, r)
					}
				}))
				host := "tcp://" + strings.Replace(strings.TrimPrefix(server.URL, "http://"), "127.0.0.1", "example.com.", 1)
				yaml := fmt.Sprintf("scrape_configs:\n- job_name: docker\n  docker_sd_configs:\n  - host: %q\n    refresh_interval: 100ms\n", host)
				if role != "docker" {
					yaml = fmt.Sprintf("scrape_configs:\n- job_name: swarm\n  dockerswarm_sd_configs:\n  - host: %q\n    role: %s\n    refresh_interval: 100ms\n", host, role)
				}
				policy, closePolicy := newNetworkPolicy()
				defer closePolicy()
				d := discoveryFromYAML(t, yaml, policy)
				// Reconnect with the same discoverer after closing server-side idle pools.
				// Changing the environment must not replace the constructor's policy.
				t.Setenv("PREFER_IPV4_STACK", "false")
				for i := 0; i < 2; i++ {
					if targets := nextDiscoveryTargets(t, d); len(targets) == 0 {
						t.Fatal("discovery returned no targets")
					}
					server.CloseClientConnections()
				}
				a, aaaa := counts()
				if requests.Load() == 0 || a == 0 || (value == "true" && aaaa != 0) || (value == "false" && aaaa == 0) {
					t.Fatalf("requests=%d A=%d AAAA=%d", requests.Load(), a, aaaa)
				}
			})
		}
	}
}

func TestDockerAndSwarmUnixPolicy(t *testing.T) {
	t.Setenv("PREFER_IPV4_STACK", "true")
	for _, role := range []string{"docker", "nodes"} {
		t.Run(role, func(t *testing.T) {
			host := dockertest.NewUnixServer(t, "1.51", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/containers/json":
					io.WriteString(w, dockerDiscoveryContainers)
				case "/networks":
					io.WriteString(w, dockerDiscoveryNetworks)
				case "/nodes":
					io.WriteString(w, swarmDiscoveryNodes)
				default:
					http.NotFound(w, r)
				}
			}))
			yaml := fmt.Sprintf("scrape_configs:\n- job_name: docker\n  docker_sd_configs:\n  - host: %q\n", host)
			if role != "docker" {
				yaml = fmt.Sprintf("scrape_configs:\n- job_name: swarm\n  dockerswarm_sd_configs:\n  - host: %q\n    role: %s\n", host, role)
			}
			policy, closePolicy := newNetworkPolicy()
			defer closePolicy()
			if targets := nextDiscoveryTargets(t, discoveryFromYAML(t, yaml, policy)); len(targets) == 0 {
				t.Fatal("Unix discovery returned no targets")
			}
		})
	}
}
