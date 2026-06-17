# Tengine 采集插件

该插件用于采集 Tengine (或配置了 `ngx_http_reqstat_module` 的 Nginx) 的高级统计指标。
与基础的 `nginx` stub_status 相比，Tengine 提供了基于主机 (Host) 维度的详细流量、连接和 HTTP 状态码（2xx, 3xx, 4xx, 5xx, 甚至是 499 等细分状态码）的分布情况，还包含了与 Upstream 交互的耗时等指标。

## 前置要求

目标 Tengine/Nginx 必须编译并开启了 `ngx_http_reqstat_module` 模块。
在 Nginx/Tengine 的配置文件中，需要包含类似以下的配置：

```nginx
req_status_zone server_name $server_name 256k;
req_status server_name;

server {
    location /reqstat {
        req_status_show;
        allow 127.0.0.1;
        deny all;
    }
}
```

## 配置说明

在 Categraf 的 `conf/input.tengine/tengine.toml` 中配置你的请求 URL：

```toml
# 采集 Tengine 的高级 HTTP 状态指标
# interval = 15

[[instances]]
# Tengine reqstat 的 HTTP 访问地址，可以配置多个
urls = ["http://127.0.0.1/reqstat"]

# HTTP 请求超时时间
# timeout = "5s"

# 可选 TLS 配置
# ca_file = "/etc/telegraf/ca.pem"
# cert_file = "/etc/telegraf/cert.pem"
# key_file = "/etc/telegraf/key.pem"
# insecure_skip_verify = false
```

## 采集指标

该插件会将每个域名的状态转化为指标，默认前缀为 `tengine_`。所有指标会携带 `host` (请求所对应的虚拟主机名) 作为核心标签。

核心指标包含：
- `tengine_bytes_in` / `tengine_bytes_out`: 进出流量字节数。
- `tengine_conn_total`: 建立的总连接数。
- `tengine_req_total`: 处理的总请求数。
- `tengine_http_2xx` / `tengine_http_3xx` / `tengine_http_4xx` / `tengine_http_5xx`: 各种主类 HTTP 状态码的数量统计。
- `tengine_http_499` / `tengine_http_502` / `tengine_http_504`: 细分错误状态码的统计。
- `tengine_rt`: 请求总耗时 (RT)。
- `tengine_ups_req` / `tengine_ups_rt` / `tengine_ups_tries`: 发送给 Upstream 后端的请求数、耗时以及重试次数。

## 监控大盘

本目录下提供了一个配套的基础 Dashboard (`dashboard.json`)，用于按域名 (Host) 拆解展示 HTTP 流量、请求率 (QPS)、各种 HTTP 状态码的分布以及后端响应延迟情况。
