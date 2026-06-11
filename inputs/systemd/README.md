# Systemd Input Plugin

This plugin collects metrics about the running state of `systemd` on Linux systems. It gathers the statuses of various units (services, sockets, timers, etc.), restart counts, startup times, and task counts.

The implementation of this plugin is forked and adapted from [node_exporter](https://github.com/prometheus/node_exporter/blob/master/collector/systemd_linux.go).
**Note:** This plugin interacts with D-Bus using pure Go. It does not require CGO to compile and run on Linux. On non-Linux systems, it compiles down to an empty implementation.

## Configuration

You can enable and configure the systemd plugin in your Categraf configuration file:

```toml
# Collect systemd unit metrics
# interval = 15

[[instances]]
# Regex: Used to match the unit names to be collected. Default is all (".+").
# unit_include = ".+"

# Regex: Used to exclude specific unit names from collection.
# If a unit matches both include and exclude regexes, it will be excluded.
# By default, automount, device, mount, scope, and slice units are excluded.
# unit_exclude = ".+\\.(automount|device|mount|scope|slice)"

# Whether to establish a private, non-D-Bus direct connection to systemd.
# (Strongly discouraged for production, requires root privileges, mainly for testing).
# systemd_private = false

# Enable gathering metrics for task counts and maximum tasks allowed per unit.
# enable_task_metrics = false

# Enable gathering metrics regarding the restart counts of units.
# enable_restarts_metrics = false

# Enable gathering start time metrics for units.
# enable_start_time_metrics = false
```

## Metrics

All metrics reported by this plugin are prefixed with `systemd_`. The core metrics include:

- `systemd_system_running`: A boolean indicating if systemd is fully running and not in a degraded or initializing state.
- `systemd_version`: systemd version info metric (value is 1, with version info in labels).
- `systemd_units`: The total count of systemd units by state (active, activating, inactive, failed, etc.).
- `systemd_unit_state`: Indicates the state of each specific unit (e.g., `state="active"`, `state="failed"`). This is extremely useful for alerting on failed services.
- `systemd_unit_tasks_current` / `systemd_unit_tasks_max`: The current and maximum allowed tasks for the unit (if `enable_task_metrics` is true).
- `systemd_service_restart_total`: The total number of times the service has restarted (if `enable_restarts_metrics` is true).
- `systemd_unit_start_time_seconds`: The timestamp when the unit started (if `enable_start_time_metrics` is true).

## Dashboards

A basic companion Dashboard (`dashboard.json`) is provided in this directory. It visualizes the overall health of systemd (`system_running`), identifies any failed units, and tracks the number of services running on the node.
