# PHP-FPM 采集插件

该插件用于监控采集 PHP-FPM 的进程状态指标。它由 telegraf 的 phpfpm 插件改造而来，支持通过 HTTP URL 或 Unix Socket 连接到 PHP-FPM 的状态页获取数据。

## 前置要求

使用该插件前，必须修改 PHP-FPM 的配置文件（通常是 `www.conf`），开启 `pm.status_path` 配置项：

```ini
pm.status_path = /status
```
修改完成后，请重启 PHP-FPM 进程使配置生效。
如果您使用 Nginx 反向代理，请确保 Nginx 也配置了对应的 `/status` 路由转发到 PHP-FPM。

## 配置说明

支持采集多个 PHP-FPM 实例，详细配置见 `conf/input.phpfpm/phpfpm.toml`：

```toml
# 采集 PHP-FPM 状态
# interval = 15

[[instances]]
# URLs 支持 HTTP 协议 或 Unix Socket
# 例如:
# urls = ["http://localhost/status", "unix:///var/run/php5-fpm.sock"]
urls = ["http://127.0.0.1/status"]

# 注意事项：
# 1. 如下超时、认证等配置仅对 HTTP URL 生效：
# response_timeout = "5s"
# username = ""
# password = ""
# headers = ["X-Custom-Header: value"]
# TLS/SSL 配置同样仅对 HTTP 生效。

# 2. 如果使用 Unix socket，需要保证 Categraf 运行用户拥有读取该 socket 文件的权限。
```

## 采集指标

所有指标前缀为 `phpfpm_`，默认携带 `url` 和 `pool` 标签。主要指标包括：

- `phpfpm_accepted_conn`: 累计接收的请求总数
- `phpfpm_listen_queue`: 请求等待队列中当前的请求数
- `phpfpm_max_listen_queue`: 请求等待队列历史最高数
- `phpfpm_listen_queue_len`: 配置的等待队列最大长度
- `phpfpm_idle_processes`: 当前空闲的进程数
- `phpfpm_active_processes`: 当前活跃（正在处理请求）的进程数
- `phpfpm_total_processes`: 当前总进程数
- `phpfpm_max_active_processes`: 历史最多同时活跃的进程数
- `phpfpm_max_children_reached`: 进程数达到 `pm.max_children` 限制的次数
- `phpfpm_slow_requests`: 处理慢的请求数 (需开启 PHP-FPM 的慢日志)

## 监控大盘

本目录下提供了一个配套的基础 Dashboard (`dashboard.json`)，可用于监控 PHP-FPM 连接池的拥挤情况、进程数分布（空闲 vs 活跃）、以及达到最大子进程数限制的告警指标。