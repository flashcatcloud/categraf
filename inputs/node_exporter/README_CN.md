# Node Exporter 采集插件

该插件直接集成了 Prometheus 官方的 [node_exporter](https://github.com/prometheus/node_exporter) 核心逻辑，用于采集 *nix 类系统的全面硬件和操作系统指标。
相比于原生的 Categraf 插件 (如 `cpu`, `mem`, `disk` 等)，该插件能够提供和官方 `node_exporter` 100% 一致的指标集，方便用户直接复用社区中丰富的基于 node_exporter 的 Grafana 看板和告警规则。

**支持平台:** Linux, macOS, BSD 等

## 配置说明

```toml
# 采集 Node Exporter 兼容指标
# interval = 15

[[instances]]
# 通常只需启用该插件即可。
# 如果有特别的 collector 开启/关闭需求，您可以在 categraf 的命令行启动参数中传入
# 例如：--collector.textfile.directory=/var/lib/node_exporter/textfile_collector
```

*注意：在 Categraf 中启用 `node_exporter` 插件时，可能会与 Categraf 自带的 `cpu`, `mem`, `disk` 等基础插件在语义上存在一定重叠，通常建议在一个机器上：要么使用 Categraf 自身的基础插件套餐，要么只开启这一个 `node_exporter` 插件。*

## 采集指标

所有的指标均遵循 Prometheus 官方 `node_exporter` 的命名规范，通常以 `node_` 开头。例如：
- `node_cpu_seconds_total`
- `node_memory_MemAvailable_bytes`
- `node_network_receive_bytes_total`
- `node_filesystem_free_bytes`
- `node_disk_read_bytes_total`

更多关于采集器的具体说明，请直接参考 [node_exporter 官方文档](https://github.com/prometheus/node_exporter)。

## 监控大盘

由于此插件 100% 兼容开源 `node_exporter`，您可以直接在 Grafana 导入社区流行的 `Node Exporter Full` 看板 (如 Dashboard ID: 1860)。
本目录下也为您提供了一个极简的基础监控 Dashboard (`dashboard.json`)，用于快速验证数据采集是否正常。
