//go:build !no_logs

package docker

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"flashcat.cloud/categraf/logs/util/containers"
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/client"
)

func TestDockerLogsIntegration(t *testing.T) {
	host, id := os.Getenv("CATEGRAF_DOCKER_TEST_HOST"), os.Getenv("CATEGRAF_DOCKER_TEST_CONTAINER")
	if host == "" || id == "" {
		t.Skip("set CATEGRAF_DOCKER_TEST_HOST and CATEGRAF_DOCKER_TEST_CONTAINER for a disposable container")
	}
	cli, err := client.New(client.WithHost(host))
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()
	d := &DockerUtil{cli: cli, queryTimeout: 10 * time.Second, cfg: &Config{filter: &containers.Filter{}}, imageNameBySha: make(map[string]string)}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	inspected, err := d.InspectNoCache(ctx, id, false)
	if err != nil {
		t.Fatal(err)
	}
	if image, err := d.ResolveImageName(ctx, inspected.Image); err != nil || image == "" {
		t.Fatalf("resolve image: %s, %v", image, err)
	}
	stats, err := d.GetContainerStats(ctx, id)
	if err != nil || stats.PreRead.IsZero() || stats.PreCPUStats.CPUUsage.TotalUsage == 0 {
		t.Fatalf("missing previous CPU sample: %v, %v", stats, err)
	}
	stream, err := d.ContainerLogs(ctx, id, client.ContainerLogsOptions{ShowStdout: true, ShowStderr: true, Tail: "10"})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	var output strings.Builder
	if inspected.Config.Tty {
		_, err = io.Copy(&output, stream)
	} else {
		_, err = stdcopy.StdCopy(&output, &output, stream)
	}
	if err != nil || !strings.Contains(output.String(), "categraf-sdk-log") {
		t.Fatalf("logs: %q, %v", output.String(), err)
	}
	events, _, err := d.LatestContainerEvents(ctx, time.Now().Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, event := range events {
		if event.ContainerID == id {
			found = true
		}
	}
	if !found {
		t.Fatal("test container not present in recent events")
	}
}
