# CPU Input Plugin

The CPU input plugin automatically collects various CPU metrics of the local machine, such as CPU usage, idle percentage, and system time.

By default, the plugin only collects global (total) CPU metrics. If you want to collect metrics for each individual CPU core, you can enable it in the configuration.

## Configuration

```toml
# Whether to collect metrics for each individual CPU core
collect_per_cpu = false
```

When `collect_per_cpu = true` is enabled, the metrics will include a `cpu` tag (e.g., `cpu="cpu0"`, `cpu="cpu1"`) to distinguish between different cores. The global summary metrics typically use the `cpu="cpu-total"` tag.

## Metrics

Common metrics include but are not limited to:
- `cpu_usage_active`: The active CPU time percentage (100 - idle)
- `cpu_usage_user`: CPU time spent in user space
- `cpu_usage_system`: CPU time spent in kernel space
- `cpu_usage_idle`: CPU idle time percentage
- `cpu_usage_iowait`: CPU time spent waiting for I/O operations

## Dashboard

It is recommended to integrate OS-level metrics (CPU, Mem, Disk, etc.) into a unified System Dashboard. However, a dedicated CPU reference Dashboard is also provided here for independent viewing.
