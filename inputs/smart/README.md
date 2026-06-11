# S.M.A.R.T. Input Plugin

This plugin uses the command-line utility `smartctl` to collect S.M.A.R.T. (Self-Monitoring, Analysis and Reporting Technology) storage device health and status metrics. SMART is a monitoring system included in computer hard disk drives (HDDs) and solid-state drives (SSDs) that detects and reports on various indicators of drive reliability, with the intent of enabling the anticipation of hardware failures.

This plugin is forked from `telegraf/smart` and adapted for Categraf.

## Prerequisites

- `smartmontools` (which includes the `smartctl` utility) must be installed on your system.
  - Ubuntu/Debian: `sudo apt-get install smartmontools`
  - CentOS/RHEL: `sudo yum install smartmontools`
- The user running Categraf generally requires `root` privileges to read disk SMART information. If you prefer to run Categraf as a non-root user, you can configure `sudo` for passwordless execution of `smartctl` and set `use_sudo = true` in the configuration.

## Configuration

```toml
# Collect S.M.A.R.T. hardware status
# interval = 60

[[instances]]
# Optionally use sudo to execute smartctl
# use_sudo = false

# (Optional) Specify the path to smartctl if it's not in the environment PATH
# path_smartctl = "/usr/sbin/smartctl"

# (Optional) List of specific devices to monitor.
# If omitted (left empty), the plugin will automatically discover all drives using `smartctl --scan`.
# devices = [ "/dev/sda", "/dev/nvme0n1" ]

# Command timeout
# timeout = "5s"

# Whether to collect detailed SMART attributes (generates more granular metrics)
attributes = true
```

## Metrics

The collected metrics are separated into two main prefixes (depending on whether `attributes` is enabled):

### smart_device (General Device Metrics)
- `smart_device_health_ok`: Disk health status, 1 for healthy (PASSED), 0 for failure
- `smart_device_temp_c`: Current disk temperature (in Celsius)
- `smart_device_power_on_hours`: Total power-on hours
- `smart_device_power_cycle_count`: Power cycle count
- ...

### smart_attribute (Detailed Attribute Metrics)
If `attributes = true` is enabled, the plugin generates the following metrics for every specific SMART Attribute (e.g., Raw_Read_Error_Rate, Reallocated_Sector_Ct, etc.):
- `smart_attribute_value`: Current normalized value
- `smart_attribute_worst`: Worst recorded value
- `smart_attribute_threshold`: The failure threshold
- `smart_attribute_raw_value`: The raw sensor value (usually the most diagnostic)

All metrics are tagged with `device` (e.g., `/dev/sda`) and the specific `serial_no` of the drive.

## Dashboards

A basic companion Dashboard (`dashboard.json`) is provided in this directory to monitor the overall health status (Health PASSED/FAILED), temperature distribution, and power-on hours of your server disks.
