//go:build !no_logs

package docker

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"flashcat.cloud/categraf/internal/dockertest"
	"flashcat.cloud/categraf/logs/util/containers"
	"github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
)

func testDockerUtil(t *testing.T, handler http.Handler) *DockerUtil {
	t.Helper()
	server := dockertest.NewServer(t, "1.51", handler)
	cli, err := client.New(client.WithHost(server.URL), client.WithScheme("http"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cli.Close() })
	return &DockerUtil{cli: cli, queryTimeout: 2 * time.Second,
		cfg: &Config{filter: &containers.Filter{}}, eventState: newEventStreamState(),
		imageNameBySha: make(map[string]string), networkMappings: make(map[string][]dockerNetwork)}
}

func TestDockerUtilOperations(t *testing.T) {
	var imageRequests atomic.Int32
	d := testDockerUtil(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		body := ""
		switch r.URL.Path {
		case "/images/json":
			body = `[{"Id":"sha256:image","RepoTags":["app:1"]}]`
		case "/images/sha256:image/json":
			imageRequests.Add(1)
			body = `{"RepoTags":["z:2","a:1"]}`
		case "/images/sha256:missing/json", "/containers/missing/json":
			w.WriteHeader(http.StatusNotFound)
			body = `{"message":"not found"}`
		case "/containers/invalid/json":
			body = `{}`
		case "/containers/id/json":
			if r.URL.Query().Get("size") != "1" {
				t.Errorf("inspect size option lost")
			}
			body = `{"Id":"id","Config":{"Image":"app:1"},"State":{"Status":"running"},"HostConfig":{}}`
		case "/containers/json":
			body = `[{"Id":"id","Labels":{"team":"logs"}}]`
		case "/volumes":
			if !strings.Contains(r.URL.Query().Get("filters"), "dangling") {
				t.Error("volume filter lost")
			}
			body = `{"Volumes":[{"Name":"volume"}]}`
		case "/info":
			body = `{"Name":"engine"}`
		case "/containers/id/logs":
			if r.URL.Query().Get("stdout") != "1" || r.URL.Query().Get("tail") != "5" {
				t.Error("log options lost")
			}
			body = "log line\n"
		default:
			t.Errorf("unexpected API: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
		_, _ = io.WriteString(w, body)
	}))
	ctx := context.Background()
	if images, err := d.Images(ctx, false); err != nil || len(images) != 1 {
		t.Fatalf("images: %v, %v", images, err)
	}
	for range 2 {
		if image, err := d.ResolveImageName(ctx, "sha256:image"); err != nil || image != "a:1" {
			t.Fatalf("image: %s, %v", image, err)
		}
	}
	if imageRequests.Load() != 1 {
		t.Fatal("image cache not used")
	}
	if image, err := d.ResolveImageName(ctx, "sha256:missing"); err != nil || image != "sha256:missing" {
		t.Fatalf("missing image: %s, %v", image, err)
	}
	if _, err := d.InspectNoCache(ctx, "missing", false); !errdefs.IsNotFound(err) {
		t.Fatalf("not-found classification: %v", err)
	}
	if _, err := d.InspectNoCache(ctx, "invalid", false); err == nil {
		t.Fatal("accepted incomplete inspect data")
	}
	if inspected, err := d.InspectNoCache(ctx, "id", true); err != nil || inspected.Config.Image != "app:1" {
		t.Fatalf("inspect: %v, %v", inspected, err)
	}
	if attached, dangling, err := d.CountVolumes(ctx); err != nil || attached != 1 || dangling != 1 {
		t.Fatalf("volumes: %d %d, %v", attached, dangling, err)
	}
	if hostname, err := d.GetHostname(ctx); err != nil || hostname != "engine" {
		t.Fatalf("hostname: %s, %v", hostname, err)
	}
	if labels, err := d.AllContainerLabels(ctx); err != nil || labels["id"]["team"] != "logs" {
		t.Fatalf("labels: %v, %v", labels, err)
	}
	logs, err := d.ContainerLogs(ctx, "id", client.ContainerLogsOptions{ShowStdout: true, Tail: "5"})
	if err != nil {
		t.Fatal(err)
	}
	defer logs.Close()
	if data, err := io.ReadAll(logs); err != nil || string(data) != "log line\n" {
		t.Fatalf("logs: %q, %v", data, err)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type trackedBody struct {
	io.Reader
	closed atomic.Bool
}

func (b *trackedBody) Close() error { b.closed.Store(true); return nil }

func TestStatsBodyClosed(t *testing.T) {
	for _, body := range []string{`{"preread":"2026-01-01T00:00:00Z","precpu_stats":{"cpu_usage":{"total_usage":10}}}`, `{invalid`} {
		t.Run(body, func(t *testing.T) {
			tracked := &trackedBody{Reader: strings.NewReader(body)}
			cli, err := client.New(client.WithAPIVersion("1.51"), client.WithScheme("http"), client.WithHTTPClient(&http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Query().Get("stream") != "false" || r.URL.Query().Get("one-shot") == "true" {
					t.Error("previous CPU sample not requested")
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: tracked}, nil
			})}))
			if err != nil {
				t.Fatal(err)
			}
			defer cli.Close()
			d := &DockerUtil{cli: cli, queryTimeout: time.Second}
			stats, err := d.GetContainerStats(context.Background(), "id")
			if strings.Contains(body, "invalid") {
				if err == nil {
					t.Fatal("accepted invalid stats")
				}
			} else if err != nil || stats.PreCPUStats.CPUUsage.TotalUsage != 10 {
				t.Fatalf("stats: %v, %v", stats, err)
			}
			if !tracked.closed.Load() {
				t.Fatal("Stats response body was not closed")
			}
		})
	}
}

func TestLogFollowCancellation(t *testing.T) {
	closed := make(chan struct{})
	d := testDockerUtil(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("follow") != "1" {
			t.Error("follow option lost")
		}
		_, _ = io.WriteString(w, "line\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(closed)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream, err := d.ContainerLogs(ctx, "id", client.ContainerLogsOptions{ShowStdout: true, Follow: true})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	data := make([]byte, 5)
	if _, err := io.ReadFull(stream, data); err != nil || string(data) != "line\n" {
		t.Fatalf("log stream: %q, %v", data, err)
	}
	cancel()
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("log HTTP stream remained open after cancellation")
	}
}

func TestNetworkTypeCompatibility(t *testing.T) {
	d := &DockerUtil{}
	ports := []container.PortSummary{{IP: netip.MustParseAddr("0.0.0.0"), PrivatePort: 80, PublicPort: 8080, Type: "tcp"}}
	settings := &container.NetworkSettingsSummary{Networks: map[string]*network.EndpointSettings{"bridge": {IPAddress: netip.MustParseAddr("172.18.0.2")}}}
	addresses := d.parseContainerNetworkAddresses("id", ports, settings, "app")
	if len(addresses) != 2 || addresses[0].IP.String() != "0.0.0.0" || addresses[0].Port != 8080 || addresses[1].IP.String() != "172.18.0.2" || addresses[1].Port != 80 {
		t.Fatalf("addresses: %v", addresses)
	}
	product := crossIPsWithPorts([]containers.NetworkAddress{{IP: net.ParseIP("127.0.0.1")}}, []network.Port{network.MustParsePort("80/tcp")})
	if len(product) != 1 || product[0].Port != 80 || product[0].Protocol != "tcp" {
		t.Fatalf("exposed ports: %v", product)
	}
}

func TestEventStreamLifecycle(t *testing.T) {
	const event = `{"Type":"container","Action":"start","Actor":{"ID":"id","Attributes":{"name":"app","image":"app:1"}},"time":123,"timeNano":123000000000}`
	t.Run("finite stream reaches EOF", func(t *testing.T) {
		d := testDockerUtil(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, event+"\n") }))
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		events, latest, err := d.LatestContainerEvents(ctx, time.Unix(120, 0))
		if err != nil || len(events) != 1 || latest.Unix() != 123 {
			t.Fatalf("events=%v latest=%v err=%v", events, latest, err)
		}
	})
	for _, mode := range []string{"reconnect", "blocked startup", "backpressure", "error backoff"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int32
			started := make(chan struct{}, 1)
			d := testDockerUtil(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				call := calls.Add(1)
				select {
				case started <- struct{}{}:
				default:
				}
				if mode == "blocked startup" {
					<-r.Context().Done()
					return
				}
				if mode == "error backoff" {
					http.Error(w, "retry", 500)
					return
				}
				if mode == "reconnect" && call == 1 {
					return
				}
				for range 10 {
					_, _ = fmt.Fprintln(w, event)
				}
				w.(http.Flusher).Flush()
				<-r.Context().Done()
			}))
			events, errs, err := d.SubscribeToContainerEvents("test")
			if err != nil {
				t.Fatal(err)
			}
			select {
			case <-started:
			case <-time.After(2 * time.Second):
				t.Fatal("stream not started")
			}
			if mode == "reconnect" {
				select {
				case <-events:
				case <-time.After(2 * time.Second):
					t.Fatal("no event after reconnect")
				}
				if calls.Load() < 2 {
					t.Fatal("stream did not reconnect")
				}
			}
			if err := d.UnsubscribeFromContainerEvents("test"); err != nil {
				t.Fatal(err)
			}
			select {
			case _, ok := <-errs:
				if ok {
					t.Fatal("error channel not closed")
				}
			case <-time.After(2 * time.Second):
				t.Fatal("unsubscribe did not cancel SDK stream")
			}
			for range events {
			}
		})
	}
}
