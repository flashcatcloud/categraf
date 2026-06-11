# Conntrack 采集插件

该插件用于监控 Linux 服务器上的 connection tracking (conntrack) 表的状态。该项目 fork 自 `telegraf/conntrack`。

运维人员经常会遇到 `nf_conntrack: table full, dropping packet` 的报错，这个插件可以帮助您实时监控 conntrack 表的使用情况。

## 采集指标

所有指标将附带在 `conntrack` 这个 measurement 下：

- `conntrack_ip_conntrack_count`: 当前 conntrack 表中的连接条目数 (count)。
- `conntrack_ip_conntrack_max`: 当前 conntrack 表的最大容量限制 (size)。

## 告警配置建议

您可以在夜莺或 Prometheus 中配置如下的告警规则，以便在 conntrack 表即将被填满时收到告警通知：

```promql
conntrack_ip_conntrack_count / conntrack_ip_conntrack_max > 0.8
```