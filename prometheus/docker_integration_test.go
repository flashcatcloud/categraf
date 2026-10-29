//go:build !no_prometheus

package prometheus

import (
	"fmt"
	"github.com/prometheus/common/model"
	"os"
	"testing"
)

func TestDockerDiscoveryIntegration(t *testing.T) {
	host, id := os.Getenv("CATEGRAF_DOCKER_TEST_HOST"), os.Getenv("CATEGRAF_DOCKER_TEST_CONTAINER")
	if host == "" || id == "" {
		t.Skip("set CATEGRAF_DOCKER_TEST_HOST and CATEGRAF_DOCKER_TEST_CONTAINER for a disposable container")
	}
	d := discoveryFromYAML(t, fmt.Sprintf(`scrape_configs:
- job_name: docker
  docker_sd_configs:
  - host: %q
    refresh_interval: 100ms
    filters:
    - name: id
      values: [%q]
`, host, id))
	targets := nextDiscoveryTargets(t, d)
	if len(targets) == 0 {
		t.Fatal("no Docker discovery targets")
	}
	for _, target := range targets {
		if string(target["__meta_docker_container_id"]) != id || target["__meta_docker_network_id"] == "" {
			t.Fatalf("unexpected Docker target: %v", target)
		}
	}
	if os.Getenv("CATEGRAF_DOCKER_TEST_SWARM") != "true" {
		return
	}
	for _, role := range []string{"nodes", "services", "tasks"} {
		t.Run(role, func(t *testing.T) {
			d := discoveryFromYAML(t, fmt.Sprintf(`scrape_configs:
- job_name: swarm
  dockerswarm_sd_configs:
  - host: %q
    role: %s
    refresh_interval: 100ms
`, host, role))
			targets := nextDiscoveryTargets(t, d)
			if len(targets) == 0 {
				t.Fatal("no Swarm discovery targets")
			}
			key := "__meta_dockerswarm_service_id"
			want := os.Getenv("CATEGRAF_DOCKER_TEST_SERVICE")
			if role == "nodes" {
				key = "__meta_dockerswarm_node_id"
				want = os.Getenv("CATEGRAF_DOCKER_TEST_NODE")
			}
			found := false
			for _, target := range targets {
				if string(target[model.LabelName(key)]) == want {
					found = true
				}
			}
			if !found {
				t.Fatalf("test Swarm object not found: %v", targets)
			}
		})
	}
}
