# XSKY API 采集插件

该插件通过调用 XSKY 星辰天合存储系统 (XEBS/XEOS 等) 的 REST API (`XmsAuthTokens`)，直接收集存储集群、存储池、卷 (Volume)、节点以及硬盘状态的相关容量与性能监控数据。

## 配置说明

你可以配置多个 XSKY 管理节点的 API Server 和对应的 Token。

```toml
# 采集 XSKY 存储指标
# interval = 60

[[instances]]
# XSKY 存储类型
# dss_type = "xsky"

# XSKY 管理节点 (XMS) 的 API 地址列表
servers = ["http://10.10.10.10:8056"]

# 与 API 地址对应的访问 Token 列表
xms_auth_tokens = ["xxxxxxxxxxxxx"]

# 请求超时时间
# response_timeout = "5s"

# (可选) 指定将哪些 JSON 字段转化为 Label (而非指标字段)
# tag_keys = ["pool_id", "volume_id"]
```

## 采集指标

插件默认会将从 `/api/v1/clusters`, `/api/v1/pools`, `/api/v1/volumes`, `/api/v1/hosts` 和 `/api/v1/disks` 等 API 获取到的状态码和计数值直接映射为指标。
所有的指标统一带有 `xskyapi_` 前缀。

典型指标举例：
- `xskyapi_cluster_status`: 集群整体健康状态。
- `xskyapi_pool_allocated_capacity`: 存储池已分配容量。
- `xskyapi_volume_iops` / `xskyapi_volume_bandwidth`: 卷的 IOPS 与吞吐性能数据（具体字段名依赖 API 实际返回）。
- `xskyapi_disk_status`: 硬盘的在位与健康状态。

## 监控大盘

本目录下提供了一个基础的 Dashboard (`dashboard.json`)，用于监控 XSKY 存储集群的整体容量、各存储池的使用率以及硬盘错误状态分布，帮助管理员提前发现存储瓶颈和硬件故障。
