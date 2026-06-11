# MySQL 插件

## 简介

`mysql` 插件通过连接 MySQL 实例并执行内置 SQL，采集 MySQL 运行状态、全局变量、InnoDB 关键指标、连接分布、库表空间占用、复制状态、Binlog 体积，以及用户自定义 SQL 结果。

它适用于以下场景：

- 监控单实例或多实例 MySQL / Percona Server / MariaDB 的基础健康状态
- 观测连接数、慢查询、缓存、锁等待、InnoDB Buffer Pool 等数据库核心指标
- 采集库级、表级磁盘占用，以及主从 / 副本延迟、Binlog 体积
- 将业务自定义 SQL 结果与内置指标统一纳入 Categraf

## 限制与兼容性

- 插件的前提是“能连上数据库并成功 `Ping`”；`mysql_up = 1` 只表示连接与认证成功，不代表所有可选采集项都成功
- 不同指标族依赖不同 SQL 能力和数据库权限；权限不足时，通常表现为部分指标缺失并伴随日志报错，而不是整实例 `up = 0`
- 只有当 `address` 以 `.sock` 结尾时，插件才会使用 Unix socket；`localhost` 不会自动切换为 socket 连接
- 主从 / 副本相关采集同时兼容 `SHOW SLAVE STATUS`、`SHOW ALL SLAVES STATUS`、`SHOW REPLICA STATUS`、`SHOW ALL REPLICAS STATUS`，但最终能输出哪些字段，取决于数据库版本与返回列
- `gather_replica_status` 虽然走的是 `SHOW REPLICA STATUS` 路径，但当前输出的指标名前缀仍然是 `mysql_slave_...`，这是现有实现的历史兼容行为
- 代码中存在两套 Binlog 采集路径：一套默认开启的 `mysql_binlog_*`，一套可选开启的 `mysql_binary_*`；它们不会互相替代

## 权限建议

如果希望采集大多数内置指标，建议为监控账号授予至少如下权限：

```sql
GRANT PROCESS, REPLICATION CLIENT ON *.* TO 'categraf'@'%';
GRANT SELECT ON *.* TO 'categraf'@'%';
```

不同模块的典型权限依赖如下：

| 模块 | 主要 SQL | 常见权限要求 | 说明 |
| --- | --- | --- | --- |
| 基础存活探测 | `Ping()` | 可登录即可 | 对应 `mysql_up` |
| 全局状态 / 全局变量 | `SHOW GLOBAL STATUS` / `SHOW GLOBAL VARIABLES` | 依 MySQL 版本而异 | 若权限不足，会导致核心状态类指标缺失 |
| InnoDB 状态 | `SHOW ENGINE INNODB STATUS` | `PROCESS` | 无此权限时，InnoDB 状态解析类指标无法获取 |
| Processlist 分布 | `information_schema.processlist` | `PROCESS` | 否则通常只能看到当前账号自己的连接，统计会失真 |
| 库 / 表大小 | `information_schema.tables` | 对目标库有 `SELECT` | 没有权限的库 / 表不会出现在结果中 |
| 主从 / 副本状态 | `SHOW SLAVE STATUS` / `SHOW REPLICA STATUS` | `REPLICATION CLIENT` | 对应复制延迟、线程状态等指标 |
| Binlog 大小 | `SHOW BINARY LOGS` | `REPLICATION CLIENT` | 同时影响 `mysql_binlog_*` 与 `mysql_binary_*` |

## 快速开始

最小可用配置：

```toml
[[instances]]
address = "127.0.0.1:3306"
username = "categraf"
password = "<PASSWORD>"
timeout_seconds = 3

# 强烈建议补一个便于区分实例的标签
labels = { instance = "prod-mysql-01:3306" }
```

说明：

- `address` 以 `.sock` 结尾时走 Unix socket，例如 `/var/run/mysqld/mysqld.sock`
- `username` / `password` 是否允许为空，取决于 MySQL 账户本身的认证方式；插件代码并不强制要求非空
- `labels` 不是插件专属字段，但在多实例场景下非常建议设置 `instance` 之类的稳定标签

### TLS 连接示例

如果需要由插件注册自定义 TLS 配置，请同时满足两件事：

1. 设置 `use_tls = true`
2. 在 `parameters` 中显式传入 `tls=custom`

示例：

```toml
[[instances]]
address = "mysql.example.com:3306"
username = "categraf"
password = "<PASSWORD>"
use_tls = true
parameters = "tls=custom"
tls_ca = "/etc/categraf/ca.pem"
tls_cert = "/etc/categraf/client.pem"
tls_key = "/etc/categraf/client-key.pem"
```

如果只设置 `use_tls = true`，但没有在 `parameters` 中加上 `tls=custom`，当前实现不会使用这套自定义 TLS 配置。

### 监控多个实例

`[[instances]]` 是数组，可以配置多个 MySQL 实例：

```toml
[[instances]]
address = "10.2.3.6:3306"
username = "categraf"
password = "<PASSWORD>"
labels = { instance = "prod-mysql-a:3306" }

[[instances]]
address = "10.2.6.9:3306"
username = "categraf"
password = "<PASSWORD>"
labels = { instance = "prod-mysql-b:3306" }

[[instances]]
address = "/var/run/mysqld/mysqld.sock"
username = "categraf"
password = "<PASSWORD>"
labels = { instance = "local-mysql.sock" }
```

## 配置项

本文重点描述 MySQL 插件相关字段。`interval`、`interval_times`、`labels` 等实例通用字段沿用 Categraf 通用语义。

### 常用通用实例字段

| 配置项 | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `interval` | int | 继承全局配置 | 采集周期，单位秒 |
| `interval_times` | int | `1` | 实际采集周期 = 全局 `interval * interval_times` |
| `labels` | map[string]string | 空 | 给当前实例的所有指标追加固定标签；多实例场景强烈建议补一个稳定的 `instance` 标签 |

### 连接与基础采集

| 配置项 | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `address` | string | 必填 | MySQL 地址。以 `.sock` 结尾时使用 Unix socket，否则使用 TCP |
| `username` | string | 空 | MySQL 用户名 |
| `password` | string | 空 | MySQL 密码 |
| `parameters` | string | 空 | 直接拼到 DSN `?` 后面的参数串，例如 `parseTime=true&loc=Local`；使用自定义 TLS 时需要写 `tls=custom` |
| `timeout_seconds` | int | `3` | 连接 / Ping 超时时间。若 DSN 自身已携带 timeout，则以 DSN 为准 |

### 内置采集开关

| 配置项 | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `extra_status_metrics` | bool | false | 扩展 `SHOW GLOBAL STATUS` 指标白名单 |
| `extra_innodb_metrics` | bool | false | 扩展 InnoDB 相关指标白名单；同时额外输出 `mysql_global_status_buffer_pool_pages_used` |
| `gather_processlist_processes_by_state` | bool | false | 采集 `information_schema.processlist` 中按状态归类的连接数 |
| `gather_processlist_processes_by_user` | bool | false | 采集 `information_schema.processlist` 中按用户归类的连接数 |
| `gather_schema_size` | bool | false | 采集库级磁盘占用 |
| `gather_table_size` | bool | false | 采集业务库表级磁盘占用 |
| `gather_system_table_size` | bool | false | 采集系统库（`mysql`、`sys`、`information_schema`、`performance_schema`）表级磁盘占用 |
| `gather_slave_status` | bool | false | 采集 `SHOW SLAVE STATUS` / `SHOW ALL SLAVES STATUS` 的一组精简指标 |
| `gather_binary_logs` | bool | false | 额外采集一组 `mysql_binary_*` Binlog 指标 |
| `gather_replica_status` | bool | false | 采集 `SHOW REPLICA STATUS` / `SHOW ALL REPLICAS STATUS` 的可解析字段 |
| `gather_all_slave_channels` | bool | false | 仅影响 `gather_replica_status`：为 `true` 时导出所有 channel；否则只取第一行 |

### 禁用开关

| 配置项 | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `disable_global_status` | bool | false | 禁用 `SHOW GLOBAL STATUS` 采集 |
| `disable_global_variables` | bool | false | 禁用 `SHOW GLOBAL VARIABLES` 采集 |
| `disable_innodb_status` | bool | false | 禁用 `SHOW ENGINE INNODB STATUS` 文本解析 |
| `disable_extra_innodb_status` | bool | false | 禁用基于缓存计算的 Buffer Pool 衍生指标 |
| `disable_binlogs` | bool | false | 禁用默认开启的旧版 Binlog 采集，关闭后不再输出 `mysql_binlog_*` |

### TLS 配置

| 配置项 | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `use_tls` | bool | false | 是否启用 TLS 配置注册 |
| `tls_ca` | string | 空 | CA 文件路径 |
| `tls_cert` | string | 空 | 客户端证书路径 |
| `tls_key` | string | 空 | 客户端私钥路径 |
| `tls_key_pwd` | string | 空 | 私钥口令 |
| `insecure_skip_verify` | bool | false | 是否跳过服务端证书校验 |
| `tls_server_name` | string | 空 | 自定义 TLS `ServerName` |
| `tls_min_version` | string | 空 | 最低 TLS 版本，可选 `1.0` / `1.1` / `1.2` / `1.3` |
| `tls_max_version` | string | 空 | 最高 TLS 版本，可选 `1.0` / `1.1` / `1.2` / `1.3` |
| `tls_cipher_suites` | []string | 空 | 显式指定 Cipher Suites |

### 自定义 SQL

支持两种作用域：

- 顶层 `[[queries]]`：对当前插件内的所有实例生效
- 实例级 `[[instances.queries]]`：只对当前实例生效

每个 query 支持如下字段：

| 配置项 | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `mesurement` | string | 空 | 自定义指标名前缀。注意当前实现要求使用这个历史拼写 |
| `metric_fields` | []string | 空 | 作为数值指标导出的列名列表 |
| `label_fields` | []string | 空 | 作为标签导出的列名列表 |
| `field_to_append` | string | 空 | 将某一列的值追加到指标名中，适合动态分组 |
| `timeout` | duration | 继承 `timeout_seconds`，再退化到 `3s` | 单条自定义 SQL 的超时 |
| `request` | string | 空 | 实际执行的 SQL |

自定义 SQL 的使用规则：

- `metric_fields`、`label_fields`、`field_to_append` 应与结果集里的小写列名或小写别名一致，因为实现会先把数据库返回列名统一转成小写后再匹配
- `metric_fields` 对应列必须能转换为数值，否则该行会报错并跳过
- 如果设置了 `field_to_append`，该列的值会被清洗后拼入指标名：空格转下划线，`%` 变为 `percent`，并统一转成小写

示例：

```toml
[[instances.queries]]
mesurement = "users"
metric_fields = ["total"]
label_fields = ["service"]
timeout = "3s"
request = '''
SELECT 'billing' AS service, COUNT(*) AS total FROM users;
'''
```

上述查询会生成 `mysql_users_total{service="billing", ...}`。

## 快速验证

MySQL 插件没有内置调试 HTTP API，最直接的验证方式是“启动插件后查基础指标，再对照日志看是否有模块级错误”。

### 1. 使用最小配置启动采集

至少保证以下配置存在：

```toml
[[instances]]
address = "127.0.0.1:3306"
username = "categraf"
password = "<PASSWORD>"
labels = { instance = "prod-mysql-01:3306" }
```

### 2. 在指标存储中检查基础指标

启动 Categraf 后，先查询以下指标：

```promql
mysql_up{address="127.0.0.1:3306"}
mysql_scrape_use_seconds{address="127.0.0.1:3306"}
mysql_global_status_threads_connected{address="127.0.0.1:3306"}
mysql_version_info{address="127.0.0.1:3306"}
```

预期现象：

- `mysql_up = 1`
- `mysql_scrape_use_seconds` 有值
- `mysql_global_status_threads_connected` 有值
- `mysql_version_info` 出现，并带有 `version`、`innodb_version`、`version_comment` 标签

### 3. 如果启用了可选模块，再检查对应指标

- 启用 `gather_schema_size` 后，检查 `mysql_schema_size_bytes`
- 启用 `gather_table_size` 后，检查 `mysql_table_size_data_bytes`
- 启用 `gather_slave_status` 后，检查 `mysql_slave_status_seconds_behind_master`
- 保持默认 `disable_binlogs = false` 时，检查 `mysql_binlog_size_bytes`

### 4. 对照 Categraf 日志

如果 `mysql_up = 1`，但某些指标不存在，再看日志中是否出现类似报错：

- `failed to query global status`
- `failed to query engine innodb status`
- `failed to query slave status`
- `failed to get table size`

这通常意味着：

- 当前账号缺权限
- 当前实例不是对应角色，例如并不是副本，却开启了复制状态采集
- 当前数据库版本不支持某条 SQL

## Metrics

所有指标都以 `mysql_` 为前缀。

### 1. 基础存活与采集耗时

| 指标 | 类型 | 说明 |
| --- | --- | --- |
| `mysql_up` | Gauge | 数据库连接与 `Ping` 是否成功。`1` 表示成功，`0` 表示失败 |
| `mysql_scrape_use_seconds` | Gauge | 单次采集耗时 |

### 2. `SHOW GLOBAL STATUS` 核心指标

默认会导出以下直接指标族：

```text
mysql_global_status_uptime
mysql_global_status_prepared_stmt_count
mysql_global_status_slow_queries
mysql_global_status_questions
mysql_global_status_queries
mysql_global_status_connections
mysql_global_status_max_used_connections
mysql_global_status_aborted_clients
mysql_global_status_aborted_connects
mysql_global_status_open_files
mysql_global_status_open_tables
mysql_global_status_bytes_sent
mysql_global_status_bytes_received
mysql_global_status_qcache_hits
mysql_global_status_qcache_inserts
mysql_global_status_qcache_lowmem_prunes
mysql_global_status_table_locks_waited
mysql_global_status_table_locks_waited_rate
mysql_global_status_created_tmp_tables
mysql_global_status_created_tmp_disk_tables
mysql_global_status_created_tmp_files
mysql_global_status_threads_connected
mysql_global_status_threads_running
mysql_global_status_key_blocks_used
mysql_global_status_key_blocks_unused
mysql_global_status_key_blocks_not_flushed
mysql_global_status_key_read_requests
mysql_global_status_key_reads
mysql_global_status_key_write_requests
mysql_global_status_key_writes
mysql_global_status_innodb_log_waits
mysql_global_status_innodb_data_reads
mysql_global_status_innodb_data_writes
mysql_global_status_innodb_os_log_fsyncs
mysql_global_status_innodb_mutex_spin_waits
mysql_global_status_innodb_mutex_spin_rounds
mysql_global_status_innodb_mutex_os_waits
mysql_global_status_innodb_row_lock_waits
mysql_global_status_innodb_row_lock_time
mysql_global_status_innodb_row_lock_current_waits
mysql_global_status_innodb_current_row_locks
mysql_global_status_innodb_buffer_pool_read_requests
mysql_global_status_innodb_buffer_pool_reads
```

另外还会导出以下按标签分组的指标族：

| 指标 | 标签 | 说明 |
| --- | --- | --- |
| `mysql_global_status_commands_total` | `command` | `com_*` 类命令计数，例如 `select`、`insert`、`update`、`delete` |
| `mysql_global_status_handlers_total` | `handler` | `handler_*` 计数 |
| `mysql_global_status_connection_errors_total` | `error` | `connection_errors_*` 计数 |
| `mysql_global_status_buffer_pool_pages_data` / `free` / `misc` / `old` / `total` / `dirty` | 无 | Buffer Pool 页数分布 |
| `mysql_global_status_buffer_pool_page_changes_total` | `operation` | Buffer Pool 页状态变化计数 |
| `mysql_global_status_innodb_row_ops_total` | `operation` | `innodb_rows_*` 行操作计数 |
| `mysql_global_status_performance_schema_lost_total` | `instrumentation` | `performance_schema_*` 丢失计数 |

当 `extra_status_metrics = true` 时，还会额外输出以下后缀的 `mysql_global_status_<suffix>` 指标：

```text
binlog_cache_disk_use
binlog_cache_use
opened_tables
qcache_total_blocks
qcache_free_blocks
qcache_free_memory
qcache_not_cached
qcache_queries_in_cache
select_full_join
select_full_range_join
select_range
select_range_check
select_scan
sort_merge_passes
sort_range
sort_rows
sort_scan
table_locks_immediate
table_locks_immediate_rate
threads_cached
threads_created
table_open_cache_hits
table_open_cache_misses
```

### 3. `SHOW GLOBAL VARIABLES` 与信息类指标

默认会导出以下 `mysql_global_variables_<suffix>` 指标：

```text
key_buffer_size
key_cache_block_size
max_connections
max_prepared_stmt_count
query_cache_size
table_open_cache
thread_cache_size
long_query_time
max_user_connections
read_only
```

另外还会导出：

| 指标 | 类型 | 说明 |
| --- | --- | --- |
| `mysql_version_info` | Info 型（值恒为 `1`） | 版本信息，标签包含 `version`、`innodb_version`、`version_comment` |
| `mysql_transaction_isolation` | Info 型（值恒为 `1`） | 当前事务隔离级别，标签 `level` |
| `mysql_galera_variables_info` | Info 型（值恒为 `1`） | Galera / PXC 集群名称，标签 `wsrep_cluster_name` |
| `mysql_galera_gcache_size_bytes` | Gauge | 从 `wsrep_provider_options` 解析出的 `gcache.size` |

### 4. InnoDB 状态与衍生指标

`SHOW ENGINE INNODB STATUS` 文本解析会导出：

| 指标 | 类型 | 说明 |
| --- | --- | --- |
| `mysql_engine_innodb_queries_inside_innodb` | Gauge | InnoDB 内部正在执行的查询数 |
| `mysql_engine_innodb_queries_in_queue` | Gauge | InnoDB 队列中的查询数 |
| `mysql_engine_innodb_read_views_open_inside_innodb` | Gauge | 当前打开的 read view 数 |

基于缓存计算的 Buffer Pool 衍生指标（默认开启，除非 `disable_extra_innodb_status = true`）：

| 指标 | 类型 | 说明 |
| --- | --- | --- |
| `mysql_global_status_buffer_pool_bytes_used` | Gauge | Buffer Pool 已使用字节数 |
| `mysql_global_status_buffer_pool_bytes_data` | Gauge | Buffer Pool 数据页字节数 |
| `mysql_global_status_buffer_pool_bytes_free` | Gauge | Buffer Pool 空闲字节数 |
| `mysql_global_status_buffer_pool_bytes_total` | Gauge | Buffer Pool 总字节数 |
| `mysql_global_status_buffer_pool_bytes_dirty` | Gauge | Buffer Pool 脏页字节数 |
| `mysql_global_status_buffer_pool_pages_utilization` | Gauge | Buffer Pool 页利用率，百分比 |

当 `extra_innodb_metrics = true` 时，还会额外输出：

- `mysql_global_status_buffer_pool_pages_used`
- 以及以下更多 `mysql_global_status_<suffix>` 指标：

```text
innodb_active_transactions
innodb_buffer_pool_bytes_data
innodb_buffer_pool_pages_data
innodb_buffer_pool_pages_dirty
innodb_buffer_pool_pages_flushed
innodb_buffer_pool_pages_free
innodb_buffer_pool_pages_total
innodb_buffer_pool_read_ahead
innodb_buffer_pool_read_ahead_evicted
innodb_buffer_pool_read_ahead_rnd
innodb_buffer_pool_wait_free
innodb_buffer_pool_write_requests
innodb_checkpoint_age
innodb_current_transactions
innodb_data_fsyncs
innodb_data_pending_fsyncs
innodb_data_pending_reads
innodb_data_pending_writes
innodb_data_read
innodb_data_written
innodb_dblwr_pages_written
innodb_dblwr_writes
innodb_hash_index_cells_total
innodb_hash_index_cells_used
innodb_history_list_length
innodb_ibuf_free_list
innodb_ibuf_merged
innodb_ibuf_merged_delete_marks
innodb_ibuf_merged_deletes
innodb_ibuf_merged_inserts
innodb_ibuf_merges
innodb_ibuf_segment_size
innodb_ibuf_size
innodb_lock_structs
innodb_locked_tables
innodb_locked_transactions
innodb_log_write_requests
innodb_log_writes
innodb_lsn_current
innodb_lsn_flushed
innodb_lsn_last_checkpoint
innodb_mem_adaptive_hash
innodb_mem_additional_pool
innodb_mem_dictionary
innodb_mem_file_system
innodb_mem_lock_system
innodb_mem_page_hash
innodb_mem_recovery_system
innodb_mem_thread_hash
innodb_mem_total
innodb_os_file_fsyncs
innodb_os_file_reads
innodb_os_file_writes
innodb_os_log_pending_fsyncs
innodb_os_log_pending_writes
innodb_os_log_written
innodb_pages_created
innodb_pages_read
innodb_pages_written
innodb_pending_aio_log_ios
innodb_pending_aio_sync_ios
innodb_pending_buffer_pool_flushes
innodb_pending_checkpoint_writes
innodb_pending_ibuf_aio_reads
innodb_pending_log_flushes
innodb_pending_log_writes
innodb_pending_normal_aio_reads
innodb_pending_normal_aio_writes
innodb_queries_inside
innodb_queries_queued
innodb_read_views
innodb_rows_deleted
innodb_rows_inserted
innodb_rows_read
innodb_rows_updated
innodb_s_lock_os_waits
innodb_s_lock_spin_rounds
innodb_s_lock_spin_waits
innodb_semaphore_wait_time
innodb_semaphore_waits
innodb_tables_in_use
innodb_x_lock_os_waits
innodb_x_lock_spin_rounds
innodb_x_lock_spin_waits
```

### 5. Processlist 指标

| 指标 | 类型 | 标签 | 说明 |
| --- | --- | --- | --- |
| `mysql_processlist_processes_by_state` | Gauge | `state` | 连接按状态归类后的数量 |
| `mysql_processlist_processes_by_user` | Gauge | `user` | 连接按用户归类后的数量 |

### 6. 库与表空间指标

| 指标 | 类型 | 标签 | 说明 |
| --- | --- | --- | --- |
| `mysql_schema_size_bytes` | Gauge | `schema` | 库级总空间大小 |
| `mysql_table_size_index_bytes` | Gauge | `schema`, `table` | 表索引空间 |
| `mysql_table_size_data_bytes` | Gauge | `schema`, `table` | 表数据空间 |
| `mysql_table_size_free_data_bytes` | Gauge | `schema`, `table` | 表空闲空间 |

### 7. 复制状态指标

`gather_slave_status = true` 时，会从 `SHOW SLAVE STATUS` / `SHOW ALL SLAVES STATUS` 路径导出一组经过筛选的指标，格式为 `mysql_slave_status_<suffix>`。常见指标包括：

```text
mysql_slave_status_seconds_behind_source
mysql_slave_status_seconds_behind_master
mysql_slave_status_slave_io_running
mysql_slave_status_slave_sql_running
mysql_slave_status_master_server_id
mysql_slave_status_source_server_id
mysql_slave_status_sql_delay
mysql_slave_status_exec_master_log_pos
mysql_slave_status_read_master_log_pos
```

这些指标会附带以下标签：

- `master_host`
- `master_uuid`
- `channel_name`

`gather_replica_status = true` 时，会从 `SHOW REPLICA STATUS` / `SHOW ALL REPLICAS STATUS` 路径导出“可解析为数值或布尔值”的列，指标名格式为：

```text
mysql_slave_<lowercase_column_name>
```

例如在较新的 MySQL 版本上，常见会看到：

```text
mysql_slave_seconds_behind_source
mysql_slave_source_server_id
mysql_slave_sql_delay
```

注意：

- 这一组指标名虽然来自 `SHOW REPLICA STATUS`，但前缀仍是 `mysql_slave_`
- 当前实现更适合导出数值型列；字符串列不会形成最终时序，某些 `YES` / `NO` 状态列也可能因为列类型是字符串而被丢弃
- `gather_all_slave_channels = true` 时，会额外打上 `channel` 标签并导出所有 channel；否则只导出第一行

### 8. Binlog 指标

默认情况下（`disable_binlogs = false`），会导出一组旧版 Binlog 指标：

| 指标 | 类型 | 说明 |
| --- | --- | --- |
| `mysql_binlog_size_bytes` | Gauge | 所有 Binlog 文件大小总和 |
| `mysql_binlog_file_count` | Gauge | Binlog 文件数量 |
| `mysql_binlog_file_number` | Gauge | 最后一个 Binlog 文件名中的序号 |

当 `gather_binary_logs = true` 时，还会额外导出一组新版 Binlog 指标：

| 指标 | 类型 | 说明 |
| --- | --- | --- |
| `mysql_binary_size_bytes` | Gauge | 所有 Binlog 文件大小总和 |
| `mysql_binary_files_count` | Gauge | Binlog 文件数量 |

两套指标都依赖 `SHOW BINARY LOGS`。如果实例未开启 Binlog 或当前账号没有 `REPLICATION CLIENT` 权限，这些指标会缺失。

### 9. 自定义 SQL 指标

自定义 SQL 会根据配置动态生成指标名：

- 不带 `field_to_append`：`mysql_<mesurement>_<metric_field>`
- 带 `field_to_append`：`mysql_<mesurement>_<normalized_field_value>_<metric_field>`

示例：

- `mesurement = "users"`
- `metric_fields = ["total"]`

则输出 `mysql_users_total`

如果再设置 `field_to_append = "state"`，并且某行 `state = 'Lock Wait'`，则会生成类似：

`mysql_users_lock_wait_total`

### 10. Galera / PXC 相关指标

如果实例暴露了 `wsrep_*` 相关状态 / 变量，还会额外输出：

| 指标 | 类型 | 说明 |
| --- | --- | --- |
| `mysql_galera_status_info` | Info 型（值恒为 `1`） | `wsrep_local_state_uuid`、`wsrep_cluster_state_uuid`、`wsrep_provider_version` |
| `mysql_galera_variables_info` | Info 型（值恒为 `1`） | `wsrep_cluster_name` |
| `mysql_galera_gcache_size_bytes` | Gauge | Galera gcache 大小 |
| `mysql_galera_evs_repl_latency_min_seconds` | Gauge | 组通信延迟最小值 |
| `mysql_galera_evs_repl_latency_avg_seconds` | Gauge | 组通信延迟平均值 |
| `mysql_galera_evs_repl_latency_max_seconds` | Gauge | 组通信延迟最大值 |
| `mysql_galera_evs_repl_latency_stdev` | Gauge | 组通信延迟标准差 |
| `mysql_galera_evs_repl_latency_sample_size` | Gauge | 样本数 |

## FAQ

### 1. 为什么 `mysql_up = 1`，但有些指标还是没有？

`mysql_up` 只代表“连接与 `Ping` 成功”。库表大小、复制状态、Binlog、Processlist 等都依赖额外权限和 SQL 能力。先看 Categraf 日志里的 `failed to query ...` 报错，再确认权限、角色和数据库版本。

### 2. 为什么把 `address` 写成 `localhost`，却没有走 Unix socket？

当前实现只有在 `address` 以 `.sock` 结尾时才走 Unix socket。`localhost:3306` 仍然按 TCP 处理。若要使用 socket，请直接写 socket 文件路径。

### 3. 为什么 TLS 配好了证书，还是连不上？

除了 `use_tls = true` 外，还需要在 `parameters` 中显式加上 `tls=custom`。例如：

```toml
use_tls = true
parameters = "tls=custom"
```

### 4. `gather_slave_status` 和 `gather_replica_status` 应该开哪个？

- 如果你更关心一组稳定、精简的复制指标，优先用 `gather_slave_status`
- 如果你希望尽量暴露 `SHOW REPLICA STATUS` 返回的数值列，可启用 `gather_replica_status`
- 两者可以同时开启，但会得到两套不同命名风格的复制指标

### 5. 为什么开了 `gather_binary_logs`，却还会看到 `mysql_binlog_*`？

因为这是两套独立的 Binlog 采集逻辑：

- `disable_binlogs = false` 时，默认输出 `mysql_binlog_*`
- `gather_binary_logs = true` 时，额外输出 `mysql_binary_*`

开启后者不会自动关闭前者。

### 6. 自定义 SQL 为什么没有数据？

常见原因有三类：

- `metric_fields` / `label_fields` / `field_to_append` 没与 SQL 结果里的小写列名或小写别名保持一致
- `metric_fields` 对应列不是数值
- 自定义 SQL 超时；当前默认会继承实例的 `timeout_seconds`，默认值是 3 秒

## 其他说明

- 同目录下的 `alerts.json`、`dashboard-by-instance.json`、`dashboard-by-ident.json` 可作为告警与看板参考
- 如果你需要极简权限模式，可以只启用基础探测与自定义 SQL，但要接受大量内置指标不可用的事实

## 许可证

Apache License 2.0
