# Linux Sysctl FS Input Plugin

This plugin collects Linux kernel filesystem-level parameter metrics, directly sourced from the `/proc/sys/fs/` directory.
It is highly recommended for monitoring system-wide file descriptor limits (file-max) and kernel inode/dentry cache usage.

**Supported Platforms:** Linux

## Configuration

```toml
# Collect Linux system file descriptor and inode status limits
[[instances]]
# This plugin requires no special configuration. Just enable it.
```

## Metrics

All collected metrics are prefixed with `linux_sysctl_fs_`.
Key metrics include:

- `linux_sysctl_fs_file-nr`: Number of allocated file handles
- `linux_sysctl_fs_file-max`: Maximum number of allowed file handles
- `linux_sysctl_fs_inode-nr`: Number of allocated inodes
- `linux_sysctl_fs_inode-free-nr`: Number of free inodes
- `linux_sysctl_fs_dentry-nr`: Number of dentry cache entries
- `linux_sysctl_fs_dentry-unused-nr`: Number of unused dentry cache entries
- `linux_sysctl_fs_aio-nr`: Current number of asynchronous I/O (AIO) requests
- `linux_sysctl_fs_aio-max-nr`: Maximum allowed number of AIO requests

## Dashboards

These metrics reflect critical system-level limits, especially the ratio between `file-nr` and `file-max` (File Descriptor Usage Rate). We have provided a default Dashboard to help you track these core limitations.
