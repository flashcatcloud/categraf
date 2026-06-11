# XSKY API Input Plugin

This plugin collects capacity and performance monitoring data from XSKY software-defined storage systems (e.g., XEBS, XEOS) by directly querying their REST APIs using `XmsAuthTokens`. It monitors clusters, storage pools, volumes, nodes, and physical disks.

## Configuration

You can configure multiple XSKY Management Server (XMS) API endpoints and their corresponding tokens.

```toml
# Collect XSKY storage metrics
# interval = 60

[[instances]]
# XSKY storage type
# dss_type = "xsky"

# List of XSKY XMS API endpoints
servers = ["http://10.10.10.10:8056"]

# List of access tokens corresponding to the servers
xms_auth_tokens = ["xxxxxxxxxxxxx"]

# Request timeout
# response_timeout = "5s"

# (Optional) Specify JSON keys to be converted into labels instead of metrics
# tag_keys = ["pool_id", "volume_id"]
```

## Metrics

By default, the plugin queries endpoints such as `/api/v1/clusters`, `/api/v1/pools`, `/api/v1/volumes`, `/api/v1/hosts`, and `/api/v1/disks`, mapping the returned status codes and counter values directly to metrics.
All metrics are prefixed with `xskyapi_`.

Typical metrics include:
- `xskyapi_cluster_status`: Overall cluster health status.
- `xskyapi_pool_allocated_capacity`: Allocated capacity of storage pools.
- `xskyapi_volume_iops` / `xskyapi_volume_bandwidth`: IOPS and bandwidth performance data for volumes (exact names depend on API returns).
- `xskyapi_disk_status`: Disk presence and health status.

## Dashboards

A basic companion Dashboard (`dashboard.json`) is provided in this directory to monitor the overall capacity of the XSKY storage cluster, the utilization rate of individual storage pools, and the distribution of disk errors, helping administrators proactively detect storage bottlenecks and hardware failures.
