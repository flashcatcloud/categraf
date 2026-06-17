# Disk Input Plugin

The Disk input plugin gathers metrics about disk usage across different partitions.
It collects information such as total disk space, used space, free space, disk usage percentage, and inode usage percentage.

The default configuration is already the recommended setting for most environments and generally does not need to be modified. If you notice unexpected file systems being monitored (e.g., too many virtual file systems), you can adjust the filtering options like `ignore_fs`.

## Configuration

```toml
# Set mount_points to restrict collection to specific mount points.
# mount_points = ["/"]

# List of filesystem types to ignore.
# ignore_fs = ["tmpfs", "devtmpfs", "devfs", "iso9660", "overlay", "aufs", "squashfs", "nsfs", "CDFS", "fuse.juicefs"]

# List of mount point prefixes to ignore.
# ignore_mount_points = ["/boot", "/var/lib/kubelet/pods"]
```

## Metrics

Common metrics include but are not limited to:
- `disk_total`: Total disk space on the partition (Bytes)
- `disk_used`: Used disk space on the partition (Bytes)
- `disk_free`: Free available disk space (Bytes)
- `disk_used_percent`: Percentage of used disk space (%)
- `disk_inodes_total`: Total number of inodes
- `disk_inodes_used`: Number of used inodes
- `disk_inodes_free`: Number of free inodes
- `disk_inodes_used_percent`: Percentage of used inodes (%)

All metrics will include tags such as `device`, `fstype`, `mode`, and `path`.

## Dashboard

It is recommended to integrate OS-level metrics (CPU, Mem, Disk, etc.) into a unified System Dashboard. However, a dedicated Disk usage reference Dashboard is also provided here for independent viewing.
