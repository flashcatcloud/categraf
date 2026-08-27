# json_exporter

该插件把远端 HTTP 接口返回的 JSON 按 JSONPath 转成 Categraf 指标。实现参考并适配自 [prometheus-community/json_exporter](https://github.com/prometheus-community/json_exporter)，不需要额外运行 exporter，也不需要再通过 Prometheus 文本格式转一次。

## 配置

默认配置位于 `conf/input.json_exporter/json_exporter.toml`。每个 `[[instances]]` 相当于上游 json_exporter 的一个 module，包含一组 `targets`、HTTP 参数和指标规则。

```toml
[[instances]]
targets = ["http://localhost:8000/data.json"]
url_label_key = "instance"
url_label_value = "{{.Host}}"

[[instances.metrics]]
name = "example_global_value"
path = "{.counter}"
labels = { environment = "beta", location = "planet-{.location}" }

[[instances.metrics]]
name = "example_value"
type = "object"
path = '''{.values[?(@.state == "ACTIVE")]}'''
labels = { id = "{.id}" }
values = { active = "1", count = "{.count}", boolean = "{.some_boolean}" }
```

对应 JSON：

```json
{
  "counter": 1234,
  "location": "mars",
  "values": [
    {"id": "id-A", "count": 1, "some_boolean": true, "state": "ACTIVE"},
    {"id": "id-B", "count": 2, "some_boolean": false, "state": "INACTIVE"}
  ]
}
```

将产生：

```text
json_exporter_global_value{environment="beta",location="planet-mars"} 1234
json_exporter_value_active{id="id-A"} 1
json_exporter_value_count{id="id-A"} 1
json_exporter_value_boolean{id="id-A"} 1
```

## 指标规则

- 未设置 `type` 时默认为 `value`，`path` 从整个 JSON 文档选择一个值。
- `type = "object"` 时，`path` 先选择对象列表；`values` 中的每个键会生成 `<name>_<键>` 指标，并在每个对象内计算对应 JSONPath。
- `labels` 的值既可以是静态文本，也可以包含 JSONPath，例如 `"planet-{.location}"`。
- 数字直接转为浮点值；布尔值转为 `true = 1`、`false = 0`。
- `allow_missing_key = true` 会跳过不存在的路径。
- `epoch_timestamp` 可指定时间字段，单位为 Unix 毫秒。
- JSONPath 语法与上游一致，参见 [Kubernetes JSONPath 文档](https://kubernetes.io/docs/reference/kubectl/jsonpath/)。

上游的 `help` 和 `value_type` 属于 Prometheus exposition 元数据，Categraf 的 sample 模型不使用这两个字段，因此本插件不提供它们。上游 `/probe` 查询参数驱动的 body 模板也不适用于周期采集；需要 POST 时直接配置 `method = "POST"` 和 `body`。

## HTTP 与状态指标

插件沿用 Categraf 通用 HTTP 配置，包括 `headers`、Basic Auth、TLS、代理、超时、重定向和 keep-alive。默认接受所有 2xx 响应，可用 `valid_status_codes` 覆盖。

每个目标还会产生：

- `json_exporter_up`：成功取得有效 JSON 时为 1，请求、状态码或 JSON 格式错误时为 0。
- `json_exporter_scrape_use_seconds`：本次目标采集耗时。

代码中保留了上游 Apache-2.0 版权说明；适配基线为 upstream commit `67ed7ce55a0035a4c5ca0246694bfb424ff5dff8`。
