# AppDynamics 采集插件

此插件通过调用 AppDynamics REST API 来抓取监控指标，并将 JSON 响应转换为 Categraf 可用的 Prometheus 指标格式。

## 配置说明

插件通过配置 `url_base` 模板和 `url_vars` 变量列表来动态拼装 API 请求地址。由于 AppDynamics API 需要指定时间范围，插件会自动替换请求中的 `$START_TIME` 和 `$END_TIME` 占位符。

```toml
[[instances]]
# AppDynamics Controller REST API 的基础请求模板
url_base = "http://your-appdynamics-controller:8090/controller/rest/applications/{{.app_id}}/metric-data?metric-path={{.metric_path}}&time-range-type=BETWEEN_TIMES&start-time=$START_TIME&end-time=$END_TIME&output=JSON"

# 注入到 url_base 模板中的变量列表，可以配置多个查询任务
url_vars = [
    { app_id = "123", metric_path = "Overall Application Performance|Calls per Minute" },
    { app_id = "123", metric_path = "Overall Application Performance|Average Response Time (ms)" }
]

# 接口基础认证 (Basic Auth)
username = "user@tenant"
password = "your-password"

# 需要提取的指标字段类型。
# 可选项: "current", "max", "min", "count", "sum", "value"。
# 若不指定，默认提取 "current"。
filters = ["current", "sum"]

# 可选: 网络与采集时间参数
timeout = "5s"
delay = "1m"
period = "1m"
```

## 采集指标

- `up`: AppDynamics 接口是否连通并返回了正常数据（1 为正常，0 为失败）。
- `appdynamics_{metric_name}_{filter}`: 动态生成的指标。`metric_name` 是从 API 路径中最后一个层级提取并经过 `snake_case` 转换得来的。
  - 例如：当请求路径为 `Overall Application Performance|Calls per Minute`，且 filter 包含 `current` 时，将产生指标 `appdynamics_calls_per_minute_current`。
