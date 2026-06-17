# Processes 采集插件

该插件用于统计操作系统的总体进程数量分布。例如，系统中当前处于 Running、Sleeping、Zombie 等状态的进程各有多少个。

**支持平台:** Linux, FreeBSD, OpenBSD, macOS

*注意：此插件在 Windows 上不受支持。Windows 系统下相关逻辑不生效。*

## 配置说明

通常无需特殊配置，保持默认启用即可。

```toml
# 采集系统进程状态分布
# 无特别配置项
```

## 采集指标

采集的指标统一使用 `processes_` 作为前缀。主要指标包括但不限于：

- `processes_total`: 系统中总的进程数量
- `processes_running`: 处于正在运行状态的进程数
- `processes_sleeping`: 处于睡眠状态的进程数
- `processes_zombies`: 处于僵尸状态的进程数
- `processes_stopped`: 被暂停的进程数
- `processes_paging`: 处于 paging 状态的进程数
- `processes_dead`: 处于 dead 状态的进程数
- `processes_idle`: 处于 idle 状态的进程数
- `processes_total_threads`: 同上，系统中总的线程数

## 监控大盘

这些指标是主机基础监控的一部分。通常，OS 的进程监控会与其他硬件指标统一放置在 **System (主机系统)** 大盘中。
本目录下也为您提供了一个仅包含进程状态分布的基础 Dashboard。
