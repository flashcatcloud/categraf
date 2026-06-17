# NFS Client Input Plugin

This plugin collects performance and operational statistics for Network File Systems (NFS) mounted on the host as a client.
It gathers metrics such as read/write bytes, request counts, and latency for various NFS operations (e.g., `GETATTR`, `READ`, `WRITE`) by parsing the `/proc/self/mountstats` file.

**Supported Platforms:** Linux

## Configuration

```toml
# Collect NFS client metrics
# interval = 60

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
- **Default READ/WRITE statistics (`nfsclient_nfsstat_*`)**: `ops`, `retrans`, `bytes`, `rtt`, `exe`, and `rtt_per_op`, distinguished by the `nfsstat_operation` label.
- **Full bytes statistics (`nfsclient_nfs_bytes_*`)**: `normalreadbytes`, `normalwritebytes`, `directreadbytes`, `directwritebytes`, etc. These require `fullstat = true`.
- **Full event statistics (`nfsclient_nfs_events_*`)**: `inoderevalidates`, `dentryrevalidates`, `datainvalidates`, etc. These require `fullstat = true`.
- **Full operation statistics (`nfsclient_nfs_ops_*`)**:
  - `ops`: Total number of requests for the operation
  - `trans`: Number of RPC requests transmitted
  - `timeouts`: Number of timeouts
  - `bytes_sent` / `bytes_recv`: Bytes sent and received for the operation
  - `queue_time`: Time spent waiting in the queue
  - `response_time`: Time spent waiting for the server to respond
  - `total_time`: Total execution time
  - `errors`: Number of operational errors

*Note: Each NFS operation (such as READ, WRITE, GETATTR) generates a corresponding set of `nfsclient_nfs_ops_*` metrics when `fullstat = true`, distinguished by the `operation` label.*

## Dashboards

A companion Dashboard (`dashboard.json`) is provided in this directory. It uses the default `nfsclient_nfsstat_*` metrics to monitor read/write throughput, latency, operations, and retransmits for each mount point.
