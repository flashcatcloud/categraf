# Kubernetes Kubelet 采集插件

该组件并非独立的 Go 原生采集插件，而是利用 Categraf 的 `prometheus` 抓取能力来采集 Kubernetes 节点组件 Kubelet 暴露的 metrics 数据 (`/metrics` 和 `/metrics/cadvisor` 等接口)。

## 配置说明

要采集 Kubelet 的指标，请使用并修改 Categraf 的 `prometheus` 插件配置。

参考配置：`prometheus.toml`

具体步骤：
1. 在 `conf/input.prometheus/prometheus.toml` 中新增一个用于抓取 Kubelet 的 `[[instances]]` 配置块。
2. 确保 Categraf 作为 DaemonSet 部署在每个 Node 上时，可以访问到当前节点的 Kubelet API（通常通过挂载 Node 的 IP 和相应的认证 Token 获取）。
3. 根据您的 Kubernetes 集群的安全配置（如是否需要 TLS，Token 文件路径），在相应的配置块中配置正确的认证信息。

## 采集指标与监控大盘

Kubelet 暴露的指标主要包含：
- 节点的 Pod 运行状态、卷挂载状态
- Kubelet 自身的 API 操作延迟 (`kubelet_runtime_operations_duration_seconds`)
- OOM 记录、PLEG 延迟
- 内置的 cAdvisor 容器指标 (`container_cpu_usage_seconds_total` 等)

本目录下提供了一个配套的 Dashboard (`dashboard.json`)，您可以在 Grafana 或夜莺中导入该看板来观测您的 Kubelet 和容器运行状态。
