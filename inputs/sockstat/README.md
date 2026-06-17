# Sockstat Input Plugin

This plugin collects global socket usage statistics and memory allocation information of the operating system by reading the `/proc/net/sockstat` and `/proc/net/sockstat6` files on Linux systems.

**Supported Platforms:** Linux

*Note: On non-Linux platforms (like Windows, macOS) where `/proc/net/sockstat` does not exist, this plugin will not collect meaningful data.*

## Configuration

Generally, no special configuration is needed; just enable the plugin.

```toml
# Collect Linux sockstat metrics
# interval = 15

# No specific configuration parameters required
```

## Metrics

All metrics are prefixed with `sockstat_`. Common core metrics include:

- `sockstat_sockets_used`: Total number of used sockets on the system
- `sockstat_tcp_inuse`: Number of currently established TCP sockets
- `sockstat_tcp_orphan`: Number of orphaned TCP sockets
- `sockstat_tcp_tw`: Number of TCP sockets in `TIME_WAIT` state
- `sockstat_tcp_alloc`: Number of TCP sockets allocated
- `sockstat_tcp_mem`: Memory used by the TCP stack (in Pages)
- `sockstat_udp_inuse`: Number of currently established UDP sockets
- `sockstat_udp_mem`: Memory used by the UDP stack (in Pages)
- `sockstat_raw_inuse`: Number of currently established RAW sockets
- `sockstat_frag_inuse`: Number of currently established IP fragment sockets
- `sockstat_frag_memory`: Memory used by fragment sockets

These fields provide a snapshot of the socket usage on the system. This is extremely useful for monitoring the network connection pressure on the OS and troubleshooting network issues caused by excessive `TIME_WAIT` or orphan connections under high concurrency.

## Dashboards

These metrics are part of basic host monitoring and are typically integrated into global **System** or **Network** dashboards.
A dedicated basic Dashboard focusing exclusively on the sockstat socket state distribution and memory usage is also provided in this directory.
