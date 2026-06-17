# Redis Sentinel 采集插件

该插件专门用于采集 Redis Sentinel（哨兵节点）的运行状态与集群拓扑指标。
它 fork 自 `telegraf/redis_sentinel`，并进行了适配和优化。使用此插件，您可以实时监控 Sentinel 对后端 Master 和 Slave 节点的监控状态。

## 配置说明

支持通过 `instances` 配置单个 Sentinel 节点或多个 Sentinel 节点，如果您配置了一个包含多个 Sentinel 的 `servers` 列表，该 Instance 内部将会并发连接每个 Sentinel，从而提供一定的冗余探测。

```toml
# 采集 Redis Sentinel 状态
# interval = 15

[[instances]]
# Sentinel 节点地址列表，格式为 "tcp://host:port" 或 "unix:///path/to/socket"
servers = ["tcp://localhost:26379"]

# (可选) Sentinel 密码
# password = "secret_password"

# TLS/SSL 配置 (如果启用了 TLS)
# insecure_skip_verify = true
```

## 采集指标

所有的指标均以 `redis_sentinel_` 作为前缀。根据采集内容不同，主要包含两类数据：

### Sentinel 自身基础指标 (`redis_sentinel_*`)
例如 `redis_sentinel_uptime_ns`, `redis_sentinel_clients`, `redis_sentinel_sentinel_masters` 等，用于反映 Sentinel 进程的存活与基础资源开销。TCP 地址会带有 `source` 和 `port` 标签，Unix Socket 地址会带有 `socket` 标签。

### Master / Replica 状态指标
这些指标携带 `master` (名字) 等标签，用于反映 Sentinel 眼中的集群拓扑：
- `redis_sentinel_masters_num_slaves`: 当前 Master 下挂载的副本数量
- `redis_sentinel_masters_num_other_sentinels`: 监控该 Master 的其他 Sentinel 节点数
- `redis_sentinel_masters_has_quorum`: Sentinel 是否认为该 Master 满足 quorum
- `redis_sentinel_replicas_slave_repl_offset`: 副本复制偏移量

## 监控大盘

本目录下提供了一个配套的 Dashboard (`dashboard.json`)，用于集中观测 Sentinel 集群监控下的 Redis Master 存活状态、Slave 挂载数量，以及 Sentinel 自身的基础运行状态。
