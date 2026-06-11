# Kubernetes Kube-Proxy 采集插件

该组件并非独立的 Go 原生采集插件，而是利用 Categraf 的 `prometheus` 抓取能力来采集 Kubernetes Kube-Proxy 组件本身暴露的 metrics 数据 (`/metrics` 接口)。

## 配置说明

要采集 Kube-Proxy 的指标，请使用并修改 Categraf 的 `prometheus` 插件配置。我们在示例配置中已经准备好了一个专用于 Kube-Proxy 的抓取模板。

参考配置：[kube_proxy.toml](../../conf/input.prometheus/kube_proxy.toml)

具体步骤：
1. 将参考配置文件 `kube_proxy.toml` 复制到您的 Categraf `conf/input.prometheus/` 目录下。
2. 确保您的 Kubernetes 集群中，kube-proxy 的 metrics 接口 (通常是 `127.0.0.1:10249/metrics` 或者节点 IP 的 `10249` 端口) 可以被 Categraf 访问到。如果在 DaemonSet 模式下，通常通过 Node IP 访问。
3. 修改配置中的 `urls` 指向正确的地址。

## 采集指标与监控大盘

Kube-Proxy 暴露的指标主要包含：
- 同步规则次数和耗时 (`kubeproxy_sync_proxy_rules_duration_seconds`)
- 网络编程延迟 (`kubeproxy_network_programming_duration_seconds`)
- REST 客户端请求状态

本目录下提供了一个配套的 Dashboard (`dashboard.json`)，您可以在 Grafana 或夜莺中导入该看板来观测您的 Kube-Proxy 运行状态。
