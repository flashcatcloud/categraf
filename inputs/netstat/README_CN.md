# Netstat 采集插件

网络连接状态监控插件。该插件主要用于采集操作系统中各类 TCP/UDP 连接的状态分布情况，例如有多少个处于 `TIME_WAIT`、`ESTABLISHED`、`CLOSE_WAIT` 状态的连接。

**支持平台:** Windows, Linux, macOS, BSD 等

## 配置说明

```toml
# 采集网络 TCP 连接状态统计
# 通常无需任何特殊配置，保持默认启用即可。
```

## 采集指标

所有收集到的指标名称前缀为 `netstat_`。主要指标如下：

- `netstat_tcp_established`: 已建立连接的 TCP 数量
- `netstat_tcp_syn_sent`: 处于 SYN_SENT 状态的连接数
- `netstat_tcp_syn_recv`: 处于 SYN_RECV 状态的连接数
- `netstat_tcp_fin_wait1`: 处于 FIN_WAIT1 状态的连接数
- `netstat_tcp_fin_wait2`: 处于 FIN_WAIT2 状态的连接数
- `netstat_tcp_time_wait`: 处于 TIME_WAIT 状态的连接数（如果过高可能预示端口耗尽）
- `netstat_tcp_close`: 处于 CLOSE 状态的连接数
- `netstat_tcp_close_wait`: 处于 CLOSE_WAIT 状态的连接数（如果过高可能预示应用程序卡死或未正确释放连接）
- `netstat_tcp_last_ack`: 处于 LAST_ACK 状态的连接数
- `netstat_tcp_listen`: 处于 LISTEN 状态的连接数
- `netstat_tcp_closing`: 处于 CLOSING 状态的连接数
- `netstat_tcp_none`: 无法获取状态的 TCP 连接数
- `netstat_udp_socket`: 活跃的 UDP Socket 数量

## 监控大盘

这些指标是主机最核心的基础监控数据之一。通常，OS 的网络连接监控大盘会与 CPU、磁盘等指标统一放置在 **System (主机系统)** 大盘下面。
为方便单独查看，本目录也提供了一个仅包含 TCP/UDP 连接状态维度的基础 Dashboard。
