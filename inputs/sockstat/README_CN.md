# Sockstat 采集插件

该插件通过读取 Linux 系统的 `/proc/net/sockstat` 和 `/proc/net/sockstat6` 文件，采集操作系统的全局 Socket 使用情况和内存分配信息。

**支持平台:** Linux

*注意：非 Linux 平台（如 Windows, macOS）由于不存在 `/proc/net/sockstat`，此插件将不会采集到有效数据。*

## 配置说明

通常无需任何特殊配置，直接启用该插件即可。

```toml
# 采集 Linux sockstat 状态
# interval = 15

# 无需任何特定配置参数
```

## 采集指标

所有指标默认会附带 `sockstat_` 作为前缀。常见的核心指标包括：

- `sockstat_sockets_used`: 系统中当前正在被使用的 Socket 总数
- `sockstat_tcp_inuse`: 当前建立的 TCP Socket 数量
- `sockstat_tcp_orphan`: 当前处于孤儿状态 (Orphan) 的 TCP Socket 数量
- `sockstat_tcp_tw`: 当前处于 `TIME_WAIT` 状态的 TCP Socket 数量
- `sockstat_tcp_alloc`: 当前已分配的 TCP Socket 数量
- `sockstat_tcp_mem`: TCP 协议栈所消耗的内存量 (单位为 Page 页数)
- `sockstat_udp_inuse`: 当前建立的 UDP Socket 数量
- `sockstat_udp_mem`: UDP 协议栈所消耗的内存量 (单位为 Page 页数)
- `sockstat_raw_inuse`: 当前建立的 RAW Socket 数量
- `sockstat_frag_inuse`: 当前正在处理的 IP 分片 (Fragment) 数量
- `sockstat_frag_memory`: IP 分片重组所消耗的内存量

这些字段提供了系统上套接字使用情况的一个快照，对于监控操作系统的网络连接压力、排查高并发下的 `TIME_WAIT` 或孤儿连接过多导致的网络问题非常有用。

## 监控大盘

这些指标是主机基础监控的一部分，通常会被整合在 **System (主机系统)** 或 **Network (网络)** 全局大盘中。
本目录下也为您提供了一个仅针对 sockstat Socket 状态分布与内存占用的专属基础 Dashboard。
