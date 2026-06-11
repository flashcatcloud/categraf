# Netstat Filter Input Plugin

This plugin monitors network connections and aggregates statistics based on user-defined filtering criteria (such as source/destination IPs or ports). It is highly useful for precisely monitoring specific critical network connections (like database connection pools).

In addition to standard connection states, this plugin can collect the `recv-Q` (receive queue) and `send-Q` (send queue) of network sockets. This is valuable for reflecting the quality of network connections (e.g., a high RTT or slow client processing will cause `send-Q` to consistently stay above 0).

**Supported Platforms:** Windows, Linux (Note: `recv-Q` and `send-Q` metrics are currently fully supported only on Linux; on Windows, they default to 0).

## Configuration

You can filter by source IP (`laddr_ip`), source port (`laddr_port`), destination IP (`raddr_ip`), and destination port (`raddr_port`). If left empty or 0, they match anything.

```toml
# Collect TCP connection statistics based on specific filters
# interval = 15

# You can configure multiple instances if you have multiple independent rules
[[instances]]
# Use labels to distinguish which rule this data belongs to
# labels = { "filter"="mysql_backend" }

# Example rule: Only collect connections related to port 3306 (local or remote)
# laddr_ip = ""
# laddr_port = 0
# raddr_ip = ""
# raddr_port = 3306

[[instances]]
# labels = { "filter"="redis_backend" }
# raddr_port = 6379
```

When a filter matches multiple connections, the plugin will **sum up** the values for these connections (e.g., the number of connections in a given state, or the total `send_queue` and `recv_queue` bytes).

## Metrics

All metrics are prefixed with `netstat_filter_`. The metrics list includes:

- `netstat_filter_tcp_established`: Number of ESTABLISHED connections matching the filter
- `netstat_filter_tcp_syn_sent`: Number of SYN_SENT connections matching the filter
- `netstat_filter_tcp_syn_recv`: Number of SYN_RECV connections matching the filter
- `netstat_filter_tcp_time_wait`: Number of TIME_WAIT connections matching the filter
- `netstat_filter_tcp_close_wait`: Number of CLOSE_WAIT connections matching the filter
- (Other standard TCP states like `fin_wait1`, `fin_wait2`, `last_ack`, `listen`, `closing`, `none` are also supported)
- `netstat_filter_tcp_send_queue`: Total bytes queued in the send queues of matching connections (Linux only)
- `netstat_filter_tcp_recv_queue`: Total bytes queued in the receive queues of matching connections (Linux only)

## Dashboards

A basic Dashboard (`dashboard.json`) is provided in this directory. It supports displaying connection pool health and queue backlogs across different filtering rules (via the `filter` label), which is extremely helpful for application-layer network tuning.
