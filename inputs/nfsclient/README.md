# NFS Client Input Plugin

This plugin collects performance and operational statistics for Network File Systems (NFS) mounted on the host as a client.
It gathers metrics such as read/write bytes, request counts, and latency for various NFS operations (e.g., `GETATTR`, `READ`, `WRITE`) by parsing the `/proc/self/mountstats` file.

**Supported Platforms:** Linux

## Configuration

```toml
# Collect NFS client metrics
# interval = 60

[[instances]]
# Whether to collect full statistics for all NFS operations (defaults to collecting only key operations)
fullstat = false

# Include/exclude specific mount points
# include_mounts = ["/mnt/nfs_share1"]
# exclude_mounts = ["/mnt/backup"]

# Include/exclude specific NFS operation types (uppercase, e.g., "READ", "WRITE")
# include_operations = []
# exclude_operations = []
```

## Metrics

The plugin supports NFSv3 and NFSv4. All metrics are tagged with `mountpoint`, `server` (NFS server address), and `export` (exported path).

Key metric categories include:
- **Bytes Statistics (`nfsclient_bytes_*`)**: `read`, `write`, `direct_read`, `direct_write`
- **Event Statistics (`nfsclient_events_*)**: `inoderevalidates`, `dentryrevalidates`, `datainvalidates`, etc.
- **Operation Statistics (`nfsclient_ops_*`)**:
  - `ops`: Total number of requests for the operation
  - `trans`: Number of RPC requests transmitted
  - `timeouts`: Number of timeouts
  - `bytes_sent` / `bytes_recv`: Bytes sent and received for the operation
  - `queue_time_ms`: Time spent waiting in the queue (in milliseconds)
  - `response_time_ms`: Time spent waiting for the server to respond (in milliseconds)
  - `total_time_ms`: Total execution time (in milliseconds)
  - `errors`: Number of operational errors

*Note: Each NFS operation (such as READ, WRITE, GETATTR) generates a corresponding set of `nfsclient_ops_*` metrics, distinguished by the `operation` label.*

## Dashboards

A companion Dashboard (`dashboard.json`) is provided in this directory. It can be used to monitor the read/write throughput, latency (Response Time / Queue Time), and timeout errors for each mount point.
