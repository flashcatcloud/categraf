# Node Exporter Input Plugin

This plugin directly integrates the core logic of the official Prometheus [node_exporter](https://github.com/prometheus/node_exporter) to collect comprehensive hardware and OS metrics for *nix systems.
Compared to Categraf's native plugins (like `cpu`, `mem`, `disk`), this plugin provides a 100% compatible metric set with the official `node_exporter`, making it easy for users to reuse the rich ecosystem of community-provided Grafana dashboards and alert rules based on node_exporter.

**Supported Platforms:** Linux, macOS, BSD, etc.

## Configuration

```toml
# Collect Node Exporter compatible metrics
# interval = 15

[[instances]]
# Typically, you just need to enable this plugin.
# If you need to toggle specific collectors, you can pass arguments to categraf's startup command line.
# Example: --collector.textfile.directory=/var/lib/node_exporter/textfile_collector
```

*Note: When enabling the `node_exporter` plugin in Categraf, its metrics may semantically overlap with Categraf's native basic plugins (`cpu`, `mem`, `disk`, etc.). It is generally recommended to either use Categraf's native basic plugin suite or solely enable this `node_exporter` plugin on a single machine.*

## Metrics

All metrics strictly follow the official Prometheus `node_exporter` naming conventions and are generally prefixed with `node_`. For example:
- `node_cpu_seconds_total`
- `node_memory_MemAvailable_bytes`
- `node_network_receive_bytes_total`
- `node_filesystem_free_bytes`
- `node_disk_read_bytes_total`

For detailed descriptions of the collectors, please refer directly to the [official node_exporter documentation](https://github.com/prometheus/node_exporter).

## Dashboards

Because this plugin is 100% compatible with the open-source `node_exporter`, you can directly import popular community dashboards in Grafana (e.g., Dashboard ID: 1860 "Node Exporter Full").
A minimalistic basic monitoring Dashboard (`dashboard.json`) is also provided in this directory for quick validation of data collection.
