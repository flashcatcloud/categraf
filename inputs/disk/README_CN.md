# Disk 采集插件

Disk 采集插件主要用于收集操作系统的磁盘分区使用情况。
该插件能够采集包括磁盘总容量、已用容量、剩余容量、磁盘使用率以及 Inode 的使用率等信息。

默认配置已经是推荐的通用配置，一般情况下无需修改。如果您发现收集到的文件系统存在不符合预期的情况（例如收集了太多不必要的虚拟文件系统），可以调整配置中的过滤项（如 `ignore_fs`）。

## 配置说明

```toml
# 设置 mount_points 后，仅采集指定挂载点。
# mount_points = ["/"]

# 按文件系统类型忽略挂载点。
# ignore_fs = ["tmpfs", "devtmpfs", "devfs", "iso9660", "overlay", "aufs", "squashfs", "nsfs", "CDFS", "fuse.juicefs"]

# 按挂载点路径前缀忽略。
# ignore_mount_points = ["/boot", "/var/lib/kubelet/pods"]
```

## 采集指标

常见指标包括但不限于：
- `disk_total`: 磁盘分区总容量 (Bytes)
- `disk_used`: 磁盘分区已用容量 (Bytes)
- `disk_free`: 磁盘分区剩余可用容量 (Bytes)
- `disk_used_percent`: 磁盘容量使用率 (%)
- `disk_inodes_total`: Inode 总数
- `disk_inodes_used`: 已使用的 Inode 数量
- `disk_inodes_free`: 剩余的 Inode 数量
- `disk_inodes_used_percent`: Inode 使用率 (%)

所有指标都会带上 `device`, `fstype`, `mode`, `path` 等标签。

## 监控大盘

建议将 OS 级别的监控 (如 CPU、Mem、Disk 等) 整合到统一的 System Dashboard 中。但为了方便独立查看，这里也提供了一份专门针对 Disk 分区使用情况的参考 Dashboard。
