# redis

redis 的监控原理，就是连上 redis，执行 info 命令，解析结果，整理成监控数据上报。

## Configuration

redis 插件的配置在 `conf/input.redis/redis.toml` 最简单的配置如下：

```toml
[[instances]]
address = "127.0.0.1:6379"
username = ""
password = ""
labels = { instance="n9e-10.23.25.2:6379" }
```

如果要监控多个 redis 实例，就增加 instances 即可：

```toml
[[instances]]
address = "10.23.25.2:6379"
username = ""
password = ""
labels = { instance="n9e-10.23.25.2:6379" }

[[instances]]
address = "10.23.25.3:6379"
username = ""
password = ""
labels = { instance="n9e-10.23.25.3:6379" }
```

建议通过 labels 配置附加一个 instance 标签，便于后面复用监控大盘。

## 监控大盘和告警规则

该 README 的同级目录下，提供了 dashboard.json 就是监控大盘的配置，alerts.json 是告警规则，可以导入夜莺使用。

## 监控指标 (Metrics)

以下指标由 `inputs/redis` 插件生成。所有指标默认带有 `redis_` 前缀。

### 基础指标 (General Metrics)

| 指标名称 (Metric Name) | 类型 (Type) | 含义 (Description) | Labels |
| :--- | :--- | :--- | :--- |
| `scrape_use_seconds` | Gauge | 从该 Redis 实例采集数据的耗时 (秒) | `address` (or `cluster_name`, `source_node`) |
| `ping_use_seconds` | Gauge | 执行 PING 命令的耗时 (秒) | 同上 |
| `up` | Gauge | 实例存活状态。`1`: 正常 (Ping 成功), `0`: 异常 | 同上 |
| `instance_role` | Gauge | 实例角色。`1`: Master, `2`: Slave, `3`: Sentinel, `4`: Other | 同上, `replica_role` |

### INFO 命令指标 (INFO Command Metrics)

插件会执行 `INFO ALL` (或 `INFO`) 命令，并将结果解析为指标。

#### Server Section
| 指标名称 | 含义 |
| :--- | :--- |
| `uptime_in_seconds` | 运行时间 (秒) |

#### Memory Section
*收集该部分的**所有**数值型字段，常见包括：*
| 指标名称 | 含义 |
| :--- | :--- |
| `used_memory` | 已分配内存总量 (Bytes) |
| `used_memory_rss` | 操作系统角度的驻留集大小 (RSS) |
| `used_memory_lua` | Lua 引擎使用的内存 |
| `maxmemory` | 配置的最大内存限制 |
| `mem_fragmentation_ratio` | 内存碎片率 |

#### Stats Section
*收集该部分的**所有**数值型字段，常见包括：*
| 指标名称 | 含义 |
| :--- | :--- |
| `total_connections_received` | 累计接受的连接总数 |
| `total_commands_processed` | 累计处理的命令总数 |
| `instantaneous_ops_per_sec` | 当前 QPS |
| `keyspace_hits` | 键空间查找命中次数 |
| `keyspace_misses` | 键空间查找未命中次数 |
| `keyspace_hitrate` | **[计算指标]** 键空间命中率 (`hits / (hits + misses)`) |
| `rejected_connections` | 因达到最大连接数限制而拒绝的连接数 |
| `expired_keys` | 已过期的 Key 数量 |
| `evicted_keys` | 因内存限制被驱逐的 Key 数量 |

#### Persistence Section
*收集该部分的**所有**数值型字段，常见包括：*
| 指标名称 | 含义 |
| :--- | :--- |
| `rdb_last_save_time` | 最后一次 RDB 成功保存的时间戳 |
| `rdb_last_save_time_elapsed` | **[计算指标]** 距离最后一次 RDB 保存经过的秒数 |
| `rdb_changes_since_last_save` | 上次 RDB 保存以来改变的 Key 数量 |
| `aof_enabled` | AOF 是否开启 (0: 否, 1: 是) |

#### Clients Section
*收集该部分的**所有**数值型字段，常见包括：*
| 指标名称 | 含义 |
| :--- | :--- |
| `connected_clients` | 当前连接的客户端数量 |
| `blocked_clients` | 正在等待阻塞命令 (BLPOP 等) 的客户端数量 |

#### Replication Section
*收集该部分的**所有**数值型字段，常见包括：*
| 指标名称 | 含义 | Labels |
| :--- | :--- | :--- |
| `connected_slaves` | 连接的从节点数量 | - |
| `master_repl_offset` | 全局复制偏移量 | - |
| `replication_lag` | 复制延迟 | - |
| `replication_<key>` | 从节点详情 (如 `offset`, `lag`) | `replica_id`, `replica_ip`, `replica_port` |

#### CPU Section
*收集该部分的**所有**数值型字段，常见包括：*
| 指标名称 | 含义 |
| :--- | :--- |
| `used_cpu_sys` | Redis 服务进程消耗的系统 CPU |
| `used_cpu_user` | Redis 服务进程消耗的用户 CPU |

#### Cluster Section
| 指标名称 | 含义 |
| :--- | :--- |
| `cluster_enabled` | 集群模式是否开启 (0/1) |

### Keyspace 指标

解析 `INFO keyspace` 部分，格式如 `db0:keys=...,expires=...,avg_ttl=...`

| 指标名称 | 含义 | Labels |
| :--- | :--- | :--- |
| `keyspace_keys` | 数据库中的 Key 总数 | `db` (e.g., "db0") |
| `keyspace_expires` | 带有过期时间的 Key 数量 | `db` |
| `keyspace_avg_ttl` | Key 的平均生存时间 (毫秒) | `db` |

### Command Stats 指标

解析 `INFO commandstats` 部分，格式如 `cmdstat_get:calls=...,usec=...,...`

| 指标名称 | 含义 | Labels |
| :--- | :--- | :--- |
| `cmdstat_calls` | 命令调用次数 | `command` (e.g., "get", "set") |
| `cmdstat_usec` | 命令执行总耗时 (微秒) | `command` |
| `cmdstat_usec_per_call` | 命令平均耗时 (微秒) | `command` |
| `cmdstat_rejected_calls` | 拒绝执行次数 | `command` |
| `cmdstat_failed_calls` | 执行失败次数 | `command` |

### 慢查询日志 (Slow Log)

仅当配置 `gather_slowlog = true` 时采集。

| 指标名称 | 含义 | Labels |
| :--- | :--- | :--- |
| `slow_log` | 慢查询执行耗时 (微秒) | `client_addr`, `client_name`, `log_id`, `cmd` |

### 自定义命令 (Custom Commands)

根据配置文件中 `commands` 列表执行的命令结果。

| 指标名称 | 含义 |
| :--- | :--- |
| `exec_result_<metric>` | 自定义命令的返回值 (需可转换为 float) |
