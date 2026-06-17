# Kubernetes Kube-Proxy 采集插件

该组件并非独立的 Go 原生采集插件，而是利用 Categraf 的 `prometheus` 抓取能力来采集 Kubernetes Kube-Proxy 组件本身暴露的 metrics 数据 (`/metrics` 接口)。

## 配置说明

要采集 Kube-Proxy 的指标，请使用并修改 Categraf 的 `prometheus` 插件配置。

参考配置：`prometheus.toml`

具体步骤：
1. 在 `conf/input.prometheus/prometheus.toml` 中新增一个用于抓取 kube-proxy 的 `[[instances]]` 配置块。
2. 确保您的 Kubernetes 集群中，kube-proxy 的 metrics 接口 (通常是 `127.0.0.1:10249/metrics` 或者节点 IP 的 `10249` 端口) 可以被 Categraf 访问到。如果在 DaemonSet 模式下，通常通过 Node IP 访问。
3. 修改配置中的 `urls` 指向正确的地址。
4. 设置 `url_label_key = "ident"` 和 `labels = { job = "kube-proxy" }`，因为本目录下的 Dashboard 使用 `job` 过滤指标，并使用 `ident` 区分实例。

示例：

```toml
[[instances]]
urls = ["http://127.0.0.1:10249/metrics"]
url_label_key = "ident"
url_label_value = "{{.Host}}"
labels = { job = "kube-proxy" }
```

## 采集指标与监控大盘

Kube-Proxy 暴露的指标主要包含：
- 同步规则次数和耗时 (`kubeproxy_sync_proxy_rules_duration_seconds`)
- 网络编程延迟 (`kubeproxy_network_programming_duration_seconds`)
- REST 客户端请求状态

本目录下提供了一个配套的 Dashboard (`dashboard.json`)，您可以在 Grafana 或夜莺中导入该看板来观测您的 Kube-Proxy 运行状态。
