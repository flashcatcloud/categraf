package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"flashcat.cloud/categraf/internal/dockertest"
	"flashcat.cloud/categraf/pkg/cfg"
	"flashcat.cloud/categraf/types"
	"github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
)

const statsFixture = `{"read":"2026-10-29T00:00:02Z","preread":"2026-10-29T00:00:01Z",
"cpu_stats":{"cpu_usage":{"total_usage":300},"system_cpu_usage":2000,"online_cpus":2},
"precpu_stats":{"cpu_usage":{"total_usage":100},"system_cpu_usage":1000},
"memory_stats":{"usage":1000,"limit":2000,"stats":{"inactive_file":200}},
"networks":{"eth0":{"rx_bytes":10,"tx_bytes":20}},
"blkio_stats":{"io_service_bytes_recursive":[{"major":8,"minor":0,"op":"Read","value":30},{"major":8,"minor":0,"op":"Write","value":40}]}}`

func metricsHandler(t *testing.T) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var body string
		switch r.URL.Path {
		case "/info":
			body = `{"Name":"engine-test","ServerVersion":"28.4.0","OSType":"linux","NCPU":2,"Containers":1}`
		case "/containers/json":
			var filters map[string]map[string]bool
			if err := json.Unmarshal([]byte(r.URL.Query().Get("filters")), &filters); err != nil || !filters["status"]["running"] {
				t.Errorf("container state filter lost: %s", r.URL.RawQuery)
			}
			body = `[{"Id":"container-test","Names":["/app"],"Image":"example/app:1","State":"running","Labels":{"team":"metrics"}}]`
		case "/containers/container-test/stats":
			if r.URL.Query().Get("stream") != "false" || r.URL.Query().Get("one-shot") == "true" {
				t.Errorf("Stats must request both CPU samples: %s", r.URL.RawQuery)
			}
			body = statsFixture
		case "/containers/container-test/json":
			body = `{"Id":"container-test","Config":{"Env":["REGION=test"]},"HostConfig":{},"State":{"Status":"running","Pid":123,"ExitCode":0,"Health":{"Status":"healthy","FailingStreak":0}}}`
		case "/services":
			body = `[{"ID":"service-test","Spec":{"Name":"app","Mode":{"Replicated":{"Replicas":2}}}}]`
		case "/tasks":
			body = `[{"ServiceID":"service-test","NodeID":"node-test","DesiredState":"running","Status":{"State":"running"}}]`
		case "/nodes":
			body = `[{"ID":"node-test","Status":{"State":"ready"}}]`
		default:
			t.Errorf("unexpected Docker API call: %s", r.URL.Path)
			http.Error(w, "unexpected API", http.StatusNotFound)
			return
		}
		_, _ = io.WriteString(w, body)
	})
}

func TestGatherWithMobySDK(t *testing.T) {
	for _, version := range []string{"1.40", "1.51", "1.54"} {
		t.Run(version, func(t *testing.T) {
			server := dockertest.NewServer(t, version, metricsHandler(t))
			var plugin Docker
			err := cfg.LoadSingleConfig(cfg.ConfigWithFormat{Format: cfg.TomlFormat, Config: fmt.Sprintf(`
[[instances]]
endpoint = %q
timeout = "5s"
gather_services = true
total_include = ["cpu", "network", "blkio"]
docker_label_include = ["team"]
tag_env = ["REGION"]
`, server.URL)}, &plugin)
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
			want := map[string]string{
				"docker_up": "1", "docker_container_cpu_usage_percent": "40",
				"docker_container_mem_usage": "800", "docker_container_mem_usage_percent": "40",
				"docker_container_net_rx_bytes": "10", "docker_container_net_tx_bytes": "20",
				"docker_swarm_tasks_running": "1", "docker_swarm_tasks_desired": "2",
				"docker_container_blkio_io_service_bytes_recursive_read":  "30",
				"docker_container_blkio_io_service_bytes_recursive_write": "40",
			}
			for _, sample := range samples.PopBackAll() {
				if expected, ok := want[sample.Metric]; ok {
					if fmt.Sprint(sample.Value) != expected {
						t.Errorf("%s = %v, want %s", sample.Metric, sample.Value, expected)
					}
					if strings.HasPrefix(sample.Metric, "docker_container_") &&
						(sample.Labels["container_name"] != "app" || sample.Labels["team"] != "metrics" || sample.Labels["REGION"] != "test") {
						t.Errorf("container labels changed: %v", sample.Labels)
					}
					delete(want, sample.Metric)
				}
			}
			if len(want) != 0 {
				t.Errorf("missing metrics: %v", want)
			}
		})
	}
}

func TestMobyClientConfiguration(t *testing.T) {
	t.Run("Unix socket", func(t *testing.T) {
		host := dockertest.NewUnixServer(t, "1.51", metricsHandler(t))
		cli, err := NewClient(host, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer cli.Close()
		if _, err := cli.Info(context.Background()); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("environment API override", func(t *testing.T) {
		server := dockertest.NewServer(t, "1.40", metricsHandler(t))
		t.Setenv("DOCKER_HOST", server.URL)
		t.Setenv("DOCKER_API_VERSION", "1.40")
		t.Setenv("DOCKER_TLS_VERIFY", "")
		t.Setenv("DOCKER_CERT_PATH", "")
		cli, err := NewEnvClient()
		if err != nil {
			t.Fatal(err)
		}
		defer cli.Close()
		if _, err := cli.Info(context.Background()); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("TLS", func(t *testing.T) {
		server := httptest.NewTLSServer(dockertest.Handler(t, "1.51", metricsHandler(t)))
		defer server.Close()
		cli, err := NewClient(server.URL, server.Client().Transport.(*http.Transport).TLSClientConfig.Clone())
		if err != nil {
			t.Fatal(err)
		}
		defer cli.Close()
		if _, err := cli.Info(context.Background()); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("old API rejected", func(t *testing.T) {
		server := dockertest.NewServer(t, "1.39", metricsHandler(t))
		cli, err := NewClient(server.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer cli.Close()
		if _, err := cli.Ping(context.Background()); err == nil || !strings.Contains(err.Error(), "minimum supported API version is 1.40") {
			t.Fatalf("expected explicit minimum version error, got %v", err)
		}
	})
	t.Run("not found", func(t *testing.T) {
		server := dockertest.NewServer(t, "1.51", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"message":"No such container"}`)
		}))
		cli, err := NewClient(server.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer cli.Close()
		if _, err := cli.ContainerInspect(context.Background(), "missing"); !errdefs.IsNotFound(err) {
			t.Fatalf("not-found classification: %v", err)
		}
	})
}

func TestCPUStatsCompatibility(t *testing.T) {
	var stats container.StatsResponse
	if err := json.Unmarshal([]byte(statsFixture), &stats); err != nil {
		t.Fatal(err)
	}
	if got := CalculateCPUPercentUnix(stats.PreCPUStats.CPUUsage.TotalUsage, stats.PreCPUStats.SystemUsage, &stats); got != 40 {
		t.Fatalf("Linux CPU = %v", got)
	}
	stats.Read = time.Unix(2, 0)
	stats.PreRead = time.Unix(0, 0)
	stats.NumProcs = 2
	stats.CPUStats.CPUUsage.TotalUsage = 30_000_000
	stats.PreCPUStats.CPUUsage.TotalUsage = 10_000_000
	if got := calculateCPUPercentWindows(&stats); got != 50 {
		t.Fatalf("Windows CPU = %v", got)
	}
	for _, name := range []string{"cache", "total_inactive_file", "inactive_file"} {
		if got := CalculateMemUsageUnixNoCache(container.MemoryStats{Usage: 1000, Stats: map[string]uint64{name: 200}}); got != 800 {
			t.Errorf("%s memory = %v", name, got)
		}
	}
}
