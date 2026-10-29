package docker

import (
	"context"
	"fmt"
	"os"
	"testing"

	"flashcat.cloud/categraf/pkg/cfg"
	"flashcat.cloud/categraf/types"
	"github.com/containerd/errdefs"
)

func TestDockerMetricsIntegration(t *testing.T) {
	host, name := os.Getenv("CATEGRAF_DOCKER_TEST_HOST"), os.Getenv("CATEGRAF_DOCKER_TEST_NAME")
	if host == "" || name == "" {
		t.Skip("set CATEGRAF_DOCKER_TEST_HOST and CATEGRAF_DOCKER_TEST_NAME for a disposable container")
	}
	for _, endpoint := range []string{host, "ENV"} {
		t.Run(endpoint, func(t *testing.T) {
			t.Setenv("DOCKER_HOST", host)
			t.Setenv("DOCKER_TLS_VERIFY", "")
			t.Setenv("DOCKER_CERT_PATH", "")
			t.Setenv("DOCKER_API_VERSION", "")
			var plugin Docker
			err := cfg.LoadSingleConfig(cfg.ConfigWithFormat{Format: cfg.TomlFormat, Config: fmt.Sprintf(`[[instances]]
endpoint = %q
timeout = "10s"
container_name_include = [%q]
total_include = ["cpu", "network", "blkio"]
gather_services = %s
`, endpoint, name, fmt.Sprint(os.Getenv("CATEGRAF_DOCKER_TEST_SWARM") == "true"))}, &plugin)
			if err != nil {
				t.Fatal(err)
			}
			ins := plugin.Instances[0]
			if err := ins.Init(); err != nil {
				t.Fatal(err)
			}
			defer ins.client.Close()
			samples := types.NewSampleList()
			ins.Gather(samples)
			foundUp, foundCPU, foundMem := false, false, false
			for _, sample := range samples.PopBackAll() {
				if sample.Metric == "docker_up" && fmt.Sprint(sample.Value) == "1" {
					foundUp = true
				}
				if sample.Labels["container_name"] == name {
					if sample.Metric == "docker_container_cpu_usage_percent" {
						foundCPU = true
						if sample.Value.(float64) <= 0 {
							t.Error("busy container did not produce a CPU delta")
						}
					}
					if sample.Metric == "docker_container_mem_usage" {
						foundMem = true
					}
				}
			}
			if !foundUp || !foundCPU || !foundMem {
				t.Fatalf("missing metrics: up=%v CPU=%v memory=%v", foundUp, foundCPU, foundMem)
			}
			if _, err := ins.client.ContainerInspect(context.Background(), "categraf-sdk-no-such-container"); !errdefs.IsNotFound(err) {
				t.Fatalf("missing container classification: %v", err)
			}
		})
	}
}
