# http_response

HTTP 探测插件，用于检测 HTTP 地址的连通性、延迟、HTTPS证书过期时间

## code meanings

```
Success          = 0
ConnectionFailed = 1
Timeout          = 2
DNSError         = 3
AddressError     = 4
BodyMismatch     = 5
CodeMismatch     = 6
```

## Configuration

最核心的配置就是 targets 配置，配置目标地址，比如想要监控两个地址：

```toml
[[instances]]
targets = [
    "http://localhost:8080",
    "https://www.baidu.com"
]
```

instances 下面的所有 targets 共享同一个 `[[instances]]` 下面的配置，比如超时时间，HTTP方法等，如果有些配置不同，可以拆成多个不同的 `[[instances]]`，比如：

```toml
[[instances]]
targets = [
    "http://localhost:8080",
    "https://www.baidu.com"
]
method = "GET"

[[instances]]
targets = [
    "http://localhost:9090"
]
method = "POST"
```

## 指标说明

- `http_response_dns_request` DNS 解析耗时，单位毫秒
- `http_response_tcp_connect` TCP 建连耗时，单位毫秒
- `http_response_tls_handshake` TLS 握手耗时，单位毫秒
- `http_response_first_byte` 首包响应耗时，单位毫秒
- `http_response_total_cost` 请求总耗时，单位毫秒
- `http_response_response_time` 响应耗时，单位秒，保留为兼容旧指标
- `http_response_response_code` HTTP 响应码
- `http_response_result_code` 探测结果码
- `http_response_cert_expire_timestamp` HTTPS 证书过期时间戳

说明：

- 使用 IP 直连或连接复用时（HTTP/HTTPS 均可能），部分阶段指标可能为 `-1`
- `http_response_cert_expire_timestamp` 仅在 HTTPS 目标且成功建立 TLS 连接时输出

## 监控大盘和告警规则

该 README 的同级目录下，提供了 dashboard.json 就是监控大盘的配置，alerts.json 是告警规则，可以导入夜莺使用。