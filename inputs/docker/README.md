# Docker Input Plugin

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
- `docker_container_status`: The running status of the container
