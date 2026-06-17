# ClickHouse 采集插件

该插件用于从 [ClickHouse](https://github.com/ClickHouse/ClickHouse) 数据库服务器收集统计数据指标。

## 配置说明

```toml
# # collect interval
# interval = 15

# 从一个或多个 ClickHouse 服务器读取指标
[[instances]]
  ## 用于在 ClickHouse 服务器上进行授权的用户名
  username = "default"

  ## 用于在 ClickHouse 服务器上进行授权的密码
  # password = ""

  ## 获取指标时的 HTTP(s) 超时时间
  ## 包含连接时间、重定向时间以及读取响应体的时间。
  # timeout = 5

  ## 要抓取指标的服务器列表
  ## 通过 HTTP(s) ClickHouse 接口抓取指标
  servers = ["http://127.0.0.1:8123"]

  ## 如果将 auto_discovery 设置为 true，插件会尝试连接到集群中可用的所有服务器
  ## (使用上面配置的 username 和 password)，并通过 system.clusters 系统表获取服务器列表。
  # auto_discovery = true

  ## 当 auto_discovery 为 true 时，使用 cluster_include 过滤要包含的集群名称
  ## (相当于 SQL 里的 WHERE cluster IN (...))
  # cluster_include = []

  ## 当 auto_discovery 为 true 时，使用 cluster_exclude 排除指定的集群名称
  ## (相当于 SQL 里的 WHERE cluster NOT IN (...))
  # cluster_exclude = []

  ## 可选的 TLS 配置
  # tls_ca = "/etc/telegraf/ca.pem"
  # tls_cert = "/etc/telegraf/cert.pem"
  # tls_key = "/etc/telegraf/key.pem"
  ## 忽略自签证书的安全校验
  # insecure_skip_verify = false
```

## 采集指标

所有指标主要来自 ClickHouse 的系统表 (如 `system.metrics`, `system.events` 等)，指标分类如下：

- `clickhouse_events`: 来源于 `system.events`
- `clickhouse_metrics`: 来源于 `system.metrics`
- `clickhouse_asynchronous_metrics`: 来源于 `system.asynchronous_metrics`
- `clickhouse_tables`: 包含数据库、表名、行数、数据大小 (`bytes`, `parts`, `rows`)
- `clickhouse_zookeeper`: ZooKeeper 状态指标 (如 `root_nodes`)
- `clickhouse_replication_queue`: 复制队列指标 (如 `too_many_tries_replicas`)
- `clickhouse_detached_parts`: 隔离的分区指标 (`detached_parts`)
- `clickhouse_dictionaries`: 字典信息指标 (`is_loaded`, `bytes_allocated`)
- `clickhouse_mutations`: 数据变更(Mutations)任务信息 (`running`, `failed`, `completed`)
- `clickhouse_disks`: 磁盘容量相关 (`free_space_percent`, `keep_free_space_percent`)
- `clickhouse_processes`: 查询进程耗时百分位数 (`percentile_50`, `percentile_90`, `longest_running`)
- `clickhouse_text_log`: 日志统计 (`messages_last_10_min`)
