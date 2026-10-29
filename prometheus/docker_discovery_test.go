//go:build !no_prometheus

package prometheus

import (
	"context"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"flashcat.cloud/categraf/internal/dockertest"
	clientmetrics "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	promconfig "github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

func TestDockerDiscoveryTransportsAndRetry(t *testing.T) {
	for _, transport := range []string{"unix", "TLS", "retry", "timeout"} {
		t.Run(transport, func(t *testing.T) {
			var calls atomic.Int32
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if (transport == "retry" || transport == "timeout") && calls.Add(1) == 1 {
					if transport == "timeout" {
						<-r.Context().Done()
					} else {
						http.Error(w, "temporary failure", 500)
					}
					return
				}
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/containers/json" {
					_, _ = io.WriteString(w, dockerDiscoveryContainers)
				} else if r.URL.Path == "/networks" {
					_, _ = io.WriteString(w, dockerDiscoveryNetworks)
				} else {
					http.NotFound(w, r)
				}
			})
			var host, extra string
			switch transport {
			case "unix":
				host = dockertest.NewUnixServer(t, "1.51", handler)
			case "TLS":
				server := httptest.NewTLSServer(dockertest.Handler(t, "1.51", handler))
				defer server.Close()
				host = server.URL
				ca := filepath.Join(t.TempDir(), "ca.pem")
				if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600); err != nil {
					t.Fatal(err)
				}
				extra = fmt.Sprintf("    tls_config:\n      ca_file: %q\n", ca)
			case "retry", "timeout":
				host = dockertest.NewServer(t, "1.51", handler).URL
			}
			d := discoveryFromYAML(t, fmt.Sprintf("scrape_configs:\n- job_name: docker\n  docker_sd_configs:\n  - host: %q\n    refresh_interval: 100ms\n%s", host, extra))
			if targets := nextDiscoveryTargets(t, d); len(targets) != 2 {
				t.Fatalf("targets: %v", targets)
			}
		})
	}
	t.Run("cancel blocked refresh", func(t *testing.T) {
		started := make(chan struct{}, 1)
		server := dockertest.NewServer(t, "1.51", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case started <- struct{}{}:
			default:
			}
			<-r.Context().Done()
		}))
		d := discoveryFromYAML(t, fmt.Sprintf("scrape_configs:\n- job_name: docker\n  docker_sd_configs:\n  - host: %q\n    refresh_interval: 10s\n", server.URL))
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		done := make(chan struct{})
		go func() { defer close(done); d.Run(ctx, make(chan []*targetgroup.Group)) }()
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatal("refresh not started")
		}
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("discovery ignored cancellation")
		}
	})
}

const dockerDiscoveryContainers = `[
{"Id":"app","Names":["/app"],"Labels":{"monitor":"true"},"HostConfig":{"NetworkMode":"bridge"},"Ports":[{"PrivatePort":80,"PublicPort":8080,"IP":"127.0.0.1","Type":"tcp"}],"NetworkSettings":{"Networks":{"bridge":{"NetworkID":"net","IPAddress":"172.18.0.2"}}}},
{"Id":"host","Names":["/host"],"HostConfig":{"NetworkMode":"host"},"NetworkSettings":{"Networks":{"host":{"NetworkID":"host"}}}}
]`
const dockerDiscoveryNetworks = `[{"Id":"net","Name":"bridge","Scope":"local","Labels":{"owner":"monitoring"}},{"Id":"host","Name":"host","Scope":"local"}]`
const swarmDiscoveryNodes = `[{"ID":"node","Spec":{"Role":"manager","Availability":"active","Labels":{"zone":"test"}},"Description":{"Hostname":"engine","Platform":{"OS":"linux","Architecture":"amd64"},"Engine":{"EngineVersion":"28.4.0"}},"Status":{"State":"ready","Addr":"192.0.2.10"}}]`
const swarmDiscoveryServices = `[{"ID":"service","Spec":{"Name":"web","Labels":{"team":"metrics"},"Mode":{"Replicated":{"Replicas":1}},"TaskTemplate":{"ContainerSpec":{"Image":"app:1","Hostname":"app"}}},"Endpoint":{"VirtualIPs":[{"NetworkID":"net","Addr":"10.0.0.2/24"}],"Ports":[{"Name":"http","Protocol":"tcp","TargetPort":80,"PublishedPort":8080,"PublishMode":"ingress"}]}}]`
const swarmDiscoveryTasks = `[{"ID":"task","ServiceID":"service","NodeID":"node","Slot":1,"DesiredState":"running","Status":{"State":"running","ContainerStatus":{"ContainerID":"app"}},"Spec":{"ContainerSpec":{"Labels":{"team":"metrics"}}},"NetworksAttachments":[{"Network":{"ID":"net"},"Addresses":["10.0.0.3/24"]}]}]`

// Load the full Prometheus YAML through the same package and install registry
// used by Categraf. Testing the upstream constructors alone would miss that path.
func discoveryFromYAML(t *testing.T, yaml string) discovery.Discoverer {
	t.Helper()
	logger := slog.New(slog.DiscardHandler)
	config, err := promconfig.Load(yaml, logger)
	if err != nil {
		t.Fatal(err)
	}
	configs := config.ScrapeConfigs[0].ServiceDiscoveryConfigs
	if len(configs) != 1 {
		t.Fatalf("discovery configs = %v", configs)
	}
	registry := clientmetrics.NewRegistry()
	refresh := discovery.NewRefreshMetrics(registry)
	if err := refresh.Register(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(refresh.Unregister)
	metrics := configs[0].NewDiscovererMetrics(registry, refresh)
	if err := metrics.Register(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(metrics.Unregister)
	d, err := configs[0].NewDiscoverer(discovery.DiscovererOptions{Logger: logger, Metrics: metrics, SetName: "docker-regression"})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func nextDiscoveryTargets(t *testing.T, d discovery.Discoverer) []model.LabelSet {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	updates := make(chan []*targetgroup.Group)
	done := make(chan struct{})
	go func() { defer close(done); d.Run(ctx, updates) }()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("discovery did not stop on cancellation")
		}
	}()
	select {
	case groups := <-updates:
		var targets []model.LabelSet
		for _, group := range groups {
			targets = append(targets, group.Targets...)
		}
		return targets
	case <-ctx.Done():
		t.Fatal("discovery did not produce targets")
		return nil
	}
}

func TestDockerDiscoveryYAML(t *testing.T) {
	for _, apiVersion := range []string{"1.40", "1.51", "1.54"} {
		t.Run(apiVersion, func(t *testing.T) {
			server := dockertest.NewServer(t, apiVersion, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/containers/json":
					if !strings.Contains(r.URL.Query().Get("filters"), "monitor=true") {
						t.Error("Docker label filter lost")
					}
					_, _ = io.WriteString(w, dockerDiscoveryContainers)
				case "/networks":
					_, _ = io.WriteString(w, dockerDiscoveryNetworks)
				default:
					t.Errorf("unexpected discovery API: %s", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			d := discoveryFromYAML(t, fmt.Sprintf(`scrape_configs:
- job_name: docker
  docker_sd_configs:
  - host: %q
    refresh_interval: 100ms
    host_networking_host: "127.0.0.9:9100"
    filters:
    - name: label
      values: ["monitor=true"]
`, server.URL))
			targets := nextDiscoveryTargets(t, d)
			if len(targets) != 2 {
				t.Fatalf("targets: %v", targets)
			}
			for _, target := range targets {
				switch target["__meta_docker_container_id"] {
				case "app":
					if target[model.AddressLabel] != "172.18.0.2:80" || target["__meta_docker_port_public"] != "8080" || target["__meta_docker_port_public_ip"] != "127.0.0.1" || target["__meta_docker_container_label_monitor"] != "true" || target["__meta_docker_network_label_owner"] != "monitoring" {
						t.Errorf("Docker target changed: %v", target)
					}
				case "host":
					if target[model.AddressLabel] != "127.0.0.9:9100" {
						t.Errorf("host target changed: %v", target)
					}
				default:
					t.Errorf("unexpected target: %v", target)
				}
			}
		})
	}
}

func TestSwarmDiscoveryYAML(t *testing.T) {
	for _, role := range []string{"nodes", "services", "tasks"} {
		t.Run(role, func(t *testing.T) {
			server := dockertest.NewServer(t, "1.51", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				var body string
				switch r.URL.Path {
				case "/nodes":
					body = swarmDiscoveryNodes
				case "/services":
					body = swarmDiscoveryServices
				case "/tasks":
					body = swarmDiscoveryTasks
				case "/networks":
					body = dockerDiscoveryNetworks
				default:
					t.Errorf("unexpected swarm API: %s", r.URL.Path)
					http.NotFound(w, r)
					return
				}
				_, _ = io.WriteString(w, body)
			}))
			d := discoveryFromYAML(t, fmt.Sprintf(`scrape_configs:
- job_name: swarm
  dockerswarm_sd_configs:
  - host: %q
    role: %s
    refresh_interval: 100ms
    port: 9100
`, server.URL, role))
			targets := nextDiscoveryTargets(t, d)
			if len(targets) != 1 {
				t.Fatalf("targets: %v", targets)
			}
			target := targets[0]
			want := map[string]string{}
			switch role {
			case "nodes":
				want = map[string]string{"__address__": "192.0.2.10:9100", "__meta_dockerswarm_node_id": "node", "__meta_dockerswarm_node_role": "manager", "__meta_dockerswarm_node_label_zone": "test"}
			case "services":
				want = map[string]string{"__address__": "10.0.0.2:8080", "__meta_dockerswarm_service_id": "service", "__meta_dockerswarm_service_name": "web", "__meta_dockerswarm_service_label_team": "metrics"}
			case "tasks":
				want = map[string]string{"__address__": "10.0.0.3:8080", "__meta_dockerswarm_task_id": "task", "__meta_dockerswarm_task_container_id": "app", "__meta_dockerswarm_service_id": "service", "__meta_dockerswarm_node_id": "node"}
			}
			for key, value := range want {
				if string(target[model.LabelName(key)]) != value {
					t.Errorf("%s = %q, want %q", key, target[model.LabelName(key)], value)
				}
			}
		})
	}
}
