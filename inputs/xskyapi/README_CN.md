# XSKY API 采集插件

该插件通过调用 XSKY 星辰天合存储系统 (XEBS/XEOS 等) 的 REST API (`XmsAuthTokens`)，直接收集存储集群、存储池、卷 (Volume)、节点以及硬盘状态的相关容量与性能监控数据。

## 配置说明

你可以配置多个 XSKY 管理节点的 API Server 和对应的 Token。

```toml
# 采集 XSKY 存储指标
# interval = 60

[[instances]]
# XSKY 存储类型
# dss_type = "oss" # or gfs, eus

# XSKY 管理节点 (XMS) 的 API 地址列表
servers = ["http://10.10.10.10:8056"]

# 与 API 地址对应的访问 Token 列表
xms_auth_tokens = ["xxxxxxxxxxxxx"]

# 请求超时时间
# response_timeout = "5s"
```

## 采集指标

插件默认会将从 `/v1/os-users`, `/v1/os-buckets`, `/v1/dfs-quotas`, `/v1/fs-folders` 和 `/v1/block-volumes` 等 API 获取到的状态码和计数值直接映射为指标。
所有的指标统一带有 `xskyapi_` 前缀。

典型指标举例：
- `xskyapi_oss_bucket_used_size`: OSS Bucket 已使用容量。
- `xskyapi_dfs_quota`: DFS Quota 指标。
- `xskyapi_block_volume_used_size`: 块存储卷（Block Volume）已使用容量。
- `xskyapi_oss_user_quota`: OSS 用户配额指标。

## 监控大盘

本目录下提供了一个基础的 Dashboard (`dashboard.json`)，用于监控 XSKY 存储集群的整体容量、各存储池的使用率以及硬盘错误状态分布，帮助管理员提前发现存储瓶颈和硬件故障。
