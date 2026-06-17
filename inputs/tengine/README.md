# Tengine Input Plugin

This plugin collects advanced statistics from Tengine (or Nginx with the `ngx_http_reqstat_module` compiled and enabled).
Compared to the basic `nginx` stub_status plugin, this Tengine plugin provides detailed, host-level (virtual server) metrics including traffic, connections, HTTP status codes (2xx, 3xx, 4xx, 5xx, and detailed codes like 499), and Upstream interaction latencies.

## Prerequisites

The target Tengine/Nginx server must have the `ngx_http_reqstat_module` compiled and enabled.
Your Nginx/Tengine configuration should contain a block similar to the following:

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

## Configuration

Configure your request URLs in Categraf's `conf/input.tengine/tengine.toml`:

```toml
# Collect Tengine HTTP reqstat metrics
# interval = 15

[[instances]]
# The HTTP URL to the Tengine reqstat endpoint. Multiple URLs can be configured.
urls = ["http://127.0.0.1/reqstat"]

# HTTP request timeout
# timeout = "5s"

# Optional TLS configuration
# ca_file = "/etc/telegraf/ca.pem"
# cert_file = "/etc/telegraf/cert.pem"
# key_file = "/etc/telegraf/key.pem"
# insecure_skip_verify = false
```

## Metrics

The plugin converts each virtual host's statistics into metrics prefixed by `tengine_`. All metrics will carry the `host` tag, which corresponds to the virtual server name processing the request.

Core metrics include:
- `tengine_bytes_in` / `tengine_bytes_out`: Inbound and outbound traffic in bytes.
- `tengine_conn_total`: Total connections established.
- `tengine_req_total`: Total requests processed.
- `tengine_http_2xx` / `tengine_http_3xx` / `tengine_http_4xx` / `tengine_http_5xx`: Counter for major HTTP status code categories.
- `tengine_http_499` / `tengine_http_502` / `tengine_http_504`: Counter for specific detailed error status codes.
- `tengine_rt`: Total request response time (RT).
- `tengine_ups_req` / `tengine_ups_rt` / `tengine_ups_tries`: Number of requests forwarded to the Upstream, Upstream latency, and retry counts.

## Dashboards

A companion basic Dashboard (`dashboard.json`) is provided in this directory. It provides a breakdown by virtual host (`host`) showing HTTP traffic, Request Rate (QPS), HTTP status code distributions, and backend Upstream latencies.
