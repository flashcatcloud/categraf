# Netstat Filter 采集插件

该插件采集网络连接情况，并允许根据用户配置的条件（例如特定源/目标 IP、端口）进行过滤统计。这使得用户可以精确监控所关心的关键网络连接（如数据库连接池状态）。

除了常规的网络状态，此插件还能采集网络 Socket 的 `recv-Q`（接收队列）和 `send-Q`（发送队列）。这对于反映网络连接的质量非常有用（例如 RTT 时间过长、客户端处理慢，会导致 `send-Q` 持续大于 0）。

**支持平台:** Windows, Linux (其中 `recv-Q` 和 `send-Q` 当前仅完整支持 Linux，Windows 下默认为 0)

## 配置说明

支持对源 IP (`laddr_ip`)、源端口 (`laddr_port`)、目标 IP (`raddr_ip`) 和目标端口 (`raddr_port`) 进行过滤。如果不填写，则匹配所有。

```toml
# 采集指定过滤条件的 TCP 连接状态统计
# interval = 15

# 如果您有多种规则需要独立统计，可以配置多个 instance
[[instances]]
# filter 标签用于区分这是哪一个规则采集的数据
# labels = { "filter"="mysql_backend" }

# 过滤规则示例：只采集连接到本地或远端 3306 端口的连接
# laddr_ip = ""
# laddr_port = 0
# raddr_ip = ""
# raddr_port = 3306

[[instances]]
# labels = { "filter"="redis_backend" }
# raddr_port = 6379
```

当过滤结果匹配多条连接时，插件会将这些连接的各项统计（如处于某种状态的连接数，以及 `send_queue` 和 `recv_queue`）进行**加和**。

## 采集指标

所有指标前缀为 `netstat_filter_`。指标列表如下：

- `netstat_filter_tcp_established`: 满足过滤条件的 ESTABLISHED 连接数
- `netstat_filter_tcp_syn_sent`: 满足条件的 SYN_SENT 连接数
- `netstat_filter_tcp_syn_recv`: 满足条件的 SYN_RECV 连接数
- `netstat_filter_tcp_time_wait`: 满足条件的 TIME_WAIT 连接数
- `netstat_filter_tcp_close_wait`: 满足条件的 CLOSE_WAIT 连接数
- (其他 TCP 状态如 `fin_wait1`, `fin_wait2`, `last_ack`, `listen`, `closing`, `none` 等类似)
- `netstat_filter_tcp_send_queue`: 匹配连接的发送队列排队字节数总和 (Linux only)
- `netstat_filter_tcp_recv_queue`: 匹配连接的接收队列排队字节数总和 (Linux only)

## 监控大盘

本目录提供了一个基础的 Dashboard (`dashboard.json`)，支持通过不同的过滤规则（`filter` label）展示目标服务连接池及队列的积压情况，对应用层网络调优很有帮助。
