# Kernel VMStat Input Plugin

This plugin collects metrics from `/proc/vmstat` on Linux. It requires a relatively modern Linux kernel.

Since `/proc/vmstat` contains a large number of metrics, we use a whitelist mechanism in the configuration file. Only the metrics explicitly enabled (set to `1` or `true`) in the whitelist will be collected.

## Configuration

```toml
# Collect kernel vmstat metrics from /proc/vmstat
[[instances]]
# No other settings are needed, the white_list below controls which fields are collected.

[white_list]
oom_kill = 1
nr_free_pages = 0
nr_alloc_batch = 0
# ... (see conf/input.kernel_vmstat/kernel_vmstat.toml for the full list)
```

## Dashboards

By default, the collected metrics will be named `kernel_vmstat_<metric_name>`.
Since this represents deep kernel memory management and paging statistics (like `oom_kill`, `pgpgin`, `pgfault`), these metrics are generally visualized in custom advanced system dashboards or troubleshooting dashboards.
