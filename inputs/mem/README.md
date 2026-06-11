# Mem (Memory) Input Plugin

This plugin collects host-level memory metrics, including total memory, available memory, usage percentage, and caches.

**Supported Platforms:** Windows, Linux, macOS, BSD, etc.

## Configuration

```toml
# Collect host physical memory metrics
[[instances]]
# Usually requires no specific configuration. Just leave it enabled.
```

## Metrics

All collected metrics are prefixed with `mem_`.
Key metrics include:

- `mem_total`: Total amount of physical memory in bytes
- `mem_available`: Available memory in bytes (The most important metric for evaluating memory pressure)
- `mem_used`: Used memory in bytes
- `mem_used_percent`: Memory usage percentage (%)
- `mem_free`: Absolute free memory in bytes
- `mem_cached`: Memory used by page cache (Linux)
- `mem_buffers`: Memory used by block device buffers (Linux)
- `mem_swap_total` / `mem_swap_free` / `mem_swap_used_percent`: Swap-related metrics

## Dashboards

Metrics collected by this plugin are among the most fundamental server monitoring data. Typically, OS memory monitoring is unified under a global **System** dashboard alongside CPU and disk metrics.
For convenience in standalone viewing, a basic Dashboard containing only memory dimensions is also provided in this directory.
