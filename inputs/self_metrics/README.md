# Self Metrics Input Plugin

This plugin collects internal operational metrics of Categraf itself. It gathers Go runtime metrics (such as Goroutine counts, GC latencies, memory allocations) as well as Categraf-specific metrics regarding the metric pushing queues.

This is critical for monitoring the health of the monitoring Agent itself, particularly for diagnosing queue backlogs or memory leaks.

## Configuration

Since it is a built-in plugin gathering its own state, the configuration is extremely simple and only needs to be enabled.

```toml
# Collect Categraf's own metrics
# interval = 15

# [[instances]]
# No specific configuration required
```

## Metrics

All relevant metrics are prefixed with `categraf_` and Go's runtime metrics like `categraf_go_` / `categraf_process_`. Core self-monitoring metrics include:

- `categraf_info`: Categraf version information (value is 1, carrying a `version` tag)
- `categraf_metrics_enqueue_sum`: Total number of metrics enqueued to the sending queue
- `categraf_metrics_enqueue_failed_sum`: Total number of metrics that failed to enqueue
- `categraf_current_queue_size`: Current number of pending metrics in the memory queue (if this value keeps rising, it means the pushing rate to the backend is slower than the scraping rate, or the backend is failing)
- `categraf_go_goroutines`: Current number of Goroutines
- `categraf_go_memstats_alloc_bytes`: Memory allocated by the Go runtime
- `categraf_process_cpu_seconds_total`: Total CPU time consumed by the Categraf process
- `categraf_process_resident_memory_bytes`: Resident Set Size (RSS) physical memory used by the Categraf process

These metrics are automatically tagged with `version` and other environmental tags.

## Dashboards

A companion basic Dashboard (`dashboard.json`) is provided in this directory to quickly monitor the Categraf process's CPU/Memory usage, Goroutine counts, and most importantly, the **metric sending queue backlog**.
