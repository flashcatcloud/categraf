# NATS 采集插件

该插件用于采集 NATS 消息服务器的运行指标。它通过访问 NATS Server 提供的监控 HTTP API（`/varz` 接口）来获取实时的统计数据。

## 配置说明

要使此插件正常工作，您的 NATS 服务器必须开启 HTTP 监控端口（在 NATS 配置文件中设置 `http_port` 或 `https_port`）。

```toml
# 采集 NATS 监控指标
# interval = 60

[[instances]]
# NATS 监控接口地址 (需包含 schema 和端口)
server = "http://localhost:8222"
```

## 采集指标

所有收集到的指标都会打上 `server` 标签，对应所抓取的接口地址。
主要包含以下指标：

- `nats_in_msgs` / `nats_out_msgs`: 收发消息总数
- `nats_in_bytes` / `nats_out_bytes`: 收发字节总数
- `nats_uptime`: NATS 服务运行时间
- `nats_cores`: 分配给 NATS 的 CPU 核心数
- `nats_mem`: NATS 占用的内存大小
- `nats_connections`: 当前连接的客户端数量
- `nats_total_connections`: 历史建立过的连接总数
- `nats_subscriptions`: 当前活跃的订阅数量
- `nats_slow_consumers`: 消费较慢的消费者数量
- `nats_routes`: 集群路由数
- `nats_remotes`: 远程连接数

## 监控大盘

本目录下提供了一个配套的 Dashboard (`dashboard.json`)，您可以在 Grafana 或夜莺中导入该看板来观测您的 NATS 服务器运行状态（包括连接数、吞吐率、订阅数量等核心指标）。
