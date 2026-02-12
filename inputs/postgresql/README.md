# postgresql

postgresql 监控采集插件

## authorization

```shell
create user categraf with password 'categraf';
 
alter user categraf set default_transaction_read_only=on;

grant usage on schema public to categraf;

grant select on all tables in schema public to categraf ;
```

## configuration

```toml
[[instances]]
address = ""
# labels = { region="", zone="" }
## postgresql 的连接信息
# address = "host=1.2.3.4 port=5432 user=postgres password=123456 sslmode=disable"

## outputaddress 相当于是addres的别名 
# outputaddress = "db01" 

## 一条连接保持活跃的最大时长， 0s表示永远
## 当查询执行时，到达最大时长的连接不会被立即强制断开
# max_lifetime = "0s"

## 忽略哪些db的采集
# ignored_databases = ["postgres", "template0", "template1"]

## 显式指定采集哪些db 
# databases = ["app_production", "testing"]

## Whether to collect statement-level metrics.
## Requires extension pg_stat_statements enabled, see https://www.postgresql.org/docs/current/pgstatstatements.html
# enable_statement_metrics = false

## Max number of statements to collect
## applies only when enable_statement_metrics=true
## 0 means no limit
# statement_metrics_limit = 100

## Whether to use prepared statements when connecting to the database.
## This should be set to false when connecting through a PgBouncer instance
## with pool_mode set to transaction.
## 是否使用prepared statements 连接数据库
# prepared_statements = true
```
![dashboard](./postgresql.png)

## metrics

The following metrics are emitted when `enable_statement_metrics = true` (requires `pg_stat_statements`). All are cumulative counters (may reset on PostgreSQL restart or when stats are reset).

| Metric | Labels | Type |
| --- | --- | --- |
| `postgresql_statements_calls_total` | `server`, `db`, `user`, `datname`, `query` | counter |
| `postgresql_statements_exec_milliseconds_total` | `server`, `db`, `user`, `datname`, `query`  | counter (milliseconds) |
| `postgresql_statements_rows_total` | `server`, `db`, `user`, `datname`, `query`  | counter (rows) |
| `postgresql_statements_block_read_milliseconds_total` | `server`, `db`, `user`, `datname`, `query` | counter (milliseconds) |
| `postgresql_statements_block_write_milliseconds_total` | `server`, `db`, `user`, `datname`, `query`  | counter (milliseconds) |

Notes:
- `query` is normalized by replacing newlines/tabs with spaces.
- For PostgreSQL ≥ 13, the exporter adapts to renamed columns; metric names above remain unchanged.
- For PostgreSQL ≥ 17, `pg_stat_bgwriter` was split into `pg_stat_bgwriter` and `pg_stat_checkpointer`. The plugin automatically queries the new view and maps the columns to the old metric names to preserve backward compatibility.
    - `pg_stat_checkpointer.num_timed` -> `checkpoints_timed`
    - `pg_stat_checkpointer.num_requested` -> `checkpoints_req`
    - `pg_stat_checkpointer.write_time` -> `checkpoint_write_time`
    - `pg_stat_checkpointer.sync_time` -> `checkpoint_sync_time`
    - `pg_stat_checkpointer.buffers_written` -> `buffers_checkpoint`
