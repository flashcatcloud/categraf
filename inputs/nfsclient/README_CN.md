# NFS Client 采集插件

该插件用于采集主机上作为 NFS 客户端挂载的网络文件系统（NFS）的性能与操作统计数据。
它通过读取系统的 `/proc/self/mountstats` 文件来收集诸如读写字节数、各项 NFS 操作（如 `GETATTR`, `READ`, `WRITE` 等）的请求次数及延迟指标。

**支持平台:** Linux

## 配置说明

```toml
# 采集 NFS 客户端指标
# interval = 60

[[instances]]
# 是否采集全量的 NFS 操作指标（默认只采集常用的关键操作）
fullstat = false

# 包含/排除特定的挂载点
# include_mounts = ["/mnt/nfs_share1"]
# exclude_mounts = ["/mnt/backup"]

# 包含/排除特定的 NFS 操作类型（大写，例如 "READ", "WRITE"）
# include_operations = []
# exclude_operations = []
```

## 采集指标

该插件支持 NFSv3 和 NFSv4，所有输出指标都会附带 `mountpoint`、`server` (NFS 服务端地址) 和 `export` (挂载的路径) 标签。

主要指标分类如下：
- **字节统计 (`nfsclient_bytes_*)**: `read`, `write`, `direct_read`, `direct_write`
- **事件统计 (`nfsclient_events_*)**: `inoderevalidates`, `dentryrevalidates`, `datainvalidates` 等
- **操作统计 (`nfsclient_ops_*`)**:
  - `ops`: 操作的总请求次数
  - `trans`: 发送的 RPC 请求次数
  - `timeouts`: 超时次数
  - `bytes_sent` / `bytes_recv`: 该操作发送和接收的字节数
  - `queue_time_ms`: 在队列中等待的时间 (单位：毫秒)
  - `response_time_ms`: 服务端响应时间 (单位：毫秒)
  - `total_time_ms`: 总耗时 (单位：毫秒)
  - `errors`: 操作错误数

*注意：每种 NFS 操作（如 READ, WRITE, GETATTR）都会生成对应的一组 `nfsclient_ops_*` 指标，并通过 `operation` 标签进行区分。*

## 监控大盘

本目录下提供了一个配套的 Dashboard (`dashboard.json`)，可用于监控各挂载点的读写吞吐量、读写延迟（Response Time / Queue Time）以及超时错误等情况。
