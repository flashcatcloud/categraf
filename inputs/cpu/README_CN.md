# CPU 采集插件

CPU 采集插件主要用于自动收集本机 CPU 的使用率、空闲率等各项指标。

默认情况下，插件只采集整机 (Global) 的汇总指标。如果需要采集单个 CPU 核心的独立指标，可以通过配置开启。

## 配置说明

```toml
# 是否采集每个独立 CPU 核心的指标
collect_per_cpu = false
```

开启 `collect_per_cpu = true` 后，各项指标会带有 `cpu` 标签（例如 `cpu="cpu0"`, `cpu="cpu1"`），以此来区分不同的核心；整机的汇总指标通常会带 `cpu="cpu-total"` 标签。

## 采集指标

常见指标包括但不限于：
- `cpu_usage_active`: CPU 活跃时间占比 (100 - idle)
- `cpu_usage_user`: 用户态消耗的 CPU 时间占比
- `cpu_usage_system`: 内核态消耗的 CPU 时间占比
- `cpu_usage_idle`: CPU 空闲时间占比
- `cpu_usage_iowait`: CPU 等待 I/O 的时间占比

## 监控大盘

建议将 OS 级别的监控 (如 CPU、Mem、Disk 等) 整合到统一的 System Dashboard 中。但为了方便独立查看，这里也提供了一份专门针对 CPU 的参考 Dashboard。
