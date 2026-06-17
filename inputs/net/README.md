# Net (Network Interfaces) Input Plugin

This plugin monitors network traffic. It primarily collects metrics for each network interface, including traffic (bytes in/out), packet counts, dropped packets, and transmission errors.

**Supported Platforms:** Windows, Linux, macOS, BSD, etc.

## Configuration

In most cases, you can leave the default configuration as is; the plugin will automatically discover and collect metrics for all active network interfaces. If you want to limit data collection to specific interfaces (e.g., for performance or noise reduction), you can use the `interfaces` option (which supports regex).

```toml
# Collect network interface metrics
# interfaces = ["eth0", "enp*"]
# ignore_interfaces = ["lo", "docker*", "veth*"]
```

## Metrics

All collected metrics are prefixed with `net_`. Key metrics include:

- `net_bytes_recv` / `net_bytes_sent`: Bytes received and sent (used to calculate bandwidth/throughput)
- `net_packets_recv` / `net_packets_sent`: Packets received and sent (used to calculate PPS)
- `net_errin` / `net_errout`: Error packets during receive and transmit
- `net_dropin` / `net_dropout`: Dropped packets during receive and transmit

All metrics are tagged with the `interface` label corresponding to the specific NIC name.

## Dashboards

These metrics are essential for basic server monitoring. Typically, network monitoring is grouped under a global **System** dashboard alongside CPU and memory metrics. 
For standalone viewing, a basic Dashboard focusing solely on network dimensions is also provided in this directory.
