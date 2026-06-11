# Exec 采集插件

Exec 插件主要用于执行用户自定义的监控脚本或程序，并将脚本输出到标准输出 (stdout) 的数据截获下来，解析后上报给服务端。
这是 Categraf 最灵活的插件之一，适用于 Categraf 官方插件库之外的特殊或高度定制化的业务监控场景。

## 脚本输出格式

被执行的脚本必须将监控数据输出到标准输出，支持以下 3 种格式 (通过 `data_format` 参数配置)：

### 1. influx
```text
mesurement,labelkey1=labelval1,labelkey2=labelval2 field1=1.2,field2=2.3
```
- 指标名 (mesurement) 和标签 (Tags) 之间用逗号分隔
- 标签之间用逗号分隔
- 标签和属性字段 (Fields) 之间用**空格**分隔
- 最终的指标名会根据 `mesurement` 和 `field` 组合生成

### 2. prometheus
直接输出 Prometheus 的标准 Exposition 格式：
```text
# HELP demo_http_requests_total Total number of http api requests
# TYPE demo_http_requests_total counter
demo_http_requests_total{api="add_product"} 4633433
```
以 `#` 开头的行会被 Categraf 忽略。

### 3. falcon
Open-Falcon JSON 格式：
```json
[
    {
        "endpoint": "test-endpoint",
        "metric": "test-metric",
        "value": 1,
        "tags": "idc=lg,loc=beijing"
    }
]
```
`timestamp`, `step`, `counterType` 等字段会被忽略，Categraf 自身会重新打上时间戳并按照全局规则上报。

## 配置说明

```toml
[[instances]]
# # 要执行的命令或脚本路径，支持 shell 的 glob 通配符
commands = [
     "/opt/categraf/scripts/*/collect_*.sh",
     "/opt/categraf/scripts/*/collect_*.py"
]

# # 脚本执行的超时时间，必须设置以防止僵尸进程
# timeout = 5

# # 解析脚本输出的格式，可选值: influx, prometheus, falcon
data_format = "influx"
```

## 采集指标与大盘

由于 Exec 插件收集的指标完全由用户脚本决定，因此没有固定的采集指标列表和统一的监控大盘。
您可以根据自己脚本输出的 metric name，在夜莺 (Nightingale) 或 Grafana 中自行绘制 Dashboard 和配置告警规则。
