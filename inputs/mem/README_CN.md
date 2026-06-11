# Mem (内存) 采集插件

内存采集插件用于收集主机级别的内存使用率、空闲内存、缓存等物理内存相关指标。

**支持平台:** Windows, Linux, macOS, BSD 等

## 配置说明

```toml
# 采集主机物理内存指标
[[instances]]
# 通常无需任何特殊配置，保持默认启用即可。
```

## 采集指标

所有收集到的指标名称前缀为 `mem_`。
部分核心指标如下：

- `mem_total`: 总物理内存字节数
- `mem_available`: 可用内存字节数 (评估系统是否有内存压力的最重要指标)
- `mem_used`: 已用内存字节数
- `mem_used_percent`: 内存使用率 (%)
- `mem_free`: 绝对空闲的内存字节数
- `mem_cached`: 页面缓存占用的内存字节数 (Linux)
- `mem_buffers`: 块设备缓存占用的内存字节数 (Linux)
- `mem_swap_total` / `mem_swap_free` / `mem_swap_used_percent`: Swap 相关指标

## 监控大盘

该插件采集的指标是服务器最基础的监控数据之一。通常，OS 的内存监控大盘会与 CPU、磁盘等指标统一放置在全局的 **System (主机系统)** 大盘下面。
为方便单独查看，本目录也提供了一个仅包含内存维度的基础 Dashboard。