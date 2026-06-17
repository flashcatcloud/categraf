# PHP-FPM Input Plugin

This plugin monitors and collects process status metrics from PHP-FPM. It is adapted from the telegraf phpfpm plugin and supports connecting to the PHP-FPM status page via HTTP URLs or Unix Sockets.

## Prerequisites

Before using this plugin, you must modify the PHP-FPM configuration file (typically `www.conf`) to enable the `pm.status_path` directive:

```ini
pm.status_path = /status
```
After making this change, restart the PHP-FPM process for it to take effect.
If you use Nginx as a reverse proxy, ensure Nginx is also configured to forward the `/status` route to PHP-FPM.

## Configuration

You can collect metrics from multiple PHP-FPM instances. Check `conf/input.phpfpm/phpfpm.toml` for details:

```toml
# Collect PHP-FPM status
# interval = 15

[[instances]]
# URLs can be HTTP endpoints or Unix Sockets
# For example:
# urls = ["http://localhost/status", "unix:///var/run/php5-fpm.sock"]
urls = ["http://127.0.0.1/status"]

# Notes:
# 1. The following timeout and authentication settings ONLY apply to HTTP URLs:
# response_timeout = "5s"
# username = ""
# password = ""
# headers = ["X-Custom-Header: value"]
# TLS/SSL configurations also only apply to HTTP.

# 2. If you are using a Unix socket, you must ensure that the user running Categraf has read permissions for the socket file.
```

## Metrics

All metrics are prefixed with `phpfpm_` and include `url` and `pool` tags by default. Key metrics include:

- `phpfpm_accepted_conn`: Total number of requests accepted
- `phpfpm_listen_queue`: Number of requests in the queue of pending connections
- `phpfpm_max_listen_queue`: Maximum number of requests in the queue of pending connections since FPM has started
- `phpfpm_listen_queue_len`: The size of the socket queue of pending connections
- `phpfpm_idle_processes`: Number of idle processes
- `phpfpm_active_processes`: Number of active processes
- `phpfpm_total_processes`: Total number of idle + active processes
- `phpfpm_max_active_processes`: Maximum number of active processes since FPM has started
- `phpfpm_max_children_reached`: Number of times the process limit `pm.max_children` has been reached
- `phpfpm_slow_requests`: Number of slow requests (requires PHP-FPM slowlog to be enabled)

## Dashboards

A basic companion Dashboard (`dashboard.json`) is provided in this directory to monitor PHP-FPM connection pool congestion, process distributions (Idle vs Active), and alerts for hitting the maximum child process limit.
