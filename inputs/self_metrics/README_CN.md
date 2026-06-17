# Self Metrics 采集插件

该插件用于采集 Categraf 自身的运行状态指标，包含 Go 运行时的基础指标（如 Goroutine 数量、GC 耗时、内存分配）以及 Categraf 特有的指标推送队列状态。

这对于监控监控客户端 (Agent) 本身的健康度至关重要，特别是判断是否存在发送队列堆积或内存泄漏。

## 配置说明

由于是内置插件采集自身状态，配置通常极其简单，只需启用即可。

```toml
# 采集 Categraf 自身指标
# interval = 15

# 无特殊配置项
```

## 采集指标

所有相关指标均以 `categraf_` 和 Go 运行时指标 `categraf_go_` / `categraf_process_` 为前缀。核心自监控指标包括：

- `categraf_info`: Categraf 版本信息，值为 1，带有 `version` 标签
- `categraf_metrics_enqueue_sum`: 指标入队总数 (推送到发送队列)
- `categraf_metrics_enqueue_failed_sum`: 指标入队失败总数
- `categraf_current_queue_size`: 当前待发送指标在内存队列中的堆积量 (如果此值持续上升，说明发送到服务端的速率跟不上采集速率，或服务端出现故障)
- `categraf_go_goroutines`: 当前 Goroutine 的数量
- `categraf_go_memstats_alloc_bytes`: Go 运行时分配的内存大小
- `categraf_process_cpu_seconds_total`: Categraf 进程累计消耗的 CPU 时间
- `categraf_process_resident_memory_bytes`: Categraf 进程占用的常驻物理内存大小 (RSS)

这些指标都会自动打上 `version` 等标签。

## 监控大盘

本目录下提供了一个配套的基础 Dashboard (`dashboard.json`)，用于快速监控 Categraf 自身进程的 CPU/内存使用率、Goroutine 数量，以及最重要的 **指标发送队列堆积情况**。
