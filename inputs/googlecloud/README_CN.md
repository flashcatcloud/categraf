# Google Cloud 监控指标采集插件

该插件用于从 Google Cloud Platform (GCP) 的 Cloud Monitoring API (原 Stackdriver) 中拉取云资源的监控指标。

## 前置条件

使用该插件前，您需要确保提供的 GCP 服务账号 (Service Account) 凭证可以读取 Cloud Monitoring 时序数据。OAuth scope 使用：
- `https://www.googleapis.com/auth/monitoring.read`

IAM 侧请授予只读监控角色，例如 Monitoring Viewer。

## 配置说明

```toml
# 采集周期，因为调用云厂商 API 可能存在计费和限流限制，建议设置 >= 60 秒
interval = 60

[[instances]]
# # 您的 GCP 项目 ID
project_id = "your-project-id"

# # 认证凭据 JSON 文件的绝对路径
credentials_file = "/path/to/your/key.json"

# # 或者直接在此处配置 JSON 字符串内容
# credentials_json = "{...}"

# # 数据延迟时间：实际查询的 End Time = Now - delay (防止由于 GCP 数据写入延迟导致拉空数据)
# delay = "2m"

# # 查询的时间窗口跨度：Start Time = Now - delay - period
# period = "1m"

# # 指标过滤器 (如果留空，默认会拉取项目中大部分指标，可能消耗极大)
# filter = "metric.type=\"compute.googleapis.com/instance/cpu/utilization\" AND resource.labels.zone=\"asia-northeast1-a\""

# # HTTP 请求超时时间
# timeout = "5s"

# # GCP 指标元数据的缓存时长 (当 filter 为空时生效，避免频繁请求元数据)
# cache_ttl = "1h"

# # 将 GCE (Google Compute Engine) 的 instance_name 提取为别名，并作为一个新的 Tag 追加到指标上
# gce_host_tag = "gce_host"

# # 最大并发请求数 (默认为 30，范围是 1 到 100)
# request_inflight = 30

# # 强制设置更大的并发请求数 (前提是您知道您在做什么并且确信不会触发 GCP 限流)
# force_request_inflight = 200
```

## 采集指标与监控大盘

由于此插件本质上是 Google Cloud Monitoring API 的一个透传代理，采集到的指标完全取决于您的 `filter` 规则以及您使用的 GCP 服务。因此：
- 指标名称会根据 GCP 的 Metric Type 自动映射。
- 没有统一的预置监控大盘，您需要在监控系统中针对具体的业务（如 GCE, Cloud SQL 等）自行配置 Dashboard。
