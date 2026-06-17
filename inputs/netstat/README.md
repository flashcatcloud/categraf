# Netstat Input Plugin

This plugin monitors network connection states. It primarily collects statistics on the distribution of various TCP/UDP connection states within the operating system, such as the number of connections in `TIME_WAIT`, `ESTABLISHED`, or `CLOSE_WAIT` states.

**Supported Platforms:** Windows, Linux, macOS, BSD, etc.

## Configuration

```toml
# Collect network TCP connection state statistics
# Usually requires no specific configuration. Just leave it enabled.
```

## Metrics

All collected metrics are prefixed with `netstat_`. Key metrics include:

- `netstat_tcp_established`: Number of TCP connections in the ESTABLISHED state
- `netstat_tcp_syn_sent`: Number of connections in the SYN_SENT state
- `netstat_tcp_syn_recv`: Number of connections in the SYN_RECV state
- `netstat_tcp_fin_wait1`: Number of connections in the FIN_WAIT1 state
- `netstat_tcp_fin_wait2`: Number of connections in the FIN_WAIT2 state
- `netstat_tcp_time_wait`: Number of connections in the TIME_WAIT state (high values may indicate port exhaustion)
- `netstat_tcp_close`: Number of connections in the CLOSE state
- `netstat_tcp_close_wait`: Number of connections in the CLOSE_WAIT state (high values may indicate an unresponsive application failing to release connections)
- `netstat_tcp_last_ack`: Number of connections in the LAST_ACK state
- `netstat_tcp_listen`: Number of sockets in the LISTEN state
- `netstat_tcp_closing`: Number of connections in the CLOSING state
- `netstat_tcp_none`: Number of TCP connections with an unknown state
- `netstat_udp_socket`: Number of active UDP sockets

## Dashboards

These metrics are essential for basic server monitoring. Typically, OS network connection monitoring is unified under a global **System** dashboard alongside CPU and disk metrics.
For standalone viewing, a basic Dashboard focusing solely on TCP/UDP connection states is also provided in this directory.
