# NATS Input Plugin

This plugin collects operational metrics from NATS message servers. It gathers real-time statistics by accessing the monitoring HTTP API (`/varz` endpoint) provided by the NATS Server.

## Configuration

For this plugin to work, your NATS server must have its HTTP monitoring port enabled (by setting `http_port` or `https_port` in the NATS configuration file).

```toml
# Collect NATS monitoring metrics
# interval = 60

[[instances]]
# NATS monitoring endpoint (must include schema and port)
server = "http://localhost:8222"
```

## Metrics

All collected metrics will be tagged with the `server` label corresponding to the scraped endpoint.
Key metrics include:

- `nats_in_msgs` / `nats_out_msgs`: Total number of messages received/sent
- `nats_in_bytes` / `nats_out_bytes`: Total number of bytes received/sent
- `nats_uptime`: NATS server uptime
- `nats_cores`: Number of CPU cores allocated to NATS
- `nats_mem`: Memory footprint of NATS
- `nats_connections`: Number of currently connected clients
- `nats_total_connections`: Total number of connections accepted historically
- `nats_subscriptions`: Number of active subscriptions
- `nats_slow_consumers`: Number of slow consumers
- `nats_routes`: Number of cluster routes
- `nats_remotes`: Number of remote connections

## Dashboards

A matching Dashboard (`dashboard.json`) is provided in this directory. You can import this dashboard into Grafana or Nightingale to monitor the operational status of your NATS servers (including connection counts, throughput, subscription counts, and other core metrics).
