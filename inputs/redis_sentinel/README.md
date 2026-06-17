# Redis Sentinel Input Plugin

This plugin is specifically designed to collect operational and cluster topology metrics from Redis Sentinel nodes.
It is forked from `telegraf/redis_sentinel` and has been adapted and optimized for Categraf. By using this plugin, you can monitor the health and topology of backend Master and Slave nodes as seen by the Sentinels in real-time.

## Configuration

You can configure single or multiple Sentinel nodes within an `instance`. If you configure a list of `servers`, the instance will concurrently connect to each Sentinel, providing redundancy.

```toml
# Collect Redis Sentinel status
# interval = 15

[[instances]]
# List of Sentinel node addresses, formatted as "tcp://host:port" or "unix:///path/to/socket"
servers = ["tcp://localhost:26379"]

# (Optional) Sentinel password
# password = "secret_password"

# TLS/SSL Configuration (if TLS is enabled)
# insecure_skip_verify = true
```

## Metrics

All metrics are prefixed with `redis_sentinel_`. Depending on the data collected, they are mainly divided into two categories:

### Basic Sentinel Metrics (`redis_sentinel_*`)
E.g., `redis_sentinel_uptime_ns`, `redis_sentinel_clients`, `redis_sentinel_sentinel_masters`, etc., which reflect the Sentinel process's liveness and basic resource usage. The source Sentinel is identified by `source` and `port` labels for TCP endpoints, or the `socket` label for Unix sockets.

### Master / Replica Status Metrics
These metrics carry labels such as `master` (the master's name) to reflect the cluster topology as seen by Sentinel:
- `redis_sentinel_masters_num_slaves`: Number of replicas attached to the current master
- `redis_sentinel_masters_num_other_sentinels`: Number of other Sentinel nodes monitoring this master
- `redis_sentinel_masters_has_quorum`: Whether Sentinel reports quorum for the master
- `redis_sentinel_replicas_slave_repl_offset`: Replica replication offset

## Dashboards

A companion Dashboard (`dashboard.json`) is provided in this directory to centrally observe the Redis Master liveness, Slave counts, and the basic operational status of the Sentinels themselves.
