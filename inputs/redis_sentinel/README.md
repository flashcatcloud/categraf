# Redis Sentinel Input Plugin

This plugin is specifically designed to collect operational and cluster topology metrics from Redis Sentinel nodes.
It is forked from `telegraf/redis_sentinel` and has been adapted and optimized for Categraf. By using this plugin, you can monitor the health and topology of backend Master and Slave nodes as seen by the Sentinels in real-time.

## Configuration

You can configure single or multiple Sentinel nodes within an `instance`. If you configure a list of `servers`, the instance will concurrently connect to each Sentinel, providing redundancy.

```toml
# Collect Redis Sentinel status
# interval = 15

[[instances]]
# List of Sentinel node addresses, formatted as "tcp://host:port" or "host:port"
servers = ["tcp://localhost:26379"]

# (Optional) Sentinel password
# password = "secret_password"

# TLS/SSL Configuration (if TLS is enabled)
# insecure_skip_verify = true
```

## Metrics

All metrics are prefixed with `redis_sentinel_`. Depending on the data collected, they are mainly divided into two categories:

### Basic Sentinel Metrics (`redis_sentinel_*`)
E.g., `redis_sentinel_uptime_in_seconds`, `redis_sentinel_connected_clients`, `redis_sentinel_mem_used`, etc., which reflect the Sentinel process's liveness and basic resource usage.

### Master / Slave Status Metrics
These metrics carry labels such as `master` (the master's name) to reflect the cluster topology as seen by Sentinel:
- `redis_sentinel_master_slaves`: Number of Slaves attached to the current Master
- `redis_sentinel_master_sentinels`: Number of Sentinel nodes monitoring this Master
- `redis_sentinel_master_status`: Master status (typically "ok" maps to 1, others map to 0 or specific error codes)
- `redis_sentinel_master_failover_state`: Current state value of the failover process

## Dashboards

A companion Dashboard (`dashboard.json`) is provided in this directory to centrally observe the Redis Master liveness, Slave counts, and the basic operational status of the Sentinels themselves.
