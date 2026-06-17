# Prometheus 采集插件

该插件的作用是通用抓取 HTTP `/metrics` 接口暴露的数据（即 Prometheus 格式数据），并将其上报给服务端。
随着云原生生态的发展，越来越多的开源组件和业务程序原生内置了 Prometheus SDK 并暴露监控数据，您可以通过这个插件非常方便地抓取它们。

*(注意：由于命名冲突和功能演进，此插件的部分功能也可能由名为 `openmetrics` 的插件承载，配置逻辑基本互通)*

## 配置说明

支持通过直接提供静态 URL 列表抓取，也支持通过 Consul 服务发现动态抓取。

```toml
# 采集通用 Prometheus 接口
# interval = 15

[[instances]]
# 静态地址抓取配置示例
urls = ["http://localhost:9100/metrics", "http://localhost:9104/metrics"]

# 指标标签提取控制
# 默认情况下，Categraf 会将抓取来源的 URL 地址作为 `instance` 标签附加到每条监控数据中。
# 您可以通过 url_label_value 自定义要保留的部分 (支持 Go Template 语法)
# url_label_key = "instance"
# url_label_value = "{{.Host}}"
```

### URL 标签的高级配置

`url_label_value` 支持多种 Go Template 变量：
`{{.Scheme}}`, `{{.Host}}`, `{{.Hostname}}`, `{{.Port}}`, `{{.Path}}`, `{{.Query}}`, `{{.Fragment}}`

如果你只想要主机名加端口：
```toml
url_label_value = "{{.Host}}"
```

## 采集指标

采集到的指标名和 Label 会 **100% 忠实保留** 目标 `/metrics` 接口返回的原始数据。

## 监控大盘

由于此插件是通用的数据抓取工具，采集的指标完全取决于它所抓取的目标服务。因此，不存在一个适用于所有情况的固定 Dashboard。
在 Grafana 或夜莺中配置面板时，请根据您具体抓取的业务服务的指标名进行自定义绘制。本目录中提供的是一个纯文本指引说明的 Dashboard。