# XSKY API Input Plugin

This plugin collects capacity and performance monitoring data from XSKY software-defined storage systems (e.g., XEBS, XEOS) by directly querying their REST APIs using `XmsAuthTokens`. It monitors clusters, storage pools, volumes, nodes, and physical disks.

## Configuration

You can configure multiple XSKY Management Server (XMS) API endpoints and their corresponding tokens.

```toml
# Collect XSKY storage metrics
# interval = 60

[[instances]]
# XSKY storage type
# dss_type = "oss" # or gfs, eus

# List of XSKY XMS API endpoints
servers = ["http://10.10.10.10:8056"]

# List of access tokens corresponding to the servers
xms_auth_tokens = ["xxxxxxxxxxxxx"]

# Request timeout
# response_timeout = "5s"

```

## Metrics

By default, the plugin queries endpoints such as `/v1/os-users`, `/v1/os-buckets`, `/v1/dfs-quotas`, `/v1/fs-folders`, and `/v1/block-volumes`, mapping the returned status codes and counter values directly to metrics.
All metrics are prefixed with `xskyapi_`.

Typical metrics include:
- `xskyapi_oss_bucket_used_size`: Used size of OSS buckets.
- `xskyapi_dfs_quota`: DFS quota metrics.
- `xskyapi_block_volume_used_size`: Used size of block volumes.
- `xskyapi_oss_user_quota`: OSS user quota metrics.

## Dashboards

A basic companion Dashboard (`dashboard.json`) is provided in this directory to monitor the overall capacity of the XSKY storage cluster, the utilization rate of individual storage pools, and the distribution of disk errors, helping administrators proactively detect storage bottlenecks and hardware failures.
