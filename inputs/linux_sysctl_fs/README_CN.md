# Linux Sysctl FS 采集插件

该插件用于采集 Linux 内核文件系统级别的参数指标，这些指标直接来源于 `/proc/sys/fs/` 目录。
它非常适合用来监控系统级的文件描述符限制 (file-max) 以及内核 inode/dentry 缓存状态。

**支持平台:** Linux

## 配置说明

```toml
# 采集 Linux 系统文件句柄与 Inode 等限制状态
[[instances]]
# 该插件无需任何特殊配置，启用即可。
```

## 采集指标

所有收集到的指标名称前缀为 `linux_sysctl_fs_`。
主要指标如下：

- `linux_sysctl_fs_file_nr`: 系统当前已经分配的文件句柄数
- `linux_sysctl_fs_file_max`: 系统允许分配的最大文件句柄数
- `linux_sysctl_fs_inode_nr`: 当前分配的 inode 数量
- `linux_sysctl_fs_inode_free_nr`: 当前空闲的 inode 数量
- `linux_sysctl_fs_dentry_nr`: dentry 缓存的数量
- `linux_sysctl_fs_dentry_unused_nr`: 未使用的 dentry 缓存数量
- `linux_sysctl_fs_aio_nr`: 当前的异步 I/O (AIO) 请求数量
- `linux_sysctl_fs_aio_max_nr`: 允许的最大异步 I/O 请求数量

## 监控大盘

这些指标反映了极其重要的系统级限制，特别是 `file-nr` 和 `file-max` 的比例（文件描述符使用率）。我们为您准备了默认的 Dashboard 来追踪这几个核心限制。
