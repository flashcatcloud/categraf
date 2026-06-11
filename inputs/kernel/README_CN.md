# Kernel 采集插件

该插件用于采集本机的 Linux 内核状态信息。
数据通常来源于 `/proc/stat` 和 `/proc/vmstat`。

**支持平台:** Linux

## 配置说明

```toml
# 采集 Linux 系统的 Kernel 指标
[[instances]]
# 无需任何特殊配置，只需启用即可
```

## 采集指标

- `kernel_boot_time`: 系统启动时间 (Epoch 秒数)
- `kernel_context_switches`: 系统启动以来的上下文切换总次数
- `kernel_interrupts`: 系统启动以来的中断总次数
- `kernel_processes_forked`: 系统启动以来的 fork() 创建的进程总数
- `kernel_entropy_avail`: 系统当前可用的熵池大小 (通常用于衡量生成随机数的速度)

## 监控大盘

该插件采集的 Kernel 指标通常属于服务器基础监控的一部分，因此在实际应用中往往会与 CPU、内存等指标一起放在全局的 `System` 大盘中。
为方便单独查看测试，这里也提供了一个简单的 Kernel 专属监控大盘。