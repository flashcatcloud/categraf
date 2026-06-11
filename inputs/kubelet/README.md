# Kubernetes Kubelet Input Plugin

This component is not an independent Go native input plugin. Instead, it leverages Categraf's `prometheus` scraping capabilities to collect metrics exposed directly by the Kubernetes node component Kubelet (via its `/metrics` and `/metrics/cadvisor` endpoints).

## Configuration

To scrape Kubelet metrics, you should configure the `prometheus` plugin.

Reference configuration: `prometheus.toml`

Steps:
1. Add a new `[[instances]]` block in your `conf/input.prometheus/prometheus.toml` for Kubelet.
2. Ensure that Categraf (usually deployed as a DaemonSet on each Node) can access the Kubelet API on the current node. This often involves using the Node IP and a service account token.
3. Configure the correct authentication in your prometheus configuration according to your Kubernetes cluster's security setup (e.g., TLS settings, token file paths).

## Metrics and Dashboards

Key metrics exposed by Kubelet include:
- Pod running status and volume mount status on the node
- Latency of Kubelet's own API operations (`kubelet_runtime_operations_duration_seconds`)
- OOM events, PLEG latency
- Built-in cAdvisor container metrics (like `container_cpu_usage_seconds_total`)

A matched Dashboard (`dashboard.json`) is provided in this directory. You can import this dashboard into Grafana or Nightingale to monitor the operational status of your Kubelet instances and containers.
