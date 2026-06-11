# Kernel Input Plugin

This plugin collects status information from the host machine's Linux kernel.
The data is typically sourced from `/proc/stat` and `/proc/vmstat`.

**Supported Platforms:** Linux

## Configuration

```toml
# Collect Linux Kernel metrics
[[instances]]
# No specific configuration is required.
```

## Metrics

- `kernel_boot_time`: System boot time (seconds since Epoch)
- `kernel_context_switches`: Total context switches since boot
- `kernel_interrupts`: Total interrupts since boot
- `kernel_processes_forked`: Total processes created via fork() since boot
- `kernel_entropy_avail`: Available entropy pool size (used for generating random numbers)

## Dashboards

Kernel metrics collected by this plugin are usually considered part of basic server monitoring and are often combined with CPU and memory metrics in a global `System` dashboard.
For convenience and testing, a simple standalone Kernel monitoring dashboard is also provided here.
