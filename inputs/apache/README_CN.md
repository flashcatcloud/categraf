# Apache 采集插件

此插件通过解析 Apache HTTP Server 的 `mod_status` 模块输出，来获取服务器的运行状态和性能指标。

## 配置说明

要使用此插件，您必须在 Apache 配置中启用 `mod_status` 模块，并确保 Categraf 能够访问该状态页面。建议在 URL 末尾加上 `?auto` 参数，以便获取机器可读的纯文本格式。

```toml
[[instances]]
# server-status 页面的 URL
# 请务必带上 '?auto' 参数
scrape_uri = "http://localhost/server-status/?auto"

# 可选: 覆盖请求的 Host 头
# host_override = "example.com"

# 可选: 跳过 TLS 证书校验
# insecure = false
```

### Apache mod_status 模块配置

在您的 `httpd.conf` 或 `apache2.conf` 文件中添加/取消注释以下内容以启用 `mod_status`：

```apache
<Location "/server-status">
    SetHandler server-status
    Require local
</Location>
```

修改后，请重启 Apache 服务以使配置生效。

## 采集指标

- `apache_accesses_total`: 服务器总处理请求数
- `apache_workers`: Apache 各类 worker 的数量 (例如 busy, idle)
- `apache_scoreboard`: 处于不同状态（如读、写、保持连接等）的 worker 数量
- `apache_up`: Apache 状态页是否可以正常连通
- `apache_uptime_seconds_total`: Apache 运行时间（秒）
