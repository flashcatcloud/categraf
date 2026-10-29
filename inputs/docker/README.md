# Docker Input Plugin

The plugin uses the standalone Moby client/API modules and requires Engine API
1.40 or newer (Docker Engine 19.03+). API negotiation is enabled by default.
For `endpoint = "ENV"`, `DOCKER_HOST`, `DOCKER_TLS_VERIFY`, `DOCKER_CERT_PATH`
and `DOCKER_API_VERSION` retain their SDK meanings. Fixed endpoints use the
plugin's TLS settings. Non-streaming Stats explicitly requests the previous
sample to preserve CPU delta calculations.

The Docker input plugin collects performance metrics (CPU, Memory, Network, Block I/O, state, etc.) from locally running Docker containers. This plugin is forked from `telegraf/inputs.docker`.

## Differences from Telegraf

1. The `container_id` is exposed as a Tag (Label) instead of a Field to enable granular querying and aggregation.
2. Several less commonly used metrics have been removed to reduce storage pressure on the time-series database.

## Configuration

```toml
[[instances]]
  # The API Endpoint for the Docker Daemon
  # Supports unix:// or tcp:// protocols
  endpoint = "unix:///var/run/docker.sock"

  # Timeout for metrics gathering
  timeout = "5s"

  # Whether to include the container_id as a tag
  container_id_label_enable = true

  # Whether to truncate the container_id to 12 characters
  container_id_label_short_style = false
```

### Disabling the Plugin

If you wish to disable this plugin, you can do so using either of the following methods:
- **Method 1**: Rename the `conf/input.docker` directory so that it no longer starts with `input.`.
- **Method 2**: Leave the `endpoint` configuration field empty.

## FAQ

### 1. Permission Issues

Categraf requires permission to read the docker socket (`unix:///var/run/docker.sock`). It is recommended to run Categraf as `root`.
If you prefer to run Categraf as a non-root user (e.g., `categraf`), you must add that user to the `docker` group:

```bash
sudo usermod -aG docker categraf
```

### 2. Running Categraf Inside a Container

If Categraf itself is running inside a Docker container, you must mount the host's docker socket into the Categraf container so it can access the Docker Daemon API.

**Via Docker CLI:**
```bash
docker run -v /var/run/docker.sock:/var/run/docker.sock ...
```

**Via Docker Compose:**
```yaml
volumes:
  - /var/run/docker.sock:/var/run/docker.sock
```

## Metrics

The plugin collects comprehensive container resource usage. Key metrics include:
- `docker_container_cpu_usage_percent`: Container CPU usage percentage
- `docker_container_mem_usage_percent`: Container Memory usage percentage
- `docker_container_mem_limit`: Container Memory limit (Bytes)
- `docker_container_net_rx_bytes`: Container network received bytes
- `docker_container_net_tx_bytes`: Container network transmitted bytes
- `docker_container_status_*`: Container state fields such as PID, exit code, restart count, and uptime. The current state is exposed as the `container_status` tag.

## SDK migration tests

Run the SDK, metrics, log/event, exec, discovery and storage regressions from the
repository root:

```sh
go test -race ./inputs/docker ./logs/util/docker ./inputs/keepalived/types/container ./prometheus ./inputs/gnmi ./internal/gnmipath
```

On Windows, `go test -v -run '^TestMobyNamedPipe$' ./inputs/docker` exercises a
real Windows named pipe with the production client constructors, API calls and
cancellation. The `Windows Named Pipe Regression` CI job runs this fixture on a
Windows runner. It needs no Docker installation; it does not replace deployment
validation against an actual Windows Docker daemon.

Real Engine tests are opt-in. Create a disposable busy container, then run the
same tests against its daemon:

```sh
docker run -d --rm --name categraf-sdk-test --cpus 0.25 --expose 18080 alpine:3.22 \
  sh -c 'echo categraf-sdk-log; while :; do :; done'
export CATEGRAF_DOCKER_TEST_HOST=unix:///var/run/docker.sock
export CATEGRAF_DOCKER_TEST_NAME=categraf-sdk-test
export CATEGRAF_DOCKER_TEST_CONTAINER=$(docker inspect --format '{{.Id}}' categraf-sdk-test)
go test -v -count=1 -run Integration ./inputs/docker ./logs/util/docker ./inputs/keepalived/types/container ./prometheus
docker stop -t 2 categraf-sdk-test
```

The fixture needs a shell, a CPU workload, and the `categraf-sdk-log` log line.
The tests inspect metrics, previous CPU samples, image resolution, logs, recent
events, exec stream demultiplexing and Docker discovery. They skip when endpoint
variables are absent. Tests configure their own client from the supplied host;
the example uses a local Unix socket without TLS.

For Swarm regression, use a separate disposable daemon with a running service,
then set `CATEGRAF_DOCKER_TEST_SWARM=true`, `CATEGRAF_DOCKER_TEST_SERVICE` to its
service ID, and `CATEGRAF_DOCKER_TEST_NODE` to its node ID. This additionally
exercises the Docker metrics service/task/node calls and all three Prometheus
Swarm discovery roles through YAML configuration.
