package container

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"syscall"
	"testing"

	"flashcat.cloud/categraf/internal/dockertest"
	"github.com/moby/moby/client"
)

func TestDockerExecSDK(t *testing.T) {
	for _, failure := range []string{"", "create", "attach"} {
		t.Run(failure, func(t *testing.T) {
			server := dockertest.NewServer(t, "1.51", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if (failure == "create" && strings.HasSuffix(r.URL.Path, "/exec")) || (failure == "attach" && strings.HasSuffix(r.URL.Path, "/start")) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(500)
					_, _ = io.WriteString(w, `{"message":"exec failure"}`)
					return
				}
				switch r.URL.Path {
				case "/containers/app/exec":
					var request struct {
						AttachStdout, AttachStderr bool
						Cmd                        []string
					}
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil || !request.AttachStdout || !request.AttachStderr || strings.Join(request.Cmd, " ") != "keepalived -v" {
						t.Errorf("exec request changed: %+v, %v", request, err)
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"Id":"exec-id"}`)
				case "/exec/exec-id/start":
					_, _ = io.Copy(io.Discard, r.Body)
					conn, buffer, err := w.(http.Hijacker).Hijack()
					if err != nil {
						t.Error(err)
						return
					}
					defer conn.Close()
					_, _ = buffer.WriteString("HTTP/1.1 101 UPGRADED\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n\r\n")
					for index, text := range []string{"Keepalived v2.3.0\n", "stderr detail\n"} {
						header := make([]byte, 8)
						header[0] = byte(index + 1)
						binary.BigEndian.PutUint32(header[4:], uint32(len(text)))
						_, _ = buffer.Write(header)
						_, _ = buffer.WriteString(text)
					}
					if err := buffer.Flush(); err != nil {
						t.Error(err)
					}
				case "/containers/app/kill":
					if r.URL.Query().Get("signal") != "10" {
						t.Errorf("signal option lost: %s", r.URL.RawQuery)
					}
					w.WriteHeader(http.StatusNoContent)
				default:
					t.Errorf("unexpected exec API: %s", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			cli, err := client.New(client.WithHost("tcp://"+strings.TrimPrefix(server.URL, "http://")), client.WithScheme("http"))
			if err != nil {
				t.Fatal(err)
			}
			defer cli.Close()
			k := &KeepalivedContainerCollectorHost{dockerCli: cli, containerName: "app"}
			output, err := k.dockerExecCmd([]string{"keepalived", "-v"})
			if failure != "" {
				if err == nil {
					t.Fatal("expected exec error")
				}
				return
			}
			if err != nil || output.String() != "Keepalived v2.3.0\nstderr detail\n" {
				t.Fatalf("exec output: %v, %v", output, err)
			}
			if err := k.dockerSignal(syscall.Signal(10)); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestDockerExecIntegration(t *testing.T) {
	host, id := os.Getenv("CATEGRAF_DOCKER_TEST_HOST"), os.Getenv("CATEGRAF_DOCKER_TEST_CONTAINER")
	if host == "" || id == "" {
		t.Skip("set CATEGRAF_DOCKER_TEST_HOST and CATEGRAF_DOCKER_TEST_CONTAINER for a disposable container")
	}
	cli, err := client.New(client.WithHost(host))
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()
	k := &KeepalivedContainerCollectorHost{dockerCli: cli, containerName: id}
	output, err := k.dockerExecCmd([]string{"sh", "-c", "printf stdout; printf stderr >&2"})
	// Docker may interleave stdout and stderr in either order. Both streams
	// must be demultiplexed without leaking their binary frame headers.
	if err != nil {
		t.Fatal(err)
	}
	if got := output.String(); len(got) != len("stdoutstderr") || !strings.Contains(got, "stdout") || !strings.Contains(got, "stderr") {
		t.Fatalf("exec output: %q", got)
	}
}
