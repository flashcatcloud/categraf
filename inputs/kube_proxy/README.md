# Kubernetes Kube-Proxy Input Plugin

This component is not an independent Go native input plugin. Instead, it leverages Categraf's `prometheus` scraping capabilities to collect metrics exposed directly by the Kubernetes Kube-Proxy component (via its `/metrics` endpoint).

## Configuration

To scrape Kube-Proxy metrics, you should configure the `prometheus` plugin. We have prepared a dedicated scraping template for Kube-Proxy in the example configuration directory.

Reference configuration: `prometheus.toml`

Steps:
1. Add a new `[[instances]]` block in your `conf/input.prometheus/prometheus.toml` for kube-proxy.
2. Ensure that Categraf can access the kube-proxy metrics endpoint (typically `127.0.0.1:10249/metrics` or `NodeIP:10249`). When running as a DaemonSet, this is usually accessed via the Node IP.
3. Modify the `urls` in the configuration to point to the correct address.

## Metrics and Dashboards

Key metrics exposed by Kube-Proxy include:
- Sync proxy rules count and duration (`kubeproxy_sync_proxy_rules_duration_seconds`)
- Network programming latency (`kubeproxy_network_programming_duration_seconds`)
- REST client request status

A matched Dashboard (`dashboard.json`) is provided in this directory. You can import this dashboard into Grafana or Nightingale to monitor the operational status of your Kube-Proxy instances.
