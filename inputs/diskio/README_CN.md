# DiskIO 采集插件

DiskIO 采集插件主要用于收集硬盘 (Block Devices) 的底层 I/O 读写情况。
通过分析这些指标，可以了解系统的磁盘读写瓶颈、I/O 吞吐量以及 I/O 操作的延迟。

## 采集指标

常见指标包括但不限于：
- `diskio_read_bytes`: 从设备读取的总字节数
- `diskio_write_bytes`: 写入设备的总字节数
- `diskio_reads`: 成功完成的读取操作总次数
- `diskio_writes`: 成功完成的写入操作总次数
- `diskio_read_time`: 读取操作消耗的总时间 (毫秒)
- `diskio_write_time`: 写入操作消耗的总时间 (毫秒)
- `diskio_io_time`: I/O 请求消耗的总时间 (毫秒)

所有指标都会带上 `name` (如 `sda`, `vda`) 等标签。

## 监控大盘

建议将 OS 级别的监控 (如 CPU、Mem、Disk、DiskIO 等) 整合到统一的 System Dashboard 中。但为了方便独立查看，这里也提供了一份专门针对 DiskIO 性能的参考 Dashboard。