//go:build windows

package docker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"flashcat.cloud/categraf/internal/dockertest"
	"github.com/Microsoft/go-winio"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

// Exercise real Windows named-pipe I/O with the same client constructors as
// production. The fixture speaks the Engine API and needs no Docker daemon.
func TestMobyNamedPipe(t *testing.T) {
	for _, endpoint := range []string{"explicit", "ENV"} {
		t.Run(endpoint, func(t *testing.T) {
			name := fmt.Sprintf("categraf-sdk-%d-%d", os.Getpid(), time.Now().UnixNano())
			listener, err := winio.ListenPipe(`\\.\pipe\`+name, nil)
			if err != nil {
				t.Fatal(err)
			}
			var blocked atomic.Bool
			handler := metricsHandler(t)
			server := &http.Server{Handler: dockertest.Handler(t, "1.51", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if blocked.Load() && r.URL.Path == "/info" {
					<-r.Context().Done()
					return
				}
				handler.ServeHTTP(w, r)
			}))}
			stopped := make(chan error, 1)
			go func() { stopped <- server.Serve(listener) }()
			t.Cleanup(func() {
				if err := server.Close(); err != nil {
					t.Error(err)
				}
				if err := <-stopped; !errors.Is(err, http.ErrServerClosed) {
					t.Error(err)
				}
			})
			host := "npipe:////./pipe/" + name
			var cli Client
			if endpoint == "ENV" {
				t.Setenv("DOCKER_HOST", host)
				t.Setenv("DOCKER_API_VERSION", "")
				t.Setenv("DOCKER_CERT_PATH", "")
				t.Setenv("DOCKER_TLS_VERIFY", "")
				cli, err = NewEnvClient()
			} else {
				cli, err = NewClient(host, nil)
			}
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := cli.Close(); err != nil {
					t.Error(err)
				}
			})
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if ping, err := cli.Ping(ctx); err != nil || ping.APIVersion != "1.51" {
				t.Fatalf("pipe negotiation: %v, %v", ping, err)
			}
			if info, err := cli.Info(ctx); err != nil || info.Name != "engine-test" {
				t.Fatalf("pipe info: %v, %v", info, err)
			}
			containers, err := cli.ContainerList(ctx, client.ContainerListOptions{Filters: make(client.Filters).Add("status", "running")})
			if err != nil || len(containers) != 1 || containers[0].ID != "container-test" {
				t.Fatalf("pipe list: %v, %v", containers, err)
			}
			if inspected, err := cli.ContainerInspect(ctx, "container-test"); err != nil || inspected.State == nil {
				t.Fatalf("pipe inspect: %v, %v", inspected, err)
			}
			stats, err := cli.ContainerStats(ctx, "container-test", false)
			if err != nil {
				t.Fatal(err)
			}
			var sample container.StatsResponse
			err = json.NewDecoder(stats.Body).Decode(&sample)
			if closeErr := stats.Body.Close(); closeErr != nil {
				t.Error(closeErr)
			}
			if err != nil || sample.PreCPUStats.CPUUsage.TotalUsage != 100 {
				t.Fatalf("pipe stats: %v, %v", sample, err)
			}
			blocked.Store(true)
			deadline, stop := context.WithTimeout(ctx, 100*time.Millisecond)
			defer stop()
			if _, err := cli.Info(deadline); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("pipe request cancellation: %v", err)
			}
		})
	}
}
