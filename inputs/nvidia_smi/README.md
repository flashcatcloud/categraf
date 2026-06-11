# NVIDIA SMI Input Plugin

This plugin collects metrics by reading the output of the `nvidia-smi` command-line tool. It integrates the core code of [nvidia_gpu_exporter](https://github.com/utkuozdemir/nvidia_gpu_exporter).

**Supported Platforms:** Linux, Windows (Requires NVIDIA GPU drivers and the `nvidia-smi` utility installed)

## Configuration

The configuration file is located at `conf/input.nvidia_smi/nvidia_smi.toml`

```toml
# Collect NVIDIA GPU status
# interval = 15

[[instances]]
# The following option is critical. To collect nvidia-smi information, uncomment it and provide the absolute path to the nvidia-smi command.
# This instructs Categraf to execute the local nvidia-smi command to get the GPU status.
# nvidia_smi_command = "/usr/bin/nvidia-smi"

# If you want to remotely collect GPU status from another machine, you can use an ssh command.
# (Since Categraf is usually deployed on every physical machine, SSH is rarely needed in practice)
# nvidia_smi_command = "ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null SSH_USER@SSH_HOST nvidia-smi"

# Comma-separated list of query fields. You can find out possible fields by running `nvidia-smi --help-query-gpus`.
# Setting the value to `AUTO` will automatically detect and query all supported fields.
query_field_names = "AUTO"
```

## Metrics

This plugin supports hundreds of GPU metrics depending on the driver version and GPU model. All metrics are prefixed with `nvidia_smi_` and automatically tagged with identifiers like `uuid` and `name` (e.g., Tesla T4).

Key metrics to monitor include:
- `nvidia_smi_utilization_gpu_ratio`: GPU computation utilization (0~1)
- `nvidia_smi_utilization_memory_ratio`: Memory bandwidth utilization (0~1)
- `nvidia_smi_memory_used_bytes` / `nvidia_smi_memory_total_bytes`: GPU memory usage and capacity
- `nvidia_smi_temperature_gpu`: GPU core temperature (Celsius)
- `nvidia_smi_power_draw_watts`: Current GPU power consumption
- `nvidia_smi_fan_speed_ratio`: Fan speed percentage

## Dashboards

A companion basic Dashboard (`dashboard.json`) is provided in this directory to help you quickly set up visualization for GPU utilization, memory usage, temperature, and power consumption.
