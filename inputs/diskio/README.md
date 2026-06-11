# DiskIO Input Plugin

The DiskIO input plugin collects block device I/O read and write statistics.
By analyzing these metrics, you can identify disk I/O bottlenecks, measure I/O throughput, and monitor operation latency.

## Metrics

Common metrics include but are not limited to:
- `diskio_read_bytes`: Total number of bytes read from the device
- `diskio_write_bytes`: Total number of bytes written to the device
- `diskio_reads`: Total number of completed read operations
- `diskio_writes`: Total number of completed write operations
- `diskio_read_time`: Total time spent in read operations (ms)
- `diskio_write_time`: Total time spent in write operations (ms)
- `diskio_io_time`: Total time spent doing I/O operations (ms)

All metrics will include the `name` tag (e.g., `sda`, `vda`) to identify the block device.

## Dashboard

It is recommended to integrate OS-level metrics (CPU, Mem, Disk, DiskIO, etc.) into a unified System Dashboard. However, a dedicated DiskIO reference Dashboard is also provided here for independent viewing.
